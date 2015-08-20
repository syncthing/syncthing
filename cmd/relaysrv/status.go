package main

import (
	"encoding/json"
	"log"
	"net/http"
	"runtime"
	"sync/atomic"
)

func statusService(addr string) {
	http.HandleFunc("/status", getStatus)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}

func getStatus(w http.ResponseWriter, r *http.Request) {
	status := make(map[string]interface{})

	sessionMut.Lock()
	status["numSessions"] = len(sessions)
	sessionMut.Unlock()
	status["numProxies"] = atomic.LoadInt64(&numProxies)
	status["bytesProxied"] = atomic.LoadInt64(&bytesProxied)
	status["goVersion"] = runtime.Version()
	status["goOS"] = runtime.GOOS
	status["goAarch"] = runtime.GOARCH
	status["goMaxProcs"] = runtime.GOMAXPROCS(-1)

	bs, err := json.MarshalIndent(status, "", "    ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(bs)
}
