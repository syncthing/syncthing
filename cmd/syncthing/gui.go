// Copyright (C) 2014 Jakob Borg and other contributors. All rights reserved.
// Use of this source code is governed by an MIT-style license that can be
// found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"mime"
	"net"
	"net/http"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"time"

	"crypto/tls"
	"code.google.com/p/go.crypto/bcrypt"
	"github.com/calmh/syncthing/auto"
	"github.com/calmh/syncthing/config"
	"github.com/calmh/syncthing/logger"
	"github.com/calmh/syncthing/model"
	"github.com/codegangsta/martini"
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
	static       func(http.ResponseWriter, *http.Request, *log.Logger)
	apiKey       string
)

const (
	unchangedPassword = "--password-unchanged--"
)

func init() {
	l.AddHandler(logger.LevelWarn, showGuiError)
}

func startGUI(cfg config.GUIConfiguration, assetDir string, m *model.Model) error {
	var listener net.Listener
	var err error
	if cfg.UseTLS {
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
		listener, err = tls.Listen("tcp", cfg.Address, tlsCfg)
		if err != nil {
			return err
		}
	} else {
		listener, err = net.Listen("tcp", cfg.Address)
		if err != nil {
			return err
		}
	}

	if len(assetDir) > 0 {
		static = martini.Static(assetDir).(func(http.ResponseWriter, *http.Request, *log.Logger))
	} else {
		static = embeddedStatic()
	}

	router := martini.NewRouter()
	router.Get("/", getRoot)
	router.Get("/rest/version", restGetVersion)
	router.Get("/rest/model", restGetModel)
	router.Get("/rest/need", restGetNeed)
	router.Get("/rest/connections", restGetConnections)
	router.Get("/rest/config", restGetConfig)
	router.Get("/rest/config/sync", restGetConfigInSync)
	router.Get("/rest/system", restGetSystem)
	router.Get("/rest/errors", restGetErrors)
	router.Get("/rest/discovery", restGetDiscovery)
	router.Get("/rest/report", restGetReport)
	router.Get("/qr/:text", getQR)

	router.Post("/rest/config", restPostConfig)
	router.Post("/rest/restart", restPostRestart)
	router.Post("/rest/reset", restPostReset)
	router.Post("/rest/shutdown", restPostShutdown)
	router.Post("/rest/error", restPostError)
	router.Post("/rest/error/clear", restClearErrors)
	router.Post("/rest/discovery/hint", restPostDiscoveryHint)
	router.Post("/rest/model/override", restPostOverride)

	mr := martini.New()
	mr.Use(csrfMiddleware)
	if len(cfg.User) > 0 && len(cfg.Password) > 0 {
		mr.Use(basic(cfg.User, cfg.Password))
	}
	mr.Use(static)
	mr.Use(martini.Recovery())
	mr.Use(restMiddleware)
	mr.Action(router.Handle)
	mr.Map(m)

	apiKey = cfg.APIKey
	loadCsrfTokens()

	go http.Serve(listener, mr)

	return nil
}

func getRoot(w http.ResponseWriter, r *http.Request) {
	r.URL.Path = "/index.html"
	static(w, r, nil)
}

func restMiddleware(w http.ResponseWriter, r *http.Request) {
	if len(r.URL.Path) >= 6 && r.URL.Path[:6] == "/rest/" {
		w.Header().Set("Cache-Control", "no-cache")
	}
}

