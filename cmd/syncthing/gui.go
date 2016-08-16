// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rcrowley/go-metrics"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/discover"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/logger"
	"github.com/syncthing/syncthing/lib/model"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/stats"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/tlsutil"
	"github.com/syncthing/syncthing/lib/upgrade"
	"github.com/vitrun/qart/qr"
	"golang.org/x/crypto/bcrypt"
)

var (
	startTime = time.Now()
)

type apiService struct {
	id                 protocol.DeviceID
	cfg                configIntf
	httpsCertFile      string
	httpsKeyFile       string
	statics            *staticsServer
	model              modelIntf
	eventSub           events.BufferedSubscription
	diskEventSub       events.BufferedSubscription
	discoverer         discover.CachingMux
	connectionsService connectionsIntf
	fss                *folderSummaryService
	systemConfigMut    sync.Mutex    // serializes posts to /rest/system/config
	stop               chan struct{} // signals intentional stop
	configChanged      chan struct{} // signals intentional listener close due to config change
	started            chan string   // signals startup complete by sending the listener address, for testing only
	startedOnce        chan struct{} // the service has started successfully at least once

	guiErrors logger.Recorder
	systemLog logger.Recorder
}

type modelIntf interface {
	GlobalDirectoryTree(folder, prefix string, levels int, dirsonly bool) map[string]interface{}
	Completion(device protocol.DeviceID, folder string) model.FolderCompletion
	Override(folder string)
	NeedFolderFiles(folder string, page, perpage int) ([]db.FileInfoTruncated, []db.FileInfoTruncated, []db.FileInfoTruncated, int)
	NeedSize(folder string) db.Counts
	ConnectionStats() map[string]interface{}
	DeviceStatistics() map[string]stats.DeviceStatistics
	FolderStatistics() map[string]stats.FolderStatistics
	CurrentFolderFile(folder string, file string) (protocol.FileInfo, bool)
	CurrentGlobalFile(folder string, file string) (protocol.FileInfo, bool)
	ResetFolder(folder string)
	Availability(folder, file string, version protocol.Vector, block protocol.BlockInfo) []model.Availability
	GetIgnores(folder string) ([]string, []string, error)
	SetIgnores(folder string, content []string) error
	DelayScan(folder string, next time.Duration)
	ScanFolder(folder string) error
	ScanFolders() map[string]error
	ScanFolderSubdirs(folder string, subs []string) error
	BringToFront(folder, file string)
	ConnectedTo(deviceID protocol.DeviceID) bool
	GlobalSize(folder string) db.Counts
	LocalSize(folder string) db.Counts
	CurrentSequence(folder string) (int64, bool)
	RemoteSequence(folder string) (int64, bool)
	State(folder string) (string, time.Time, error)
}

type configIntf interface {
	GUI() config.GUIConfiguration
	RawCopy() config.Configuration
	Options() config.OptionsConfiguration
	Replace(cfg config.Configuration) error
	Subscribe(c config.Committer)
	Folders() map[string]config.FolderConfiguration
	Devices() map[protocol.DeviceID]config.DeviceConfiguration
	Save() error
	ListenAddresses() []string
	RequiresRestart() bool
}

type connectionsIntf interface {
	Status() map[string]interface{}
}

func newAPIService(id protocol.DeviceID, cfg configIntf, httpsCertFile, httpsKeyFile, assetDir string, m modelIntf, eventSub events.BufferedSubscription, diskEventSub events.BufferedSubscription, discoverer discover.CachingMux, connectionsService connectionsIntf, errors, systemLog logger.Recorder) *apiService {
	service := &apiService{
		id:                 id,
		cfg:                cfg,
		httpsCertFile:      httpsCertFile,
		httpsKeyFile:       httpsKeyFile,
		statics:            newStaticsServer(cfg.GUI().Theme, assetDir),
		model:              m,
		eventSub:           eventSub,
		diskEventSub:       diskEventSub,
		discoverer:         discoverer,
		connectionsService: connectionsService,
		systemConfigMut:    sync.NewMutex(),
		stop:               make(chan struct{}),
		configChanged:      make(chan struct{}),
		startedOnce:        make(chan struct{}),
		guiErrors:          errors,
		systemLog:          systemLog,
	}

	return service
}

