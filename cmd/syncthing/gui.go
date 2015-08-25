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
	"github.com/syncthing/syncthing/lib/auto"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/discover"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/model"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/upgrade"
	"github.com/vitrun/qart/qr"
	"golang.org/x/crypto/bcrypt"
)

type guiError struct {
	Time  time.Time `json:"time"`
	Error string    `json:"error"`
}

var (
	configInSync = true
	guiErrors    = []guiError{}
	guiErrorsMut = sync.NewMutex()
	startTime    = time.Now()
)

type apiSvc struct {
	id              protocol.DeviceID
	cfg             config.GUIConfiguration
	assetDir        string
	model           *model.Model
	listener        net.Listener
	fss             *folderSummarySvc
	stop            chan struct{}
	systemConfigMut sync.Mutex
	eventSub        *events.BufferedSubscription
}

func newAPISvc(id protocol.DeviceID, cfg config.GUIConfiguration, assetDir string, m *model.Model, eventSub *events.BufferedSubscription) (*apiSvc, error) {
	svc := &apiSvc{
		id:              id,
		cfg:             cfg,
		assetDir:        assetDir,
		model:           m,
		systemConfigMut: sync.NewMutex(),
		eventSub:        eventSub,
	}

	var err error
	svc.listener, err = svc.getListener(cfg)
	return svc, err
}

func (s *apiSvc) getListener(cfg config.GUIConfiguration) (net.Listener, error) {
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
		return nil, err
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
		return nil, err
	}

	listener := &DowngradingListener{rawListener, tlsCfg}
	return listener, nil
}

func (s *apiSvc) Serve() {
	s.stop = make(chan struct{})

	l.AddHandler(logger.LevelWarn, s.showGuiError)

	// The GET handlers
	getRestMux := http.NewServeMux()
	getRestMux.HandleFunc("/rest/db/completion", s.getDBCompletion)              // device folder
	getRestMux.HandleFunc("/rest/db/file", s.getDBFile)                          // folder file
	getRestMux.HandleFunc("/rest/db/ignores", s.getDBIgnores)                    // folder
	getRestMux.HandleFunc("/rest/db/need", s.getDBNeed)                          // folder [perpage] [page]
	getRestMux.HandleFunc("/rest/db/status", s.getDBStatus)                      // folder
	getRestMux.HandleFunc("/rest/db/browse", s.getDBBrowse)                      // folder [prefix] [dirsonly] [levels]
	getRestMux.HandleFunc("/rest/events", s.getEvents)                           // since [limit]
	getRestMux.HandleFunc("/rest/stats/device", s.getDeviceStats)                // -
	getRestMux.HandleFunc("/rest/stats/folder", s.getFolderStats)                // -
	getRestMux.HandleFunc("/rest/svc/deviceid", s.getDeviceID)                   // id
	getRestMux.HandleFunc("/rest/svc/lang", s.getLang)                           // -
	getRestMux.HandleFunc("/rest/svc/report", s.getReport)                       // -
	getRestMux.HandleFunc("/rest/system/browse", s.getSystemBrowse)              // current
	getRestMux.HandleFunc("/rest/system/config", s.getSystemConfig)              // -
	getRestMux.HandleFunc("/rest/system/config/insync", s.getSystemConfigInsync) // -
	getRestMux.HandleFunc("/rest/system/connections", s.getSystemConnections)    // -
	getRestMux.HandleFunc("/rest/system/discovery", s.getSystemDiscovery)        // -
	getRestMux.HandleFunc("/rest/system/error", s.getSystemError)                // -
	getRestMux.HandleFunc("/rest/system/ping", s.restPing)                       // -
	getRestMux.HandleFunc("/rest/system/status", s.getSystemStatus)              // -
	getRestMux.HandleFunc("/rest/system/upgrade", s.getSystemUpgrade)            // -
	getRestMux.HandleFunc("/rest/system/version", s.getSystemVersion)            // -

	// The POST handlers
	postRestMux := http.NewServeMux()
	postRestMux.HandleFunc("/rest/db/prio", s.postDBPrio)                      // folder file [perpage] [page]
	postRestMux.HandleFunc("/rest/db/ignores", s.postDBIgnores)                // folder
	postRestMux.HandleFunc("/rest/db/override", s.postDBOverride)              // folder
	postRestMux.HandleFunc("/rest/db/scan", s.postDBScan)                      // folder [sub...] [delay]
	postRestMux.HandleFunc("/rest/system/config", s.postSystemConfig)          // <body>
	postRestMux.HandleFunc("/rest/system/discovery", s.postSystemDiscovery)    // device addr
	postRestMux.HandleFunc("/rest/system/error", s.postSystemError)            // <body>
	postRestMux.HandleFunc("/rest/system/error/clear", s.postSystemErrorClear) // -
	postRestMux.HandleFunc("/rest/system/ping", s.restPing)                    // -
	postRestMux.HandleFunc("/rest/system/reset", s.postSystemReset)            // [folder]
	postRestMux.HandleFunc("/rest/system/restart", s.postSystemRestart)        // -
	postRestMux.HandleFunc("/rest/system/shutdown", s.postSystemShutdown)      // -
	postRestMux.HandleFunc("/rest/system/upgrade", s.postSystemUpgrade)        // -
	postRestMux.HandleFunc("/rest/system/pause", s.postSystemPause)            // device
	postRestMux.HandleFunc("/rest/system/resume", s.postSystemResume)          // device

	// Debug endpoints, not for general use
	getRestMux.HandleFunc("/rest/debug/peerCompletion", s.getPeerCompletion)

	// A handler that splits requests between the two above and disables
	// caching
	restMux := noCacheMiddleware(getPostHandler(getRestMux, postRestMux))

	// The main routing handler
	mux := http.NewServeMux()
	mux.Handle("/rest/", restMux)
	mux.HandleFunc("/qr/", s.getQR)

	// Serve compiled in assets unless an asset directory was set (for development)
	mux.Handle("/", embeddedStatic{
		assetDir: s.assetDir,
		assets:   auto.Assets(),
	})

	// Wrap everything in CSRF protection. The /rest prefix should be
	// protected, other requests will grant cookies.
	handler := csrfMiddleware(s.id.String()[:5], "/rest", s.cfg.APIKey, mux)

	// Add our version and ID as a header to responses
	handler = withDetailsMiddleware(s.id, handler)

	// Wrap everything in basic auth, if user/password is set.
	if len(s.cfg.User) > 0 && len(s.cfg.Password) > 0 {
		handler = basicAuthAndSessionMiddleware("sessionid-"+s.id.String()[:5], s.cfg, handler)
	}

	// Redirect to HTTPS if we are supposed to
	if s.cfg.UseTLS {
		handler = redirectToHTTPSMiddleware(handler)
	}

	if debugHTTP {
		handler = debugMiddleware(handler)
	}

	srv := http.Server{
		Handler:     handler,
		ReadTimeout: 10 * time.Second,
	}

	s.fss = newFolderSummarySvc(s.model)
	defer s.fss.Stop()
	s.fss.ServeBackground()

	l.Infoln("API listening on", s.listener.Addr())
	err := srv.Serve(s.listener)

	// The return could be due to an intentional close. Wait for the stop
	// signal before returning. IF there is no stop signal within a second, we
	// assume it was unintentional and log the error before retrying.
	select {
	case <-s.stop:
	case <-time.After(time.Second):
		l.Warnln("API:", err)
	}
}

