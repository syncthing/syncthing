package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"bitbucket.org/tebeka/nrsc"

	"github.com/codegangsta/martini"
)

func startGUI(addr string, m *Model) {
	router := martini.NewRouter()
	router.Get("/", getRoot)
	router.Get("/rest/version", restGetVersion)
	router.Get("/rest/model", restGetModel)
	router.Get("/rest/connections", restGetConnections)
	router.Get("/rest/config", restGetConfig)
	router.Get("/rest/need", restGetNeed)

	go func() {
		mr := martini.New()
		mr.Use(nrscStatic("gui"))
		mr.Use(martini.Recovery())
		mr.Action(router.Handle)
		mr.Map(m)
		http.ListenAndServe(addr, mr)
	}()
}

func getRoot(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/index.html", 302)
}

func restGetVersion() string {
	return Version
}

func restGetModel(m *Model, w http.ResponseWriter) {
	var res = make(map[string]interface{})

	res["globalFiles"], res["globalDeleted"], res["globalBytes"] = m.GlobalSize()
	res["localFiles"], res["localDeleted"], res["localBytes"] = m.LocalSize()
	files, total := m.NeedFiles()
	res["needFiles"], res["needBytes"] = len(files), total

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func restGetConnections(m *Model, w http.ResponseWriter) {
	var res = m.ConnectionStats()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func restGetConfig(w http.ResponseWriter) {
	var res = make(map[string]interface{})
	res["repository"] = config.OptionMap("repository")
	res["nodes"] = config.OptionMap("nodes")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func restGetNeed(m *Model, w http.ResponseWriter) {
	files, _ := m.NeedFiles()
	if files == nil {
		// We don't want the empty list to serialize as "null\n"
		files = make([]FileInfo, 0)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
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