func restGetVersion() string {
	return Version
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

	res["state"] = m.State(repo)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func restPostOverride(m *model.Model, r *http.Request) {
	var qs = r.URL.Query()
	var repo = qs.Get("repo")
	m.Override(repo)
}

func restGetNeed(m *model.Model, w http.ResponseWriter, r *http.Request) {
	var qs = r.URL.Query()
	var repo = qs.Get("repo")

	files := m.NeedFilesRepo(repo)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

func restGetConnections(m *model.Model, w http.ResponseWriter) {
	var res = m.ConnectionStats()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func restGetConfig(w http.ResponseWriter) {
	encCfg := cfg
	if encCfg.GUI.Password != "" {
		encCfg.GUI.Password = unchangedPassword
	}
	json.NewEncoder(w).Encode(encCfg)
}

func restPostConfig(req *http.Request, m *model.Model) {
	var newCfg config.Configuration
	err := json.NewDecoder(req.Body).Decode(&newCfg)
	if err != nil {
		l.Warnln(err)
	} else {
		if newCfg.GUI.Password == "" {
			// Leave it empty
		} else if newCfg.GUI.Password == unchangedPassword {
			newCfg.GUI.Password = cfg.GUI.Password
		} else {
			hash, err := bcrypt.GenerateFromPassword([]byte(newCfg.GUI.Password), 0)
			if err != nil {
				l.Warnln(err)
			} else {
				newCfg.GUI.Password = string(hash)
			}
		}

		// Figure out if any changes require a restart

		if len(cfg.Repositories) != len(newCfg.Repositories) {
			configInSync = false
		} else {
			om := cfg.RepoMap()
			nm := newCfg.RepoMap()
			for id := range om {
				if !reflect.DeepEqual(om[id], nm[id]) {
					configInSync = false
					break
				}
			}
		}

		if len(cfg.Nodes) != len(newCfg.Nodes) {
			configInSync = false
		} else {
			om := cfg.NodeMap()
			nm := newCfg.NodeMap()
			for k := range om {
				if _, ok := nm[k]; !ok {
					configInSync = false
					break
				}
			}
		}

		if newCfg.Options.UREnabled && !cfg.Options.UREnabled {
			// UR was enabled
			cfg.Options.UREnabled = true
			cfg.Options.URDeclined = false
			cfg.Options.URAccepted = usageReportVersion
			// Set the corresponding options in newCfg so we don't trigger the restart check if this was the only option change
			newCfg.Options.URDeclined = false
			newCfg.Options.URAccepted = usageReportVersion
			err := sendUsageReport(m)
			if err != nil {
				l.Infoln("Usage report:", err)
			}
			go usageReportingLoop(m)
		} else if !newCfg.Options.UREnabled && cfg.Options.UREnabled {
			// UR was disabled
			cfg.Options.UREnabled = false
			cfg.Options.URDeclined = true
			cfg.Options.URAccepted = 0
			// Set the corresponding options in newCfg so we don't trigger the restart check if this was the only option change
			newCfg.Options.URDeclined = true
			newCfg.Options.URAccepted = 0
			stopUsageReporting()
		} else {
			cfg.Options.URDeclined = newCfg.Options.URDeclined
		}

		if !reflect.DeepEqual(cfg.Options, newCfg.Options) || !reflect.DeepEqual(cfg.GUI, newCfg.GUI) {
			configInSync = false
		}

		// Activate and save

		cfg = newCfg
		saveConfig()
	}
}

func restGetConfigInSync(w http.ResponseWriter) {
	json.NewEncoder(w).Encode(map[string]bool{"configInSync": configInSync})
}

func restPostRestart(w http.ResponseWriter) {
	flushResponse(`{"ok": "restarting"}`, w)
	go restart()
}

func restPostReset(w http.ResponseWriter) {
	flushResponse(`{"ok": "resetting repos"}`, w)
	resetRepositories()
	go restart()
}

func restPostShutdown(w http.ResponseWriter) {
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

func restGetSystem(w http.ResponseWriter) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	res := make(map[string]interface{})
	res["myID"] = myID
	res["goroutines"] = runtime.NumGoroutine()
	res["alloc"] = m.Alloc
	res["sys"] = m.Sys
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func restGetErrors(w http.ResponseWriter) {
	guiErrorsMut.Lock()
	json.NewEncoder(w).Encode(guiErrors)
	guiErrorsMut.Unlock()
}

func restPostError(req *http.Request) {
	bs, _ := ioutil.ReadAll(req.Body)
	req.Body.Close()
	showGuiError(0, string(bs))
}

func restClearErrors() {
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

func restPostDiscoveryHint(r *http.Request) {
	var qs = r.URL.Query()
	var node = qs.Get("node")
	var addr = qs.Get("addr")
	if len(node) != 0 && len(addr) != 0 && discoverer != nil {
		discoverer.Hint(node, []string{addr})
	}
}

func restGetDiscovery(w http.ResponseWriter) {
	json.NewEncoder(w).Encode(discoverer.All())
}

func restGetReport(w http.ResponseWriter, m *model.Model) {
	json.NewEncoder(w).Encode(reportData(m))
}

func getQR(w http.ResponseWriter, params martini.Params) {
	code, err := qr.Encode(params["text"], qr.M)
	if err != nil {
		http.Error(w, "Invalid", 500)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Write(code.PNG())
}

func basic(username string, passhash string) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		if validAPIKey(req.Header.Get("X-API-Key")) {
			return
		}

		error := func() {
			time.Sleep(time.Duration(rand.Intn(100)+100) * time.Millisecond)
			res.Header().Set("WWW-Authenticate", "Basic realm=\"Authorization Required\"")
			http.Error(res, "Not Authorized", http.StatusUnauthorized)
		}

		hdr := req.Header.Get("Authorization")
		if len(hdr) < len("Basic ") || hdr[:6] != "Basic " {
			error()
			return
		}

		hdr = hdr[6:]
		bs, err := base64.StdEncoding.DecodeString(hdr)
		if err != nil {
			error()
			return
		}

		fields := bytes.SplitN(bs, []byte(":"), 2)
		if len(fields) != 2 {
			error()
			return
		}

		if string(fields[0]) != username {
			error()
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(passhash), fields[1]); err != nil {
			error()
			return
		}
	}
}

func validAPIKey(k string) bool {
	return len(apiKey) > 0 && k == apiKey
}

func embeddedStatic() func(http.ResponseWriter, *http.Request, *log.Logger) {
	var modt = time.Now().UTC().Format(http.TimeFormat)

	return func(res http.ResponseWriter, req *http.Request, log *log.Logger) {
		file := req.URL.Path

		if file[0] == '/' {
			file = file[1:]
		}

		bs, ok := auto.Assets[file]
		if !ok {
			return
		}

		mtype := mime.TypeByExtension(filepath.Ext(req.URL.Path))
		if len(mtype) != 0 {
			res.Header().Set("Content-Type", mtype)
		}
		res.Header().Set("Content-Length", fmt.Sprintf("%d", len(bs)))
		res.Header().Set("Last-Modified", modt)

		res.Write(bs)
	}
}