func (s *apiSvc) Stop() {
	close(s.stop)
	s.listener.Close()
}

func (s *apiSvc) String() string {
	return fmt.Sprintf("apiSvc@%p", s)
}

func (s *apiSvc) VerifyConfiguration(from, to config.Configuration) error {
	return nil
}

func (s *apiSvc) CommitConfiguration(from, to config.Configuration) bool {
	if to.GUI == from.GUI {
		return true
	}

	// Order here is important. We must close the listener to stop Serve(). We
	// must create a new listener before Serve() starts again. We can't create
	// a new listener on the same port before the previous listener is closed.
	// To assist in this little dance the Serve() method will wait for a
	// signal on the stop channel after the listener has closed.

	s.listener.Close()

	var err error
	s.listener, err = s.getListener(to.GUI)
	if err != nil {
		// Ideally this should be a verification error, but we check it by
		// creating a new listener which requires shutting down the previous
		// one first, which is too destructive for the VerifyConfiguration
		// method.
		return false
	}
	s.cfg = to.GUI

	close(s.stop)

	return true
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
		w.Header().Set("Cache-Control", "max-age=0, no-cache, no-store")
		w.Header().Set("Expires", time.Now().UTC().Format(http.TimeFormat))
		w.Header().Set("Pragma", "no-cache")
		h.ServeHTTP(w, r)
	})
}

func withDetailsMiddleware(id protocol.DeviceID, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Syncthing-Version", Version)
		w.Header().Set("X-Syncthing-ID", id.String())
		h.ServeHTTP(w, r)
	})
}

func (s *apiSvc) restPing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]string{
		"ping": "pong",
	})
}

