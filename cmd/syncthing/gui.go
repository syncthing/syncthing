// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/calmh/logger"
	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/internal/auto"
	"github.com/syncthing/syncthing/internal/config"
	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/internal/discover"
	"github.com/syncthing/syncthing/internal/events"
	"github.com/syncthing/syncthing/internal/model"
	"github.com/syncthing/syncthing/internal/osutil"
	"github.com/syncthing/syncthing/internal/sync"
	"github.com/syncthing/syncthing/internal/upgrade"
	"github.com/vitrun/qart/qr"
	"golang.org/x/crypto/bcrypt"
)

type guiError struct {
	Time  time.Time `json:"time"`
	Error string    `json:"error"`
}

var (
	configInSync            = true
	guiErrors               = []guiError{}
	guiErrorsMut sync.Mutex = sync.NewMutex()
	startTime               = time.Now()
	eventSub     *events.BufferedSubscription
)

var (
	lastEventRequest    time.Time
	lastEventRequestMut sync.Mutex = sync.NewMutex()
)

func startGUI(cfg config.GUIConfiguration, assetDir string, m *model.Model) error {
	var err error

	l.AddHandler(logger.LevelWarn, showGuiError)
	sub := events.Default.Subscribe(events.AllEvents)
	eventSub = events.NewBufferedSubscription(sub, 1000)

	cert, err := tls.LoadX509KeyPair(locations[locHTTPSCertFile], locations[locHTTPSKeyFile])
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

		cert, err = newCertificate(locations[locHTTPSCertFile], locations[locHTTPSKeyFile], name)
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
	getRestMux.HandleFunc("/rest/db/completion", withModel(m, restGetDBCompletion))           // device folder
	getRestMux.HandleFunc("/rest/db/file", withModel(m, restGetDBFile))                       // folder file
	getRestMux.HandleFunc("/rest/db/ignores", withModel(m, restGetDBIgnores))                 // folder
	getRestMux.HandleFunc("/rest/db/need", withModel(m, restGetDBNeed))                       // folder
	getRestMux.HandleFunc("/rest/db/status", withModel(m, restGetDBStatus))                   // folder
	getRestMux.HandleFunc("/rest/db/browse", withModel(m, restGetDBBrowse))                   // folder [prefix] [dirsonly] [levels]
	getRestMux.HandleFunc("/rest/events", restGetEvents)                                      // since [limit]
	getRestMux.HandleFunc("/rest/stats/device", withModel(m, restGetDeviceStats))             // -
	getRestMux.HandleFunc("/rest/stats/folder", withModel(m, restGetFolderStats))             // -
	getRestMux.HandleFunc("/rest/svc/deviceid", restGetDeviceID)                              // id
	getRestMux.HandleFunc("/rest/svc/lang", restGetLang)                                      // -
	getRestMux.HandleFunc("/rest/svc/report", withModel(m, restGetReport))                    // -
	getRestMux.HandleFunc("/rest/system/browse", restGetSystemBrowse)                         // current
	getRestMux.HandleFunc("/rest/system/config", restGetSystemConfig)                         // -
	getRestMux.HandleFunc("/rest/system/config/insync", RestGetSystemConfigInsync)            // -
	getRestMux.HandleFunc("/rest/system/connections", withModel(m, restGetSystemConnections)) // -
	getRestMux.HandleFunc("/rest/system/discovery", restGetSystemDiscovery)                   // -
	getRestMux.HandleFunc("/rest/system/error", restGetSystemError)                           // -
	getRestMux.HandleFunc("/rest/system/ping", restPing)                                      // -
	getRestMux.HandleFunc("/rest/system/status", restGetSystemStatus)                         // -
	getRestMux.HandleFunc("/rest/system/upgrade", restGetSystemUpgrade)                       // -
	getRestMux.HandleFunc("/rest/system/version", restGetSystemVersion)                       // -

	// The POST handlers
	postRestMux := http.NewServeMux()
	postRestMux.HandleFunc("/rest/db/prio", withModel(m, restPostDBPrio))             // folder file
	postRestMux.HandleFunc("/rest/db/ignores", withModel(m, restPostDBIgnores))       // folder
	postRestMux.HandleFunc("/rest/db/override", withModel(m, restPostDBOverride))     // folder
	postRestMux.HandleFunc("/rest/db/scan", withModel(m, restPostDBScan))             // folder [sub...]
	postRestMux.HandleFunc("/rest/system/config", withModel(m, restPostSystemConfig)) // <body>
	postRestMux.HandleFunc("/rest/system/discovery", restPostSystemDiscovery)         // device addr
	postRestMux.HandleFunc("/rest/system/error", restPostSystemError)                 // <body>
	postRestMux.HandleFunc("/rest/system/error/clear", restPostSystemErrorClear)      // -
	postRestMux.HandleFunc("/rest/system/ping", restPing)                             // -
	postRestMux.HandleFunc("/rest/system/reset", withModel(m, restPostSystemReset))   // [folder]
	postRestMux.HandleFunc("/rest/system/restart", restPostSystemRestart)             // -
	postRestMux.HandleFunc("/rest/system/shutdown", restPostSystemShutdown)           // -
	postRestMux.HandleFunc("/rest/system/upgrade", restPostSystemUpgrade)             // -

	// Debug endpoints, not for general use
	getRestMux.HandleFunc("/rest/debug/peerCompletion", withModel(m, restGetPeerCompletion))

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

	if debugHTTP {
		handler = debugMiddleware(handler)
	}

	srv := http.Server{
		Handler:     handler,
		ReadTimeout: 10 * time.Second,
	}

	csrv := &folderSummarySvc{model: m}
	go csrv.Serve()

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

func debugMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t0 := time.Now()
		h.ServeHTTP(w, r)
		ms := 1000 * time.Since(t0).Seconds()

		// The variable `w` is most likely a *http.response, which we can't do
		// much with since it's a non exported type. We can however peek into
		// it with reflection to get at the status code and number of bytes
		// written.
		var status, written int64
		if rw := reflect.Indirect(reflect.ValueOf(w)); rw.IsValid() && rw.Kind() == reflect.Struct {
			if rf := rw.FieldByName("status"); rf.IsValid() && rf.Kind() == reflect.Int {
				status = rf.Int()
			}
			if rf := rw.FieldByName("written"); rf.IsValid() && rf.Kind() == reflect.Int64 {
				written = rf.Int()
			}
		}

		l.Debugf("http: %s %q: status %d, %d bytes in %.02f ms", r.Method, r.URL.String(), status, written, ms)
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

func restGetSystemVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]string{
		"version":     Version,
		"longVersion": LongVersion,
		"os":          runtime.GOOS,
		"arch":        runtime.GOARCH,
	})
}

