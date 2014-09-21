// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"code.google.com/p/go.crypto/bcrypt"
	"github.com/syncthing/syncthing/auto"
	"github.com/syncthing/syncthing/config"
	"github.com/syncthing/syncthing/events"
	"github.com/syncthing/syncthing/logger"
	"github.com/syncthing/syncthing/model"
	"github.com/syncthing/syncthing/protocol"
	"github.com/syncthing/syncthing/upgrade"
	"github.com/vitrun/qart/qr"
)

type guiError struct {
	Time  time.Time
	Error string
}

var (
	configInSync = true
	guiErrors    = []guiError{}
	guiErrorsMut sync.Mutex
	modt         = time.Now().UTC().Format(http.TimeFormat)
	eventSub     *events.BufferedSubscription
)

const MIME_TYPE_DIR = "inode/directory"

func init() {
	l.AddHandler(logger.LevelWarn, showGuiError)
	sub := events.Default.Subscribe(events.AllEvents)
	eventSub = events.NewBufferedSubscription(sub, 1000)
}

func startGUI(cfg config.GUIConfiguration, assetDir string, m *model.Model) error {
	var err error

	cert, err := loadCert(confDir, "https-")
	if err != nil {
		l.Infoln("Loading HTTPS certificate:", err)
		l.Infoln("Creating new HTTPS certificate")
		newCertificate(confDir, "https-")
		cert, err = loadCert(confDir, "https-")
	}
	if err != nil {
		return err
	}
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ServerName:   "syncthing",
	}

	rawListener, err := net.Listen("tcp", cfg.Address)
	if err != nil {
		return err
	}
	listener := &DowngradingListener{rawListener, tlsCfg}

	// The GET handlers
	getRestMux := http.NewServeMux()
	getRestMux.HandleFunc("/rest/ping", restPing)
	getRestMux.HandleFunc("/rest/completion", withModel(m, restGetCompletion))
	getRestMux.HandleFunc("/rest/config", restGetConfig)
	getRestMux.HandleFunc("/rest/config/sync", restGetConfigInSync)
	getRestMux.HandleFunc("/rest/connections", withModel(m, restGetConnections))
	getRestMux.HandleFunc("/rest/discovery", restGetDiscovery)
	getRestMux.HandleFunc("/rest/errors", restGetErrors)
	getRestMux.HandleFunc("/rest/events", restGetEvents)
	getRestMux.HandleFunc("/rest/lang", restGetLang)
	getRestMux.HandleFunc("/rest/model", withModel(m, restGetModel))
	getRestMux.HandleFunc("/rest/model/version", withModel(m, restGetModelVersion))
	getRestMux.HandleFunc("/rest/need", withModel(m, restGetNeed))
	getRestMux.HandleFunc("/rest/nodeid", restGetNodeID)
	getRestMux.HandleFunc("/rest/report", withModel(m, restGetReport))
	getRestMux.HandleFunc("/rest/system", restGetSystem)
	getRestMux.HandleFunc("/rest/upgrade", restGetUpgrade)
	getRestMux.HandleFunc("/rest/version", restGetVersion)
	getRestMux.HandleFunc("/rest/stats/node", withModel(m, restGetNodeStats))
	getRestMux.HandleFunc("/rest/file/list", restGetFolderContents)

	// Debug endpoints, not for general use
	getRestMux.HandleFunc("/rest/debug/peerCompletion", withModel(m, restGetPeerCompletion))

	// The POST handlers
	postRestMux := http.NewServeMux()
	postRestMux.HandleFunc("/rest/ping", restPing)
	postRestMux.HandleFunc("/rest/config", withModel(m, restPostConfig))
	postRestMux.HandleFunc("/rest/discovery/hint", restPostDiscoveryHint)
	postRestMux.HandleFunc("/rest/error", restPostError)
	postRestMux.HandleFunc("/rest/error/clear", restClearErrors)
	postRestMux.HandleFunc("/rest/model/override", withModel(m, restPostOverride))
	postRestMux.HandleFunc("/rest/reset", restPostReset)
	postRestMux.HandleFunc("/rest/restart", restPostRestart)
	postRestMux.HandleFunc("/rest/shutdown", restPostShutdown)
	postRestMux.HandleFunc("/rest/upgrade", restPostUpgrade)
	postRestMux.HandleFunc("/rest/scan", withModel(m, restPostScan))
	getRestMux.HandleFunc("/rest/file/create", withModel(m, restPostCreateFile))
	getRestMux.HandleFunc("/rest/file/delete", withModel(m, restPostDeleteFile))

	// A handler that splits requests between the two above and disables
	// caching
	restMux := noCacheMiddleware(getPostHandler(getRestMux, postRestMux))

	// The main routing handler
	mux := http.NewServeMux()
	mux.Handle("/rest/", restMux)
	mux.HandleFunc("/qr/", getQR)

	// Serve compiled in assets unless an asset directory was set (for development)
	mux.Handle("/", embeddedStatic(assetDir))

	// Wrap everything in CSRF protection. The /rest prefix should be
	// protected, other requests will grant cookies.
	handler := csrfMiddleware("/rest", cfg.APIKey, mux)

	// Add our version as a header to responses
	handler = withVersionMiddleware(handler)

	// Wrap everything in basic auth, if user/password is set.
	if len(cfg.User) > 0 && len(cfg.Password) > 0 {
		handler = basicAuthAndSessionMiddleware(cfg, handler)
	}

	// Redirect to HTTPS if we are supposed to
	if cfg.UseTLS {
		handler = redirectToHTTPSMiddleware(handler)
	}

	go func() {
		err := http.Serve(listener, handler)
		if err != nil {
			panic(err)
		}
	}()
	return nil
}

