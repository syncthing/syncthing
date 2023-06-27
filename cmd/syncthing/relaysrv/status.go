// Copyright (C) 2015 Audrius Butkevicius and Contributors.

package relaysrv

import (
	"encoding/json"
	"log"
	"net/http"
	"net/http/pprof"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/syncthing/syncthing/lib/build"
)

var rc *rateCalculator

func (cli *CLI) statusService() {
	rc = newRateCalculator(360, 10*time.Second, &bytesProxied)

	handler := http.NewServeMux()
	handler.HandleFunc("/status", cli.getStatus)
	if cli.Pprof {
		handler.HandleFunc("/debug/pprof/", pprof.Index)
	}

	srv := http.Server{
		Addr:        cli.StatusAddr,
		Handler:     handler,
		ReadTimeout: 15 * time.Second,
	}
	srv.SetKeepAlivesEnabled(false)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func (cli *CLI) getStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	status := make(map[string]interface{})

	sessionMut.Lock()
	// This can potentially be double the number of pending sessions, as each session has two keys, one for each side.
	status["version"] = build.Version
	status["buildHost"] = build.Host
	status["buildUser"] = build.User
	status["buildDate"] = build.Date
	status["startTime"] = rc.startTime
	status["uptimeSeconds"] = time.Since(rc.startTime) / time.Second
	status["numPendingSessionKeys"] = len(pendingSessions)
	status["numActiveSessions"] = len(activeSessions)
	sessionMut.Unlock()
	status["numConnections"] = numConnections.Load()
	status["numProxies"] = numProxies.Load()
	status["bytesProxied"] = bytesProxied.Load()
	status["goVersion"] = runtime.Version()
	status["goOS"] = runtime.GOOS
	status["goArch"] = runtime.GOARCH
	status["goMaxProcs"] = runtime.GOMAXPROCS(-1)
	status["goNumRoutine"] = runtime.NumGoroutine()
	status["kbps10s1m5m15m30m60m"] = []int64{
		rc.rate(1) * 8 / 1000, // each interval is 10s
		rc.rate(60/10) * 8 / 1000,
		rc.rate(5*60/10) * 8 / 1000,
		rc.rate(15*60/10) * 8 / 1000,
		rc.rate(30*60/10) * 8 / 1000,
		rc.rate(60*60/10) * 8 / 1000,
	}
	status["options"] = map[string]interface{}{
		"network-timeout":  cli.NetworkTimeout / time.Second,
		"ping-interval":    cli.PingInterval / time.Second,
		"message-timeout":  cli.MessageTimeout / time.Second,
		"per-session-rate": cli.SessionLimitBps,
		"global-rate":      cli.GlobalLimitBps,
		"pools":            cli.Pools,
		"provided-by":      cli.ProvidedBy,
	}

	bs, err := json.MarshalIndent(status, "", "    ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(bs)
}

type rateCalculator struct {
	counter   *atomic.Int64
	rates     []int64
	prev      int64
	startTime time.Time
}

func newRateCalculator(keepIntervals int, interval time.Duration, counter *atomic.Int64) *rateCalculator {
	r := &rateCalculator{
		rates:     make([]int64, keepIntervals),
		counter:   counter,
		startTime: time.Now(),
	}

	go r.updateRates(interval)

	return r
}

func (r *rateCalculator) updateRates(interval time.Duration) {
	for {
		now := time.Now()
		next := now.Truncate(interval).Add(interval)
		time.Sleep(next.Sub(now))

		cur := r.counter.Load()
		rate := int64(float64(cur-r.prev) / interval.Seconds())
		copy(r.rates[1:], r.rates)
		r.rates[0] = rate
		r.prev = cur
	}
}

func (r *rateCalculator) rate(periods int) int64 {
	var tot int64
	for i := 0; i < periods; i++ {
		tot += r.rates[i]
	}
	return tot / int64(periods)
}