func restGetDBBrowse(m *model.Model, w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	folder := qs.Get("folder")
	prefix := qs.Get("prefix")
	dirsonly := qs.Get("dirsonly") != ""

	levels, err := strconv.Atoi(qs.Get("levels"))
	if err != nil {
		levels = -1
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	tree := m.GlobalDirectoryTree(folder, prefix, levels, dirsonly)

	json.NewEncoder(w).Encode(tree)
}

func restGetDBCompletion(m *model.Model, w http.ResponseWriter, r *http.Request) {
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

func restGetDBStatus(m *model.Model, w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	folder := qs.Get("folder")
	res := folderSummary(m, folder)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(res)
}

func folderSummary(m *model.Model, folder string) map[string]interface{} {
	var res = make(map[string]interface{})

	res["invalid"] = cfg.Folders()[folder].Invalid

	globalFiles, globalDeleted, globalBytes := m.GlobalSize(folder)
	res["globalFiles"], res["globalDeleted"], res["globalBytes"] = globalFiles, globalDeleted, globalBytes

	localFiles, localDeleted, localBytes := m.LocalSize(folder)
	res["localFiles"], res["localDeleted"], res["localBytes"] = localFiles, localDeleted, localBytes

	needFiles, needBytes := m.NeedSize(folder)
	res["needFiles"], res["needBytes"] = needFiles, needBytes

	res["inSyncFiles"], res["inSyncBytes"] = globalFiles-needFiles, globalBytes-needBytes

	var err error
	res["state"], res["stateChanged"], err = m.State(folder)
	if err != nil {
		res["error"] = err.Error()
	}

	res["version"] = m.CurrentLocalVersion(folder) + m.RemoteLocalVersion(folder)

	ignorePatterns, _, _ := m.GetIgnores(folder)
	res["ignorePatterns"] = false
	for _, line := range ignorePatterns {
		if len(line) > 0 && !strings.HasPrefix(line, "//") {
			res["ignorePatterns"] = true
			break
		}
	}

	return res
}

func restPostDBOverride(m *model.Model, w http.ResponseWriter, r *http.Request) {
	var qs = r.URL.Query()
	var folder = qs.Get("folder")
	go m.Override(folder)
}

func restGetDBNeed(m *model.Model, w http.ResponseWriter, r *http.Request) {
	var qs = r.URL.Query()
	var folder = qs.Get("folder")

	progress, queued, rest := m.NeedFolderFiles(folder, 100)
	// Convert the struct to a more loose structure, and inject the size.
	output := map[string][]jsonDBFileInfo{
		"progress": toNeedSlice(progress),
		"queued":   toNeedSlice(queued),
		"rest":     toNeedSlice(rest),
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(output)
}

func restGetSystemConnections(m *model.Model, w http.ResponseWriter, r *http.Request) {
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

func restGetDBFile(m *model.Model, w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	folder := qs.Get("folder")
	file := qs.Get("file")
	gf, _ := m.CurrentGlobalFile(folder, file)
	lf, _ := m.CurrentFolderFile(folder, file)

	av := m.Availability(folder, file)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"global":       jsonFileInfo(gf),
		"local":        jsonFileInfo(lf),
		"availability": av,
	})
}

func restGetSystemConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(cfg.Raw())
}

func restPostSystemConfig(m *model.Model, w http.ResponseWriter, r *http.Request) {
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

func RestGetSystemConfigInsync(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]bool{"configInSync": configInSync})
}

