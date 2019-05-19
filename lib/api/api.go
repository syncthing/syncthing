// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package api

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"time"

	metrics "github.com/rcrowley/go-metrics"
	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/connections"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/discover"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/locations"
	"github.com/syncthing/syncthing/lib/logger"
	"github.com/syncthing/syncthing/lib/model"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/tlsutil"
	"github.com/syncthing/syncthing/lib/upgrade"
	"github.com/syncthing/syncthing/lib/ur"
	"github.com/thejerf/suture"
	"github.com/vitrun/qart/qr"
	"golang.org/x/crypto/bcrypt"
)

// matches a bcrypt hash and not too much else
var bcryptExpr = regexp.MustCompile(`^\$2[aby]\$\d+\$.{50,}`)

const (
	DefaultEventMask    = events.AllEvents &^ events.LocalChangeDetected &^ events.RemoteChangeDetected
	DiskEventMask       = events.LocalChangeDetected | events.RemoteChangeDetected
	EventSubBufferSize  = 1000
	defaultEventTimeout = time.Minute
)

type service struct {
	id                   protocol.DeviceID
	cfg                  config.Wrapper
	statics              *staticsServer
	model                model.Model
	eventSubs            map[events.EventType]events.BufferedSubscription
	eventSubsMut         sync.Mutex
	discoverer           discover.CachingMux
	connectionsService   connections.Service
	fss                  model.FolderSummaryService
	urService            *ur.Service
	systemConfigMut      sync.Mutex // serializes posts to /rest/system/config
	cpu                  Rater
	contr                Controller
	noUpgrade            bool
	tlsDefaultCommonName string
	stop                 chan struct{} // signals intentional stop
	configChanged        chan struct{} // signals intentional listener close due to config change
	started              chan string   // signals startup complete by sending the listener address, for testing only
	startedOnce          chan struct{} // the service has started successfully at least once
	startupErr           error

	guiErrors logger.Recorder
	systemLog logger.Recorder
}

type Rater interface {
	Rate() float64
}

type Controller interface {
	ExitUpgrading()
	Restart()
	Shutdown()
}

type Service interface {
	suture.Service
	config.Committer
	WaitForStart() error
}

func New(id protocol.DeviceID, cfg config.Wrapper, assetDir, tlsDefaultCommonName string, m model.Model, defaultSub, diskSub events.BufferedSubscription, discoverer discover.CachingMux, connectionsService connections.Service, urService *ur.Service, fss model.FolderSummaryService, errors, systemLog logger.Recorder, cpu Rater, contr Controller, noUpgrade bool) Service {
	return &service{
		id:      id,
		cfg:     cfg,
		statics: newStaticsServer(cfg.GUI().Theme, assetDir),
		model:   m,
		eventSubs: map[events.EventType]events.BufferedSubscription{
			DefaultEventMask: defaultSub,
			DiskEventMask:    diskSub,
		},
		eventSubsMut:         sync.NewMutex(),
		discoverer:           discoverer,
		connectionsService:   connectionsService,
		fss:                  fss,
		urService:            urService,
		systemConfigMut:      sync.NewMutex(),
		guiErrors:            errors,
		systemLog:            systemLog,
		cpu:                  cpu,
		contr:                contr,
		noUpgrade:            noUpgrade,
		tlsDefaultCommonName: tlsDefaultCommonName,
		stop:                 make(chan struct{}),
		configChanged:        make(chan struct{}),
		startedOnce:          make(chan struct{}),
	}
}

func (s *service) WaitForStart() error {
	<-s.startedOnce
	return s.startupErr
}

