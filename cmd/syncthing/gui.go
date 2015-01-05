// Copyright (C) 2014 The Syncthing Authors.
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

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

	"github.com/calmh/logger"
	"github.com/syncthing/syncthing/internal/auto"
	"github.com/syncthing/syncthing/internal/config"
	"github.com/syncthing/syncthing/internal/discover"
	"github.com/syncthing/syncthing/internal/events"
	"github.com/syncthing/syncthing/internal/model"
	"github.com/syncthing/syncthing/internal/osutil"
	"github.com/syncthing/syncthing/internal/protocol"
	"github.com/syncthing/syncthing/internal/upgrade"
	"github.com/vitrun/qart/qr"
	"golang.org/x/crypto/bcrypt"
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

		// When generating the HTTPS certificate, use the system host name per
		// default. If that isn't available, use the "syncthing" default.
		var name string
		name, err = os.Hostname()
		if err != nil {
			name = tlsDefaultCommonName
		}

		newCertificate(confDir, "https-", name)
		cert, err = loadCert(confDir, "https-")
	}
	if err != nil {
		return err
	}
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS10, // No SSLv3
		CipherSuites: []uint16{
			// No RC4
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
			tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
			tls.TLS_RSA_WITH_AES_128_CBC_SHA,
			tls.TLS_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
			tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
		},
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
	getRestMux.HandleFunc("/rest/autocomplete/directory", restGetAutocompleteDirectory)
	getRestMux.HandleFunc("/rest/discovery", restGetDiscovery)
	getRestMux.HandleFunc("/rest/errors", restGetErrors)
	getRestMux.HandleFunc("/rest/events", restGetEvents)
	getRestMux.HandleFunc("/rest/ignores", withModel(m, restGetIgnores))
	getRestMux.HandleFunc("/rest/lang", restGetLang)
	getRestMux.HandleFunc("/rest/model", withModel(m, restGetModel))
	getRestMux.HandleFunc("/rest/need", withModel(m, restGetNeed))
	getRestMux.HandleFunc("/rest/deviceid", restGetDeviceID)
	getRestMux.HandleFunc("/rest/report", withModel(m, restGetReport))
	getRestMux.HandleFunc("/rest/system", restGetSystem)
	getRestMux.HandleFunc("/rest/upgrade", restGetUpgrade)
	getRestMux.HandleFunc("/rest/version", restGetVersion)
	getRestMux.HandleFunc("/rest/stats/device", withModel(m, restGetDeviceStats))
	getRestMux.HandleFunc("/rest/stats/folder", withModel(m, restGetFolderStats))

	// Debug endpoints, not for general use
	getRestMux.HandleFunc("/rest/debug/peerCompletion", withModel(m, restGetPeerCompletion))

	// The POST handlers
	postRestMux := http.NewServeMux()
	postRestMux.HandleFunc("/rest/ping", restPing)
	postRestMux.HandleFunc("/rest/config", withModel(m, restPostConfig))
	postRestMux.HandleFunc("/rest/discovery/hint", restPostDiscoveryHint)
	postRestMux.HandleFunc("/rest/error", restPostError)
	postRestMux.HandleFunc("/rest/error/clear", restClearErrors)
	postRestMux.HandleFunc("/rest/ignores", withModel(m, restPostIgnores))
	postRestMux.HandleFunc("/rest/model/override", withModel(m, restPostOverride))
	postRestMux.HandleFunc("/rest/reset", restPostReset)
	postRestMux.HandleFunc("/rest/restart", restPostRestart)
	postRestMux.HandleFunc("/rest/shutdown", restPostShutdown)
	postRestMux.HandleFunc("/rest/upgrade", restPostUpgrade)
	postRestMux.HandleFunc("/rest/scan", withModel(m, restPostScan))
	postRestMux.HandleFunc("/rest/bump", withModel(m, restPostBump))

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

	srv := http.Server{
		Handler:     handler,
		ReadTimeout: 10 * time.Second,
	}

	go func() {
		err := srv.Serve(listener)
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
	var folder = qs.Get("folder")
	var deviceStr = qs.Get("device")

	device, err := protocol.DeviceIDFromString(deviceStr)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	res := map[string]float64{
		"completion": m.Completion(device, folder),
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(res)
}

func restGetModel(m *model.Model, w http.ResponseWriter, r *http.Request) {
	var qs = r.URL.Query()
	var folder = qs.Get("folder")
	var res = make(map[string]interface{})

	res["invalid"] = cfg.Folders()[folder].Invalid

	globalFiles, globalDeleted, globalBytes := m.GlobalSize(folder)
	res["globalFiles"], res["globalDeleted"], res["globalBytes"] = globalFiles, globalDeleted, globalBytes

	localFiles, localDeleted, localBytes := m.LocalSize(folder)
	res["localFiles"], res["localDeleted"], res["localBytes"] = localFiles, localDeleted, localBytes

	needFiles, needBytes := m.NeedSize(folder)
	res["needFiles"], res["needBytes"] = needFiles, needBytes

	res["inSyncFiles"], res["inSyncBytes"] = globalFiles-needFiles, globalBytes-needBytes

	res["state"], res["stateChanged"] = m.State(folder)
	res["version"] = m.CurrentLocalVersion(folder) + m.RemoteLocalVersion(folder)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(res)
}

func restPostOverride(m *model.Model, w http.ResponseWriter, r *http.Request) {
	var qs = r.URL.Query()
	var folder = qs.Get("folder")
	go m.Override(folder)
}

func restGetNeed(m *model.Model, w http.ResponseWriter, r *http.Request) {
	var qs = r.URL.Query()
	var folder = qs.Get("folder")

	progress, queued, rest := m.NeedFolderFiles(folder, 100)
	// Convert the struct to a more loose structure, and inject the size.
	output := map[string][]map[string]interface{}{
		"progress": toNeedSlice(progress),
		"queued":   toNeedSlice(queued),
		"rest":     toNeedSlice(rest),
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(output)
}

func restGetConnections(m *model.Model, w http.ResponseWriter, r *http.Request) {
	var res = m.ConnectionStats()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(res)
}

func restGetDeviceStats(m *model.Model, w http.ResponseWriter, r *http.Request) {
	var res = m.DeviceStatistics()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(res)
}

func restGetFolderStats(m *model.Model, w http.ResponseWriter, r *http.Request) {
	var res = m.FolderStatistics()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(res)
}

func restGetConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(cfg.Raw())
}

func restPostConfig(m *model.Model, w http.ResponseWriter, r *http.Request) {
	var newCfg config.Configuration
	err := json.NewDecoder(r.Body).Decode(&newCfg)
	if err != nil {
		l.Warnln("decoding posted config:", err)
		http.Error(w, err.Error(), 500)
		return
	}

	if newCfg.GUI.Password != cfg.GUI().Password {
		if newCfg.GUI.Password != "" {
			hash, err := bcrypt.GenerateFromPassword([]byte(newCfg.GUI.Password), 0)
			if err != nil {
				l.Warnln("bcrypting password:", err)
				http.Error(w, err.Error(), 500)
				return
			}

			newCfg.GUI.Password = string(hash)
		}
	}

	// Start or stop usage reporting as appropriate

	if curAcc := cfg.Options().URAccepted; newCfg.Options.URAccepted > curAcc {
		// UR was enabled
		newCfg.Options.URAccepted = usageReportVersion
		newCfg.Options.URUniqueID = randomString(8)
		err := sendUsageReport(m)
		if err != nil {
			l.Infoln("Usage report:", err)
		}
		go usageReportingLoop(m)
	} else if newCfg.Options.URAccepted < curAcc {
		// UR was disabled
		newCfg.Options.URAccepted = -1
		newCfg.Options.URUniqueID = ""
		stopUsageReporting()
	}

	// Activate and save

	configInSync = !config.ChangeRequiresRestart(cfg.Raw(), newCfg)
	cfg.Replace(newCfg)
	cfg.Save()
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
	flushResponse(`{"ok": "resetting folders"}`, w)
	resetFolders()
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

	tilde, _ := osutil.ExpandTilde("~")
	res := make(map[string]interface{})
	res["myID"] = myID.String()
	res["goroutines"] = runtime.NumGoroutine()
	res["alloc"] = m.Alloc
	res["sys"] = m.Sys - m.HeapReleased
	res["tilde"] = tilde
	if cfg.Options().GlobalAnnEnabled && discoverer != nil {
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
	var device = qs.Get("device")
	var addr = qs.Get("addr")
	if len(device) != 0 && len(addr) != 0 && discoverer != nil {
		discoverer.Hint(device, []string{addr})
	}
}

func restGetDiscovery(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	devices := map[string][]discover.CacheEntry{}

	if discoverer != nil {
		// Device ids can't be marshalled as keys so we need to manually
		// rebuild this map using strings. Discoverer may be nil if discovery
		// has not started yet.
		for device, entries := range discoverer.All() {
			devices[device.String()] = entries
		}
	}

	json.NewEncoder(w).Encode(devices)
}

func restGetReport(m *model.Model, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(reportData(m))
}

func restGetIgnores(m *model.Model, w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	ignores, patterns, err := m.GetIgnores(qs.Get("folder"))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	json.NewEncoder(w).Encode(map[string][]string{
		"ignore":   ignores,
		"patterns": patterns,
	})
}

func restPostIgnores(m *model.Model, w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()

	var data map[string][]string
	err := json.NewDecoder(r.Body).Decode(&data)
	r.Body.Close()

	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	err = m.SetIgnores(qs.Get("folder"), data["ignore"])
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	restGetIgnores(m, w, r)
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

func restGetDeviceID(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	idStr := qs.Get("id")
	id, err := protocol.DeviceIDFromString(idStr)
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
		err = upgrade.To(rel)
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
	folder := qs.Get("folder")
	sub := qs.Get("sub")
	err := m.ScanFolderSub(folder, sub)
	if err != nil {
		http.Error(w, err.Error(), 500)
	}
}

func restPostBump(m *model.Model, w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	folder := qs.Get("folder")
	file := qs.Get("file")
	m.BringToFront(folder, file)
	restGetNeed(m, w, r)
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

	for _, folder := range cfg.Folders() {
		for _, device := range folder.DeviceIDs() {
			deviceStr := device.String()
			if m.ConnectedTo(device) {
				tot[deviceStr] += m.Completion(device, folder.ID)
			} else {
				tot[deviceStr] = 0
			}
			count[deviceStr]++
		}
	}

	comp := map[string]int{}
	for device := range tot {
		comp[device] = int(tot[device] / count[device])
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(comp)
}

func restGetAutocompleteDirectory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	qs := r.URL.Query()
	current := qs.Get("current")
	search, _ := osutil.ExpandTilde(current)
	pathSeparator := string(os.PathSeparator)
	if strings.HasSuffix(current, pathSeparator) && !strings.HasSuffix(search, pathSeparator) {
		search = search + pathSeparator
	}
	subdirectories, _ := filepath.Glob(search + "*")
	ret := make([]string, 0, 10)
	for _, subdirectory := range subdirectories {
		info, err := os.Stat(subdirectory)
		if err == nil && info.IsDir() {
			ret = append(ret, subdirectory+pathSeparator)
			if len(ret) > 9 {
				break
			}
		}
	}
	json.NewEncoder(w).Encode(ret)
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

func toNeedSlice(files []protocol.FileInfoTruncated) []map[string]interface{} {
	output := make([]map[string]interface{}, len(files))
	for i, file := range files {
		output[i] = map[string]interface{}{
			"Name":         file.Name,
			"Flags":        file.Flags,
			"Modified":     file.Modified,
			"Version":      file.Version,
			"LocalVersion": file.LocalVersion,
			"NumBlocks":    file.NumBlocks,
			"Size":         protocol.BlocksToSize(file.NumBlocks),
		}
	}
	return output
}