func (s *apiService) getListener(guiCfg config.GUIConfiguration) (net.Listener, error) {
	cert, err := tls.LoadX509KeyPair(s.httpsCertFile, s.httpsKeyFile)
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

		cert, err = tlsutil.NewCertificate(s.httpsCertFile, s.httpsKeyFile, name, httpsRSABits)
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

	rawListener, err := net.Listen("tcp", guiCfg.Address())
	if err != nil {
		return nil, err
	}

	listener := &tlsutil.DowngradingListener{
		Listener:  rawListener,
		TLSConfig: tlsCfg,
	}
	return listener, nil
}

func sendJSON(w http.ResponseWriter, jsonObject interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	// Marshalling might fail, in which case we should return a 500 with the
	// actual error.
	bs, err := json.Marshal(jsonObject)
	if err != nil {
		// This Marshal() can't fail though.
		bs, _ = json.Marshal(map[string]string{"error": err.Error()})
		http.Error(w, string(bs), http.StatusInternalServerError)
		return
	}
	w.Write(bs)
}

func (s *apiService) Serve() {
	listener, err := s.getListener(s.cfg.GUI())
	if err != nil {
		select {
		case <-s.startedOnce:
			// We let this be a loud user-visible warning as it may be the only
			// indication they get that the GUI won't be available.
			l.Warnln("Starting API/GUI:", err)
			return

		default:
			// This is during initialization. A failure here should be fatal
			// as there will be no way for the user to communicate with us
			// otherwise anyway.
			l.Fatalln("Starting API/GUI:", err)
		}
	}

	if listener == nil {
		// Not much we can do here other than exit quickly. The supervisor
		// will log an error at some point.
		return
	}

	defer listener.Close()

	// The GET handlers
	getRestMux := http.NewServeMux()
	getRestMux.HandleFunc("/rest/db/completion", s.getDBCompletion)              // device folder
	getRestMux.HandleFunc("/rest/db/file", s.getDBFile)                          // folder file
	getRestMux.HandleFunc("/rest/db/ignores", s.getDBIgnores)                    // folder
	getRestMux.HandleFunc("/rest/db/need", s.getDBNeed)                          // folder [perpage] [page]
	getRestMux.HandleFunc("/rest/db/status", s.getDBStatus)                      // folder
	getRestMux.HandleFunc("/rest/db/browse", s.getDBBrowse)                      // folder [prefix] [dirsonly] [levels]
	getRestMux.HandleFunc("/rest/events", s.getIndexEvents)                      // since [limit]
	getRestMux.HandleFunc("/rest/events/disk", s.getDiskEvents)                  // since [limit]
	getRestMux.HandleFunc("/rest/stats/device", s.getDeviceStats)                // -
	getRestMux.HandleFunc("/rest/stats/folder", s.getFolderStats)                // -
	getRestMux.HandleFunc("/rest/svc/deviceid", s.getDeviceID)                   // id
	getRestMux.HandleFunc("/rest/svc/lang", s.getLang)                           // -
	getRestMux.HandleFunc("/rest/svc/report", s.getReport)                       // -
	getRestMux.HandleFunc("/rest/svc/random/string", s.getRandomString)          // [length]
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
	getRestMux.HandleFunc("/rest/system/debug", s.getSystemDebug)                // -
	getRestMux.HandleFunc("/rest/system/log", s.getSystemLog)                    // [since]
	getRestMux.HandleFunc("/rest/system/log.txt", s.getSystemLogTxt)             // [since]

	// The POST handlers
	postRestMux := http.NewServeMux()
	postRestMux.HandleFunc("/rest/db/prio", s.postDBPrio)                      // folder file [perpage] [page]
	postRestMux.HandleFunc("/rest/db/ignores", s.postDBIgnores)                // folder
	postRestMux.HandleFunc("/rest/db/override", s.postDBOverride)              // folder
	postRestMux.HandleFunc("/rest/db/scan", s.postDBScan)                      // folder [sub...] [delay]
	postRestMux.HandleFunc("/rest/system/config", s.postSystemConfig)          // <body>
	postRestMux.HandleFunc("/rest/system/error", s.postSystemError)            // <body>
	postRestMux.HandleFunc("/rest/system/error/clear", s.postSystemErrorClear) // -
	postRestMux.HandleFunc("/rest/system/ping", s.restPing)                    // -
	postRestMux.HandleFunc("/rest/system/reset", s.postSystemReset)            // [folder]
	postRestMux.HandleFunc("/rest/system/restart", s.postSystemRestart)        // -
	postRestMux.HandleFunc("/rest/system/shutdown", s.postSystemShutdown)      // -
	postRestMux.HandleFunc("/rest/system/upgrade", s.postSystemUpgrade)        // -
	postRestMux.HandleFunc("/rest/system/debug", s.postSystemDebug)            // [enable] [disable]

	// Debug endpoints, not for general use
	debugMux := http.NewServeMux()
	debugMux.HandleFunc("/rest/debug/peerCompletion", s.getPeerCompletion)
	debugMux.HandleFunc("/rest/debug/httpmetrics", s.getSystemHTTPMetrics)
	debugMux.HandleFunc("/rest/debug/cpuprof", s.getCPUProf) // duration
	debugMux.HandleFunc("/rest/debug/heapprof", s.getHeapProf)
	getRestMux.Handle("/rest/debug/", s.whenDebugging(debugMux))

	// A handler that splits requests between the two above and disables
	// caching
	restMux := noCacheMiddleware(metricsMiddleware(getPostHandler(getRestMux, postRestMux)))

	// The main routing handler
	mux := http.NewServeMux()
	mux.Handle("/rest/", restMux)
	mux.HandleFunc("/qr/", s.getQR)

	// Serve compiled in assets unless an asset directory was set (for development)
	mux.Handle("/", s.statics)

	// Handle the special meta.js path
	mux.HandleFunc("/meta.js", s.getJSMetadata)

	guiCfg := s.cfg.GUI()

	// Wrap everything in CSRF protection. The /rest prefix should be
	// protected, other requests will grant cookies.
	handler := csrfMiddleware(s.id.String()[:5], "/rest", guiCfg, mux)

	// Add our version and ID as a header to responses
	handler = withDetailsMiddleware(s.id, handler)

	// Wrap everything in basic auth, if user/password is set.
	if len(guiCfg.User) > 0 && len(guiCfg.Password) > 0 {
		handler = basicAuthAndSessionMiddleware("sessionid-"+s.id.String()[:5], guiCfg, handler)
	}

	// Redirect to HTTPS if we are supposed to
	if guiCfg.UseTLS() {
		handler = redirectToHTTPSMiddleware(handler)
	}

	// Add the CORS handling
	handler = corsMiddleware(handler)

	if addressIsLocalhost(guiCfg.Address()) && !guiCfg.InsecureSkipHostCheck {
		// Verify source host
		handler = localhostMiddleware(handler)
	}

	handler = debugMiddleware(handler)

	srv := http.Server{
		Handler:     handler,
		ReadTimeout: 10 * time.Second,
	}

	s.fss = newFolderSummaryService(s.cfg, s.model)
	defer s.fss.Stop()
	s.fss.ServeBackground()

	l.Infoln("GUI and API listening on", listener.Addr())
	l.Infoln("Access the GUI via the following URL:", guiCfg.URL())
	if s.started != nil {
		// only set when run by the tests
		s.started <- listener.Addr().String()
	}

	// Indicate successfull initial startup, to ourselves and to interested
	// listeners (i.e. the thing that starts the browser).
	select {
	case <-s.startedOnce:
	default:
		close(s.startedOnce)
	}

	// Serve in the background

	serveError := make(chan error, 1)
	go func() {
		serveError <- srv.Serve(listener)
	}()

	// Wait for stop, restart or error signals

	select {
	case <-s.stop:
		// Shutting down permanently
		l.Debugln("shutting down (stop)")
	case <-s.configChanged:
		// Soft restart due to configuration change
		l.Debugln("restarting (config changed)")
	case <-serveError:
		// Restart due to listen/serve failure
		l.Warnln("GUI/API:", err, "(restarting)")
	}
}