func getPostHandler(get, post http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			get.ServeHTTP(w, r)
		case "POST":
			post.ServeHTTP(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

func redirectToHTTPSMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add a generous access-control-allow-origin header since we may be
		// redirecting REST requests over protocols
		w.Header().Add("Access-Control-Allow-Origin", "*")

		if r.TLS == nil {
			// Redirect HTTP requests to HTTPS
			r.URL.Host = r.Host
			r.URL.Scheme = "https"
			http.Redirect(w, r, r.URL.String(), http.StatusFound)
		} else {
			h.ServeHTTP(w, r)
		}
	})
}

func noCacheMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		h.ServeHTTP(w, r)
	})
}

func withVersionMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Syncthing-Version", Version)
		h.ServeHTTP(w, r)
	})
}

func withModel(m *model.Model, h func(m *model.Model, w http.ResponseWriter, r *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h(m, w, r)
	}
}

func restPing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]string{
		"ping": "pong",
	})
}

func restGetVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]string{
		"version":     Version,
		"longVersion": LongVersion,
		"os":          runtime.GOOS,
		"arch":        runtime.GOARCH,
	})
}

func restGetCompletion(m *model.Model, w http.ResponseWriter, r *http.Request) {
	var qs = r.URL.Query()
	var repo = qs.Get("repo")
	var nodeStr = qs.Get("node")

	node, err := protocol.NodeIDFromString(nodeStr)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	res := map[string]float64{
		"completion": m.Completion(node, repo),
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(res)
}

func restGetModelVersion(m *model.Model, w http.ResponseWriter, r *http.Request) {
	var qs = r.URL.Query()
	var repo = qs.Get("repo")
	var res = make(map[string]interface{})

	res["version"] = m.LocalVersion(repo)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(res)
}

func restGetModel(m *model.Model, w http.ResponseWriter, r *http.Request) {
	var qs = r.URL.Query()
	var repo = qs.Get("repo")
	var res = make(map[string]interface{})

	for _, cr := range cfg.Repositories {
		if cr.ID == repo {
			res["invalid"] = cr.Invalid
			break
		}
	}

	globalFiles, globalDeleted, globalBytes := m.GlobalSize(repo)
	res["globalFiles"], res["globalDeleted"], res["globalBytes"] = globalFiles, globalDeleted, globalBytes

	localFiles, localDeleted, localBytes := m.LocalSize(repo)
	res["localFiles"], res["localDeleted"], res["localBytes"] = localFiles, localDeleted, localBytes

	needFiles, needBytes := m.NeedSize(repo)
	res["needFiles"], res["needBytes"] = needFiles, needBytes

	res["inSyncFiles"], res["inSyncBytes"] = globalFiles-needFiles, globalBytes-needBytes

	res["state"], res["stateChanged"] = m.State(repo)
	res["version"] = m.LocalVersion(repo)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(res)
}

func restPostOverride(m *model.Model, w http.ResponseWriter, r *http.Request) {
	var qs = r.URL.Query()
	var repo = qs.Get("repo")
	go m.Override(repo)
}

func restGetNeed(m *model.Model, w http.ResponseWriter, r *http.Request) {
	var qs = r.URL.Query()
	var repo = qs.Get("repo")

	files := m.NeedFilesRepoLimited(repo, 100, 2500) // max 100 files or 2500 blocks

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(files)
}

func restGetConnections(m *model.Model, w http.ResponseWriter, r *http.Request) {
	var res = m.ConnectionStats()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(res)
}

func restGetNodeStats(m *model.Model, w http.ResponseWriter, r *http.Request) {
	var res = m.NodeStatistics()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(res)
}

func restGetConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(cfg)
}

