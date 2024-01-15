// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package serve

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/syncthing/syncthing/cmd/ursrv/blob"
	"github.com/syncthing/syncthing/cmd/ursrv/report"
	"github.com/syncthing/syncthing/lib/ur/contract"
)

type CLI struct {
	Debug  bool   `env:"UR_DEBUG"`
	Listen string `env:"UR_LISTEN_V2" default:"0.0.0.0:8081"`
	_      string `env:"UR_REPORTS_PROXY" help:"Old address to send the incoming reports to (temporary)"`
}

const maxCacheTime = 15 * time.Minute

var (
	//go:embed static
	statics embed.FS
	tpl     *template.Template
)

func (cli *CLI) Run(s3Config blob.S3Config) error {
	// Template
	fd, err := statics.Open("static/index.html")
	if err != nil {
		log.Fatalln("template:", err)
	}
	bs, err := io.ReadAll(fd)
	if err != nil {
		log.Fatalln("template:", err)
	}
	fd.Close()
	tpl = template.Must(template.New("index.html").Funcs(funcs).Parse(string(bs)))

	// Initialize the storage and store.
	b := blob.NewBlobStorage(s3Config)
	store := blob.NewUrsrvStore(b)

	// Listening
	listener, err := net.Listen("tcp", cli.Listen)
	if err != nil {
		log.Fatalln("listen:", err)
	}

	srv := &server{
		store:             store,
		debug:             cli.Debug,
		cachedSummary:     newSummary(),
		cachedBlockstats:  newBlockStats(),
		cachedPerformance: newPerformance(),
	}
	http.HandleFunc("/", srv.rootHandler)
	http.HandleFunc("/newdata", srv.newDataHandler)
	http.HandleFunc("/summary.json", srv.summaryHandler)
	http.HandleFunc("/performance.json", srv.performanceHandler)
	http.HandleFunc("/blockstats.json", srv.blockStatsHandler)
	http.HandleFunc("/locations.json", srv.locationsHandler)
	http.Handle("/metrics", promhttp.Handler())
	http.Handle("/static/", http.FileServer(http.FS(statics)))

	go srv.cacheRefresher()

	httpSrv := http.Server{
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 15 * time.Second,
	}
	return httpSrv.Serve(listener)
}

type server struct {
	debug bool
	store *blob.UrsrvStore

	cacheMut           sync.Mutex
	cachedLatestReport report.AggregatedReport
	cachedSummary      summary
	cachedBlockstats   [][]any
	cachedPerformance  [][]any
	cacheTime          time.Time
}

func (s *server) resetCachedStats() {
	s.cachedSummary = newSummary()
	s.cachedBlockstats = newBlockStats()
	s.cachedPerformance = newPerformance()
}

func (s *server) cacheRefresher() {
	ticker := time.NewTicker(maxCacheTime - time.Minute)
	defer ticker.Stop()
	for ; true; <-ticker.C {
		s.cacheMut.Lock()
		if err := s.refreshCacheLocked(); err != nil {
			log.Println(err)
		}
		s.cacheMut.Unlock()
	}
}

func (s *server) refreshCacheLocked() error {
	rep, err := s.store.LatestAggregatedReport()
	if err != nil {
		return err
	}
	storedReportsCount, err := s.store.CountAggregatedReports()
	if err != nil {
		return err
	}

	if rep.Date.Equal(s.cachedLatestReport.Date) && s.cachedReportCount() == storedReportsCount {
		// The latest report is already cached and the presentation data
		// contains data from all the existing reports. Update not required.
		return nil
	}

	var reportsToCache []report.AggregatedReport
	if rep.Date.After(s.cachedLatestReport.Date) {
		// The latest report from the store is more recent than the cached
		// report.
		reportsToCache = []report.AggregatedReport{rep}
	}

	if s.cachedReportCount()+len(reportsToCache) != storedReportsCount {
		// There's a discrepancy in the amount of data (to be) cached and the
		// amount available via the stored daily aggregated reports. (Re)load
		// all the reports.
		s.resetCachedStats()
		reportsToCache, err = s.store.ListAggregatedReports()
		if err != nil {
			return err
		}
	}

	if len(reportsToCache) > 0 {
		s.cachePresentationData(reportsToCache)
	}

	s.cachedLatestReport = rep
	s.cacheTime = time.Now()

	return nil
}

func (s *server) rootHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" || r.URL.Path == "/index.html" {
		s.cacheMut.Lock()
		defer s.cacheMut.Unlock()

		if time.Since(s.cacheTime) > maxCacheTime {
			if err := s.refreshCacheLocked(); err != nil {
				log.Println(err)
				http.Error(w, "Template Error", http.StatusInternalServerError)
				return
			}
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		buf := new(bytes.Buffer)
		err := tpl.Execute(buf, s.cachedLatestReport)
		if err != nil {
			return
		}
		w.Write(buf.Bytes())
	} else {
		http.Error(w, "Not found", 404)
		return
	}
}