func (s *service) getListener(guiCfg config.GUIConfiguration) (net.Listener, error) {
	httpsCertFile := locations.Get(locations.HTTPSCertFile)
	httpsKeyFile := locations.Get(locations.HTTPSKeyFile)
	cert, err := tls.LoadX509KeyPair(httpsCertFile, httpsKeyFile)
	if err != nil {
		l.Infoln("Loading HTTPS certificate:", err)
		l.Infoln("Creating new HTTPS certificate")

		// When generating the HTTPS certificate, use the system host name per
		// default. If that isn't available, use the "syncthing" default.
		var name string
		name, err = os.Hostname()
		if err != nil {
			name = s.tlsDefaultCommonName
		}

		cert, err = tlsutil.NewCertificate(httpsCertFile, httpsKeyFile, name)
	}
	if err != nil {
		return nil, err
	}
	tlsCfg := tlsutil.SecureDefault()
	tlsCfg.Certificates = []tls.Certificate{cert}

	if guiCfg.Network() == "unix" {
		// When listening on a UNIX socket we should unlink before bind,
		// lest we get a "bind: address already in use". We don't
		// particularly care if this succeeds or not.
		os.Remove(guiCfg.Address())
	}
	rawListener, err := net.Listen(guiCfg.Network(), guiCfg.Address())
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
	bs, err := json.MarshalIndent(jsonObject, "", "  ")
	if err != nil {
		// This Marshal() can't fail though.
		bs, _ = json.Marshal(map[string]string{"error": err.Error()})
		http.Error(w, string(bs), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s\n", bs)
}

func (s *service) Serve() {
	listener, err := s.getListener(s.cfg.GUI())
	if err != nil {
		select {
		case <-s.startedOnce:
			// We let this be a loud user-visible warning as it may be the only
			// indication they get that the GUI won't be available.
			l.Warnln("Starting API/GUI:", err)

		default:
			// This is during initialization. A failure here should be fatal
			// as there will be no way for the user to communicate with us
			// otherwise anyway.
			s.startupErr = err
			close(s.startedOnce)
		}
		return
	}

	if listener == nil {
		// Not much we can do here other than exit quickly. The supervisor
		// will log an error at some point.
		return
	}

	defer listener.Close()

	s.cfg.Subscribe(s)
	defer s.cfg.Unsubscribe(s)

	// The GET handlers
	getRestMux := http.NewServeMux()
	getRestMux.HandleFunc("/rest/db/completion", s.getDBCompletion)              // device folder
	getRestMux.HandleFunc("/rest/db/file", s.getDBFile)                          // folder file
	getRestMux.HandleFunc("/rest/db/ignores", s.getDBIgnores)                    // folder
	getRestMux.HandleFunc("/rest/db/need", s.getDBNeed)                          // folder [perpage] [page]
	getRestMux.HandleFunc("/rest/db/remoteneed", s.getDBRemoteNeed)              // device folder [perpage] [page]
	getRestMux.HandleFunc("/rest/db/localchanged", s.getDBLocalChanged)          // folder
	getRestMux.HandleFunc("/rest/db/status", s.getDBStatus)                      // folder
	getRestMux.HandleFunc("/rest/db/browse", s.getDBBrowse)                      // folder [prefix] [dirsonly] [levels]
	getRestMux.HandleFunc("/rest/folder/versions", s.getFolderVersions)          // folder
	getRestMux.HandleFunc("/rest/folder/errors", s.getFolderErrors)              // folder
	getRestMux.HandleFunc("/rest/folder/pullerrors", s.getFolderErrors)          // folder (deprecated)
	getRestMux.HandleFunc("/rest/events", s.getIndexEvents)                      // [since] [limit] [timeout] [events]
	getRestMux.HandleFunc("/rest/events/disk", s.getDiskEvents)                  // [since] [limit] [timeout]
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
	postRestMux.HandleFunc("/rest/db/prio", s.postDBPrio)                          // folder file [perpage] [page]
	postRestMux.HandleFunc("/rest/db/ignores", s.postDBIgnores)                    // folder
	postRestMux.HandleFunc("/rest/db/override", s.postDBOverride)                  // folder
	postRestMux.HandleFunc("/rest/db/revert", s.postDBRevert)                      // folder
	postRestMux.HandleFunc("/rest/db/scan", s.postDBScan)                          // folder [sub...] [delay]
	postRestMux.HandleFunc("/rest/folder/versions", s.postFolderVersionsRestore)   // folder <body>
	postRestMux.HandleFunc("/rest/system/config", s.postSystemConfig)              // <body>
	postRestMux.HandleFunc("/rest/system/error", s.postSystemError)                // <body>
	postRestMux.HandleFunc("/rest/system/error/clear", s.postSystemErrorClear)     // -
	postRestMux.HandleFunc("/rest/system/ping", s.restPing)                        // -
	postRestMux.HandleFunc("/rest/system/reset", s.postSystemReset)                // [folder]
	postRestMux.HandleFunc("/rest/system/restart", s.postSystemRestart)            // -
	postRestMux.HandleFunc("/rest/system/shutdown", s.postSystemShutdown)          // -
	postRestMux.HandleFunc("/rest/system/upgrade", s.postSystemUpgrade)            // -
	postRestMux.HandleFunc("/rest/system/pause", s.makeDevicePauseHandler(true))   // [device]
	postRestMux.HandleFunc("/rest/system/resume", s.makeDevicePauseHandler(false)) // [device]
	postRestMux.HandleFunc("/rest/system/debug", s.postSystemDebug)                // [enable] [disable]

	// Debug endpoints, not for general use
	debugMux := http.NewServeMux()
	debugMux.HandleFunc("/rest/debug/peerCompletion", s.getPeerCompletion)
	debugMux.HandleFunc("/rest/debug/httpmetrics", s.getSystemHTTPMetrics)
	debugMux.HandleFunc("/rest/debug/cpuprof", s.getCPUProf) // duration
	debugMux.HandleFunc("/rest/debug/heapprof", s.getHeapProf)
	debugMux.HandleFunc("/rest/debug/support", s.getSupportBundle)
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
	if guiCfg.IsAuthEnabled() {
		handler = basicAuthAndSessionMiddleware("sessionid-"+s.id.String()[:5], guiCfg, s.cfg.LDAP(), handler)
	}

	// Redirect to HTTPS if we are supposed to
	if guiCfg.UseTLS() {
		handler = redirectToHTTPSMiddleware(handler)
	}

	// Add the CORS handling
	handler = corsMiddleware(handler, guiCfg.InsecureAllowFrameLoading)

	if addressIsLocalhost(guiCfg.Address()) && !guiCfg.InsecureSkipHostCheck {
		// Verify source host
		handler = localhostMiddleware(handler)
	}

	handler = debugMiddleware(handler)

	srv := http.Server{
		Handler: handler,
		// ReadTimeout must be longer than SyncthingController $scope.refresh
		// interval to avoid HTTP keepalive/GUI refresh race.
		ReadTimeout: 15 * time.Second,
	}

	l.Infoln("GUI and API listening on", listener.Addr())
	l.Infoln("Access the GUI via the following URL:", guiCfg.URL())
	if s.started != nil {
		// only set when run by the tests
		s.started <- listener.Addr().String()
	}

	// Indicate successful initial startup, to ourselves and to interested
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

// Complete implements suture.IsCompletable, which signifies to the supervisor
// whether to stop restarting the service.
func (s *service) Complete() bool {
	select {
	case <-s.startedOnce:
		return s.startupErr != nil
	case <-s.stop:
		return true
	default:
	}
	return false
}

func (s *service) Stop() {
	close(s.stop)
}

func (s *service) String() string {
	return fmt.Sprintf("api.service@%p", s)
}

func (s *service) VerifyConfiguration(from, to config.Configuration) error {
	if to.GUI.Network() != "tcp" {
		return nil
	}
	_, err := net.ResolveTCPAddr("tcp", to.GUI.Address())
	return err
}

func (s *service) CommitConfiguration(from, to config.Configuration) bool {
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
			l.Debugf("http: %s %q: status %d, %d bytes in %.02f ms", r.Method, r.URL.String(), status, written, ms)
		}
	})
}