func restPostConfig(m *model.Model, w http.ResponseWriter, r *http.Request) {
	var newCfg config.Configuration
	err := json.NewDecoder(r.Body).Decode(&newCfg)
	if err != nil {
		l.Warnln("decoding posted config:", err)
		http.Error(w, err.Error(), 500)
		return
	} else {
		if newCfg.GUI.Password != cfg.GUI.Password {
			if newCfg.GUI.Password != "" {
				hash, err := bcrypt.GenerateFromPassword([]byte(newCfg.GUI.Password), 0)
				if err != nil {
					l.Warnln("bcrypting password:", err)
					http.Error(w, err.Error(), 500)
					return
				} else {
					newCfg.GUI.Password = string(hash)
				}
			}
		}

		// Start or stop usage reporting as appropriate

		if newCfg.Options.URAccepted > cfg.Options.URAccepted {
			// UR was enabled
			newCfg.Options.URAccepted = usageReportVersion
			err := sendUsageReport(m)
			if err != nil {
				l.Infoln("Usage report:", err)
			}
			go usageReportingLoop(m)
		} else if newCfg.Options.URAccepted < cfg.Options.URAccepted {
			// UR was disabled
			newCfg.Options.URAccepted = -1
			stopUsageReporting()
		}

		// Activate and save

		configInSync = !config.ChangeRequiresRestart(cfg, newCfg)
		newCfg.Location = cfg.Location
		newCfg.Save()
		cfg = newCfg
	}
}

func restGetConfigInSync(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]bool{"configInSync": configInSync})
}

func restPostRestart(w http.ResponseWriter, r *http.Request) {
	flushResponse(`{"ok": "restarting"}`, w)
	go restart()
}

func restPostReset(w http.ResponseWriter, r *http.Request) {
	flushResponse(`{"ok": "resetting repos"}`, w)
	resetRepositories()
	go restart()
}

func restPostShutdown(w http.ResponseWriter, r *http.Request) {
	flushResponse(`{"ok": "shutting down"}`, w)
	go shutdown()
}

func flushResponse(s string, w http.ResponseWriter) {
	w.Write([]byte(s + "\n"))
	f := w.(http.Flusher)
	f.Flush()
}

var cpuUsagePercent [10]float64 // The last ten seconds
var cpuUsageLock sync.RWMutex

func restGetSystem(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	res := make(map[string]interface{})
	res["myID"] = myID.String()
	res["goroutines"] = runtime.NumGoroutine()
	res["alloc"] = m.Alloc
	res["sys"] = m.Sys - m.HeapReleased
	res["tilde"] = expandTilde("~")
	if cfg.Options.GlobalAnnEnabled && discoverer != nil {
		res["extAnnounceOK"] = discoverer.ExtAnnounceOK()
	}
	cpuUsageLock.RLock()
	var cpusum float64
	for _, p := range cpuUsagePercent {
		cpusum += p
	}
	cpuUsageLock.RUnlock()
	res["cpuPercent"] = cpusum / 10

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(res)
}

func restGetErrors(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	guiErrorsMut.Lock()
	json.NewEncoder(w).Encode(map[string][]guiError{"errors": guiErrors})
	guiErrorsMut.Unlock()
}

func restPostError(w http.ResponseWriter, r *http.Request) {
	bs, _ := ioutil.ReadAll(r.Body)
	r.Body.Close()
	showGuiError(0, string(bs))
}

func restClearErrors(w http.ResponseWriter, r *http.Request) {
	guiErrorsMut.Lock()
	guiErrors = []guiError{}
	guiErrorsMut.Unlock()
}

