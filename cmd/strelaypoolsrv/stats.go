// Copyright (C) 2018 Audrius Butkevicius and Contributors (see the CONTRIBUTORS file).

package main

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/syncthing/syncthing/lib/sync"
)

func init() {
	processCollectorOpts := collectors.ProcessCollectorOpts{
		Namespace: "syncthing_relaypoolsrv",
		PidFn: func() (int, error) {
			return os.Getpid(), nil
		},
	}

	prometheus.MustRegister(
		collectors.NewProcessCollector(processCollectorOpts),
	)
}

var (
	statusClient = http.Client{
		Timeout: 5 * time.Second,
	}

	apiRequestsTotal   = makeCounter("api_requests_total", "Number of API requests.", "type", "result")
	apiRequestsSeconds = makeSummary("api_requests_seconds", "Latency of API requests.", "type")

	relayTestsTotal         = makeCounter("tests_total", "Number of relay tests.", "result")
	relayTestActionsSeconds = makeSummary("test_actions_seconds", "Latency of relay test actions.", "type")

	locationLookupSeconds = makeSummary("location_lookup_seconds", "Latency of location lookups.").WithLabelValues()

	metricsRequestsSeconds = makeSummary("metrics_requests_seconds", "Latency of metric requests.").WithLabelValues()
	scrapeSeconds          = makeSummary("relay_scrape_seconds", "Latency of metric scrapes from remote relays.", "result")

	relayUptime             = makeGauge("relay_uptime", "Uptime of relay", "relay")
	relayPendingSessionKeys = makeGauge("relay_pending_session_keys", "Number of pending session keys (two keys per session, one per each side of the connection)", "relay")
	relayActiveSessions     = makeGauge("relay_active_sessions", "Number of sessions that are happening, a session contains two parties", "relay")
	relayConnections        = makeGauge("relay_connections", "Number of devices connected to the relay", "relay")
	relayProxies            = makeGauge("relay_proxies", "Number of active proxy routines sending data between peers (two proxies per session, one for each way)", "relay")
	relayBytesProxied       = makeGauge("relay_bytes_proxied", "Number of bytes proxied by the relay", "relay")
	relayGoRoutines         = makeGauge("relay_go_routines", "Number of Go routines in the process", "relay")
	relaySessionRate        = makeGauge("relay_session_rate", "Rate applied per session", "relay")
	relayGlobalRate         = makeGauge("relay_global_rate", "Global rate applied on the whole relay", "relay")
	relayBuildInfo          = makeGauge("relay_build_info", "Build information about a relay", "relay", "go_version", "go_os", "go_arch")
	relayLocationInfo       = makeGauge("relay_location_info", "Location information about a relay", "relay", "city", "country", "continent")

	lastStats = make(map[string]stats)
)

func makeGauge(name string, help string, labels ...string) *prometheus.GaugeVec {
	gauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "syncthing",
			Subsystem: "relaypoolsrv",
			Name:      name,
			Help:      help,
		},
		labels,
	)
	prometheus.MustRegister(gauge)
	return gauge
}

func makeSummary(name string, help string, labels ...string) *prometheus.SummaryVec {
	summary := prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace:  "syncthing",
			Subsystem:  "relaypoolsrv",
			Name:       name,
			Help:       help,
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
		labels,
	)
	prometheus.MustRegister(summary)
	return summary
}

func makeCounter(name string, help string, labels ...string) *prometheus.CounterVec {
	counter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "syncthing",
			Subsystem: "relaypoolsrv",
			Name:      name,
			Help:      help,
		},
		labels,
	)
	prometheus.MustRegister(counter)
	return counter
}

func statsRefresher(interval time.Duration) {
	ticker := time.NewTicker(interval)
	for range ticker.C {
		refreshStats()
	}
}

type statsFetchResult struct {
	relay *relay
	stats *stats
}

func refreshStats() {
	mut.RLock()
	relays := append(permanentRelays, knownRelays...)
	mut.RUnlock()

	now := time.Now()
	wg := sync.NewWaitGroup()

	results := make(chan statsFetchResult, len(relays))
	for _, rel := range relays {
		wg.Add(1)
		go func(rel *relay) {
			t0 := time.Now()
			stats := fetchStats(rel)
			duration := time.Since(t0).Seconds()
			result := "success"
			if stats == nil {
				result = "failed"
			}
			scrapeSeconds.WithLabelValues(result).Observe(duration)

			results <- statsFetchResult{
				relay: rel,
				stats: fetchStats(rel),
			}
			wg.Done()
		}(rel)
	}

	wg.Wait()
	close(results)

	mut.Lock()
	relayBuildInfo.Reset()
	relayLocationInfo.Reset()
	for result := range results {
		result.relay.StatsRetrieved = now
		result.relay.Stats = result.stats
		if result.stats == nil {
			deleteMetrics(result.relay.uri.Host)
		} else {
			updateMetrics(result.relay.uri.Host, *result.stats, result.relay.Location)
		}
	}
	mut.Unlock()
}