func restPostSystemRestart(w http.ResponseWriter, r *http.Request) {
	flushResponse(`{"ok": "restarting"}`, w)
	go restart()
}

func restPostSystemReset(m *model.Model, w http.ResponseWriter, r *http.Request) {
	var qs = r.URL.Query()
	folder := qs.Get("folder")
	var err error
	if len(folder) == 0 {
		err = resetDB()
	} else {
		err = m.ResetFolder(folder)
	}
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if len(folder) == 0 {
		flushResponse(`{"ok": "resetting database"}`, w)
	} else {
		flushResponse(`{"ok": "resetting folder " + folder}`, w)
	}
	go restart()
}

func restPostSystemShutdown(w http.ResponseWriter, r *http.Request) {
	flushResponse(`{"ok": "shutting down"}`, w)
	go shutdown()
}

func flushResponse(s string, w http.ResponseWriter) {
	w.Write([]byte(s + "\n"))
	f := w.(http.Flusher)
	f.Flush()
}

var cpuUsagePercent [10]float64 // The last ten seconds
var cpuUsageLock sync.RWMutex = sync.NewRWMutex()

func restGetSystemStatus(w http.ResponseWriter, r *http.Request) {
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
	res["cpuPercent"] = cpusum / float64(len(cpuUsagePercent)) / float64(runtime.NumCPU())
	res["pathSeparator"] = string(filepath.Separator)
	res["uptime"] = int(time.Since(startTime).Seconds())

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(res)
}

func restGetSystemError(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	guiErrorsMut.Lock()
	json.NewEncoder(w).Encode(map[string][]guiError{"errors": guiErrors})
	guiErrorsMut.Unlock()
}

func restPostSystemError(w http.ResponseWriter, r *http.Request) {
	bs, _ := ioutil.ReadAll(r.Body)
	r.Body.Close()
	showGuiError(0, string(bs))
}

