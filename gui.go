package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"runtime"
	"sync"

	"bitbucket.org/tebeka/nrsc"
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

	go func() {
		mr := martini.New()
		mr.Use(nrscStatic("gui"))
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

func nrscStatic(path string) interface{} {
	if err := nrsc.Initialize(); err != nil {
		panic("Unable to initialize nrsc: " + err.Error())
	}
	return func(res http.ResponseWriter, req *http.Request, log *log.Logger) {
		file := req.URL.Path

		// nrsc expects there not to be a leading slash
		if file[0] == '/' {
			file = file[1:]
		}

		f := nrsc.Get(file)
		if f == nil {
			return
		}

		rdr, err := f.Open()
		if err != nil {
			http.Error(res, "Internal Server Error", http.StatusInternalServerError)
		}
		defer rdr.Close()

		mtype := mime.TypeByExtension(filepath.Ext(req.URL.Path))
		if len(mtype) != 0 {
			res.Header().Set("Content-Type", mtype)
		}
		res.Header().Set("Content-Size", fmt.Sprintf("%d", f.Size()))
		res.Header().Set("Last-Modified", f.ModTime().UTC().Format(http.TimeFormat))

		io.Copy(res, rdr)
	}
}