func fetchStats(relay *relay) *stats {
	statusAddr := relay.uri.Query().Get("statusAddr")
	if statusAddr == "" {
		statusAddr = ":22070"
	}

	statusHost, statusPort, err := net.SplitHostPort(statusAddr)
	if err != nil {
		return nil
	}

	if statusHost == "" {
		if host, _, err := net.SplitHostPort(relay.uri.Host); err != nil {
			return nil
		} else {
			statusHost = host
		}
	}

	url := "http://" + net.JoinHostPort(statusHost, statusPort) + "/status"

	response, err := statusClient.Get(url)
	if err != nil {
		return nil
	}

	var stats stats

	if json.NewDecoder(response.Body).Decode(&stats); err != nil {
		return nil
	}
	return &stats
}

func updateMetrics(host string, stats stats, location location) {
	if stats.GoVersion != "" || stats.GoOS != "" || stats.GoArch != "" {
		relayBuildInfo.WithLabelValues(host, stats.GoVersion, stats.GoOS, stats.GoArch).Add(1)
	}
	if location.City != "" || location.Country != "" || location.Continent != "" {
		relayLocationInfo.WithLabelValues(host, location.City, location.Country, location.Continent).Add(1)
	}

	if lastStat, ok := lastStats[host]; ok {
		stats = mergeStats(stats, lastStat)
	}

	relayUptime.WithLabelValues(host).Set(float64(stats.UptimeSeconds))
	relayPendingSessionKeys.WithLabelValues(host).Set(float64(stats.PendingSessionKeys))
	relayActiveSessions.WithLabelValues(host).Set(float64(stats.ActiveSessions))
	relayConnections.WithLabelValues(host).Set(float64(stats.Connections))
	relayProxies.WithLabelValues(host).Set(float64(stats.Proxies))
	relayBytesProxied.WithLabelValues(host).Set(float64(stats.BytesProxied))
	relayGoRoutines.WithLabelValues(host).Set(float64(stats.GoRoutines))
	relaySessionRate.WithLabelValues(host).Set(float64(stats.Options.SessionRate))
	relayGlobalRate.WithLabelValues(host).Set(float64(stats.Options.GlobalRate))
	lastStats[host] = stats
}

func deleteMetrics(host string) {
	relayUptime.DeleteLabelValues(host)
	relayPendingSessionKeys.DeleteLabelValues(host)
	relayActiveSessions.DeleteLabelValues(host)
	relayConnections.DeleteLabelValues(host)
	relayProxies.DeleteLabelValues(host)
	relayBytesProxied.DeleteLabelValues(host)
	relayGoRoutines.DeleteLabelValues(host)
	relaySessionRate.DeleteLabelValues(host)
	relayGlobalRate.DeleteLabelValues(host)
	delete(lastStats, host)
}

// Due to some unexplainable behaviour, some of the numbers sometimes travel slightly backwards (by less than 1%)
// This happens between scrapes, which is 30s, so this can't be a race.
// This causes prometheus to assume a "rate reset", hence causes phenomenal spikes.
// One of the number that moves backwards is BytesProxied, which atomically increments a counter with numeric value
// returned by net.Conn.Read(). I don't think that can return a negative value, so I have no idea what's going on.
func mergeStats(new stats, old stats) stats {
	new.UptimeSeconds = mergeValue(new.UptimeSeconds, old.UptimeSeconds)
	new.PendingSessionKeys = mergeValue(new.PendingSessionKeys, old.PendingSessionKeys)
	new.ActiveSessions = mergeValue(new.ActiveSessions, old.ActiveSessions)
	new.Connections = mergeValue(new.Connections, old.Connections)
	new.Proxies = mergeValue(new.Proxies, old.Proxies)
	new.BytesProxied = mergeValue(new.BytesProxied, old.BytesProxied)
	new.GoRoutines = mergeValue(new.GoRoutines, old.GoRoutines)
	new.Options.SessionRate = mergeValue(new.Options.SessionRate, old.Options.SessionRate)
	new.Options.GlobalRate = mergeValue(new.Options.GlobalRate, old.Options.GlobalRate)
	return new
}

func mergeValue(new, old int) int {
	if new >= old {
		return new // normal increase
	}
	if float64(new) > 0.99*float64(old) {
		return old // slight backward movement
	}
	return new // reset (relay restart)
}