func (s *apiSvc) getSystemVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]string{
		"version":     Version,
		"codename":    Codename,
		"longVersion": LongVersion,
		"os":          runtime.GOOS,
		"arch":        runtime.GOARCH,
	})
}

func (s *apiSvc) getDBBrowse(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	folder := qs.Get("folder")
	prefix := qs.Get("prefix")
	dirsonly := qs.Get("dirsonly") != ""

	levels, err := strconv.Atoi(qs.Get("levels"))
	if err != nil {
		levels = -1
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	tree := s.model.GlobalDirectoryTree(folder, prefix, levels, dirsonly)

	json.NewEncoder(w).Encode(tree)
}

func (s *apiSvc) getDBCompletion(w http.ResponseWriter, r *http.Request) {
	var qs = r.URL.Query()
	var folder = qs.Get("folder")
	var deviceStr = qs.Get("device")

	device, err := protocol.DeviceIDFromString(deviceStr)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	res := map[string]float64{
		"completion": s.model.Completion(device, folder),
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(res)
}

func (s *apiSvc) getDBStatus(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	folder := qs.Get("folder")
	res := folderSummary(s.model, folder)
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

	lv, _ := m.CurrentLocalVersion(folder)
	rv, _ := m.RemoteLocalVersion(folder)

	res["version"] = lv + rv

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

func (s *apiSvc) postDBOverride(w http.ResponseWriter, r *http.Request) {
	var qs = r.URL.Query()
	var folder = qs.Get("folder")
	go s.model.Override(folder)
}

func (s *apiSvc) getDBNeed(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()

	folder := qs.Get("folder")

	page, err := strconv.Atoi(qs.Get("page"))
	if err != nil || page < 1 {
		page = 1
	}
	perpage, err := strconv.Atoi(qs.Get("perpage"))
	if err != nil || perpage < 1 {
		perpage = 1 << 16
	}

	progress, queued, rest, total := s.model.NeedFolderFiles(folder, page, perpage)

	// Convert the struct to a more loose structure, and inject the size.
	output := map[string]interface{}{
		"progress": s.toNeedSlice(progress),
		"queued":   s.toNeedSlice(queued),
		"rest":     s.toNeedSlice(rest),
		"total":    total,
		"page":     page,
		"perpage":  perpage,
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(output)
}

func (s *apiSvc) getSystemConnections(w http.ResponseWriter, r *http.Request) {
	var res = s.model.ConnectionStats()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(res)
}

func (s *apiSvc) getDeviceStats(w http.ResponseWriter, r *http.Request) {
	var res = s.model.DeviceStatistics()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(res)
}

func (s *apiSvc) getFolderStats(w http.ResponseWriter, r *http.Request) {
	var res = s.model.FolderStatistics()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(res)
}

func (s *apiSvc) getDBFile(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	folder := qs.Get("folder")
	file := qs.Get("file")
	gf, _ := s.model.CurrentGlobalFile(folder, file)
	lf, _ := s.model.CurrentFolderFile(folder, file)

	av := s.model.Availability(folder, file)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"global":       jsonFileInfo(gf),
		"local":        jsonFileInfo(lf),
		"availability": av,
	})
}

func (s *apiSvc) getSystemConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(cfg.Raw())
}

func (s *apiSvc) postSystemConfig(w http.ResponseWriter, r *http.Request) {
	s.systemConfigMut.Lock()
	defer s.systemConfigMut.Unlock()

	var to config.Configuration
	err := json.NewDecoder(r.Body).Decode(&to)
	if err != nil {
		l.Warnln("decoding posted config:", err)
		http.Error(w, err.Error(), 500)
		return
	}

	if to.GUI.Password != cfg.GUI().Password {
		if to.GUI.Password != "" {
			hash, err := bcrypt.GenerateFromPassword([]byte(to.GUI.Password), 0)
			if err != nil {
				l.Warnln("bcrypting password:", err)
				http.Error(w, err.Error(), 500)
				return
			}

			to.GUI.Password = string(hash)
		}
	}

	// Fixup usage reporting settings

	if curAcc := cfg.Options().URAccepted; to.Options.URAccepted > curAcc {
		// UR was enabled
		to.Options.URAccepted = usageReportVersion
		to.Options.URUniqueID = randomString(8)
	} else if to.Options.URAccepted < curAcc {
		// UR was disabled
		to.Options.URAccepted = -1
		to.Options.URUniqueID = ""
	}

	// Activate and save

	resp := cfg.Replace(to)
	configInSync = !resp.RequiresRestart
	cfg.Save()
}

func (s *apiSvc) getSystemConfigInsync(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]bool{"configInSync": configInSync})
}

