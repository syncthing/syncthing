package main

import (
	"encoding/json"
	"log"
	"net/http"
	"runtime"
	"sync/atomic"
	"time"
)

var rc *rateCalculator

func statusService(addr string) {
	rc = newRateCalculator(360, 10*time.Second, &bytesProxied)

	http.HandleFunc("/status", getStatus)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}

func getStatus(w http.ResponseWriter, r *http.Request) {
	status := make(map[string]interface{})

	sessionMut.Lock()
	// This can potentially be double the number of pending sessions, as each session has two keys, one for each side.
	status["numPendingSessionKeys"] = len(pendingSessions)
	status["numActiveSessions"] = len(activeSessions)
	sessionMut.Unlock()
	status["numConnections"] = atomic.LoadInt64(&numConnections)
	status["numProxies"] = atomic.LoadInt64(&numProxies)
	status["bytesProxied"] = atomic.LoadInt64(&bytesProxied)
	status["goVersion"] = runtime.Version()
	status["goOS"] = runtime.GOOS
	status["goAarch"] = runtime.GOARCH
	status["goMaxProcs"] = runtime.GOMAXPROCS(-1)
	status["kbps10s1m5m15m30m60m"] = []int64{
		rc.rate(10/10) * 8 / 1000,
		rc.rate(60/10) * 8 / 1000,
		rc.rate(5*60/10) * 8 / 1000,
		rc.rate(15*60/10) * 8 / 1000,
		rc.rate(30*60/10) * 8 / 1000,
		rc.rate(60*60/10) * 8 / 1000,
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
	rates   []int64
	prev    int64
	counter *int64
}

func newRateCalculator(keepIntervals int, interval time.Duration, counter *int64) *rateCalculator {
	r := &rateCalculator{
		rates:   make([]int64, keepIntervals),
		counter: counter,
	}

	go r.updateRates(interval)

	return r
}

func (r *rateCalculator) updateRates(interval time.Duration) {
	for {
		now := time.Now()
		next := now.Truncate(interval).Add(interval)
		time.Sleep(next.Sub(now))

		cur := atomic.LoadInt64(r.counter)
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