func (s *apiService) Stop() {
	close(s.stop)
}

func (s *apiService) String() string {
	return fmt.Sprintf("apiService@%p", s)
}

func (s *apiService) VerifyConfiguration(from, to config.Configuration) error {
	if _, err := net.ResolveTCPAddr("tcp", to.GUI.Address()); err != nil {
		return err
	}
	return nil
}

func (s *apiService) CommitConfiguration(from, to config.Configuration) bool {
	// No action required when this changes, so mask the fact that it changed at all.
	from.GUI.Debugging = to.GUI.Debugging

	if to.GUI == from.GUI {
		return true
	}

	if to.GUI.Theme != from.GUI.Theme {
		s.statics.setTheme(to.GUI.Theme)
	}

	// Tell the serve loop to restart
	s.configChanged <- struct{}{}

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

		if shouldDebugHTTP() {
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
			httpl.Debugf("http: %s %q: status %d, %d bytes in %.02f ms", r.Method, r.URL.String(), status, written, ms)
		}
	})
}

func corsMiddleware(next http.Handler) http.Handler {
	// Handle CORS headers and CORS OPTIONS request.
	// CORS OPTIONS request are typically sent by browser during AJAX preflight
	// when the browser initiate a POST request.
	//
	// As the OPTIONS request is unauthorized, this handler must be the first
	// of the chain (hence added at the end).
	//
	// See https://www.w3.org/TR/cors/ for details.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Process OPTIONS requests
		if r.Method == "OPTIONS" {
			// Add a generous access-control-allow-origin header for CORS requests
			w.Header().Add("Access-Control-Allow-Origin", "*")
			// Only GET/POST Methods are supported
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST")
			// Only these headers can be set
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key")
			// The request is meant to be cached 10 minutes
			w.Header().Set("Access-Control-Max-Age", "600")

			// Indicate that no content will be returned
			w.WriteHeader(204)

			return
		}

		// For everything else, pass to the next handler
		next.ServeHTTP(w, r)
		return
	})
}

func metricsMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t := metrics.GetOrRegisterTimer(r.URL.Path, nil)
		t0 := time.Now()
		h.ServeHTTP(w, r)
		t.UpdateSince(t0)
	})
}

func redirectToHTTPSMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil {
			// Redirect HTTP requests to HTTPS
			r.URL.Host = r.Host
			r.URL.Scheme = "https"
			http.Redirect(w, r, r.URL.String(), http.StatusTemporaryRedirect)
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

func localhostMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if addressIsLocalhost(r.Host) {
			h.ServeHTTP(w, r)
			return
		}

		http.Error(w, "Host check error", http.StatusForbidden)
	})
}

func (s *apiService) whenDebugging(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.GUI().Debugging {
			h.ServeHTTP(w, r)
			return
		}

		http.Error(w, "Debugging disabled", http.StatusBadRequest)
	})
}

func (s *apiService) restPing(w http.ResponseWriter, r *http.Request) {
	sendJSON(w, map[string]string{"ping": "pong"})
}

func (s *apiService) getJSMetadata(w http.ResponseWriter, r *http.Request) {
	meta, _ := json.Marshal(map[string]string{
		"deviceID": s.id.String(),
	})
	w.Header().Set("Content-Type", "application/javascript")
	fmt.Fprintf(w, "var metadata = %s;\n", meta)
}

func (s *apiService) getSystemVersion(w http.ResponseWriter, r *http.Request) {
	sendJSON(w, map[string]string{
		"version":     Version,
		"codename":    Codename,
		"longVersion": LongVersion,
		"os":          runtime.GOOS,
		"arch":        runtime.GOARCH,
	})
}

func (s *apiService) getSystemDebug(w http.ResponseWriter, r *http.Request) {
	names := l.Facilities()
	enabled := l.FacilityDebugging()
	sort.Strings(enabled)
	sendJSON(w, map[string]interface{}{
		"facilities": names,
		"enabled":    enabled,
	})
}

func (s *apiService) postSystemDebug(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	q := r.URL.Query()
	for _, f := range strings.Split(q.Get("enable"), ",") {
		if f == "" || l.ShouldDebug(f) {
			continue
		}
		l.SetDebug(f, true)
		l.Infof("Enabled debug data for %q", f)
	}
	for _, f := range strings.Split(q.Get("disable"), ",") {
		if f == "" || !l.ShouldDebug(f) {
			continue
		}
		l.SetDebug(f, false)
		l.Infof("Disabled debug data for %q", f)
	}
}

func (s *apiService) getDBBrowse(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	folder := qs.Get("folder")
	prefix := qs.Get("prefix")
	dirsonly := qs.Get("dirsonly") != ""

	levels, err := strconv.Atoi(qs.Get("levels"))
	if err != nil {
		levels = -1
	}

	sendJSON(w, s.model.GlobalDirectoryTree(folder, prefix, levels, dirsonly))
}

func (s *apiService) getDBCompletion(w http.ResponseWriter, r *http.Request) {
	var qs = r.URL.Query()
	var folder = qs.Get("folder")
	var deviceStr = qs.Get("device")

	device, err := protocol.DeviceIDFromString(deviceStr)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	comp := s.model.Completion(device, folder)
	sendJSON(w, map[string]interface{}{
		"completion":  comp.CompletionPct,
		"needBytes":   comp.NeedBytes,
		"globalBytes": comp.GlobalBytes,
		"needDeletes": comp.NeedDeletes,
	})
}

