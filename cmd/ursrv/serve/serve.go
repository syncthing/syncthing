// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public License,
// v. 2.0. If a copy of the MPL was not distributed with this file, You can
// obtain one at https://mozilla.org/MPL/2.0/.

package serve

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/syncthing/syncthing/cmd/ursrv/blob"
	"github.com/syncthing/syncthing/cmd/ursrv/report"
	"github.com/syncthing/syncthing/lib/geoip"
	"github.com/syncthing/syncthing/lib/ur"
	"github.com/syncthing/syncthing/lib/ur/contract"
)

type CLI struct {
	Debug           bool   `env:"UR_DEBUG"`
	Listen          string `env:"UR_LISTEN_V2" default:"0.0.0.0:8081"`
	_               string `env:"UR_REPORTS_PROXY" help:"Old address to send the incoming reports to (temporary)"`
	GeoIPLicenseKey string `env:"UR_GEOIP_LICENSE_KEY"`
	GeoIPAccountID  int    `env:"UR_GEOIP_ACCOUNT_ID"`
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

	// Initialize the geoip provider.
	geoip, err := geoip.NewGeoLite2CityProvider(context.Background(),
		cli.GeoIPAccountID, cli.GeoIPLicenseKey, os.TempDir())
	if err != nil {
		log.Fatalln("geoip:", err)
	}
	go geoip.Serve(context.TODO())

	srv := &server{
		store:             store,
		debug:             cli.Debug,
		geoip:             geoip,
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
	geoip *geoip.Provider

	cacheMut           sync.Mutex
	cachedAggregation  ur.Aggregation          // Cached Aggregated data
	cachedLatestReport report.AggregatedReport // Cached presentation-data (GUI-formatted Aggregation data)
	cachedSummary      summary                 // Used in the versions-graph
	cachedBlockstats   [][]any                 // Used in the saved-data table
	cachedPerformance  [][]any                 // Used in the performance graphs
	cacheTime          time.Time               // Time of the last cache update
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

		if s.debug {
			fmt.Printf("\nNew cache:\nSummary:\t%v\nBlockstats:\t%v\nPerformance:\t%v\n", s.cachedSummary, s.cachedBlockstats, s.cachedPerformance)
		}
	}
}