func showGuiError(l logger.LogLevel, err string) {
	guiErrorsMut.Lock()
	guiErrors = append(guiErrors, guiError{time.Now(), err})
	if len(guiErrors) > 5 {
		guiErrors = guiErrors[len(guiErrors)-5:]
	}
	guiErrorsMut.Unlock()
}

func restPostDiscoveryHint(w http.ResponseWriter, r *http.Request) {
	var qs = r.URL.Query()
	var node = qs.Get("node")
	var addr = qs.Get("addr")
	if len(node) != 0 && len(addr) != 0 && discoverer != nil {
		discoverer.Hint(node, []string{addr})
	}
}

func restGetDiscovery(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(discoverer.All())
}

func restGetReport(m *model.Model, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(reportData(m))
}

func restGetEvents(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	sinceStr := qs.Get("since")
	limitStr := qs.Get("limit")
	since, _ := strconv.Atoi(sinceStr)
	limit, _ := strconv.Atoi(limitStr)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	// Flush before blocking, to indicate that we've received the request
	// and that it should not be retried.
	f := w.(http.Flusher)
	f.Flush()

	evs := eventSub.Since(since, nil)
	if 0 < limit && limit < len(evs) {
		evs = evs[len(evs)-limit:]
	}

	json.NewEncoder(w).Encode(evs)
}

func restGetUpgrade(w http.ResponseWriter, r *http.Request) {
	rel, err := upgrade.LatestRelease(strings.Contains(Version, "-beta"))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	res := make(map[string]interface{})
	res["running"] = Version
	res["latest"] = rel.Tag
	res["newer"] = upgrade.CompareVersions(rel.Tag, Version) == 1

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(res)
}

func restGetNodeID(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	idStr := qs.Get("id")
	id, err := protocol.NodeIDFromString(idStr)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err == nil {
		json.NewEncoder(w).Encode(map[string]string{
			"id": id.String(),
		})
	} else {
		json.NewEncoder(w).Encode(map[string]string{
			"error": err.Error(),
		})
	}
}

func restGetLang(w http.ResponseWriter, r *http.Request) {
	lang := r.Header.Get("Accept-Language")
	var langs []string
	for _, l := range strings.Split(lang, ",") {
		parts := strings.SplitN(l, ";", 2)
		langs = append(langs, strings.ToLower(strings.TrimSpace(parts[0])))
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(langs)
}

func restPostUpgrade(w http.ResponseWriter, r *http.Request) {
	rel, err := upgrade.LatestRelease(strings.Contains(Version, "-beta"))
	if err != nil {
		l.Warnln("getting latest release:", err)
		http.Error(w, err.Error(), 500)
		return
	}

	if upgrade.CompareVersions(rel.Tag, Version) == 1 {
		err = upgrade.UpgradeTo(rel, GoArchExtra)
		if err != nil {
			l.Warnln("upgrading:", err)
			http.Error(w, err.Error(), 500)
			return
		}

		flushResponse(`{"ok": "restarting"}`, w)
		l.Infoln("Upgrading")
		stop <- exitUpgrading
	}
}

func restPostScan(m *model.Model, w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	repo := qs.Get("repo")
	sub := qs.Get("sub")
	err := m.ScanRepoSub(repo, sub)
	if err != nil {
		http.Error(w, err.Error(), 500)
	}
}

func getQR(w http.ResponseWriter, r *http.Request) {
	var qs = r.URL.Query()
	var text = qs.Get("text")
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		http.Error(w, "Invalid", 500)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Write(code.PNG())
}

func restGetPeerCompletion(m *model.Model, w http.ResponseWriter, r *http.Request) {
	tot := map[string]float64{}
	count := map[string]float64{}

	for _, repo := range cfg.Repositories {
		for _, node := range repo.NodeIDs() {
			nodeStr := node.String()
			if m.ConnectedTo(node) {
				tot[nodeStr] += m.Completion(node, repo.ID)
			} else {
				tot[nodeStr] = 0
			}
			count[nodeStr]++
		}
	}

	comp := map[string]int{}
	for node := range tot {
		comp[node] = int(tot[node] / count[node])
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(comp)
}

func restPostCreateFile(m *model.Model, w http.ResponseWriter, r *http.Request) {
	var qs = r.URL.Query()
	var repoId = qs.Get("repo")
	var repo, repoExists = cfg.RepoMap()[repoId]
	var path = qs.Get("path")
	var isDir = path[len(path)-1] == '/'
	path = filepath.Clean(repo.Directory + "/" + qs.Get("path"))

	if !repoExists {
		flushResponse(`{"error": "Repository `+repoId+` does not exist"}`, w)
		return
	}

	if !strings.HasPrefix(path, repo.Directory) {
		flushResponse(`{"error": "Must not create file outside repository"}`, w)
		return
	}

	var err error
	if isDir {
		err = os.Mkdir(path, 0775)
	} else {
		_, err = os.Create(path)
	}
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{
			"error": err.Error(),
		})
		return
	}

	m.ScanRepoSub(repoId, path)
	flushResponse(`{"ok": "file created"}`, w)
}