func (s *apiService) getDBStatus(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	folder := qs.Get("folder")
	sendJSON(w, folderSummary(s.cfg, s.model, folder))
}

func folderSummary(cfg configIntf, m modelIntf, folder string) map[string]interface{} {
	var res = make(map[string]interface{})

	res["invalid"] = "" // Deprecated, retains external API for now

	global := m.GlobalSize(folder)
	res["globalFiles"], res["globalDirectories"], res["globalSymlinks"], res["globalDeleted"], res["globalBytes"] = global.Files, global.Directories, global.Symlinks, global.Deleted, global.Bytes

	local := m.LocalSize(folder)
	res["localFiles"], res["localDirectories"], res["localSymlinks"], res["localDeleted"], res["localBytes"] = local.Files, local.Directories, local.Symlinks, local.Deleted, local.Bytes

	need := m.NeedSize(folder)
	res["needFiles"], res["needDirectories"], res["needSymlinks"], res["needDeletes"], res["needBytes"] = need.Files, need.Directories, need.Symlinks, need.Deleted, need.Bytes

	res["inSyncFiles"], res["inSyncBytes"] = global.Files-need.Files, global.Bytes-need.Bytes

	var err error
	res["state"], res["stateChanged"], err = m.State(folder)
	if err != nil {
		res["error"] = err.Error()
	}

	ourSeq, _ := m.CurrentSequence(folder)
	remoteSeq, _ := m.RemoteSequence(folder)

	res["version"] = ourSeq + remoteSeq  // legacy
	res["sequence"] = ourSeq + remoteSeq // new name

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

func (s *apiService) postDBOverride(w http.ResponseWriter, r *http.Request) {
	var qs = r.URL.Query()
	var folder = qs.Get("folder")
	go s.model.Override(folder)
}

func (s *apiService) getDBNeed(w http.ResponseWriter, r *http.Request) {
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
	sendJSON(w, map[string]interface{}{
		"progress": s.toNeedSlice(progress),
		"queued":   s.toNeedSlice(queued),
		"rest":     s.toNeedSlice(rest),
		"total":    total,
		"page":     page,
		"perpage":  perpage,
	})
}

func (s *apiService) getSystemConnections(w http.ResponseWriter, r *http.Request) {
	sendJSON(w, s.model.ConnectionStats())
}

func (s *apiService) getDeviceStats(w http.ResponseWriter, r *http.Request) {
	sendJSON(w, s.model.DeviceStatistics())
}

func (s *apiService) getFolderStats(w http.ResponseWriter, r *http.Request) {
	sendJSON(w, s.model.FolderStatistics())
}

func (s *apiService) getDBFile(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	folder := qs.Get("folder")
	file := qs.Get("file")
	gf, gfOk := s.model.CurrentGlobalFile(folder, file)
	lf, lfOk := s.model.CurrentFolderFile(folder, file)

	if !(gfOk || lfOk) {
		// This file for sure does not exist.
		http.Error(w, "No such object in the index", http.StatusNotFound)
		return
	}

	av := s.model.Availability(folder, file, protocol.Vector{}, protocol.BlockInfo{})
	sendJSON(w, map[string]interface{}{
		"global":       jsonFileInfo(gf),
		"local":        jsonFileInfo(lf),
		"availability": av,
	})
}

func (s *apiService) getSystemConfig(w http.ResponseWriter, r *http.Request) {
	sendJSON(w, s.cfg.RawCopy())
}

func (s *apiService) postSystemConfig(w http.ResponseWriter, r *http.Request) {
	s.systemConfigMut.Lock()
	defer s.systemConfigMut.Unlock()

	to, err := config.ReadJSON(r.Body, myID)
	r.Body.Close()
	if err != nil {
		l.Warnln("Decoding posted config:", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if to.GUI.Password != s.cfg.GUI().Password {
		if to.GUI.Password != "" {
			hash, err := bcrypt.GenerateFromPassword([]byte(to.GUI.Password), 0)
			if err != nil {
				l.Warnln("bcrypting password:", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			to.GUI.Password = string(hash)
		}
	}

	// Fixup usage reporting settings

	if curAcc := s.cfg.Options().URAccepted; to.Options.URAccepted > curAcc {
		// UR was enabled
		to.Options.URAccepted = usageReportVersion
		to.Options.URUniqueID = rand.String(8)
	} else if to.Options.URAccepted < curAcc {
		// UR was disabled
		to.Options.URAccepted = -1
		to.Options.URUniqueID = ""
	}

	// Activate and save

	if err := s.cfg.Replace(to); err != nil {
		l.Warnln("Replacing config:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := s.cfg.Save(); err != nil {
		l.Warnln("Saving config:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *apiService) getSystemConfigInsync(w http.ResponseWriter, r *http.Request) {
	sendJSON(w, map[string]bool{"configInSync": !s.cfg.RequiresRestart()})
}

func (s *apiService) postSystemRestart(w http.ResponseWriter, r *http.Request) {
	s.flushResponse(`{"ok": "restarting"}`, w)
	go restart()
}

func (s *apiService) postSystemReset(w http.ResponseWriter, r *http.Request) {
	var qs = r.URL.Query()
	folder := qs.Get("folder")

	if len(folder) > 0 {
		if _, ok := s.cfg.Folders()[folder]; !ok {
			http.Error(w, "Invalid folder ID", 500)
			return
		}
	}

	if len(folder) == 0 {
		// Reset all folders.
		for folder := range s.cfg.Folders() {
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

func (s *apiService) postSystemShutdown(w http.ResponseWriter, r *http.Request) {
	s.flushResponse(`{"ok": "shutting down"}`, w)
	go shutdown()
}

func (s *apiService) flushResponse(resp string, w http.ResponseWriter) {
	w.Write([]byte(resp + "\n"))
	f := w.(http.Flusher)
	f.Flush()
}

var cpuUsagePercent [10]float64 // The last ten seconds
var cpuUsageLock = sync.NewRWMutex()

func (s *apiService) getSystemStatus(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	tilde, _ := osutil.ExpandTilde("~")
	res := make(map[string]interface{})
	res["myID"] = myID.String()
	res["goroutines"] = runtime.NumGoroutine()
	res["alloc"] = m.Alloc
	res["sys"] = m.Sys - m.HeapReleased
	res["tilde"] = tilde
	if s.cfg.Options().LocalAnnEnabled || s.cfg.Options().GlobalAnnEnabled {
		res["discoveryEnabled"] = true
		discoErrors := make(map[string]string)
		discoMethods := 0
		for disco, err := range s.discoverer.ChildErrors() {
			discoMethods++
			if err != nil {
				discoErrors[disco] = err.Error()
			}
		}
		res["discoveryMethods"] = discoMethods
		res["discoveryErrors"] = discoErrors
	}

	res["connectionServiceStatus"] = s.connectionsService.Status()

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

	sendJSON(w, res)
}

func (s *apiService) getSystemError(w http.ResponseWriter, r *http.Request) {
	sendJSON(w, map[string][]logger.Line{
		"errors": s.guiErrors.Since(time.Time{}),
	})
}

func (s *apiService) postSystemError(w http.ResponseWriter, r *http.Request) {
	bs, _ := ioutil.ReadAll(r.Body)
	r.Body.Close()
	l.Warnln(string(bs))
}

func (s *apiService) postSystemErrorClear(w http.ResponseWriter, r *http.Request) {
	s.guiErrors.Clear()
}

func (s *apiService) getSystemLog(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	since, err := time.Parse(time.RFC3339, q.Get("since"))
	l.Debugln(err)
	sendJSON(w, map[string][]logger.Line{
		"messages": s.systemLog.Since(since),
	})
}

func (s *apiService) getSystemLogTxt(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	since, err := time.Parse(time.RFC3339, q.Get("since"))
	l.Debugln(err)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	for _, line := range s.systemLog.Since(since) {
		fmt.Fprintf(w, "%s: %s\n", line.When.Format(time.RFC3339), line.Message)
	}
}

func (s *apiService) getSystemHTTPMetrics(w http.ResponseWriter, r *http.Request) {
	stats := make(map[string]interface{})
	metrics.Each(func(name string, intf interface{}) {
		if m, ok := intf.(*metrics.StandardTimer); ok {
			pct := m.Percentiles([]float64{0.50, 0.95, 0.99})
			for i := range pct {
				pct[i] /= 1e6 // ns to ms
			}
			stats[name] = map[string]interface{}{
				"count":         m.Count(),
				"sumMs":         m.Sum() / 1e6, // ns to ms
				"ratesPerS":     []float64{m.Rate1(), m.Rate5(), m.Rate15()},
				"percentilesMs": pct,
			}
		}
	})
	bs, _ := json.MarshalIndent(stats, "", "  ")
	w.Write(bs)
}

func (s *apiService) getSystemDiscovery(w http.ResponseWriter, r *http.Request) {
	devices := make(map[string]discover.CacheEntry)

	if s.discoverer != nil {
		// Device ids can't be marshalled as keys so we need to manually
		// rebuild this map using strings. Discoverer may be nil if discovery
		// has not started yet.
		for device, entry := range s.discoverer.Cache() {
			devices[device.String()] = entry
		}
	}

	sendJSON(w, devices)
}

func (s *apiService) getReport(w http.ResponseWriter, r *http.Request) {
	sendJSON(w, reportData(s.cfg, s.model))
}

func (s *apiService) getRandomString(w http.ResponseWriter, r *http.Request) {
	length := 32
	if val, _ := strconv.Atoi(r.URL.Query().Get("length")); val > 0 {
		length = val
	}
	str := rand.String(length)

	sendJSON(w, map[string]string{"random": str})
}

func (s *apiService) getDBIgnores(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()

	ignores, patterns, err := s.model.GetIgnores(qs.Get("folder"))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	sendJSON(w, map[string][]string{
		"ignore":   ignores,
		"expanded": patterns,
	})
}

func (s *apiService) postDBIgnores(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()

	bs, err := ioutil.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	var data map[string][]string
	err = json.Unmarshal(bs, &data)
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

func (s *apiService) getIndexEvents(w http.ResponseWriter, r *http.Request) {
	s.fss.gotEventRequest()
	s.getEvents(w, r, s.eventSub)
}

func (s *apiService) getDiskEvents(w http.ResponseWriter, r *http.Request) {
	s.getEvents(w, r, s.diskEventSub)
}

func (s *apiService) getEvents(w http.ResponseWriter, r *http.Request, eventSub events.BufferedSubscription) {
	qs := r.URL.Query()
	sinceStr := qs.Get("since")
	limitStr := qs.Get("limit")
	since, _ := strconv.Atoi(sinceStr)
	limit, _ := strconv.Atoi(limitStr)

	// Flush before blocking, to indicate that we've received the request and
	// that it should not be retried. Must set Content-Type header before
	// flushing.
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	f := w.(http.Flusher)
	f.Flush()

	evs := eventSub.Since(since, nil)
	if 0 < limit && limit < len(evs) {
		evs = evs[len(evs)-limit:]
	}

	sendJSON(w, evs)
}

func (s *apiService) getSystemUpgrade(w http.ResponseWriter, r *http.Request) {
	if noUpgrade {
		http.Error(w, upgrade.ErrUpgradeUnsupported.Error(), 500)
		return
	}
	rel, err := upgrade.LatestRelease(s.cfg.Options().ReleasesURL, Version)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	res := make(map[string]interface{})
	res["running"] = Version
	res["latest"] = rel.Tag
	res["newer"] = upgrade.CompareVersions(rel.Tag, Version) == upgrade.Newer
	res["majorNewer"] = upgrade.CompareVersions(rel.Tag, Version) == upgrade.MajorNewer

	sendJSON(w, res)
}

func (s *apiService) getDeviceID(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	idStr := qs.Get("id")
	id, err := protocol.DeviceIDFromString(idStr)

	if err == nil {
		sendJSON(w, map[string]string{
			"id": id.String(),
		})
	} else {
		sendJSON(w, map[string]string{
			"error": err.Error(),
		})
	}
}

func (s *apiService) getLang(w http.ResponseWriter, r *http.Request) {
	lang := r.Header.Get("Accept-Language")
	var langs []string
	for _, l := range strings.Split(lang, ",") {
		parts := strings.SplitN(l, ";", 2)
		langs = append(langs, strings.ToLower(strings.TrimSpace(parts[0])))
	}
	sendJSON(w, langs)
}

func (s *apiService) postSystemUpgrade(w http.ResponseWriter, r *http.Request) {
	rel, err := upgrade.LatestRelease(s.cfg.Options().ReleasesURL, Version)
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

func (s *apiService) postDBScan(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	folder := qs.Get("folder")
	if folder != "" {
		nextStr := qs.Get("next")
		next, err := strconv.Atoi(nextStr)
		if err == nil {
			s.model.DelayScan(folder, time.Duration(next)*time.Second)
		}

		subs := qs["sub"]
		err = s.model.ScanFolderSubdirs(folder, subs)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	} else {
		errors := s.model.ScanFolders()
		if len(errors) > 0 {
			http.Error(w, "Error scanning folders", 500)
			sendJSON(w, errors)
			return
		}
	}
}

func (s *apiService) postDBPrio(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	folder := qs.Get("folder")
	file := qs.Get("file")
	s.model.BringToFront(folder, file)
	s.getDBNeed(w, r)
}

func (s *apiService) getQR(w http.ResponseWriter, r *http.Request) {
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

func (s *apiService) getPeerCompletion(w http.ResponseWriter, r *http.Request) {
	tot := map[string]float64{}
	count := map[string]float64{}

	for _, folder := range s.cfg.Folders() {
		for _, device := range folder.DeviceIDs() {
			deviceStr := device.String()
			if s.model.ConnectedTo(device) {
				tot[deviceStr] += s.model.Completion(device, folder.ID).CompletionPct
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

	sendJSON(w, comp)
}

func (s *apiService) getSystemBrowse(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	current := qs.Get("current")
	if current == "" {
		if roots, err := osutil.GetFilesystemRoots(); err == nil {
			sendJSON(w, roots)
		} else {
			http.Error(w, err.Error(), 500)
		}
		return
	}
	search, _ := osutil.ExpandTilde(current)
	pathSeparator := string(os.PathSeparator)
	if strings.HasSuffix(current, pathSeparator) && !strings.HasSuffix(search, pathSeparator) {
		search = search + pathSeparator
	}
	subdirectories, _ := osutil.Glob(search + "*")
	ret := make([]string, 0, len(subdirectories))
	for _, subdirectory := range subdirectories {
		info, err := os.Stat(subdirectory)
		if err == nil && info.IsDir() {
			ret = append(ret, subdirectory+pathSeparator)
		}
	}

	sendJSON(w, ret)
}

func (s *apiService) getCPUProf(w http.ResponseWriter, r *http.Request) {
	duration, err := time.ParseDuration(r.FormValue("duration"))
	if err != nil {
		duration = 30 * time.Second
	}

	filename := fmt.Sprintf("syncthing-cpu-%s-%s-%s-%s.pprof", runtime.GOOS, runtime.GOARCH, Version, time.Now().Format("150405")) // hhmmss

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)

	pprof.StartCPUProfile(w)
	time.Sleep(duration)
	pprof.StopCPUProfile()
}

func (s *apiService) getHeapProf(w http.ResponseWriter, r *http.Request) {
	filename := fmt.Sprintf("syncthing-heap-%s-%s-%s-%s.pprof", runtime.GOOS, runtime.GOARCH, Version, time.Now().Format("150405")) // hhmmss

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)

	runtime.GC()
	pprof.WriteHeapProfile(w)
}

func (s *apiService) toNeedSlice(fs []db.FileInfoTruncated) []jsonDBFileInfo {
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
		"name":          f.Name,
		"type":          f.Type,
		"size":          f.Size,
		"permissions":   fmt.Sprintf("%#o", f.Permissions),
		"deleted":       f.Deleted,
		"invalid":       f.Invalid,
		"noPermissions": f.NoPermissions,
		"modified":      protocol.FileInfo(f).ModTime(),
		"sequence":      f.Sequence,
		"numBlocks":     len(f.Blocks),
		"version":       jsonVersionVector(f.Version),
	})
}

type jsonDBFileInfo db.FileInfoTruncated

func (f jsonDBFileInfo) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"name":          f.Name,
		"type":          f.Type,
		"size":          f.Size,
		"permissions":   fmt.Sprintf("%#o", f.Permissions),
		"deleted":       f.Deleted,
		"invalid":       f.Invalid,
		"noPermissions": f.NoPermissions,
		"modified":      db.FileInfoTruncated(f).ModTime(),
		"sequence":      f.Sequence,
	})
}

type jsonVersionVector protocol.Vector

func (v jsonVersionVector) MarshalJSON() ([]byte, error) {
	res := make([]string, len(v.Counters))
	for i, c := range v.Counters {
		res[i] = fmt.Sprintf("%v:%d", c.ID, c.Value)
	}
	return json.Marshal(res)
}

func dirNames(dir string) []string {
	fd, err := os.Open(dir)
	if err != nil {
		return nil
	}
	defer fd.Close()

	fis, err := fd.Readdir(-1)
	if err != nil {
		return nil
	}

	var dirs []string
	for _, fi := range fis {
		if fi.IsDir() {
			dirs = append(dirs, filepath.Base(fi.Name()))
		}
	}

	sort.Strings(dirs)
	return dirs
}

func addressIsLocalhost(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// There was no port, so we assume the address was just a hostname
		host = addr
	}
	switch strings.ToLower(host) {
	case "127.0.0.1", "::1", "localhost":
		return true
	default:
		return false
	}
}
