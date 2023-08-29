// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package api

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/calmh/incontainer"
	"github.com/julienschmidt/httprouter"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rcrowley/go-metrics"
	"github.com/thejerf/suture/v4"
	"github.com/vitrun/qart/qr"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/connections"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/discover"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/syncthing/syncthing/lib/locations"
	"github.com/syncthing/syncthing/lib/logger"
	"github.com/syncthing/syncthing/lib/model"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/svcutil"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/tlsutil"
	"github.com/syncthing/syncthing/lib/upgrade"
	"github.com/syncthing/syncthing/lib/ur"
)

const (
	// Default mask excludes these very noisy event types to avoid filling the pipe.
	// FIXME: ItemStarted and ItemFinished should be excluded for the same reason.
	DefaultEventMask      = events.AllEvents &^ events.LocalChangeDetected &^ events.RemoteChangeDetected
	DiskEventMask         = events.LocalChangeDetected | events.RemoteChangeDetected
	EventSubBufferSize    = 1000
	defaultEventTimeout   = time.Minute
	httpsCertLifetimeDays = 820
)

type service struct {
	suture.Service

	id                   protocol.DeviceID
	cfg                  config.Wrapper
	statics              *staticsServer
	model                model.Model
	eventSubs            map[events.EventType]events.BufferedSubscription
	eventSubsMut         sync.Mutex
	evLogger             events.Logger
	discoverer           discover.Manager
	connectionsService   connections.Service
	fss                  model.FolderSummaryService
	urService            *ur.Service
	noUpgrade            bool
	tlsDefaultCommonName string
	configChanged        chan struct{} // signals intentional listener close due to config change
	started              chan string   // signals startup complete by sending the listener address, for testing only
	startedOnce          chan struct{} // the service has started successfully at least once
	startupErr           error
	listenerAddr         net.Addr
	exitChan             chan *svcutil.FatalErr

	guiErrors logger.Recorder
	systemLog logger.Recorder
}

var _ config.Verifier = &service{}

type Service interface {
	suture.Service
	config.Committer
	WaitForStart() error
}

