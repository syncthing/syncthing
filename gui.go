package main

import (
	"encoding/json"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/calmh/syncthing/model"
	"github.com/codegangsta/martini"
	"github.com/cratonica/embed"
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

	fs, err := embed.Unpack(Resources)
	if err != nil {
		panic(err)
	}

	var modt time.Time
	fi, err := os.Stat(os.Args[0])
	if err != nil {
		modt = fi.ModTime()
	}

	go func() {
		mr := martini.New()
		mr.Use(embeddedStatic(fs, modt.UTC().Format(http.TimeFormat)))
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
	var res = make(map[string]interface{})
	res["myID"] = myID
	res["repository"] = config.OptionMap("repository")
	res["nodes"] = config.OptionMap("nodes")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
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
	res["goroutines"] = runtime.NumGoroutine()
	res["alloc"] = m.Alloc
	res["sys"] = m.Sys
	cpuUsageLock.RLock()
	res["cpuPercent"] = cpuUsagePercent
	cpuUsageLock.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func embeddedStatic(fs map[string][]byte, modt string) interface{} {
	return func(res http.ResponseWriter, req *http.Request, log *log.Logger) {
		file := req.URL.Path

		if file[0] == '/' {
			file = file[1:]
		}

		bs, ok := fs[file]
		if !ok {
			return
		}

		mtype := mime.TypeByExtension(filepath.Ext(req.URL.Path))
		if len(mtype) != 0 {
			res.Header().Set("Content-Type", mtype)
		}
		res.Header().Set("Content-Size", fmt.Sprintf("%d", len(bs)))
		res.Header().Set("Last-Modified", modt)

		res.Write(bs)
	}
}