func restPostSystemErrorClear(w http.ResponseWriter, r *http.Request) {
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

func restPostSystemDiscovery(w http.ResponseWriter, r *http.Request) {
	var qs = r.URL.Query()
	var device = qs.Get("device")
	var addr = qs.Get("addr")
	if len(device) != 0 && len(addr) != 0 && discoverer != nil {
		discoverer.Hint(device, []string{addr})
	}
}

func restGetSystemDiscovery(w http.ResponseWriter, r *http.Request) {
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

func restGetDBIgnores(m *model.Model, w http.ResponseWriter, r *http.Request) {
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

func restPostDBIgnores(m *model.Model, w http.ResponseWriter, r *http.Request) {
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

	restGetDBIgnores(m, w, r)
}

func restGetEvents(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	sinceStr := qs.Get("since")
	limitStr := qs.Get("limit")
	since, _ := strconv.Atoi(sinceStr)
	limit, _ := strconv.Atoi(limitStr)

	lastEventRequestMut.Lock()
	lastEventRequest = time.Now()
	lastEventRequestMut.Unlock()

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

func restGetSystemUpgrade(w http.ResponseWriter, r *http.Request) {
	if noUpgrade {
		http.Error(w, upgrade.ErrUpgradeUnsupported.Error(), 500)
		return
	}
	rel, err := upgrade.LatestRelease(Version)
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

func restPostSystemUpgrade(w http.ResponseWriter, r *http.Request) {
	rel, err := upgrade.LatestRelease(Version)
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

func restPostDBScan(m *model.Model, w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	folder := qs.Get("folder")
	if folder != "" {
		subs := qs["sub"]
		err := m.ScanFolderSubs(folder, subs)
		if err != nil {
			http.Error(w, err.Error(), 500)
		}
	} else {
		errors := m.ScanFolders()
		if len(errors) > 0 {
			http.Error(w, "Error scanning folders", 500)
			json.NewEncoder(w).Encode(errors)
		}
	}
}

func restPostDBPrio(m *model.Model, w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	folder := qs.Get("folder")
	file := qs.Get("file")
	m.BringToFront(folder, file)
	restGetDBNeed(m, w, r)
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

func restGetSystemBrowse(w http.ResponseWriter, r *http.Request) {
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
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			w.Header().Set("Content-Encoding", "gzip")
		} else {
			// ungzip if browser not send gzip accepted header
			var gr *gzip.Reader
			gr, _ = gzip.NewReader(bytes.NewReader(bs))
			bs, _ = ioutil.ReadAll(gr)
			gr.Close()
		}
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(bs)))
		w.Header().Set("Last-Modified", auto.AssetsBuildDate)

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
	case ".svg":
		return "image/svg+xml"
	default:
		return mime.TypeByExtension(ext)
	}
}

func toNeedSlice(fs []db.FileInfoTruncated) []jsonDBFileInfo {
	res := make([]jsonDBFileInfo, len(fs))
	for i, f := range fs {
		res[i] = jsonDBFileInfo(f)
	}
	return res
}

// Type wrappers for nice JSON serialization

type jsonFileInfo protocol.FileInfo

func (f jsonFileInfo) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"name":         f.Name,
		"size":         protocol.FileInfo(f).Size(),
		"flags":        fmt.Sprintf("%#o", f.Flags),
		"modified":     time.Unix(f.Modified, 0),
		"localVersion": f.LocalVersion,
		"numBlocks":    len(f.Blocks),
		"version":      jsonVersionVector(f.Version),
	})
}

type jsonDBFileInfo db.FileInfoTruncated

func (f jsonDBFileInfo) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"name":         f.Name,
		"size":         db.FileInfoTruncated(f).Size(),
		"flags":        fmt.Sprintf("%#o", f.Flags),
		"modified":     time.Unix(f.Modified, 0),
		"localVersion": f.LocalVersion,
		"version":      jsonVersionVector(f.Version),
	})
}

type jsonVersionVector protocol.Vector

func (v jsonVersionVector) MarshalJSON() ([]byte, error) {
	res := make([]string, len(v))
	for i, c := range v {
		res[i] = fmt.Sprintf("%d:%d", c.ID, c.Value)
	}
	return json.Marshal(res)
}