func New(id protocol.DeviceID, cfg config.Wrapper, assetDir, tlsDefaultCommonName string, m model.Model, defaultSub, diskSub events.BufferedSubscription, evLogger events.Logger, discoverer discover.Manager, connectionsService connections.Service, urService *ur.Service, fss model.FolderSummaryService, errors, systemLog logger.Recorder, noUpgrade bool) Service {
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
		evLogger:             evLogger,
		discoverer:           discoverer,
		connectionsService:   connectionsService,
		fss:                  fss,
		urService:            urService,
		guiErrors:            errors,
		systemLog:            systemLog,
		noUpgrade:            noUpgrade,
		tlsDefaultCommonName: tlsDefaultCommonName,
		configChanged:        make(chan struct{}),
		startedOnce:          make(chan struct{}),
		exitChan:             make(chan *svcutil.FatalErr, 1),
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

	// If the certificate has expired or will expire in the next month, fail
	// it and generate a new one.
	if err == nil {
		err = shouldRegenerateCertificate(cert)
	}
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
		name, err = sanitizedHostname(name)
		if err != nil {
			name = s.tlsDefaultCommonName
		}

		cert, err = tlsutil.NewCertificate(httpsCertFile, httpsKeyFile, name, httpsCertLifetimeDays)
	}
	if err != nil {
		return nil, err
	}
	tlsCfg := tlsutil.SecureDefaultWithTLS12()
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

	if guiCfg.Network() == "unix" && guiCfg.UnixSocketPermissions() != 0 {
		// We should error if this fails under the assumption that these permissions are
		// required for operation.
		err = os.Chmod(guiCfg.Address(), guiCfg.UnixSocketPermissions())
		if err != nil {
			return nil, err
		}
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

func (s *service) Serve(ctx context.Context) error {
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
		return err
	}

	if listener == nil {
		// Not much we can do here other than exit quickly. The supervisor
		// will log an error at some point.
		return nil
	}

	s.listenerAddr = listener.Addr()
	defer listener.Close()

	s.cfg.Subscribe(s)
	defer s.cfg.Unsubscribe(s)

	restMux := httprouter.New()

	// The GET handlers
	restMux.HandlerFunc(http.MethodGet, "/rest/cluster/pending/devices", s.getPendingDevices) // -
	restMux.HandlerFunc(http.MethodGet, "/rest/cluster/pending/folders", s.getPendingFolders) // [device]
	restMux.HandlerFunc(http.MethodGet, "/rest/db/completion", s.getDBCompletion)             // [device] [folder]
	restMux.HandlerFunc(http.MethodGet, "/rest/db/file", s.getDBFile)                         // folder file
	restMux.HandlerFunc(http.MethodGet, "/rest/db/ignores", s.getDBIgnores)                   // folder
	restMux.HandlerFunc(http.MethodGet, "/rest/db/need", s.getDBNeed)                         // folder [perpage] [page]
	restMux.HandlerFunc(http.MethodGet, "/rest/db/remoteneed", s.getDBRemoteNeed)             // device folder [perpage] [page]
	restMux.HandlerFunc(http.MethodGet, "/rest/db/localchanged", s.getDBLocalChanged)         // folder [perpage] [page]
	restMux.HandlerFunc(http.MethodGet, "/rest/db/status", s.getDBStatus)                     // folder
	restMux.HandlerFunc(http.MethodGet, "/rest/db/browse", s.getDBBrowse)                     // folder [prefix] [dirsonly] [levels]
	restMux.HandlerFunc(http.MethodGet, "/rest/folder/versions", s.getFolderVersions)         // folder
	restMux.HandlerFunc(http.MethodGet, "/rest/folder/errors", s.getFolderErrors)             // folder [perpage] [page]
	restMux.HandlerFunc(http.MethodGet, "/rest/folder/pullerrors", s.getFolderErrors)         // folder (deprecated)
	restMux.HandlerFunc(http.MethodGet, "/rest/events", s.getIndexEvents)                     // [since] [limit] [timeout] [events]
	restMux.HandlerFunc(http.MethodGet, "/rest/events/disk", s.getDiskEvents)                 // [since] [limit] [timeout]
	restMux.HandlerFunc(http.MethodGet, "/rest/noauth/health", s.getHealth)                   // -
	restMux.HandlerFunc(http.MethodGet, "/rest/stats/device", s.getDeviceStats)               // -
	restMux.HandlerFunc(http.MethodGet, "/rest/stats/folder", s.getFolderStats)               // -
	restMux.HandlerFunc(http.MethodGet, "/rest/svc/deviceid", s.getDeviceID)                  // id
	restMux.HandlerFunc(http.MethodGet, "/rest/svc/lang", s.getLang)                          // -
	restMux.HandlerFunc(http.MethodGet, "/rest/svc/report", s.getReport)                      // -
	restMux.HandlerFunc(http.MethodGet, "/rest/svc/random/string", s.getRandomString)         // [length]
	restMux.HandlerFunc(http.MethodGet, "/rest/system/browse", s.getSystemBrowse)             // current
	restMux.HandlerFunc(http.MethodGet, "/rest/system/connections", s.getSystemConnections)   // -
	restMux.HandlerFunc(http.MethodGet, "/rest/system/discovery", s.getSystemDiscovery)       // -
	restMux.HandlerFunc(http.MethodGet, "/rest/system/error", s.getSystemError)               // -
	restMux.HandlerFunc(http.MethodGet, "/rest/system/paths", s.getSystemPaths)               // -
	restMux.HandlerFunc(http.MethodGet, "/rest/system/ping", s.restPing)                      // -
	restMux.HandlerFunc(http.MethodGet, "/rest/system/status", s.getSystemStatus)             // -
	restMux.HandlerFunc(http.MethodGet, "/rest/system/upgrade", s.getSystemUpgrade)           // -
	restMux.HandlerFunc(http.MethodGet, "/rest/system/version", s.getSystemVersion)           // -
	restMux.HandlerFunc(http.MethodGet, "/rest/system/debug", s.getSystemDebug)               // -
	restMux.HandlerFunc(http.MethodGet, "/rest/system/log", s.getSystemLog)                   // [since]
	restMux.HandlerFunc(http.MethodGet, "/rest/system/log.txt", s.getSystemLogTxt)            // [since]

	// The POST handlers
	restMux.HandlerFunc(http.MethodPost, "/rest/db/prio", s.postDBPrio)                          // folder file
	restMux.HandlerFunc(http.MethodPost, "/rest/db/ignores", s.postDBIgnores)                    // folder
	restMux.HandlerFunc(http.MethodPost, "/rest/db/override", s.postDBOverride)                  // folder
	restMux.HandlerFunc(http.MethodPost, "/rest/db/revert", s.postDBRevert)                      // folder
	restMux.HandlerFunc(http.MethodPost, "/rest/db/scan", s.postDBScan)                          // folder [sub...] [delay]
	restMux.HandlerFunc(http.MethodPost, "/rest/folder/versions", s.postFolderVersionsRestore)   // folder <body>
	restMux.HandlerFunc(http.MethodPost, "/rest/system/error", s.postSystemError)                // <body>
	restMux.HandlerFunc(http.MethodPost, "/rest/system/error/clear", s.postSystemErrorClear)     // -
	restMux.HandlerFunc(http.MethodPost, "/rest/system/ping", s.restPing)                        // -
	restMux.HandlerFunc(http.MethodPost, "/rest/system/reset", s.postSystemReset)                // [folder]
	restMux.HandlerFunc(http.MethodPost, "/rest/system/restart", s.postSystemRestart)            // -
	restMux.HandlerFunc(http.MethodPost, "/rest/system/shutdown", s.postSystemShutdown)          // -
	restMux.HandlerFunc(http.MethodPost, "/rest/system/upgrade", s.postSystemUpgrade)            // -
	restMux.HandlerFunc(http.MethodPost, "/rest/system/pause", s.makeDevicePauseHandler(true))   // [device]
	restMux.HandlerFunc(http.MethodPost, "/rest/system/resume", s.makeDevicePauseHandler(false)) // [device]
	restMux.HandlerFunc(http.MethodPost, "/rest/system/debug", s.postSystemDebug)                // [enable] [disable]

	// The DELETE handlers
	restMux.HandlerFunc(http.MethodDelete, "/rest/cluster/pending/devices", s.deletePendingDevices) // device
	restMux.HandlerFunc(http.MethodDelete, "/rest/cluster/pending/folders", s.deletePendingFolders) // folder [device]

	// Config endpoints

	configBuilder := &configMuxBuilder{
		Router: restMux,
		id:     s.id,
		cfg:    s.cfg,
	}

	configBuilder.registerConfig("/rest/config")
	configBuilder.registerConfigInsync("/rest/config/insync") // deprecated
	configBuilder.registerConfigRequiresRestart("/rest/config/restart-required")
	configBuilder.registerFolders("/rest/config/folders")
	configBuilder.registerDevices("/rest/config/devices")
	configBuilder.registerFolder("/rest/config/folders/:id")
	configBuilder.registerDevice("/rest/config/devices/:id")
	configBuilder.registerDefaultFolder("/rest/config/defaults/folder")
	configBuilder.registerDefaultDevice("/rest/config/defaults/device")
	configBuilder.registerDefaultIgnores("/rest/config/defaults/ignores")
	configBuilder.registerOptions("/rest/config/options")
	configBuilder.registerLDAP("/rest/config/ldap")
	configBuilder.registerGUI("/rest/config/gui")

	// Deprecated config endpoints
	configBuilder.registerConfigDeprecated("/rest/system/config") // POST instead of PUT
	configBuilder.registerConfigInsync("/rest/system/config/insync")

	// Debug endpoints, not for general use
	debugMux := http.NewServeMux()
	debugMux.HandleFunc("/rest/debug/peerCompletion", s.getPeerCompletion)
	debugMux.HandleFunc("/rest/debug/httpmetrics", s.getSystemHTTPMetrics)
	debugMux.HandleFunc("/rest/debug/cpuprof", s.getCPUProf) // duration
	debugMux.HandleFunc("/rest/debug/heapprof", s.getHeapProf)
	debugMux.HandleFunc("/rest/debug/support", s.getSupportBundle)
	debugMux.HandleFunc("/rest/debug/file", s.getDebugFile)
	restMux.Handler(http.MethodGet, "/rest/debug/*method", s.whenDebugging(debugMux))

	// A handler that disables caching
	noCacheRestMux := noCacheMiddleware(metricsMiddleware(restMux))

	// The main routing handler
	mux := http.NewServeMux()
	mux.Handle("/rest/", noCacheRestMux)
	mux.HandleFunc("/qr/", s.getQR)

	// Serve compiled in assets unless an asset directory was set (for development)
	mux.Handle("/", s.statics)

	// Handle the special meta.js path
	mux.HandleFunc("/meta.js", s.getJSMetadata)

	// Handle Prometheus metrics
	promHttpHandler := promhttp.Handler()
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, req *http.Request) {
		// fetching metrics counts as an event, for the purpose of whether
		// we should prepare folder summaries etc.
		s.fss.OnEventRequest()
		promHttpHandler.ServeHTTP(w, req)
	})

	guiCfg := s.cfg.GUI()

	// Wrap everything in CSRF protection. The /rest prefix should be
	// protected, other requests will grant cookies.
	var handler http.Handler = newCsrfManager(s.id.String()[:5], "/rest", guiCfg, mux, locations.Get(locations.CsrfTokens))

	// Add our version and ID as a header to responses
	handler = withDetailsMiddleware(s.id, handler)

	// Wrap everything in basic auth, if user/password is set.
	if guiCfg.IsAuthEnabled() {
		handler = basicAuthAndSessionMiddleware("sessionid-"+s.id.String()[:5], guiCfg, s.cfg.LDAP(), handler, s.evLogger)
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
		// Prevent the HTTP server from logging stuff on its own. The things we
		// care about we log ourselves from the handlers.
		ErrorLog: log.New(io.Discard, "", 0),
	}

	l.Infoln("GUI and API listening on", listener.Addr())
	l.Infoln("Access the GUI via the following URL:", guiCfg.URL())
	if s.started != nil {
		// only set when run by the tests
		select {
		case <-ctx.Done(): // Shouldn't return directly due to cleanup below
		case s.started <- listener.Addr().String():
		}
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
		select {
		case serveError <- srv.Serve(listener):
		case <-ctx.Done():
		}
	}()

	// Wait for stop, restart or error signals

	err = nil
	select {
	case <-ctx.Done():
		// Shutting down permanently
		l.Debugln("shutting down (stop)")
	case <-s.configChanged:
		// Soft restart due to configuration change
		l.Debugln("restarting (config changed)")
	case err = <-s.exitChan:
	case err = <-serveError:
		// Restart due to listen/serve failure
		l.Warnln("GUI/API:", err, "(restarting)")
	}
	// Give it a moment to shut down gracefully, e.g. if we are restarting
	// due to a config change through the API, let that finish successfully.
	timeout, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := srv.Shutdown(timeout); err == timeout.Err() {
		srv.Close()
	}

	return err
}