func (s *apiSvc) postSystemRestart(w http.ResponseWriter, r *http.Request) {
	s.flushResponse(`{"ok": "restarting"}`, w)
	go restart()
}

func (s *apiSvc) postSystemReset(w http.ResponseWriter, r *http.Request) {
	var qs = r.URL.Query()
	folder := qs.Get("folder")

	if len(folder) > 0 {
		if _, ok := cfg.Folders()[folder]; !ok {
			http.Error(w, "Invalid folder ID", 500)
			return
		}
	}

	if len(folder) == 0 {
		// Reset all folders.
		for folder := range cfg.Folders() {
			s.model.ResetFolder(folder)
		}
		s.flushResponse(`{"ok": "resetting database"}`, w)
	} else {
		// Reset a specific folder, assuming it's supposed to exist.
		s.model.ResetFolder(folder)
		s.flushResponse(`{"ok": "resetting folder `+folder+`"}`, w)
	}

	go restart()
}

func (s *apiSvc) postSystemShutdown(w http.ResponseWriter, r *http.Request) {
	s.flushResponse(`{"ok": "shutting down"}`, w)
	go shutdown()
}

func (s *apiSvc) flushResponse(resp string, w http.ResponseWriter) {
	w.Write([]byte(resp + "\n"))
	f := w.(http.Flusher)
	f.Flush()
}

var cpuUsagePercent [10]float64 // The last ten seconds
var cpuUsageLock = sync.NewRWMutex()