func (s *server) refreshCacheLocked() error {
	rep, err := s.store.LatestAggregatedReport()
	if err != nil {
		return err
	}

	fromDate := time.Now().UTC().AddDate(-3, 0, 0)
	storedReportsCount, err := s.store.CountAggregatedReports(fromDate)
	if err != nil {
		return err
	}

	repDate := time.Unix(rep.Date, 0).UTC()
	cachedRepDate := time.Unix(s.cachedAggregation.Date, 0).UTC()
	if repDate.Equal(cachedRepDate) && s.cachedReportCount() == storedReportsCount {
		// The latest report is already cached and the presentation data
		// contains data from all the existing reports. Update not required.
		return nil
	}

	var reportsToCache []ur.Aggregation
	if repDate.After(cachedRepDate) {
		// The latest report from the store is more recent than the cached
		// report.
		reportsToCache = []ur.Aggregation{rep}
	}

	if s.cachedReportCount()+len(reportsToCache) != storedReportsCount {
		// There's a discrepancy in the amount of data (to be) cached and the
		// amount available via the stored daily aggregated reports. (Re)load
		// all the reports.
		s.resetCachedStats()
		reportsToCache, err = s.store.ListAggregatedReports(fromDate)
		if err != nil {
			return err
		}
	}

	if len(reportsToCache) > 0 {
		// Cache historical data.
		s.cacheHistoricalPresentationData(reportsToCache)
	}

	s.cacheCurrentPresentationData(rep) // Refresh current data.
	s.cachedAggregation = rep
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
			// We already have a report today for the same unique ID; drop this
			// one without complaining.
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
	min, err := strconv.Atoi(r.URL.Query().Get("min"))
	if err != nil || min == 0 {
		min = 50
	}

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

func (s *server) cachedReportCount() int {
	return len(s.cachedBlockstats) - 1
}

func (s *server) cacheHistoricalPresentationData(reports []ur.Aggregation) {
	log.Print("caching historical presentation data")
	for _, rep := range reports {
		date := time.Unix(rep.Date, 0).UTC().Format(time.DateOnly)

		versions, _ := histogramAnalysis(rep, "version")
		s.cachedSummary.setCountsV2(date, versions)

		if blockStats := parseBlockStatsV2(rep, date); blockStats != nil {
			s.cachedBlockstats = append(s.cachedBlockstats, blockStats)
		}

		perfTotFiles, _, _ := intAnalysis(rep, "totFiles")
		perfTotMib, _, _ := intAnalysis(rep, "totMib")
		perfSha256, _ := floatAnalysis(rep, "sha256Perf")
		perfMemSize, _, _ := intAnalysis(rep, "memorySize")
		perfMemUsageMib, _, _ := intAnalysis(rep, "memoryUsageMib")

		s.cachedPerformance = append(s.cachedPerformance, []any{
			date,
			perfTotFiles.Avg,
			perfTotMib.Avg,
			float64(int(perfSha256.Avg*10)) / 10,
			perfMemSize.Avg,
			perfMemUsageMib.Avg,
		})
	}
}

func (s *server) cacheCurrentPresentationData(rep ur.Aggregation) {
	r := report.AggregatedReport{
		Nodes:        int(rep.Count),
		VersionNodes: map[string]int64{"v2": rep.CountV2, "v3": rep.CountV3},
	}
	// Features
	r.Features = make(map[string][]report.Feature)
	r.Features["Various"] = parseVariousFeatures(rep)
	r.Features["Folder"] = parseFolderFeatures(rep)
	r.Features["Device"] = parseDeviceFeatures(rep)
	r.Features["Connection"] = parseConnectionFeatures(rep)
	r.Features["GUI"] = parseGUIFeatures(rep)

	// FeatureGroups
	r.FeatureGroups = parseFeatureGroups(rep)

	// Categories
	r.Categories = parseCategories(rep)

	// Versions VersionPenetrations
	if versions, err := histogramAnalysis(rep, "version"); err == nil {
		analytics := analyticsFor(versions, 50, int(rep.Count))
		r.Versions = group(byVersion, analytics, 5, 1.0)
		r.VersionPenetrations = penetrationLevels(analytics, []float64{50, 75, 90, 95})
	}

	// Platforms
	if platform, err := histogramAnalysis(rep, "platform"); err == nil {
		r.Platforms = group(byPlatform, analyticsFor(platform, 50, int(rep.Count)), 10, 0.0)
	}

	// Compilers
	if compiler, err := histogramAnalysis(rep, "compiler"); err == nil {
		r.Compilers = group(byCompiler, analyticsFor(compiler, 50, int(rep.Count)), 5, 1.0)
	}

	// Builders
	if builders, err := histogramAnalysis(rep, "builder"); err == nil {
		r.Builders = analyticsFor(builders, 12, int(rep.Count))
	}

	// Distributions
	if distribution, err := histogramAnalysis(rep, "distribution"); err == nil {
		r.Distributions = analyticsFor(distribution, len(knownDistributions), int(rep.Count))
	}

	// FeatureOrder
	r.FeatureOrder = append(r.FeatureOrder, "Various", "Folder", "Device", "Connection", "GUI")

	// Locations
	r.Locations = parseLocations(rep)

	// Countries
	r.Countries = parseCountries(rep)

	s.cachedLatestReport = r
}

func parseCategories(rep ur.Aggregation) []report.Category {
	var categories []report.Category
	catIntValues := []report.Category{
		{Descr: "totFiles", Type: report.NumberMetric},
		{Descr: "folderMaxFiles", Type: report.NumberMetric},
		{Descr: "totMib", Type: report.NumberBinary, Unit: "B"},
		{Descr: "folderMaxMiB", Type: report.NumberBinary, Unit: "B"},
		{Descr: "numDevices", Type: report.NumberMetric},
		{Descr: "numFolders", Type: report.NumberMetric},
		{Descr: "memoryUsageMiB", Type: report.NumberBinary, Unit: "B"},
		{Descr: "memorySize", Type: report.NumberBinary, Unit: "B"},
		{Descr: "numCPU", Type: report.NumberMetric},
		{Descr: "uptime", Type: report.NumberDuration},
	}

	for _, iv := range catIntValues {
		if stats, _, err := intAnalysis(rep, iv.Descr); err == nil {
			if len(stats.Percentiles) != 4 {
				continue
			}
			iv.Values = stats.Percentiles
			if iv.Type == report.NumberBinary {
				for i, perc := range iv.Values {
					iv.Values[i] = perc * (1 << 20)
				}
			}
			categories = append(categories, iv)
		}
	}
	sha256Perf, err := floatAnalysis(rep, "sha256Perf")
	if err == nil {
		for i, perc := range sha256Perf.Percentiles {
			sha256Perf.Percentiles[i] = perc * (1 << 20)
		}
		categories = append(categories, report.Category{
			Values: sha256Perf.Percentiles,
			Descr:  "sha256Perf",
			Unit:   "B/s",
			Type:   report.NumberBinary,
		})
	}

	return categories
}

func parseFolderFeatures(rep ur.Aggregation) []report.Feature {
	features := []string{
		"folderUses.autoNormalize",
		"folderUsesV3.fsWatcherEnabled",
		"folderUses.ignorePerms",
		"folderUses.receiveOnly",
		"folderUses.sendOnly",
		"folderUsesV3.disableTempIndexes",
		"folderUsesV3.caseSensitiveFS",
		"folderUses.ignoreDelete",
		"folderUsesV3.receiveEncrypted",
		"folderUsesV3.disableSparseFiles",
		"folderUsesV3.scanProgressDisabled",
		"folderUsesV3.customWeakHashThreshold",
		"folderUsesV3.alwaysWeakHash",
	}

	return formatFeatures(rep, features)
}

func parseGUIFeatures(rep ur.Aggregation) []report.Feature {
	features := []string{
		"guiStats.useAuth",
		"guiStats.useTLS",
		"guiStats.insecureAdminAccess",
		"guiStats.insecureSkipHostCheck",
		"guiStats.insecureAllowFrameLoading",
	}
	return formatFeatures(rep, features)
}

func parseVariousFeatures(rep ur.Aggregation) []report.Feature {
	features := []string{
		"alwaysLocalNets",
		"cacheIgnoredFiles",
		"overwriteRemoteDeviceNames",
		"progressEmitterEnabled",
		"customDefaultFolderPath",
		"customTrafficClass",
		"customTempIndexMinBlocks",
		"weakHashEnabled",
		"limitBandwidthInLan",
		"customReleaseURL",
		"restartOnWakeup",
		"customStunServers",
		"ignoreStats.lines",
	}
	return formatFeatures(rep, features)
}

func parseConnectionFeatures(rep ur.Aggregation) []report.Feature {
	features := []string{
		"relays.enabled",
		"announce.globalEnabled",
		"announce.localEnabled",
	}
	return formatFeatures(rep, features)
}

func parseDeviceFeatures(rep ur.Aggregation) []report.Feature {
	features := []string{
		"deviceUses.customCertName",
		"deviceUses.introducer",
	}
	return formatFeatures(rep, features)
}

func parseFeatureGroups(rep ur.Aggregation) map[string][]report.FeatureGroup {
	featureGroups := make(map[string][]report.FeatureGroup)

	// Versioning
	numFolders, _, _ := intAnalysis(rep, "numFolders")
	features := []string{
		"folderUses.simpleVersioning",
		"folderUses.externalVersioning",
		"folderUses.staggeredVersioning",
		"folderUses.trashcanVersioning",
	}
	group := parsedFeatureGroup(rep, "Versioning", features)
	group.Counts["Disabled"] = int(numFolders.Sum) - group.SumCounts()
	featureGroups["Folder"] = append(featureGroups["Folder"], group)

	// Conflicts
	features = []string{
		"folderUsesV3.conflictsDisabled",
		"folderUsesV3.conflictsUnlimited",
		"folderUsesV3.conflictsOther",
	}
	group = parsedFeatureGroup(rep, "Conflicts", features)
	featureGroups["Folder"] = append(featureGroups["Folder"], group)

	// Pull Order
	pullOrder, _ := mapIntAnalysis(rep, "folderUsesV3.pullOrder")
	group = report.FeatureGroup{
		Key:     "Pull Order",
		Version: "v3",
		Counts:  map[string]int{},
	}
	for key, value := range pullOrder.Map {
		group.Counts[prettyCase(key)] = int(value.Sum)
	}
	featureGroups["Folder"] = append(featureGroups["Folder"], group)

	// Copy Range Method
	copyRangeMethod, _ := mapIntAnalysis(rep, "folderUsesV3.copyRangeMethod")
	group = report.FeatureGroup{
		Key:     "Copy Range Method",
		Version: "v3",
		Counts:  map[string]int{},
	}
	for key, value := range copyRangeMethod.Map {
		group.Counts[prettyCase(key)] = int(value.Sum)
	}
	featureGroups["Folder"] = append(featureGroups["Folder"], group)

	// Compress
	features = []string{
		"deviceUses.compressAlways",
		"deviceUses.compressMetadata",
		"deviceUses.compressNever",
	}
	group = parsedFeatureGroup(rep, "Compress", features)
	featureGroups["Device"] = append(featureGroups["Device"], group)

	// Addresses
	features = []string{
		"deviceUses.dynamicAddr",
		"deviceUses.staticAddr",
	}
	group = parsedFeatureGroup(rep, "Addresses", features)
	featureGroups["Device"] = append(featureGroups["Device"], group)

	// Disccovery
	features = []string{
		"announce.defaultServersDNS",
		"announce.defaultServersIP",
		"announce.otherServers",
	}
	group = parsedFeatureGroup(rep, "Discovery", features)
	featureGroups["Connection"] = append(featureGroups["Connection"], group)

	// Relaying
	features = []string{
		"relays.defaultServers",
		"relays.otherServers",
	}
	group = parsedFeatureGroup(rep, "Relaying", features)
	featureGroups["Connection"] = append(featureGroups["Connection"], group)

	// Transport
	transportStats, _ := mapIntAnalysis(rep, "transportStats")
	transportGroup := report.FeatureGroup{
		Key:     "Transport",
		Version: "v3",
		Counts:  map[string]int{},
	}
	group = report.FeatureGroup{
		Key:     "IP version",
		Version: "v3",
		Counts:  map[string]int{},
	}
	for key, value := range transportStats.Map {
		transportGroup.Counts[cases.Title(language.English).String(key)] = int(value.Sum)
		if strings.HasSuffix(key, "4") {
			group.Counts["IPv4"] = int(value.Sum)
		} else if strings.HasSuffix(key, "6") {
			group.Counts["IPv6"] = int(value.Sum)
		} else {
			group.Counts["Unknown"] = int(value.Sum)
		}
	}
	featureGroups["Connection"] = append(featureGroups["Connection"], transportGroup)
	featureGroups["Connection"] = append(featureGroups["Connection"], group)

	// Nat Type
	natType, _ := histogramAnalysis(rep, "natType")
	group = report.FeatureGroup{
		Key:     "NAT Type",
		Version: "v3",
		Counts:  map[string]int{},
	}
	for key, value := range natType.Map {
		group.Counts[key] = int(value)
	}
	featureGroups["Various"] = append(featureGroups["Various"], group)

	// Temporary Retention
	features = []string{
		"temporariesDisabled",
		"temporariesCustom",
	}
	group = parsedFeatureGroup(rep, "Temporary Retention", features)
	group.Counts["Default"] = int(rep.Count) - group.SumCounts()
	featureGroups["Various"] = append(featureGroups["Various"], group)

	// Upgrade
	features = []string{
		"upgradeAllowedPre",
		"upgradeAllowedAuto",
		"upgradeAllowedManual",
	}
	group = parsedFeatureGroup(rep, "Upgrades", features)
	group.Counts["Disabled"] = int(rep.Count) - group.SumCounts()
	featureGroups["Various"] = append(featureGroups["Various"], group)

	// GUIStats listen address
	features = []string{
		"guiStats.listenLocal",
		"guiStats.listenUnspecified",
		"guiStats.enabled",
	}
	group = parsedFeatureGroup(rep, "Listen address", features)
	guiEnabled, _, _ := intAnalysis(rep, "guiStats.enabled")
	group.Counts["Other"] = int(guiEnabled.Sum) - group.SumCounts()
	featureGroups["GUI"] = append(featureGroups["GUI"], group)

	// GUI Theme
	guiTheme, _ := mapIntAnalysis(rep, "guiStats.theme")
	group = report.FeatureGroup{
		Key:     "Theme",
		Version: "v3",
		Counts:  map[string]int{},
	}
	for key, value := range guiTheme.Map {
		group.Counts[key] = int(value.Sum)
	}
	featureGroups["GUI"] = append(featureGroups["GUI"], group)

	return featureGroups
}

func parseLocations(rep ur.Aggregation) []report.Location {
	locations, _ := histogramAnalysis(rep, "locations")
	var locationList []report.Location

	for location, count := range locations.Map {
		loc, err := stringToLocation(location)
		if err != nil {
			continue
		}
		loc.Count = count

		locationList = append(locationList, loc)
	}

	return locationList
}

func parseCountries(rep ur.Aggregation) []report.Feature {
	countries, _ := histogramAnalysis(rep, "countries")
	var countryList []report.Feature

	for country, count := range countries.Map {
		countryList = append(countryList, report.Feature{
			Key:   country,
			Count: int(count),
			Pct:   (100 * float64(count)) / float64(countries.Count),
		})
		sort.Sort(sort.Reverse(sortableFeatureList(countryList)))
	}

	return countryList
}

func formatFeatures(rep ur.Aggregation, features []string) []report.Feature {
	var feats []report.Feature
	for _, feature := range features {
		intValue, since, _ := intAnalysis(rep, feature)
		// if key := strings.Split(feature.Key, "."); len(key) > 1 {
		//  features[i].Key = key[1]
		// }
		feats = append(feats, report.Feature{
			Key:     feature,
			Count:   int(intValue.Count),
			Version: formatVersion(since),
			Pct:     float64(intValue.Count * 100 / rep.Count),
		})
	}
	return feats
}

func parsedFeatureGroup(rep ur.Aggregation, groupTitle string, features []string) report.FeatureGroup {
	counts := make(map[string]int)
	var since int64
	for _, feature := range features {
		if analysis, s, err := intAnalysis(rep, feature); err == nil {
			counts[feature] = int(analysis.Sum)
			since = s
		}
	}

	return report.FeatureGroup{
		Key:     groupTitle,
		Version: formatVersion(since),
		Counts:  counts,
	}
}

func formatVersion(v int64) string {
	if v == 0 {
		return "v??"
	}
	return fmt.Sprintf("v%d", v)
}

func stringToLocation(s string) (report.Location, error) {
	loc := report.Location{}

	split := strings.Split(s, "~")
	if len(split) != 2 {
		return loc, errors.New("unexpected slice length")
	}

	lat, err := strconv.ParseFloat(split[0], 64)
	if err != nil {
		return loc, err
	}
	loc.Latitude = lat

	lon, err := strconv.ParseFloat(split[1], 64)
	if err != nil {
		return loc, err
	}
	loc.Longitude = lon

	return loc, nil
}