// Complete implements suture.IsCompletable, which signifies to the supervisor
// whether to stop restarting the service.
func (s *service) Complete() bool {
	select {
	case <-s.startedOnce:
		return s.startupErr != nil
	default:
	}
	return false
}

func (s *service) String() string {
	return fmt.Sprintf("api.service@%p", s)
}

func (*service) VerifyConfiguration(_, to config.Configuration) error {
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
		// No GUI changes, we're done here.
		return true
	}

	if to.GUI.Theme != from.GUI.Theme {
		s.statics.setTheme(to.GUI.Theme)
	}

	// Tell the serve loop to restart
	s.configChanged <- struct{}{}

	return true
}

func (s *service) fatal(err *svcutil.FatalErr) {
	// s.exitChan is 1-buffered and whoever is first gets handled.
	select {
	case s.exitChan <- err:
	default:
	}
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
			// Only GET/POST/OPTIONS Methods are supported
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
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

func (s *service) getPendingDevices(w http.ResponseWriter, _ *http.Request) {
	devices, err := s.model.PendingDevices()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sendJSON(w, devices)
}

func (s *service) deletePendingDevices(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()

	device := qs.Get("device")
	deviceID, err := protocol.DeviceIDFromString(device)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.model.DismissPendingDevice(deviceID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *service) getPendingFolders(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()

	device := qs.Get("device")
	deviceID, err := protocol.DeviceIDFromString(device)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	folders, err := s.model.PendingFolders(deviceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sendJSON(w, folders)
}

func (s *service) deletePendingFolders(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()

	device := qs.Get("device")
	deviceID, err := protocol.DeviceIDFromString(device)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	folderID := qs.Get("folder")

	if err := s.model.DismissPendingFolder(deviceID, folderID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (*service) restPing(w http.ResponseWriter, _ *http.Request) {
	sendJSON(w, map[string]string{"ping": "pong"})
}

func (*service) getSystemPaths(w http.ResponseWriter, _ *http.Request) {
	sendJSON(w, locations.ListExpandedPaths())
}

func (s *service) getJSMetadata(w http.ResponseWriter, _ *http.Request) {
	meta, _ := json.Marshal(map[string]string{
		"deviceID": s.id.String(),
	})
	w.Header().Set("Content-Type", "application/javascript")
	fmt.Fprintf(w, "var metadata = %s;\n", meta)
}

func (*service) getSystemVersion(w http.ResponseWriter, _ *http.Request) {
	sendJSON(w, map[string]interface{}{
		"version":     build.Version,
		"codename":    build.Codename,
		"longVersion": build.LongVersion,
		"extra":       build.Extra,
		"os":          runtime.GOOS,
		"arch":        runtime.GOARCH,
		"isBeta":      build.IsBeta,
		"isCandidate": build.IsCandidate,
		"isRelease":   build.IsRelease,
		"date":        build.Date,
		"tags":        build.TagsList(),
		"stamp":       build.Stamp,
		"user":        build.User,
		"container":   incontainer.Detect(),
	})
}

func (*service) getSystemDebug(w http.ResponseWriter, _ *http.Request) {
	names := l.Facilities()
	enabled := l.FacilityDebugging()
	sort.Strings(enabled)
	sendJSON(w, map[string]interface{}{
		"facilities": names,
		"enabled":    enabled,
	})
}

func (*service) postSystemDebug(w http.ResponseWriter, r *http.Request) {
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
	dirsOnly := qs.Get("dirsonly") != ""

	levels, err := strconv.Atoi(qs.Get("levels"))
	if err != nil {
		levels = -1
	}
	result, err := s.model.GlobalDirectoryTree(folder, prefix, levels, dirsOnly)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sendJSON(w, result)
}

func (s *service) getDBCompletion(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	folder := qs.Get("folder")    // empty means all folders
	deviceStr := qs.Get("device") // empty means local device ID

	// We will check completion status for either the local device, or a
	// specific given device ID.

	device := protocol.LocalDeviceID
	if deviceStr != "" {
		var err error
		device, err = protocol.DeviceIDFromString(deviceStr)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	if comp, err := s.model.Completion(device, folder); err != nil {
		status := http.StatusInternalServerError
		if isFolderNotFound(err) {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
	} else {
		sendJSON(w, comp.Map())
	}
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

func (s *service) postDBOverride(_ http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	folder := qs.Get("folder")
	go s.model.Override(folder)
}

func (s *service) postDBRevert(_ http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	folder := qs.Get("folder")
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

	progress, queued, rest, err := s.model.NeedFolderFiles(folder, page, perpage)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	page, perpage := getPagingParams(qs)

	files, err := s.model.RemoteNeedFolderFiles(folder, deviceID, page, perpage)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	sendJSON(w, map[string]interface{}{
		"files":   toJsonFileInfoSlice(files),
		"page":    page,
		"perpage": perpage,
	})
}

func (s *service) getDBLocalChanged(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()

	folder := qs.Get("folder")

	page, perpage := getPagingParams(qs)

	files, err := s.model.LocalChangedFolderFiles(folder, page, perpage)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	sendJSON(w, map[string]interface{}{
		"files":   toJsonFileInfoSlice(files),
		"page":    page,
		"perpage": perpage,
	})
}

func (s *service) getSystemConnections(w http.ResponseWriter, _ *http.Request) {
	sendJSON(w, s.model.ConnectionStats())
}

func (s *service) getDeviceStats(w http.ResponseWriter, _ *http.Request) {
	stats, err := s.model.DeviceStatistics()
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	sendJSON(w, stats)
}

func (s *service) getFolderStats(w http.ResponseWriter, _ *http.Request) {
	stats, err := s.model.FolderStatistics()
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	sendJSON(w, stats)
}

func (s *service) getDBFile(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	folder := qs.Get("folder")
	file := qs.Get("file")

	errStatus := http.StatusInternalServerError
	gf, gfOk, err := s.model.CurrentGlobalFile(folder, file)
	if err != nil {
		if isFolderNotFound(err) {
			errStatus = http.StatusNotFound
		}
		http.Error(w, err.Error(), errStatus)
		return
	}

	lf, lfOk, err := s.model.CurrentFolderFile(folder, file)
	if err != nil {
		if isFolderNotFound(err) {
			errStatus = http.StatusNotFound
		}
		http.Error(w, err.Error(), errStatus)
		return
	}

	if !(gfOk || lfOk) {
		// This file for sure does not exist.
		http.Error(w, "No such object in the index", http.StatusNotFound)
		return
	}

	av, err := s.model.Availability(folder, gf, protocol.BlockInfo{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	mtimeMapping, mtimeErr := s.model.GetMtimeMapping(folder, file)

	sendJSON(w, map[string]interface{}{
		"global":       jsonFileInfo(gf),
		"local":        jsonFileInfo(lf),
		"availability": av,
		"mtime": map[string]interface{}{
			"err":   mtimeErr,
			"value": mtimeMapping,
		},
	})
}

func (s *service) getDebugFile(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	folder := qs.Get("folder")
	file := qs.Get("file")

	snap, err := s.model.DBSnapshot(folder)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	mtimeMapping, mtimeErr := s.model.GetMtimeMapping(folder, file)

	lf, _ := snap.Get(protocol.LocalDeviceID, file)
	gf, _ := snap.GetGlobal(file)
	av := snap.Availability(file)
	vl := snap.DebugGlobalVersions(file)

	sendJSON(w, map[string]interface{}{
		"global":         jsonFileInfo(gf),
		"local":          jsonFileInfo(lf),
		"availability":   av,
		"globalVersions": vl.String(),
		"mtime": map[string]interface{}{
			"err":   mtimeErr,
			"value": mtimeMapping,
		},
	})
}

func (s *service) postSystemRestart(w http.ResponseWriter, _ *http.Request) {
	s.flushResponse(`{"ok": "restarting"}`, w)

	s.fatal(&svcutil.FatalErr{
		Err:    errors.New("restart initiated by rest API"),
		Status: svcutil.ExitRestart,
	})
}

func (s *service) postSystemReset(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	folder := qs.Get("folder")

	if len(folder) > 0 {
		if _, ok := s.cfg.Folders()[folder]; !ok {
			http.Error(w, "Invalid folder ID", http.StatusInternalServerError)
			return
		}
	}

	if folder == "" {
		// Reset all folders.
		for folder := range s.cfg.Folders() {
			if err := s.model.ResetFolder(folder); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		s.flushResponse(`{"ok": "resetting database"}`, w)
	} else {
		// Reset a specific folder, assuming it's supposed to exist.
		if err := s.model.ResetFolder(folder); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.flushResponse(`{"ok": "resetting folder `+folder+`"}`, w)
	}

	s.fatal(&svcutil.FatalErr{
		Err:    errors.New("restart after db reset initiated by rest API"),
		Status: svcutil.ExitRestart,
	})
}

func (s *service) postSystemShutdown(w http.ResponseWriter, _ *http.Request) {
	s.flushResponse(`{"ok": "shutting down"}`, w)
	s.fatal(&svcutil.FatalErr{
		Err:    errors.New("shutdown initiated by rest API"),
		Status: svcutil.ExitSuccess,
	})
}

func (*service) flushResponse(resp string, w http.ResponseWriter) {
	w.Write([]byte(resp + "\n"))
	f := w.(http.Flusher)
	f.Flush()
}

func (s *service) getSystemStatus(w http.ResponseWriter, _ *http.Request) {
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
		discoStatus := s.discoverer.ChildErrors()
		res["discoveryStatus"] = discoveryStatusMap(discoStatus)
		res["discoveryMethods"] = len(discoStatus) // DEPRECATED: Redundant, only for backwards compatibility, should be removed.
		discoErrors := make(map[string]*string, len(discoStatus))
		for s, e := range discoStatus {
			if e != nil {
				discoErrors[s] = errorString(e)
			}
		}
		res["discoveryErrors"] = discoErrors // DEPRECATED: Redundant, only for backwards compatibility, should be removed.
	}

	res["connectionServiceStatus"] = s.connectionsService.ListenerStatus()
	res["lastDialStatus"] = s.connectionsService.ConnectionStatus()
	res["cpuPercent"] = 0 // deprecated from API
	res["pathSeparator"] = string(filepath.Separator)
	res["urVersionMax"] = ur.Version
	res["uptime"] = s.urService.UptimeS()
	res["startTime"] = ur.StartTime
	res["guiAddressOverridden"] = s.cfg.GUI().IsOverridden()
	res["guiAddressUsed"] = s.listenerAddr.String()

	sendJSON(w, res)
}

func (s *service) getSystemError(w http.ResponseWriter, _ *http.Request) {
	sendJSON(w, map[string][]logger.Line{
		"errors": s.guiErrors.Since(time.Time{}),
	})
}

func (*service) postSystemError(_ http.ResponseWriter, r *http.Request) {
	bs, _ := io.ReadAll(r.Body)
	r.Body.Close()
	l.Warnln(string(bs))
}

func (s *service) postSystemErrorClear(_ http.ResponseWriter, _ *http.Request) {
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
	if jsonConfig, err := json.MarshalIndent(getRedactedConfig(s), "", "  "); err != nil {
		l.Warnln("Support bundle: failed to create config.json:", err)
	} else {
		files = append(files, fileEntry{name: "config.json.txt", data: jsonConfig})
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
			l.Warnln("Support bundle: failed to create errors.json:", err)
		} else {
			files = append(files, fileEntry{name: "errors.json.txt", data: jsonError})
		}
	}

	// Panic files
	if panicFiles, err := filepath.Glob(filepath.Join(locations.GetBaseDir(locations.ConfigBaseDir), "panic*")); err == nil {
		for _, f := range panicFiles {
			if panicFile, err := os.ReadFile(f); err != nil {
				l.Warnf("Support bundle: failed to load %s: %s", filepath.Base(f), err)
			} else {
				files = append(files, fileEntry{name: filepath.Base(f), data: panicFile})
			}
		}
	}

	// Archived log (default on Windows)
	if logFile, err := os.ReadFile(locations.Get(locations.LogFile)); err == nil {
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
	if r, err := s.urService.ReportDataPreview(r.Context(), ur.Version); err != nil {
		l.Warnln("Support bundle: failed to create usage-reporting.json.txt:", err)
	} else {
		if usageReportingData, err := json.MarshalIndent(r, "", "  "); err != nil {
			l.Warnln("Support bundle: failed to serialize usage-reporting.json.txt", err)
		} else {
			files = append(files, fileEntry{name: "usage-reporting.json.txt", data: usageReportingData})
		}
	}

	// Metrics data as text
	buf := bytes.NewBuffer(nil)
	wr := bufferedResponseWriter{Writer: buf}
	promhttp.Handler().ServeHTTP(wr, &http.Request{Method: http.MethodGet})
	files = append(files, fileEntry{name: "metrics.txt", data: buf.Bytes()})

	// Connection data as JSON
	connStats := s.model.ConnectionStats()
	if connStatsJSON, err := json.MarshalIndent(connStats, "", "  "); err != nil {
		l.Warnln("Support bundle: failed to serialize connection-stats.json.txt", err)
	} else {
		files = append(files, fileEntry{name: "connection-stats.json.txt", data: connStatsJSON})
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
	if err := os.WriteFile(zipFilePath, zipFilesBuffer.Bytes(), 0o600); err != nil {
		l.Warnln("Support bundle: support bundle zip could not be created:", err)
	}

	// Serve the buffer zip to client for download
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename="+zipFileName)
	io.Copy(w, &zipFilesBuffer)
}

func (*service) getSystemHTTPMetrics(w http.ResponseWriter, _ *http.Request) {
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

func (s *service) getSystemDiscovery(w http.ResponseWriter, _ *http.Request) {
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
	if r, err := s.urService.ReportDataPreview(context.TODO(), version); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	} else {
		sendJSON(w, r)
	}
}

func (*service) getRandomString(w http.ResponseWriter, r *http.Request) {
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

	lines, patterns, err := s.model.LoadIgnores(folder)
	if err != nil && !ignore.IsParseError(err) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sendJSON(w, map[string]interface{}{
		"ignore":   lines,
		"expanded": patterns,
		"error":    errorString(err),
	})
}

func (s *service) postDBIgnores(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()

	bs, err := io.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var data map[string][]string
	err = json.Unmarshal(bs, &data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = s.model.SetIgnores(qs.Get("folder"), data["ignore"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.getDBIgnores(w, r)
}

func (s *service) getIndexEvents(w http.ResponseWriter, r *http.Request) {
	mask := s.getEventMask(r.URL.Query().Get("events"))
	sub := s.getEventSub(mask)
	s.getEvents(w, r, sub)
}

func (s *service) getDiskEvents(w http.ResponseWriter, r *http.Request) {
	sub := s.getEventSub(DiskEventMask)
	s.getEvents(w, r, sub)
}

func (s *service) getEvents(w http.ResponseWriter, r *http.Request, eventSub events.BufferedSubscription) {
	if eventSub.Mask()&(events.FolderSummary|events.FolderCompletion) != 0 {
		s.fss.OnEventRequest()
	}

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

func (*service) getEventMask(evs string) events.EventType {
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
		evsub := s.evLogger.Subscribe(mask)
		bufsub = events.NewBufferedSubscription(evsub, EventSubBufferSize)
		s.eventSubs[mask] = bufsub
	}
	s.eventSubsMut.Unlock()

	return bufsub
}

func (s *service) getSystemUpgrade(w http.ResponseWriter, _ *http.Request) {
	if s.noUpgrade {
		http.Error(w, upgrade.ErrUpgradeUnsupported.Error(), http.StatusNotImplemented)
		return
	}
	opts := s.cfg.Options()
	rel, err := upgrade.LatestRelease(opts.ReleasesURL, build.Version, opts.UpgradeToPreReleases)
	if err != nil {
		httpError(w, err)
		return
	}
	res := make(map[string]interface{})
	res["running"] = build.Version
	res["latest"] = rel.Tag
	res["newer"] = upgrade.CompareVersions(rel.Tag, build.Version) == upgrade.Newer
	res["majorNewer"] = upgrade.CompareVersions(rel.Tag, build.Version) == upgrade.MajorNewer

	sendJSON(w, res)
}

func (*service) getDeviceID(w http.ResponseWriter, r *http.Request) {
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

func (*service) getLang(w http.ResponseWriter, r *http.Request) {
	lang := r.Header.Get("Accept-Language")
	var langs []string
	for _, l := range strings.Split(lang, ",") {
		parts := strings.SplitN(l, ";", 2)
		langs = append(langs, strings.ToLower(strings.TrimSpace(parts[0])))
	}
	sendJSON(w, langs)
}

func (s *service) postSystemUpgrade(w http.ResponseWriter, _ *http.Request) {
	opts := s.cfg.Options()
	rel, err := upgrade.LatestRelease(opts.ReleasesURL, build.Version, opts.UpgradeToPreReleases)
	if err != nil {
		httpError(w, err)
		return
	}

	if upgrade.CompareVersions(rel.Tag, build.Version) > upgrade.Equal {
		err = upgrade.To(rel)
		if err != nil {
			l.Warnln("upgrading:", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		s.flushResponse(`{"ok": "restarting"}`, w)
		s.fatal(&svcutil.FatalErr{
			Err:    errors.New("exit after upgrade initiated by rest API"),
			Status: svcutil.ExitUpgrade,
		})
	}
}

func (s *service) makeDevicePauseHandler(paused bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		qs := r.URL.Query()
		deviceStr := qs.Get("device")

		var msg string
		var status int
		_, err := s.cfg.Modify(func(cfg *config.Configuration) {
			if deviceStr == "" {
				for i := range cfg.Devices {
					cfg.Devices[i].Paused = paused
				}
				return
			}

			device, err := protocol.DeviceIDFromString(deviceStr)
			if err != nil {
				msg = err.Error()
				status = 500
				return
			}

			_, i, ok := cfg.Device(device)
			if !ok {
				msg = "not found"
				status = http.StatusNotFound
				return
			}

			cfg.Devices[i].Paused = paused
		})

		if msg != "" {
			http.Error(w, msg, status)
		} else if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
			http.Error(w, "Error scanning folders", http.StatusInternalServerError)
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

func (*service) getHealth(w http.ResponseWriter, _ *http.Request) {
	sendJSON(w, map[string]string{"status": "OK"})
}

func (*service) getQR(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	text := qs.Get("text")
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		http.Error(w, "Invalid", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Write(code.PNG())
}

func (s *service) getPeerCompletion(w http.ResponseWriter, _ *http.Request) {
	tot := map[string]float64{}
	count := map[string]float64{}

	for _, folder := range s.cfg.Folders() {
		for _, device := range folder.DeviceIDs() {
			deviceStr := device.String()
			if s.model.ConnectedTo(device) {
				comp, err := s.model.Completion(device, folder.ID)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				tot[deviceStr] += comp.CompletionPct
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sendJSON(w, versions)
}

func (s *service) postFolderVersionsRestore(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()

	bs, err := io.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var versions map[string]time.Time
	err = json.Unmarshal(bs, &versions)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ferr, err := s.model.RestoreFolderVersions(qs.Get("folder"), versions)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sendJSON(w, errorStringMap(ferr))
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

func (*service) getSystemBrowse(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	current := qs.Get("current")

	// Default value or in case of error unmarshalling ends up being basic fs.
	var fsType fs.FilesystemType
	fsType.UnmarshalText([]byte(qs.Get("filesystem")))

	sendJSON(w, browse(fsType, current))
}

func browse(fsType fs.FilesystemType, current string) []string {
	if current == "" {
		return browseRoots(fsType)
	}

	parent, base := parentAndBase(current)
	ffs := fs.NewFilesystem(fsType, parent)
	files := browseFiles(ffs, base)
	for i := range files {
		files[i] = filepath.Join(parent, files[i])
	}
	return files
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

func browseRoots(fsType fs.FilesystemType) []string {
	filesystem := fs.NewFilesystem(fsType, "")
	if roots, err := filesystem.Roots(); err == nil {
		return roots
	}

	return nil
}

// parentAndBase returns the parent directory and the remaining base of the
// path. The base may be empty if the path ends with a path separator.
func parentAndBase(current string) (string, string) {
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

	return searchDir, searchFile
}

func browseFiles(ffs fs.Filesystem, search string) []string {
	subdirectories, _ := ffs.DirNames(".")
	pathSeparator := string(fs.PathSeparator)

	exactMatches := make([]string, 0, len(subdirectories))
	caseInsMatches := make([]string, 0, len(subdirectories))

	for _, subdirectory := range subdirectories {
		info, err := ffs.Stat(subdirectory)
		if err != nil || !info.IsDir() {
			continue
		}

		switch checkPrefixMatch(subdirectory, search) {
		case matchExact:
			exactMatches = append(exactMatches, subdirectory+pathSeparator)
		case matchCaseIns:
			caseInsMatches = append(caseInsMatches, subdirectory+pathSeparator)
		}
	}

	// sort to return matches in deterministic order (don't depend on file system order)
	sort.Strings(exactMatches)
	sort.Strings(caseInsMatches)
	return append(exactMatches, caseInsMatches...)
}

func (*service) getCPUProf(w http.ResponseWriter, r *http.Request) {
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

func (*service) getHeapProf(w http.ResponseWriter, _ *http.Request) {
	filename := fmt.Sprintf("syncthing-heap-%s-%s-%s-%s.pprof", runtime.GOOS, runtime.GOARCH, build.Version, time.Now().Format("150405")) // hhmmss

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)

	runtime.GC()
	pprof.WriteHeapProfile(w)
}

func toJsonFileInfoSlice(fs []db.FileInfoTruncated) []jsonFileInfoTrunc {
	res := make([]jsonFileInfoTrunc, len(fs))
	for i, f := range fs {
		res[i] = jsonFileInfoTrunc(f)
	}
	return res
}

// Type wrappers for nice JSON serialization

type jsonFileInfo protocol.FileInfo

func (f jsonFileInfo) MarshalJSON() ([]byte, error) {
	m := fileIntfJSONMap(protocol.FileInfo(f))
	m["numBlocks"] = len(f.Blocks)
	return json.Marshal(m)
}

type jsonFileInfoTrunc db.FileInfoTruncated

func (f jsonFileInfoTrunc) MarshalJSON() ([]byte, error) {
	m := fileIntfJSONMap(db.FileInfoTruncated(f))
	m["numBlocks"] = nil // explicitly unknown
	return json.Marshal(m)
}

func fileIntfJSONMap(f protocol.FileIntf) map[string]interface{} {
	out := map[string]interface{}{
		"name":          f.FileName(),
		"type":          f.FileType().String(),
		"size":          f.FileSize(),
		"deleted":       f.IsDeleted(),
		"invalid":       f.IsInvalid(),
		"ignored":       f.IsIgnored(),
		"mustRescan":    f.MustRescan(),
		"noPermissions": !f.HasPermissionBits(),
		"modified":      f.ModTime(),
		"modifiedBy":    f.FileModifiedBy().String(),
		"sequence":      f.SequenceNo(),
		"version":       jsonVersionVector(f.FileVersion()),
		"localFlags":    f.FileLocalFlags(),
		"platform":      f.PlatformData(),
		"inodeChange":   f.InodeChangeTime(),
		"blocksHash":    f.FileBlocksHash(),
	}
	if f.HasPermissionBits() {
		out["permissions"] = fmt.Sprintf("%#o", f.FilePermissions())
	}
	return out
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
	fis, err := os.ReadDir(dir)
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
	host = strings.ToLower(host)
	switch {
	case host == "localhost":
		return true
	case host == "localhost.":
		return true
	case strings.HasSuffix(host, ".localhost"):
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

// shouldRegenerateCertificate checks for certificate expiry or other known
// issues with our API/GUI certificate and returns either nil (leave the
// certificate alone) or an error describing the reason the certificate
// should be regenerated.
func shouldRegenerateCertificate(cert tls.Certificate) error {
	leaf := cert.Leaf
	if leaf == nil {
		// Leaf can be nil or not, depending on how parsed the certificate
		// was when we got it.
		if len(cert.Certificate) < 1 {
			// can't happen
			return errors.New("no certificate in certificate")
		}
		var err error
		leaf, err = x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return err
		}
	}

	if leaf.Subject.String() != leaf.Issuer.String() || len(leaf.IPAddresses) != 0 {
		// The certificate is not self signed, or has IP attributes we don't
		// add, so we leave it alone.
		return nil
	}
	if len(leaf.DNSNames) > 1 {
		// The certificate has more DNS SANs attributes than we ever add, so
		// we leave it alone.
		return nil
	}
	if len(leaf.DNSNames) == 1 && leaf.DNSNames[0] != leaf.Issuer.CommonName {
		// The one SAN is different from the issuer, so it's not one of our
		// newer self signed certificates.
		return nil
	}

	if leaf.NotAfter.Before(time.Now()) {
		return errors.New("certificate has expired")
	}
	if leaf.NotAfter.Before(time.Now().Add(30 * 24 * time.Hour)) {
		return errors.New("certificate will soon expire")
	}

	// On macOS, check for certificates issued on or after July 1st, 2019,
	// with a longer validity time than 825 days.
	cutoff := time.Date(2019, 7, 1, 0, 0, 0, 0, time.UTC)
	if build.IsDarwin &&
		leaf.NotBefore.After(cutoff) &&
		leaf.NotAfter.Sub(leaf.NotBefore) > 825*24*time.Hour {
		return errors.New("certificate incompatible with macOS 10.15 (Catalina)")
	}

	return nil
}

func errorStringMap(errs map[string]error) map[string]*string {
	out := make(map[string]*string, len(errs))
	for s, e := range errs {
		out[s] = errorString(e)
	}
	return out
}

func errorString(err error) *string {
	if err != nil {
		msg := err.Error()
		return &msg
	}
	return nil
}

type discoveryStatusEntry struct {
	Error *string `json:"error"`
}

func discoveryStatusMap(errs map[string]error) map[string]discoveryStatusEntry {
	out := make(map[string]discoveryStatusEntry, len(errs))
	for s, e := range errs {
		out[s] = discoveryStatusEntry{
			Error: errorString(e),
		}
	}
	return out
}

// sanitizedHostname returns the given name in a suitable form for use as
// the common name in a certificate, or an error.
func sanitizedHostname(name string) (string, error) {
	// Remove diacritics and non-alphanumerics. This works by first
	// transforming into normalization form D (things with diacriticals are
	// split into the base character and the mark) and then removing
	// undesired characters.
	t := transform.Chain(
		// Split runes with diacritics into base character and mark.
		norm.NFD,
		// Leave only [A-Za-z0-9-.].
		runes.Remove(runes.Predicate(func(r rune) bool {
			return r > unicode.MaxASCII ||
				!unicode.IsLetter(r) && !unicode.IsNumber(r) &&
					r != '.' && r != '-'
		})))
	name, _, err := transform.String(t, name)
	if err != nil {
		return "", err
	}

	// Name should not start or end with a dash or dot.
	name = strings.Trim(name, "-.")

	// Name should not be empty.
	if name == "" {
		return "", errors.New("no suitable name")
	}

	return strings.ToLower(name), nil
}

func isFolderNotFound(err error) bool {
	for _, target := range []error{
		model.ErrFolderMissing,
		model.ErrFolderPaused,
		model.ErrFolderNotRunning,
	} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

func httpError(w http.ResponseWriter, err error) {
	if errors.Is(err, upgrade.ErrUpgradeUnsupported) {
		http.Error(w, upgrade.ErrUpgradeUnsupported.Error(), http.StatusNotImplemented)
	} else {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

type bufferedResponseWriter struct {
	io.Writer
}

func (w bufferedResponseWriter) WriteHeader(int) {}
func (w bufferedResponseWriter) Header() http.Header {
	return http.Header{}
}