func corsMiddleware(next http.Handler, allowFrameLoading bool) http.Handler {
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

		// Other security related headers that should be present.
		// https://www.owasp.org/index.php/Security_Headers

		if !allowFrameLoading {
			// We don't want to be rendered in an <iframe>,
			// <frame> or <object>. (Unless we do it ourselves.
			// This is also an escape hatch for people who serve
			// Syncthing GUI as part of their own website
			// through a proxy, so they don't need to set the
			// allowFrameLoading bool.)
			w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		}

		// If the browser senses an XSS attack it's allowed to take
		// action. (How this would not always be the default I
		// don't fully understand.)
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// Our content type headers are correct. Don't guess.
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// For everything else, pass to the next handler
		next.ServeHTTP(w, r)
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
		w.Header().Set("X-Syncthing-Version", build.Version)
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

func (s *service) whenDebugging(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.GUI().Debugging {
			h.ServeHTTP(w, r)
			return
		}

		http.Error(w, "Debugging disabled", http.StatusForbidden)
	})
}

func (s *service) restPing(w http.ResponseWriter, r *http.Request) {
	sendJSON(w, map[string]string{"ping": "pong"})
}

func (s *service) getJSMetadata(w http.ResponseWriter, r *http.Request) {
	meta, _ := json.Marshal(map[string]string{
		"deviceID": s.id.String(),
	})
	w.Header().Set("Content-Type", "application/javascript")
	fmt.Fprintf(w, "var metadata = %s;\n", meta)
}

func (s *service) getSystemVersion(w http.ResponseWriter, r *http.Request) {
	sendJSON(w, map[string]interface{}{
		"version":     build.Version,
		"codename":    build.Codename,
		"longVersion": build.LongVersion,
		"os":          runtime.GOOS,
		"arch":        runtime.GOARCH,
		"isBeta":      build.IsBeta,
		"isCandidate": build.IsCandidate,
		"isRelease":   build.IsRelease,
	})
}

func (s *service) getSystemDebug(w http.ResponseWriter, r *http.Request) {
	names := l.Facilities()
	enabled := l.FacilityDebugging()
	sort.Strings(enabled)
	sendJSON(w, map[string]interface{}{
		"facilities": names,
		"enabled":    enabled,
	})
}