func (s *apiSvc) getSystemStatus(w http.ResponseWriter, r *http.Request) {
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
	if relaySvc != nil {
		res["relayClientStatus"] = relaySvc.ClientStatus()
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
	res["startTime"] = startTime

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(res)
}

func (s *apiSvc) getSystemError(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	guiErrorsMut.Lock()
	json.NewEncoder(w).Encode(map[string][]guiError{"errors": guiErrors})
	guiErrorsMut.Unlock()
}

func (s *apiSvc) postSystemError(w http.ResponseWriter, r *http.Request) {
	bs, _ := ioutil.ReadAll(r.Body)
	r.Body.Close()
	s.showGuiError(0, string(bs))
}

func (s *apiSvc) postSystemErrorClear(w http.ResponseWriter, r *http.Request) {
	guiErrorsMut.Lock()
	guiErrors = []guiError{}
	guiErrorsMut.Unlock()
}

func (s *apiSvc) showGuiError(l logger.LogLevel, err string) {
	guiErrorsMut.Lock()
	guiErrors = append(guiErrors, guiError{time.Now(), err})
	if len(guiErrors) > 5 {
		guiErrors = guiErrors[len(guiErrors)-5:]
	}
	guiErrorsMut.Unlock()
}

func (s *apiSvc) postSystemDiscovery(w http.ResponseWriter, r *http.Request) {
	var qs = r.URL.Query()
	var device = qs.Get("device")
	var addr = qs.Get("addr")
	if len(device) != 0 && len(addr) != 0 && discoverer != nil {
		discoverer.Hint(device, []string{addr})
	}
}

func (s *apiSvc) getSystemDiscovery(w http.ResponseWriter, r *http.Request) {
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

func (s *apiSvc) getReport(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(reportData(s.model))
}

func (s *apiSvc) getDBIgnores(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	ignores, patterns, err := s.model.GetIgnores(qs.Get("folder"))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	json.NewEncoder(w).Encode(map[string][]string{
		"ignore":   ignores,
		"patterns": patterns,
	})
}

func (s *apiSvc) postDBIgnores(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()

	var data map[string][]string
	err := json.NewDecoder(r.Body).Decode(&data)
	r.Body.Close()

	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	err = s.model.SetIgnores(qs.Get("folder"), data["ignore"])
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	s.getDBIgnores(w, r)
}

func (s *apiSvc) getEvents(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	sinceStr := qs.Get("since")
	limitStr := qs.Get("limit")
	since, _ := strconv.Atoi(sinceStr)
	limit, _ := strconv.Atoi(limitStr)

	s.fss.gotEventRequest()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	// Flush before blocking, to indicate that we've received the request
	// and that it should not be retried.
	f := w.(http.Flusher)
	f.Flush()

	evs := s.eventSub.Since(since, nil)
	if 0 < limit && limit < len(evs) {
		evs = evs[len(evs)-limit:]
	}

	json.NewEncoder(w).Encode(evs)
}

func (s *apiSvc) getSystemUpgrade(w http.ResponseWriter, r *http.Request) {
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
	res["newer"] = upgrade.CompareVersions(rel.Tag, Version) == upgrade.Newer
	res["majorNewer"] = upgrade.CompareVersions(rel.Tag, Version) == upgrade.MajorNewer

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(res)
}

func (s *apiSvc) getDeviceID(w http.ResponseWriter, r *http.Request) {
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

func (s *apiSvc) getLang(w http.ResponseWriter, r *http.Request) {
	lang := r.Header.Get("Accept-Language")
	var langs []string
	for _, l := range strings.Split(lang, ",") {
		parts := strings.SplitN(l, ";", 2)
		langs = append(langs, strings.ToLower(strings.TrimSpace(parts[0])))
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(langs)
}

func (s *apiSvc) postSystemUpgrade(w http.ResponseWriter, r *http.Request) {
	rel, err := upgrade.LatestRelease(Version)
	if err != nil {
		l.Warnln("getting latest release:", err)
		http.Error(w, err.Error(), 500)
		return
	}

	if upgrade.CompareVersions(rel.Tag, Version) > upgrade.Equal {
		err = upgrade.To(rel)
		if err != nil {
			l.Warnln("upgrading:", err)
			http.Error(w, err.Error(), 500)
			return
		}

		s.flushResponse(`{"ok": "restarting"}`, w)
		l.Infoln("Upgrading")
		stop <- exitUpgrading
	}
}

func (s *apiSvc) postSystemPause(w http.ResponseWriter, r *http.Request) {
	var qs = r.URL.Query()
	var deviceStr = qs.Get("device")

	device, err := protocol.DeviceIDFromString(deviceStr)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	s.model.PauseDevice(device)
}

func (s *apiSvc) postSystemResume(w http.ResponseWriter, r *http.Request) {
	var qs = r.URL.Query()
	var deviceStr = qs.Get("device")

	device, err := protocol.DeviceIDFromString(deviceStr)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	s.model.ResumeDevice(device)
}

func (s *apiSvc) postDBScan(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	folder := qs.Get("folder")
	if folder != "" {
		nextStr := qs.Get("next")
		next, err := strconv.Atoi(nextStr)
		if err == nil {
			s.model.DelayScan(folder, time.Duration(next)*time.Second)
		}

		subs := qs["sub"]
		err = s.model.ScanFolderSubs(folder, subs)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	} else {
		errors := s.model.ScanFolders()
		if len(errors) > 0 {
			http.Error(w, "Error scanning folders", 500)
			json.NewEncoder(w).Encode(errors)
			return
		}
	}
}

func (s *apiSvc) postDBPrio(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	folder := qs.Get("folder")
	file := qs.Get("file")
	s.model.BringToFront(folder, file)
	s.getDBNeed(w, r)
}

func (s *apiSvc) getQR(w http.ResponseWriter, r *http.Request) {
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

func (s *apiSvc) getPeerCompletion(w http.ResponseWriter, r *http.Request) {
	tot := map[string]float64{}
	count := map[string]float64{}

	for _, folder := range cfg.Folders() {
		for _, device := range folder.DeviceIDs() {
			deviceStr := device.String()
			if s.model.ConnectedTo(device) {
				tot[deviceStr] += s.model.Completion(device, folder.ID)
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

func (s *apiSvc) getSystemBrowse(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	qs := r.URL.Query()
	current := qs.Get("current")
	search, _ := osutil.ExpandTilde(current)
	pathSeparator := string(os.PathSeparator)
	if strings.HasSuffix(current, pathSeparator) && !strings.HasSuffix(search, pathSeparator) {
		search = search + pathSeparator
	}
	subdirectories, _ := osutil.Glob(search + "*")
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

type embeddedStatic struct {
	assetDir string
	assets   map[string][]byte
}

func (s embeddedStatic) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	file := r.URL.Path

	if file[0] == '/' {
		file = file[1:]
	}

	if len(file) == 0 {
		file = "index.html"
	}

	if s.assetDir != "" {
		p := filepath.Join(s.assetDir, filepath.FromSlash(file))
		_, err := os.Stat(p)
		if err == nil {
			http.ServeFile(w, r, p)
			return
		}
	}

	bs, ok := s.assets[file]
	if !ok {
		http.NotFound(w, r)
		return
	}

	if r.Header.Get("If-Modified-Since") == auto.AssetsBuildDate {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	mtype := s.mimeTypeForFile(file)
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
	w.Header().Set("Cache-Control", "public")

	w.Write(bs)
}

func (s embeddedStatic) mimeTypeForFile(file string) string {
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

func (s *apiSvc) toNeedSlice(fs []db.FileInfoTruncated) []jsonDBFileInfo {
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
