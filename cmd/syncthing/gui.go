package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"runtime"
	"sync"
	"time"

	"code.google.com/p/go.crypto/bcrypt"
	"github.com/codegangsta/martini"
)

type guiError struct {
	Time  time.Time
	Error string
}

var (
	configInSync = true
	guiErrors    = []guiError{}
	guiErrorsMut sync.Mutex
	static       = embeddedStatic()
	staticFunc   = static.(func(http.ResponseWriter, *http.Request, *log.Logger))
)

const (
	unchangedPassword = "--password-unchanged--"
)

func startGUI(cfg GUIConfiguration, m *Model) error {
	l, err := net.Listen("tcp", cfg.Address)
	if err != nil {
		return err
	}

	router := martini.NewRouter()
	router.Get("/", getRoot)
	router.Get("/rest/version", restGetVersion)
	router.Get("/rest/model", restGetModel)
	router.Get("/rest/connections", restGetConnections)
	router.Get("/rest/config", restGetConfig)
	router.Get("/rest/config/sync", restGetConfigInSync)
	router.Get("/rest/system", restGetSystem)
	router.Get("/rest/errors", restGetErrors)
	router.Get("/rest/discovery", restGetDiscovery)

	router.Post("/rest/config", restPostConfig)
	router.Post("/rest/restart", restPostRestart)
	router.Post("/rest/reset", restPostReset)
	router.Post("/rest/shutdown", restPostShutdown)
	router.Post("/rest/error", restPostError)
	router.Post("/rest/error/clear", restClearErrors)
	router.Post("/rest/discovery/hint", restPostDiscoveryHint)

	mr := martini.New()
	if len(cfg.User) > 0 && len(cfg.Password) > 0 {
		mr.Use(basic(cfg.User, cfg.Password))
	}
	mr.Use(static)
	mr.Use(martini.Recovery())
	mr.Use(restMiddleware)
	mr.Action(router.Handle)
	mr.Map(m)

	go http.Serve(l, mr)

	return nil
}

func getRoot(w http.ResponseWriter, r *http.Request) {
	r.URL.Path = "/index.html"
	staticFunc(w, r, nil)
}

func restMiddleware(w http.ResponseWriter, r *http.Request) {
	if len(r.URL.Path) >= 6 && r.URL.Path[:6] == "/rest/" {
		w.Header().Set("Cache-Control", "no-cache")
	}
}

func restGetVersion() string {
	return Version
}

func restGetModel(m *Model, w http.ResponseWriter, r *http.Request) {
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

func restGetConnections(m *Model, w http.ResponseWriter) {
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

func restPostConfig(req *http.Request) {
	var prevPassHash = cfg.GUI.Password
	err := json.NewDecoder(req.Body).Decode(&cfg)
	if err != nil {
		warnln(err)
	} else {
		if cfg.GUI.Password == "" {
			// Leave it empty
		} else if cfg.GUI.Password != unchangedPassword {
			hash, err := bcrypt.GenerateFromPassword([]byte(cfg.GUI.Password), 0)
			if err != nil {
				warnln(err)
			} else {
				cfg.GUI.Password = string(hash)
			}
		} else {
			cfg.GUI.Password = prevPassHash
		}
		saveConfig()
		configInSync = false
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
	showGuiError(string(bs))
}

func restClearErrors() {
	guiErrorsMut.Lock()
	guiErrors = nil
	guiErrorsMut.Unlock()
}

func showGuiError(err string) {
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

func basic(username string, passhash string) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
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