func (s *service) postSystemDebug(w http.ResponseWriter, r *http.Request) {
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

func (s *service) getDBBrowse(w http.ResponseWriter, r *http.Request) {
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

func (s *service) getDBCompletion(w http.ResponseWriter, r *http.Request) {
	var qs = r.URL.Query()
	var folder = qs.Get("folder")
	var deviceStr = qs.Get("device")

	device, err := protocol.DeviceIDFromString(deviceStr)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	sendJSON(w, s.model.Completion(device, folder).Map())
}

func (s *service) getDBStatus(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	folder := qs.Get("folder")
	if sum, err := s.fss.Summary(folder); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
	} else {
		sendJSON(w, sum)
	}
}

func (s *service) postDBOverride(w http.ResponseWriter, r *http.Request) {
	var qs = r.URL.Query()
	var folder = qs.Get("folder")
	go s.model.Override(folder)
}

func (s *service) postDBRevert(w http.ResponseWriter, r *http.Request) {
	var qs = r.URL.Query()
	var folder = qs.Get("folder")
	go s.model.Revert(folder)
}

func getPagingParams(qs url.Values) (int, int) {
	page, err := strconv.Atoi(qs.Get("page"))
	if err != nil || page < 1 {
		page = 1
	}
	perpage, err := strconv.Atoi(qs.Get("perpage"))
	if err != nil || perpage < 1 {
		perpage = 1 << 16
	}
	return page, perpage
}

func (s *service) getDBNeed(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()

	folder := qs.Get("folder")

	page, perpage := getPagingParams(qs)

	progress, queued, rest := s.model.NeedFolderFiles(folder, page, perpage)

	// Convert the struct to a more loose structure, and inject the size.
	sendJSON(w, map[string]interface{}{
		"progress": toJsonFileInfoSlice(progress),
		"queued":   toJsonFileInfoSlice(queued),
		"rest":     toJsonFileInfoSlice(rest),
		"page":     page,
		"perpage":  perpage,
	})
}

func (s *service) getDBRemoteNeed(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()

	folder := qs.Get("folder")
	device := qs.Get("device")
	deviceID, err := protocol.DeviceIDFromString(device)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	page, perpage := getPagingParams(qs)

	if files, err := s.model.RemoteNeedFolderFiles(deviceID, folder, page, perpage); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
	} else {
		sendJSON(w, map[string]interface{}{
			"files":   toJsonFileInfoSlice(files),
			"page":    page,
			"perpage": perpage,
		})
	}
}

func (s *service) getDBLocalChanged(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()

	folder := qs.Get("folder")

	page, perpage := getPagingParams(qs)

	files := s.model.LocalChangedFiles(folder, page, perpage)

	sendJSON(w, map[string]interface{}{
		"files":   toJsonFileInfoSlice(files),
		"page":    page,
		"perpage": perpage,
	})
}

func (s *service) getSystemConnections(w http.ResponseWriter, r *http.Request) {
	sendJSON(w, s.model.ConnectionStats())
}

func (s *service) getDeviceStats(w http.ResponseWriter, r *http.Request) {
	sendJSON(w, s.model.DeviceStatistics())
}

func (s *service) getFolderStats(w http.ResponseWriter, r *http.Request) {
	sendJSON(w, s.model.FolderStatistics())
}

func (s *service) getDBFile(w http.ResponseWriter, r *http.Request) {
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

	av := s.model.Availability(folder, gf, protocol.BlockInfo{})
	sendJSON(w, map[string]interface{}{
		"global":       jsonFileInfo(gf),
		"local":        jsonFileInfo(lf),
		"availability": av,
	})
}

func (s *service) getSystemConfig(w http.ResponseWriter, r *http.Request) {
	sendJSON(w, s.cfg.RawCopy())
}

