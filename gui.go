package main

import (
	"encoding/json"
	"log"
	"net/http"
	"runtime"
	"sync"

	"github.com/calmh/syncthing/model"
	"github.com/codegangsta/martini"
)

func startGUI(addr string, m *model.Model) {
	router := martini.NewRouter()
	router.Get("/", getRoot)
	router.Get("/rest/version", restGetVersion)
	router.Get("/rest/model", restGetModel)
	router.Get("/rest/connections", restGetConnections)
	router.Get("/rest/config", restGetConfig)
	router.Get("/rest/need", restGetNeed)
	router.Get("/rest/system", restGetSystem)

	router.Post("/rest/config", restPostConfig)

	go func() {
		mr := martini.New()
		mr.Use(embeddedStatic())
		mr.Use(martini.Recovery())
		mr.Action(router.Handle)
		mr.Map(m)
		err := http.ListenAndServe(addr, mr)
		if err != nil {
			warnln("GUI not possible:", err)
		}
	}()
}

func getRoot(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/index.html", 302)
}

func restGetVersion() string {
	return Version
}

func restGetModel(m *model.Model, w http.ResponseWriter) {
	var res = make(map[string]interface{})

	globalFiles, globalDeleted, globalBytes := m.GlobalSize()
	res["globalFiles"], res["globalDeleted"], res["globalBytes"] = globalFiles, globalDeleted, globalBytes

	localFiles, localDeleted, localBytes := m.LocalSize()
	res["localFiles"], res["localDeleted"], res["localBytes"] = localFiles, localDeleted, localBytes

	inSyncFiles, inSyncBytes := m.InSyncSize()
	res["inSyncFiles"], res["inSyncBytes"] = inSyncFiles, inSyncBytes

	files, total := m.NeedFiles()
	res["needFiles"], res["needBytes"] = len(files), total

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func restGetConnections(m *model.Model, w http.ResponseWriter) {
	var res = m.ConnectionStats()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func restGetConfig(w http.ResponseWriter) {
	json.NewEncoder(w).Encode(cfg)
}

func restPostConfig(req *http.Request) {
	err := json.NewDecoder(req.Body).Decode(&cfg)
	if err != nil {
		log.Println(err)
	} else {
		saveConfig()
	}
}

type guiFile model.File

func (f guiFile) MarshalJSON() ([]byte, error) {
	type t struct {
		Name string
		Size int
	}
	return json.Marshal(t{
		Name: f.Name,
		Size: model.File(f).Size(),
	})
}

func restGetNeed(m *model.Model, w http.ResponseWriter) {
	files, _ := m.NeedFiles()
	gfs := make([]guiFile, len(files))
	for i, f := range files {
		gfs[i] = guiFile(f)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(gfs)
}

var cpuUsagePercent float64
var cpuUsageLock sync.RWMutex

func restGetSystem(w http.ResponseWriter) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	res := make(map[string]interface{})
	res["myID"] = myID
	res["goroutines"] = runtime.NumGoroutine()
	res["alloc"] = m.Alloc
	res["sys"] = m.Sys
	cpuUsageLock.RLock()
	res["cpuPercent"] = cpuUsagePercent
	cpuUsageLock.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}