func restPostDeleteFile(m *model.Model, w http.ResponseWriter, r *http.Request) {
	var qs = r.URL.Query()
	var repoId = qs.Get("repo")
	var repo, repoExists = cfg.RepoMap()[repoId]
	var path = filepath.Clean(repo.Directory + "/" + qs.Get("path"))

	if !repoExists {
		flushResponse(`{"error": "Repository `+repoId+` does not exist"}`, w)
		return
	}

	if !strings.HasPrefix(path, repo.Directory) {
		flushResponse(`{"error": "Must not delete file outside repository"}`, w)
		return
	}

	var err = os.Remove(path)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{
			"error": err.Error(),
		})
		return
	}

	m.ScanRepoSub(repoId, path)
	flushResponse(`{"ok": "file deleted"}`, w)
}

func restGetFolderContents(w http.ResponseWriter, r *http.Request) {
	var qs = r.URL.Query()
	var repoId = qs.Get("repo")
	var repo, repoExists = cfg.RepoMap()[repoId]
	var path = filepath.Clean(repo.Directory + "/" + qs.Get("path"))

	if !repoExists {
		flushResponse(`{"error": "Repository `+repoId+` does not exist"}`, w)
		return
	}

	if !strings.HasPrefix(path, repo.Directory) {
		flushResponse(`{"error": "Must not access file outside repository"}`, w)
		return
	}

	var fi, err = ioutil.ReadDir(path)
	if err != nil {
		flushResponse(`{"error": "`+err.Error()+`"}`, w)
		return
	}

	var contents = make([]map[string]string, len(fi))
	for i, f := range fi {
		var mimetype string
		if f.IsDir() {
			mimetype = MIME_TYPE_DIR
		} else {
			mimetype = mime.TypeByExtension(filepath.Ext(f.Name()))
		}
		contents[i] = map[string]string{
			"name":     f.Name(),
			"size":     strconv.FormatInt(f.Size(), 10),
			"modified": strconv.FormatInt(f.ModTime().Unix(), 10),
			"mime":     mimetype,
		}
	}
	json.NewEncoder(w).Encode(contents)
}

func embeddedStatic(assetDir string) http.Handler {
	assets := auto.Assets()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		file := r.URL.Path

		if file[0] == '/' {
			file = file[1:]
		}

		if len(file) == 0 {
			file = "index.html"
		}

		if assetDir != "" {
			p := filepath.Join(assetDir, filepath.FromSlash(file))
			_, err := os.Stat(p)
			if err == nil {
				http.ServeFile(w, r, p)
				return
			}
		}

		bs, ok := assets[file]
		if !ok {
			http.NotFound(w, r)
			return
		}

		mtype := mimeTypeForFile(file)
		if len(mtype) != 0 {
			w.Header().Set("Content-Type", mtype)
		}
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(bs)))
		w.Header().Set("Last-Modified", modt)

		w.Write(bs)
	})
}

func mimeTypeForFile(file string) string {
	// We use a built in table of the common types since the system
	// TypeByExtension might be unreliable. But if we don't know, we delegate
	// to the system.
	ext := filepath.Ext(file)
	switch ext {
	case ".htm", ".html":
		return "text/html"
	case ".css":
		return "text/css"
	case ".js":
		return "application/javascript"
	case ".json":
		return "application/json"
	case ".png":
		return "image/png"
	case ".ttf":
		return "application/x-font-ttf"
	case ".woff":
		return "application/x-font-woff"
	default:
		return mime.TypeByExtension(ext)
	}
}