func (s *service) postSystemConfig(w http.ResponseWriter, r *http.Request) {
	s.systemConfigMut.Lock()
	defer s.systemConfigMut.Unlock()

	to, err := config.ReadJSON(r.Body, s.id)
	r.Body.Close()
	if err != nil {
		l.Warnln("Decoding posted config:", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if to.GUI.Password != s.cfg.GUI().Password {
		if to.GUI.Password != "" && !bcryptExpr.MatchString(to.GUI.Password) {
			hash, err := bcrypt.GenerateFromPassword([]byte(to.GUI.Password), 0)
			if err != nil {
				l.Warnln("bcrypting password:", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			to.GUI.Password = string(hash)
		}
	}

	// Activate and save. Wait for the configuration to become active before
	// completing the request.

	if wg, err := s.cfg.Replace(to); err != nil {
		l.Warnln("Replacing config:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	} else {
		wg.Wait()
	}

	if err := s.cfg.Save(); err != nil {
		l.Warnln("Saving config:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *service) getSystemConfigInsync(w http.ResponseWriter, r *http.Request) {
	sendJSON(w, map[string]bool{"configInSync": !s.cfg.RequiresRestart()})
}

func (s *service) postSystemRestart(w http.ResponseWriter, r *http.Request) {
	s.flushResponse(`{"ok": "restarting"}`, w)
	go s.contr.Restart()
}

func (s *service) postSystemReset(w http.ResponseWriter, r *http.Request) {
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

	go s.contr.Restart()
}

func (s *service) postSystemShutdown(w http.ResponseWriter, r *http.Request) {
	s.flushResponse(`{"ok": "shutting down"}`, w)
	go s.contr.Shutdown()
}

func (s *service) flushResponse(resp string, w http.ResponseWriter) {
	w.Write([]byte(resp + "\n"))
	f := w.(http.Flusher)
	f.Flush()
}

func (s *service) getSystemStatus(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	tilde, _ := fs.ExpandTilde("~")
	res := make(map[string]interface{})
	res["myID"] = s.id.String()
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

	res["connectionServiceStatus"] = s.connectionsService.ListenerStatus()
	res["lastDialStatus"] = s.connectionsService.ConnectionStatus()
	// cpuUsage.Rate() is in milliseconds per second, so dividing by ten
	// gives us percent
	res["cpuPercent"] = s.cpu.Rate() / 10 / float64(runtime.NumCPU())
	res["pathSeparator"] = string(filepath.Separator)
	res["urVersionMax"] = ur.Version
	res["uptime"] = s.urService.UptimeS()
	res["startTime"] = ur.StartTime
	res["guiAddressOverridden"] = s.cfg.GUI().IsOverridden()

	sendJSON(w, res)
}

func (s *service) getSystemError(w http.ResponseWriter, r *http.Request) {
	sendJSON(w, map[string][]logger.Line{
		"errors": s.guiErrors.Since(time.Time{}),
	})
}

func (s *service) postSystemError(w http.ResponseWriter, r *http.Request) {
	bs, _ := ioutil.ReadAll(r.Body)
	r.Body.Close()
	l.Warnln(string(bs))
}

func (s *service) postSystemErrorClear(w http.ResponseWriter, r *http.Request) {
	s.guiErrors.Clear()
}

func (s *service) getSystemLog(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	since, err := time.Parse(time.RFC3339, q.Get("since"))
	if err != nil {
		l.Debugln(err)
	}
	sendJSON(w, map[string][]logger.Line{
		"messages": s.systemLog.Since(since),
	})
}

func (s *service) getSystemLogTxt(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	since, err := time.Parse(time.RFC3339, q.Get("since"))
	if err != nil {
		l.Debugln(err)
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	for _, line := range s.systemLog.Since(since) {
		fmt.Fprintf(w, "%s: %s\n", line.When.Format(time.RFC3339), line.Message)
	}
}

type fileEntry struct {
	name string
	data []byte
}

func (s *service) getSupportBundle(w http.ResponseWriter, r *http.Request) {
	var files []fileEntry

	// Redacted configuration as a JSON
	if jsonConfig, err := json.MarshalIndent(getRedactedConfig(s), "", "  "); err == nil {
		files = append(files, fileEntry{name: "config.json.txt", data: jsonConfig})
	} else {
		l.Warnln("Support bundle: failed to create config.json:", err)
	}

	// Log as a text
	var buflog bytes.Buffer
	for _, line := range s.systemLog.Since(time.Time{}) {
		fmt.Fprintf(&buflog, "%s: %s\n", line.When.Format(time.RFC3339), line.Message)
	}
	files = append(files, fileEntry{name: "log-inmemory.txt", data: buflog.Bytes()})

	// Errors as a JSON
	if errs := s.guiErrors.Since(time.Time{}); len(errs) > 0 {
		if jsonError, err := json.MarshalIndent(errs, "", "  "); err != nil {
			files = append(files, fileEntry{name: "errors.json.txt", data: jsonError})
		} else {
			l.Warnln("Support bundle: failed to create errors.json:", err)
		}
	}

	// Panic files
	if panicFiles, err := filepath.Glob(filepath.Join(locations.GetBaseDir(locations.ConfigBaseDir), "panic*")); err == nil {
		for _, f := range panicFiles {
			if panicFile, err := ioutil.ReadFile(f); err != nil {
				l.Warnf("Support bundle: failed to load %s: %s", filepath.Base(f), err)
			} else {
				files = append(files, fileEntry{name: filepath.Base(f), data: panicFile})
			}
		}
	}

	// Archived log (default on Windows)
	if logFile, err := ioutil.ReadFile(locations.Get(locations.LogFile)); err == nil {
		files = append(files, fileEntry{name: "log-ondisk.txt", data: logFile})
	}

	// Version and platform information as a JSON
	if versionPlatform, err := json.MarshalIndent(map[string]string{
		"now":         time.Now().Format(time.RFC3339),
		"version":     build.Version,
		"codename":    build.Codename,
		"longVersion": build.LongVersion,
		"os":          runtime.GOOS,
		"arch":        runtime.GOARCH,
	}, "", "  "); err == nil {
		files = append(files, fileEntry{name: "version-platform.json.txt", data: versionPlatform})
	} else {
		l.Warnln("Failed to create versionPlatform.json: ", err)
	}

	// Report Data as a JSON
	if usageReportingData, err := json.MarshalIndent(s.urService.ReportData(), "", "  "); err != nil {
		l.Warnln("Support bundle: failed to create versionPlatform.json:", err)
	} else {
		files = append(files, fileEntry{name: "usage-reporting.json.txt", data: usageReportingData})
	}

	// Heap and CPU Proofs as a pprof extension
	var heapBuffer, cpuBuffer bytes.Buffer
	filename := fmt.Sprintf("syncthing-heap-%s-%s-%s-%s.pprof", runtime.GOOS, runtime.GOARCH, build.Version, time.Now().Format("150405")) // hhmmss
	runtime.GC()
	if err := pprof.WriteHeapProfile(&heapBuffer); err == nil {
		files = append(files, fileEntry{name: filename, data: heapBuffer.Bytes()})
	}

	const duration = 4 * time.Second
	filename = fmt.Sprintf("syncthing-cpu-%s-%s-%s-%s.pprof", runtime.GOOS, runtime.GOARCH, build.Version, time.Now().Format("150405")) // hhmmss
	if err := pprof.StartCPUProfile(&cpuBuffer); err == nil {
		time.Sleep(duration)
		pprof.StopCPUProfile()
		files = append(files, fileEntry{name: filename, data: cpuBuffer.Bytes()})
	}

	// Add buffer files to buffer zip
	var zipFilesBuffer bytes.Buffer
	if err := writeZip(&zipFilesBuffer, files); err != nil {
		l.Warnln("Support bundle: failed to create support bundle zip:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Set zip file name and path
	zipFileName := fmt.Sprintf("support-bundle-%s-%s.zip", s.id.Short().String(), time.Now().Format("2006-01-02T150405"))
	zipFilePath := filepath.Join(locations.GetBaseDir(locations.ConfigBaseDir), zipFileName)

	// Write buffer zip to local zip file (back up)
	if err := ioutil.WriteFile(zipFilePath, zipFilesBuffer.Bytes(), 0600); err != nil {
		l.Warnln("Support bundle: support bundle zip could not be created:", err)
	}

	// Serve the buffer zip to client for download
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename="+zipFileName)
	io.Copy(w, &zipFilesBuffer)
}

func (s *service) getSystemHTTPMetrics(w http.ResponseWriter, r *http.Request) {
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

func (s *service) getSystemDiscovery(w http.ResponseWriter, r *http.Request) {
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

func (s *service) getReport(w http.ResponseWriter, r *http.Request) {
	version := ur.Version
	if val, _ := strconv.Atoi(r.URL.Query().Get("version")); val > 0 {
		version = val
	}
	sendJSON(w, s.urService.ReportDataPreview(version))
}

func (s *service) getRandomString(w http.ResponseWriter, r *http.Request) {
	length := 32
	if val, _ := strconv.Atoi(r.URL.Query().Get("length")); val > 0 {
		length = val
	}
	str := rand.String(length)

	sendJSON(w, map[string]string{"random": str})
}

func (s *service) getDBIgnores(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()

	folder := qs.Get("folder")

	ignores, patterns, err := s.model.GetIgnores(folder)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	sendJSON(w, map[string][]string{
		"ignore":   ignores,
		"expanded": patterns,
	})
}

func (s *service) postDBIgnores(w http.ResponseWriter, r *http.Request) {
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

func (s *service) getIndexEvents(w http.ResponseWriter, r *http.Request) {
	s.fss.OnEventRequest()
	mask := s.getEventMask(r.URL.Query().Get("events"))
	sub := s.getEventSub(mask)
	s.getEvents(w, r, sub)
}

func (s *service) getDiskEvents(w http.ResponseWriter, r *http.Request) {
	sub := s.getEventSub(DiskEventMask)
	s.getEvents(w, r, sub)
}

func (s *service) getEvents(w http.ResponseWriter, r *http.Request, eventSub events.BufferedSubscription) {
	qs := r.URL.Query()
	sinceStr := qs.Get("since")
	limitStr := qs.Get("limit")
	timeoutStr := qs.Get("timeout")
	since, _ := strconv.Atoi(sinceStr)
	limit, _ := strconv.Atoi(limitStr)

	timeout := defaultEventTimeout
	if timeoutSec, timeoutErr := strconv.Atoi(timeoutStr); timeoutErr == nil && timeoutSec >= 0 { // 0 is a valid timeout
		timeout = time.Duration(timeoutSec) * time.Second
	}

	// Flush before blocking, to indicate that we've received the request and
	// that it should not be retried. Must set Content-Type header before
	// flushing.
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	f := w.(http.Flusher)
	f.Flush()

	// If there are no events available return an empty slice, as this gets serialized as `[]`
	evs := eventSub.Since(since, []events.Event{}, timeout)
	if 0 < limit && limit < len(evs) {
		evs = evs[len(evs)-limit:]
	}

	sendJSON(w, evs)
}

func (s *service) getEventMask(evs string) events.EventType {
	eventMask := DefaultEventMask
	if evs != "" {
		eventList := strings.Split(evs, ",")
		eventMask = 0
		for _, ev := range eventList {
			eventMask |= events.UnmarshalEventType(strings.TrimSpace(ev))
		}
	}
	return eventMask
}

func (s *service) getEventSub(mask events.EventType) events.BufferedSubscription {
	s.eventSubsMut.Lock()
	bufsub, ok := s.eventSubs[mask]
	if !ok {
		evsub := events.Default.Subscribe(mask)
		bufsub = events.NewBufferedSubscription(evsub, EventSubBufferSize)
		s.eventSubs[mask] = bufsub
	}
	s.eventSubsMut.Unlock()

	return bufsub
}

func (s *service) getSystemUpgrade(w http.ResponseWriter, r *http.Request) {
	if s.noUpgrade {
		http.Error(w, upgrade.ErrUpgradeUnsupported.Error(), 500)
		return
	}
	opts := s.cfg.Options()
	rel, err := upgrade.LatestRelease(opts.ReleasesURL, build.Version, opts.UpgradeToPreReleases)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	res := make(map[string]interface{})
	res["running"] = build.Version
	res["latest"] = rel.Tag
	res["newer"] = upgrade.CompareVersions(rel.Tag, build.Version) == upgrade.Newer
	res["majorNewer"] = upgrade.CompareVersions(rel.Tag, build.Version) == upgrade.MajorNewer

	sendJSON(w, res)
}

func (s *service) getDeviceID(w http.ResponseWriter, r *http.Request) {
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

func (s *service) getLang(w http.ResponseWriter, r *http.Request) {
	lang := r.Header.Get("Accept-Language")
	var langs []string
	for _, l := range strings.Split(lang, ",") {
		parts := strings.SplitN(l, ";", 2)
		langs = append(langs, strings.ToLower(strings.TrimSpace(parts[0])))
	}
	sendJSON(w, langs)
}

func (s *service) postSystemUpgrade(w http.ResponseWriter, r *http.Request) {
	opts := s.cfg.Options()
	rel, err := upgrade.LatestRelease(opts.ReleasesURL, build.Version, opts.UpgradeToPreReleases)
	if err != nil {
		l.Warnln("getting latest release:", err)
		http.Error(w, err.Error(), 500)
		return
	}

	if upgrade.CompareVersions(rel.Tag, build.Version) > upgrade.Equal {
		err = upgrade.To(rel)
		if err != nil {
			l.Warnln("upgrading:", err)
			http.Error(w, err.Error(), 500)
			return
		}

		s.flushResponse(`{"ok": "restarting"}`, w)
		s.contr.ExitUpgrading()
	}
}

func (s *service) makeDevicePauseHandler(paused bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var qs = r.URL.Query()
		var deviceStr = qs.Get("device")

		var cfgs []config.DeviceConfiguration

		if deviceStr == "" {
			for _, cfg := range s.cfg.Devices() {
				cfg.Paused = paused
				cfgs = append(cfgs, cfg)
			}
		} else {
			device, err := protocol.DeviceIDFromString(deviceStr)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}

			cfg, ok := s.cfg.Devices()[device]
			if !ok {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}

			cfg.Paused = paused
			cfgs = append(cfgs, cfg)
		}

		if _, err := s.cfg.SetDevices(cfgs); err != nil {
			http.Error(w, err.Error(), 500)
		}
	}
}

func (s *service) postDBScan(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	folder := qs.Get("folder")
	if folder != "" {
		subs := qs["sub"]
		err := s.model.ScanFolderSubdirs(folder, subs)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		nextStr := qs.Get("next")
		next, err := strconv.Atoi(nextStr)
		if err == nil {
			s.model.DelayScan(folder, time.Duration(next)*time.Second)
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

func (s *service) postDBPrio(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	folder := qs.Get("folder")
	file := qs.Get("file")
	s.model.BringToFront(folder, file)
	s.getDBNeed(w, r)
}

func (s *service) getQR(w http.ResponseWriter, r *http.Request) {
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

func (s *service) getPeerCompletion(w http.ResponseWriter, r *http.Request) {
	tot := map[string]float64{}
	count := map[string]float64{}

	for _, folder := range s.cfg.Folders() {
		for _, device := range folder.DeviceIDs() {
			deviceStr := device.String()
			if _, ok := s.model.Connection(device); ok {
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

func (s *service) getFolderVersions(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	versions, err := s.model.GetFolderVersions(qs.Get("folder"))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	sendJSON(w, versions)
}

func (s *service) postFolderVersionsRestore(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()

	bs, err := ioutil.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	var versions map[string]time.Time
	err = json.Unmarshal(bs, &versions)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	ferr, err := s.model.RestoreFolderVersions(qs.Get("folder"), versions)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	sendJSON(w, ferr)
}

func (s *service) getFolderErrors(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	folder := qs.Get("folder")
	page, perpage := getPagingParams(qs)

	errors, err := s.model.FolderErrors(folder)

	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	start := (page - 1) * perpage
	if start >= len(errors) {
		errors = nil
	} else {
		errors = errors[start:]
		if perpage < len(errors) {
			errors = errors[:perpage]
		}
	}

	sendJSON(w, map[string]interface{}{
		"folder":  folder,
		"errors":  errors,
		"page":    page,
		"perpage": perpage,
	})
}

func (s *service) getSystemBrowse(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	current := qs.Get("current")

	// Default value or in case of error unmarshalling ends up being basic fs.
	var fsType fs.FilesystemType
	fsType.UnmarshalText([]byte(qs.Get("filesystem")))

	sendJSON(w, browseFiles(current, fsType))
}

const (
	matchExact int = iota
	matchCaseIns
	noMatch
)

func checkPrefixMatch(s, prefix string) int {
	if strings.HasPrefix(s, prefix) {
		return matchExact
	}

	if strings.HasPrefix(strings.ToLower(s), strings.ToLower(prefix)) {
		return matchCaseIns
	}

	return noMatch
}

func browseFiles(current string, fsType fs.FilesystemType) []string {
	if current == "" {
		filesystem := fs.NewFilesystem(fsType, "")
		if roots, err := filesystem.Roots(); err == nil {
			return roots
		}
		return nil
	}
	search, _ := fs.ExpandTilde(current)
	pathSeparator := string(fs.PathSeparator)

	if strings.HasSuffix(current, pathSeparator) && !strings.HasSuffix(search, pathSeparator) {
		search = search + pathSeparator
	}
	searchDir := filepath.Dir(search)

	// The searchFile should be the last component of search, or empty if it
	// ends with a path separator
	var searchFile string
	if !strings.HasSuffix(search, pathSeparator) {
		searchFile = filepath.Base(search)
	}

	fs := fs.NewFilesystem(fsType, searchDir)

	subdirectories, _ := fs.DirNames(".")

	exactMatches := make([]string, 0, len(subdirectories))
	caseInsMatches := make([]string, 0, len(subdirectories))

	for _, subdirectory := range subdirectories {
		info, err := fs.Stat(subdirectory)
		if err != nil || !info.IsDir() {
			continue
		}

		switch checkPrefixMatch(subdirectory, searchFile) {
		case matchExact:
			exactMatches = append(exactMatches, filepath.Join(searchDir, subdirectory)+pathSeparator)
		case matchCaseIns:
			caseInsMatches = append(caseInsMatches, filepath.Join(searchDir, subdirectory)+pathSeparator)
		}
	}

	// sort to return matches in deterministic order (don't depend on file system order)
	sort.Strings(exactMatches)
	sort.Strings(caseInsMatches)
	return append(exactMatches, caseInsMatches...)
}

func (s *service) getCPUProf(w http.ResponseWriter, r *http.Request) {
	duration, err := time.ParseDuration(r.FormValue("duration"))
	if err != nil {
		duration = 30 * time.Second
	}

	filename := fmt.Sprintf("syncthing-cpu-%s-%s-%s-%s.pprof", runtime.GOOS, runtime.GOARCH, build.Version, time.Now().Format("150405")) // hhmmss

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)

	if err := pprof.StartCPUProfile(w); err == nil {
		time.Sleep(duration)
		pprof.StopCPUProfile()
	}
}

func (s *service) getHeapProf(w http.ResponseWriter, r *http.Request) {
	filename := fmt.Sprintf("syncthing-heap-%s-%s-%s-%s.pprof", runtime.GOOS, runtime.GOARCH, build.Version, time.Now().Format("150405")) // hhmmss

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)

	runtime.GC()
	pprof.WriteHeapProfile(w)
}

func toJsonFileInfoSlice(fs []db.FileInfoTruncated) []jsonDBFileInfo {
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
		"invalid":       protocol.FileInfo(f).IsInvalid(),
		"ignored":       protocol.FileInfo(f).IsIgnored(),
		"mustRescan":    protocol.FileInfo(f).MustRescan(),
		"noPermissions": f.NoPermissions,
		"modified":      protocol.FileInfo(f).ModTime(),
		"modifiedBy":    f.ModifiedBy.String(),
		"sequence":      f.Sequence,
		"numBlocks":     len(f.Blocks),
		"version":       jsonVersionVector(f.Version),
		"localFlags":    f.LocalFlags,
	})
}

type jsonDBFileInfo db.FileInfoTruncated

func (f jsonDBFileInfo) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"name":          f.Name,
		"type":          f.Type.String(),
		"size":          f.Size,
		"permissions":   fmt.Sprintf("%#o", f.Permissions),
		"deleted":       f.Deleted,
		"invalid":       db.FileInfoTruncated(f).IsInvalid(),
		"ignored":       db.FileInfoTruncated(f).IsIgnored(),
		"mustRescan":    db.FileInfoTruncated(f).MustRescan(),
		"noPermissions": f.NoPermissions,
		"modified":      db.FileInfoTruncated(f).ModTime(),
		"modifiedBy":    f.ModifiedBy.String(),
		"sequence":      f.Sequence,
		"numBlocks":     nil, // explicitly unknown
		"version":       jsonVersionVector(f.Version),
		"localFlags":    f.LocalFlags,
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
	case "localhost", "localhost.":
		return true
	default:
		ip := net.ParseIP(host)
		if ip == nil {
			// not an IP address
			return false
		}
		return ip.IsLoopback()
	}
}