func (s *server) locationsHandler(w http.ResponseWriter, _ *http.Request) {
	s.cacheMut.Lock()
	defer s.cacheMut.Unlock()

	if time.Since(s.cacheTime) > maxCacheTime {
		if err := s.refreshCacheLocked(); err != nil {
			log.Println(err)
			http.Error(w, "Template Error", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	locations, _ := json.Marshal(s.cachedLatestReport.Locations)
	w.Write(locations)
}

func (s *server) newDataHandler(w http.ResponseWriter, r *http.Request) {
	version := "fail"
	defer func() {
		// Version is "fail", "duplicate", "v2", "v3", ...
		metricReportsTotal.WithLabelValues(version).Inc()
	}()

	defer r.Body.Close()

	addr := r.Header.Get("X-Forwarded-For")
	if addr != "" {
		addr = strings.Split(addr, ", ")[0]
	} else {
		addr = r.RemoteAddr
	}

	if host, _, err := net.SplitHostPort(addr); err == nil {
		addr = host
	}

	if net.ParseIP(addr) == nil {
		addr = ""
	}

	var rep contract.Report
	received := time.Now().UTC()
	rep.Date = received.Format("20060102")
	rep.Address = addr

	lr := &io.LimitedReader{R: r.Body, N: 40 * 1024}
	bs, _ := io.ReadAll(lr)
	if err := json.Unmarshal(bs, &rep); err != nil {
		log.Println("decode:", err)
		if s.debug {
			log.Printf("%s", bs)
		}
		http.Error(w, "JSON Decode Error", http.StatusInternalServerError)
		return
	}

	if err := rep.Validate(); err != nil {
		log.Println("validate:", err)
		if s.debug {
			log.Printf("%#v", rep)
		}
		http.Error(w, "Validation Error", http.StatusInternalServerError)
		return
	}

	if err := s.store.PutUsageReport(rep, received); err != nil {
		if err.Error() == "already exists" {
			// We already have a report today for the same unique ID; drop
			// this one without complaining.
			version = "duplicate"
			return
		}

		if s.debug {
			log.Printf("%#v", rep)
		}
		http.Error(w, "Store Error", http.StatusInternalServerError)
		return
	}

	version = fmt.Sprintf("v%d", rep.URVersion)

	// Pass the incoming report through to the old report handler, this is
	// solely used while migrating from the old version to the updated one to
	// make sure that the incoming reports are available in both versions.
	url, err := url.Parse(os.Getenv("UR_REPORTS_PROXY"))
	if err != nil {
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(url)
	_ = r.Body.Close()
	proxy.ServeHTTP(w, r)
}

func (s *server) summaryHandler(w http.ResponseWriter, r *http.Request) {
	min, _ := strconv.Atoi(r.URL.Query().Get("min"))
	s.cacheMut.Lock()
	defer s.cacheMut.Unlock()

	if time.Since(s.cacheTime) > maxCacheTime {
		if err := s.refreshCacheLocked(); err != nil {
			log.Println(err)
			http.Error(w, "Template Error", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	summary, _ := s.cachedSummary.MarshalJSON()
	s.cachedSummary.filter(min)
	w.Write(summary)
}

func (s *server) performanceHandler(w http.ResponseWriter, _ *http.Request) {
	s.cacheMut.Lock()
	defer s.cacheMut.Unlock()

	if time.Since(s.cacheTime) > maxCacheTime {
		if err := s.refreshCacheLocked(); err != nil {
			log.Println(err)
			http.Error(w, "Template Error", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	performance, _ := json.Marshal(s.cachedPerformance)
	w.Write(performance)
}

func (s *server) blockStatsHandler(w http.ResponseWriter, _ *http.Request) {
	s.cacheMut.Lock()
	defer s.cacheMut.Unlock()

	if time.Since(s.cacheTime) > maxCacheTime {
		if err := s.refreshCacheLocked(); err != nil {
			log.Println(err)
			http.Error(w, "Template Error", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	blockstats, _ := json.Marshal(s.cachedBlockstats)
	w.Write(blockstats)
}

func (s *server) cachePresentationData(reports []report.AggregatedReport) {
	for _, rep := range reports {
		date := rep.Date.UTC().Format(time.DateOnly)

		s.cachedSummary.setCounts(date, rep.VersionCount)
		if blockStats := parseBlockStats(date, rep.Nodes, rep.BlockStats); blockStats != nil {
			s.cachedBlockstats = append(s.cachedBlockstats, blockStats)
		}
		s.cachedPerformance = append(s.cachedPerformance, []any{
			date, rep.Performance.TotFiles, rep.Performance.TotMib, float64(int(rep.Performance.Sha256Perf*10)) / 10, rep.Performance.MemorySize, rep.Performance.MemoryUsageMib,
		})
	}
}

func (s *server) cachedReportCount() int {
	return len(s.cachedBlockstats) - 1
}
