// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"bytes"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/oschwald/geoip2-golang"

	"github.com/syncthing/syncthing/lib/upgrade"
	"github.com/syncthing/syncthing/lib/ur/contract"
)

var (
	useHTTP            = os.Getenv("UR_USE_HTTP") != ""
	debug              = os.Getenv("UR_DEBUG") != ""
	keyFile            = getEnvDefault("UR_KEY_FILE", "key.pem")
	certFile           = getEnvDefault("UR_CRT_FILE", "crt.pem")
	dbConn             = getEnvDefault("UR_DB_URL", "postgres://user:password@localhost/ur?sslmode=disable")
	listenAddr         = getEnvDefault("UR_LISTEN", "0.0.0.0:8443")
	geoIPPath          = getEnvDefault("UR_GEOIP", "GeoLite2-City.mmdb")
	tpl                *template.Template
	compilerRe         = regexp.MustCompile(`\(([A-Za-z0-9()., -]+) \w+-\w+(?:| android| default)\) ([\w@.-]+)`)
	progressBarClass   = []string{"", "progress-bar-success", "progress-bar-info", "progress-bar-warning", "progress-bar-danger"}
	featureOrder       = []string{"Various", "Folder", "Device", "Connection", "GUI"}
	knownVersions      = []string{"v2", "v3"}
	knownDistributions = []distributionMatch{
		// Maps well known builders to the official distribution method that
		// they represent
		{regexp.MustCompile("android-.*teamcity@build.syncthing.net"), "Google Play"},
		{regexp.MustCompile("teamcity@build.syncthing.net"), "GitHub"},
		{regexp.MustCompile("deb@build.syncthing.net"), "APT"},
		{regexp.MustCompile("docker@syncthing.net"), "Docker Hub"},
		{regexp.MustCompile("jenkins@build.syncthing.net"), "GitHub"},
		{regexp.MustCompile("snap@build.syncthing.net"), "Snapcraft"},
		{regexp.MustCompile("android-.*vagrant@basebox-stretch64"), "F-Droid"},
		{regexp.MustCompile("builduser@(archlinux|svetlemodry)"), "Arch (3rd party)"},
		{regexp.MustCompile("synology@kastelo.net"), "Synology (Kastelo)"},
		{regexp.MustCompile("@debian"), "Debian (3rd party)"},
		{regexp.MustCompile("@fedora"), "Fedora (3rd party)"},
		{regexp.MustCompile(`\bbrew@`), "Homebrew (3rd party)"},
		{regexp.MustCompile("."), "Others"},
	}
)

type distributionMatch struct {
	matcher      *regexp.Regexp
	distribution string
}

var funcs = map[string]interface{}{
	"commatize":  commatize,
	"number":     number,
	"proportion": proportion,
	"counter": func() *counter {
		return &counter{}
	},
	"progressBarClassByIndex": func(a int) string {
		return progressBarClass[a%len(progressBarClass)]
	},
	"slice": func(numParts, whichPart int, input []feature) []feature {
		var part []feature
		perPart := (len(input) / numParts) + len(input)%2

		parts := make([][]feature, 0, numParts)
		for len(input) >= perPart {
			part, input = input[:perPart], input[perPart:]
			parts = append(parts, part)
		}
		if len(input) > 0 {
			parts = append(parts, input)
		}
		return parts[whichPart-1]
	},
}

func getEnvDefault(key, def string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return def
}

func setupDB(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS ReportsJson (
		Received TIMESTAMP NOT NULL,
		Report JSONB NOT NULL
	)`)
	if err != nil {
		return err
	}

	var t string
	if err := db.QueryRow(`SELECT 'UniqueIDJsonIndex'::regclass`).Scan(&t); err != nil {
		if _, err = db.Exec(`CREATE UNIQUE INDEX UniqueIDJsonIndex ON ReportsJson ((Report->>'date'), (Report->>'uniqueID'))`); err != nil {
			return err
		}
	}

	if err := db.QueryRow(`SELECT 'ReceivedJsonIndex'::regclass`).Scan(&t); err != nil {
		if _, err = db.Exec(`CREATE INDEX ReceivedJsonIndex ON ReportsJson (Received)`); err != nil {
			return err
		}
	}

	if err := db.QueryRow(`SELECT 'ReportVersionJsonIndex'::regclass`).Scan(&t); err != nil {
		if _, err = db.Exec(`CREATE INDEX ReportVersionJsonIndex ON ReportsJson (cast((Report->>'urVersion') as numeric))`); err != nil {
			return err
		}
	}

	// Migrate from old schema to new schema if the table exists.
	if err := migrate(db); err != nil {
		return err
	}

	return nil
}

func insertReport(db *sql.DB, r contract.Report) error {
	_, err := db.Exec("INSERT INTO ReportsJson (Report, Received) VALUES ($1, $2)", r, time.Now().UTC())

	return err
}

type withDBFunc func(*sql.DB, http.ResponseWriter, *http.Request)

func withDB(db *sql.DB, f withDBFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f(db, w, r)
	}
}

func main() {
	log.SetFlags(log.Ltime | log.Ldate | log.Lshortfile)
	log.SetOutput(os.Stdout)

	// Template

	fd, err := os.Open("static/index.html")
	if err != nil {
		log.Fatalln("template:", err)
	}
	bs, err := io.ReadAll(fd)
	if err != nil {
		log.Fatalln("template:", err)
	}
	fd.Close()
	tpl = template.Must(template.New("index.html").Funcs(funcs).Parse(string(bs)))

	// DB

	db, err := sql.Open("postgres", dbConn)
	if err != nil {
		log.Fatalln("database:", err)
	}
	err = setupDB(db)
	if err != nil {
		log.Fatalln("database:", err)
	}

	// TLS & Listening

	var listener net.Listener
	if useHTTP {
		listener, err = net.Listen("tcp", listenAddr)
	} else {
		var cert tls.Certificate
		cert, err = tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			log.Fatalln("tls:", err)
		}

		cfg := &tls.Config{
			Certificates:           []tls.Certificate{cert},
			SessionTicketsDisabled: true,
		}
		listener, err = tls.Listen("tcp", listenAddr, cfg)
	}
	if err != nil {
		log.Fatalln("listen:", err)
	}

	srv := http.Server{
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	http.HandleFunc("/", withDB(db, rootHandler))
	http.HandleFunc("/newdata", withDB(db, newDataHandler))
	http.HandleFunc("/summary.json", withDB(db, summaryHandler))
	http.HandleFunc("/movement.json", withDB(db, movementHandler))
	http.HandleFunc("/performance.json", withDB(db, performanceHandler))
	http.HandleFunc("/blockstats.json", withDB(db, blockStatsHandler))
	http.HandleFunc("/locations.json", withDB(db, locationsHandler))
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	go cacheRefresher(db)

	err = srv.Serve(listener)
	if err != nil {
		log.Fatalln("https:", err)
	}
}

var (
	cachedIndex     []byte
	cachedLocations []byte
	cacheTime       time.Time
	cacheMut        sync.Mutex
)

const maxCacheTime = 15 * time.Minute

func cacheRefresher(db *sql.DB) {
	ticker := time.NewTicker(maxCacheTime - time.Minute)
	defer ticker.Stop()
	for ; true; <-ticker.C {
		cacheMut.Lock()
		if err := refreshCacheLocked(db); err != nil {
			log.Println(err)
		}
		cacheMut.Unlock()
	}
}

func refreshCacheLocked(db *sql.DB) error {
	rep := getReport(db)
	buf := new(bytes.Buffer)
	err := tpl.Execute(buf, rep)
	if err != nil {
		return err
	}
	cachedIndex = buf.Bytes()
	cacheTime = time.Now()

	locs := rep["locations"].(map[location]int)
	wlocs := make([]weightedLocation, 0, len(locs))
	for loc, w := range locs {
		wlocs = append(wlocs, weightedLocation{loc, w})
	}
	cachedLocations, _ = json.Marshal(wlocs)
	return nil
}

func rootHandler(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" || r.URL.Path == "/index.html" {
		cacheMut.Lock()
		defer cacheMut.Unlock()

		if time.Since(cacheTime) > maxCacheTime {
			if err := refreshCacheLocked(db); err != nil {
				log.Println(err)
				http.Error(w, "Template Error", http.StatusInternalServerError)
				return
			}
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(cachedIndex)
	} else {
		http.Error(w, "Not found", 404)
		return
	}
}

func locationsHandler(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	cacheMut.Lock()
	defer cacheMut.Unlock()

	if time.Since(cacheTime) > maxCacheTime {
		if err := refreshCacheLocked(db); err != nil {
			log.Println(err)
			http.Error(w, "Template Error", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write(cachedLocations)
}

func newDataHandler(db *sql.DB, w http.ResponseWriter, r *http.Request) {
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
	rep.Date = time.Now().UTC().Format("20060102")
	rep.Address = addr

	lr := &io.LimitedReader{R: r.Body, N: 40 * 1024}
	bs, _ := io.ReadAll(lr)
	if err := json.Unmarshal(bs, &rep); err != nil {
		log.Println("decode:", err)
		if debug {
			log.Printf("%s", bs)
		}
		http.Error(w, "JSON Decode Error", http.StatusInternalServerError)
		return
	}

	if err := rep.Validate(); err != nil {
		log.Println("validate:", err)
		if debug {
			log.Printf("%#v", rep)
		}
		http.Error(w, "Validation Error", http.StatusInternalServerError)
		return
	}

	if err := insertReport(db, rep); err != nil {
		if err.Error() == `pq: duplicate key value violates unique constraint "uniqueidjsonindex"` {
			// We already have a report today for the same unique ID; drop
			// this one without complaining.
			return
		}
		log.Println("insert:", err)
		if debug {
			log.Printf("%#v", rep)
		}
		http.Error(w, "Database Error", http.StatusInternalServerError)
		return
	}
}

func summaryHandler(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	min, _ := strconv.Atoi(r.URL.Query().Get("min"))
	s, err := getSummary(db, min)
	if err != nil {
		log.Println("summaryHandler:", err)
		http.Error(w, "Database Error", http.StatusInternalServerError)
		return
	}

	bs, err := s.MarshalJSON()
	if err != nil {
		log.Println("summaryHandler:", err)
		http.Error(w, "JSON Encode Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(bs)
}

func movementHandler(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	s, err := getMovement(db)
	if err != nil {
		log.Println("movementHandler:", err)
		http.Error(w, "Database Error", http.StatusInternalServerError)
		return
	}

	bs, err := json.Marshal(s)
	if err != nil {
		log.Println("movementHandler:", err)
		http.Error(w, "JSON Encode Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(bs)
}

func performanceHandler(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	s, err := getPerformance(db)
	if err != nil {
		log.Println("performanceHandler:", err)
		http.Error(w, "Database Error", http.StatusInternalServerError)
		return
	}

	bs, err := json.Marshal(s)
	if err != nil {
		log.Println("performanceHandler:", err)
		http.Error(w, "JSON Encode Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(bs)
}

func blockStatsHandler(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	s, err := getBlockStats(db)
	if err != nil {
		log.Println("blockStatsHandler:", err)
		http.Error(w, "Database Error", http.StatusInternalServerError)
		return
	}

	bs, err := json.Marshal(s)
	if err != nil {
		log.Println("blockStatsHandler:", err)
		http.Error(w, "JSON Encode Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(bs)
}

type category struct {
	Values [4]float64
	Key    string
	Descr  string
	Unit   string
	Type   NumberType
}

type feature struct {
	Key     string
	Version string
	Count   int
	Pct     float64
}

type featureGroup struct {
	Key     string
	Version string
	Counts  map[string]int
}

// Used in the templates
type counter struct {
	n int
}

func (c *counter) Current() int {
	return c.n
}

func (c *counter) Increment() string {
	c.n++
	return ""
}

func (c *counter) DrawTwoDivider() bool {
	return c.n != 0 && c.n%2 == 0
}

// add sets a key in a nested map, initializing things if needed as we go.
func add(storage map[string]map[string]int, parent, child string, value int) {
	n, ok := storage[parent]
	if !ok {
		n = make(map[string]int)
		storage[parent] = n
	}
	n[child] += value
}

// inc makes sure that even for unused features, we initialize them in the
// feature map. Furthermore, this acts as a helper that accepts booleans
// to increment by one, or integers to increment by that integer.
func inc(storage map[string]int, key string, i interface{}) {
	cv := storage[key]
	switch v := i.(type) {
	case bool:
		if v {
			cv++
		}
	case int:
		cv += v
	}
	storage[key] = cv
}

type location struct {
	Latitude  float64 `json:"lat"`
	Longitude float64 `json:"lon"`
}

type weightedLocation struct {
	location
	Weight int `json:"weight"`
}

func getReport(db *sql.DB) map[string]interface{} {
	geoip, err := geoip2.Open(geoIPPath)
	if err != nil {
		log.Println("opening geoip db", err)
		geoip = nil
	} else {
		defer geoip.Close()
	}

	nodes := 0
	countriesTotal := 0
	var versions []string
	var platforms []string
	var numFolders []int
	var numDevices []int
	var totFiles []int
	var maxFiles []int
	var totMiB []int64
	var maxMiB []int64
	var memoryUsage []int64
	var sha256Perf []float64
	var memorySize []int64
	var uptime []int
	var compilers []string
	var builders []string
	var distributions []string
	locations := make(map[location]int)
	countries := make(map[string]int)

	reports := make(map[string]int)
	totals := make(map[string]int)

	// category -> version -> feature -> count
	features := make(map[string]map[string]map[string]int)
	// category -> version -> feature -> group -> count
	featureGroups := make(map[string]map[string]map[string]map[string]int)
	for _, category := range featureOrder {
		features[category] = make(map[string]map[string]int)
		featureGroups[category] = make(map[string]map[string]map[string]int)
		for _, version := range knownVersions {
			features[category][version] = make(map[string]int)
			featureGroups[category][version] = make(map[string]map[string]int)
		}
	}

	// Initialize some features that hide behind if conditions, and might not
	// be initialized.
	add(featureGroups["Various"]["v2"], "Upgrades", "Pre-release", 0)
	add(featureGroups["Various"]["v2"], "Upgrades", "Automatic", 0)
	add(featureGroups["Various"]["v2"], "Upgrades", "Manual", 0)
	add(featureGroups["Various"]["v2"], "Upgrades", "Disabled", 0)
	add(featureGroups["Various"]["v3"], "Temporary Retention", "Disabled", 0)
	add(featureGroups["Various"]["v3"], "Temporary Retention", "Custom", 0)
	add(featureGroups["Various"]["v3"], "Temporary Retention", "Default", 0)
	add(featureGroups["Connection"]["v3"], "IP version", "IPv4", 0)
	add(featureGroups["Connection"]["v3"], "IP version", "IPv6", 0)
	add(featureGroups["Connection"]["v3"], "IP version", "Unknown", 0)

	var numCPU []int

	var rep contract.Report

	rows, err := db.Query(`SELECT Received, Report FROM ReportsJson WHERE Received > now() - '1 day'::INTERVAL`)
	if err != nil {
		log.Println("sql:", err)
		return nil
	}
	defer rows.Close()

	for rows.Next() {
		err := rows.Scan(&rep.Received, &rep)

		if err != nil {
			log.Println("sql:", err)
			return nil
		}

		if geoip != nil && rep.Address != "" {
			if addr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(rep.Address, "0")); err == nil {
				city, err := geoip.City(addr.IP)
				if err == nil {
					loc := location{
						Latitude:  city.Location.Latitude,
						Longitude: city.Location.Longitude,
					}
					locations[loc]++
					countries[city.Country.Names["en"]]++
					countriesTotal++
				}
			}
		}

		nodes++
		versions = append(versions, transformVersion(rep.Version))
		platforms = append(platforms, rep.Platform)

		if m := compilerRe.FindStringSubmatch(rep.LongVersion); len(m) == 3 {
			compilers = append(compilers, m[1])
			builders = append(builders, m[2])
		loop:
			for _, d := range knownDistributions {
				if d.matcher.MatchString(rep.LongVersion) {
					distributions = append(distributions, d.distribution)
					break loop
				}
			}
		}

		if rep.NumFolders > 0 {
			numFolders = append(numFolders, rep.NumFolders)
		}
		if rep.NumDevices > 0 {
			numDevices = append(numDevices, rep.NumDevices)
		}
		if rep.TotFiles > 0 {
			totFiles = append(totFiles, rep.TotFiles)
		}
		if rep.FolderMaxFiles > 0 {
			maxFiles = append(maxFiles, rep.FolderMaxFiles)
		}
		if rep.TotMiB > 0 {
			totMiB = append(totMiB, int64(rep.TotMiB)*(1<<20))
		}
		if rep.FolderMaxMiB > 0 {
			maxMiB = append(maxMiB, int64(rep.FolderMaxMiB)*(1<<20))
		}
		if rep.MemoryUsageMiB > 0 {
			memoryUsage = append(memoryUsage, int64(rep.MemoryUsageMiB)*(1<<20))
		}
		if rep.SHA256Perf > 0 {
			sha256Perf = append(sha256Perf, rep.SHA256Perf*(1<<20))
		}
		if rep.MemorySize > 0 {
			memorySize = append(memorySize, int64(rep.MemorySize)*(1<<20))
		}
		if rep.Uptime > 0 {
			uptime = append(uptime, rep.Uptime)
		}

		totals["Device"] += rep.NumDevices
		totals["Folder"] += rep.NumFolders

		if rep.URVersion >= 2 {
			reports["v2"]++
			numCPU = append(numCPU, rep.NumCPU)

			// Various
			inc(features["Various"]["v2"], "Rate limiting", rep.UsesRateLimit)

			if rep.UpgradeAllowedPre {
				add(featureGroups["Various"]["v2"], "Upgrades", "Pre-release", 1)
			} else if rep.UpgradeAllowedAuto {
				add(featureGroups["Various"]["v2"], "Upgrades", "Automatic", 1)
			} else if rep.UpgradeAllowedManual {
				add(featureGroups["Various"]["v2"], "Upgrades", "Manual", 1)
			} else {
				add(featureGroups["Various"]["v2"], "Upgrades", "Disabled", 1)
			}

			// Folders
			inc(features["Folder"]["v2"], "Automatic normalization", rep.FolderUses.AutoNormalize)
			inc(features["Folder"]["v2"], "Ignore deletes", rep.FolderUses.IgnoreDelete)
			inc(features["Folder"]["v2"], "Ignore permissions", rep.FolderUses.IgnorePerms)
			inc(features["Folder"]["v2"], "Mode, send only", rep.FolderUses.SendOnly)
			inc(features["Folder"]["v2"], "Mode, receive only", rep.FolderUses.ReceiveOnly)

			add(featureGroups["Folder"]["v2"], "Versioning", "Simple", rep.FolderUses.SimpleVersioning)
			add(featureGroups["Folder"]["v2"], "Versioning", "External", rep.FolderUses.ExternalVersioning)
			add(featureGroups["Folder"]["v2"], "Versioning", "Staggered", rep.FolderUses.StaggeredVersioning)
			add(featureGroups["Folder"]["v2"], "Versioning", "Trashcan", rep.FolderUses.TrashcanVersioning)
			add(featureGroups["Folder"]["v2"], "Versioning", "Disabled", rep.NumFolders-rep.FolderUses.SimpleVersioning-rep.FolderUses.ExternalVersioning-rep.FolderUses.StaggeredVersioning-rep.FolderUses.TrashcanVersioning)

			// Device
			inc(features["Device"]["v2"], "Custom certificate", rep.DeviceUses.CustomCertName)
			inc(features["Device"]["v2"], "Introducer", rep.DeviceUses.Introducer)

			add(featureGroups["Device"]["v2"], "Compress", "Always", rep.DeviceUses.CompressAlways)
			add(featureGroups["Device"]["v2"], "Compress", "Metadata", rep.DeviceUses.CompressMetadata)
			add(featureGroups["Device"]["v2"], "Compress", "Nothing", rep.DeviceUses.CompressNever)

			add(featureGroups["Device"]["v2"], "Addresses", "Dynamic", rep.DeviceUses.DynamicAddr)
			add(featureGroups["Device"]["v2"], "Addresses", "Static", rep.DeviceUses.StaticAddr)

			// Connections
			inc(features["Connection"]["v2"], "Relaying, enabled", rep.Relays.Enabled)
			inc(features["Connection"]["v2"], "Discovery, global enabled", rep.Announce.GlobalEnabled)
			inc(features["Connection"]["v2"], "Discovery, local enabled", rep.Announce.LocalEnabled)

			add(featureGroups["Connection"]["v2"], "Discovery", "Default servers (using DNS)", rep.Announce.DefaultServersDNS)
			add(featureGroups["Connection"]["v2"], "Discovery", "Default servers (using IP)", rep.Announce.DefaultServersIP)
			add(featureGroups["Connection"]["v2"], "Discovery", "Other servers", rep.Announce.DefaultServersIP)

			add(featureGroups["Connection"]["v2"], "Relaying", "Default relays", rep.Relays.DefaultServers)
			add(featureGroups["Connection"]["v2"], "Relaying", "Other relays", rep.Relays.OtherServers)
		}

		if rep.URVersion >= 3 {
			reports["v3"]++

			inc(features["Various"]["v3"], "Custom LAN classification", rep.AlwaysLocalNets)
			inc(features["Various"]["v3"], "Ignore caching", rep.CacheIgnoredFiles)
			inc(features["Various"]["v3"], "Overwrite device names", rep.OverwriteRemoteDeviceNames)
			inc(features["Various"]["v3"], "Download progress disabled", !rep.ProgressEmitterEnabled)
			inc(features["Various"]["v3"], "Custom default path", rep.CustomDefaultFolderPath)
			inc(features["Various"]["v3"], "Custom traffic class", rep.CustomTrafficClass)
			inc(features["Various"]["v3"], "Custom temporary index threshold", rep.CustomTempIndexMinBlocks)
			inc(features["Various"]["v3"], "Weak hash enabled", rep.WeakHashEnabled)
			inc(features["Various"]["v3"], "LAN rate limiting", rep.LimitBandwidthInLan)
			inc(features["Various"]["v3"], "Custom release server", rep.CustomReleaseURL)
			inc(features["Various"]["v3"], "Restart after suspend", rep.RestartOnWakeup)
			inc(features["Various"]["v3"], "Custom stun servers", rep.CustomStunServers)
			inc(features["Various"]["v3"], "Ignore patterns", rep.IgnoreStats.Lines > 0)

			if rep.NATType != "" {
				natType := rep.NATType
				natType = strings.ReplaceAll(natType, "unknown", "Unknown")
				natType = strings.ReplaceAll(natType, "Symetric", "Symmetric")
				add(featureGroups["Various"]["v3"], "NAT Type", natType, 1)
			}

			if rep.TemporariesDisabled {
				add(featureGroups["Various"]["v3"], "Temporary Retention", "Disabled", 1)
			} else if rep.TemporariesCustom {
				add(featureGroups["Various"]["v3"], "Temporary Retention", "Custom", 1)
			} else {
				add(featureGroups["Various"]["v3"], "Temporary Retention", "Default", 1)
			}

			inc(features["Folder"]["v3"], "Scan progress disabled", rep.FolderUsesV3.ScanProgressDisabled)
			inc(features["Folder"]["v3"], "Disable sharing of partial files", rep.FolderUsesV3.DisableTempIndexes)
			inc(features["Folder"]["v3"], "Disable sparse files", rep.FolderUsesV3.DisableSparseFiles)
			inc(features["Folder"]["v3"], "Weak hash, always", rep.FolderUsesV3.AlwaysWeakHash)
			inc(features["Folder"]["v3"], "Weak hash, custom threshold", rep.FolderUsesV3.CustomWeakHashThreshold)
			inc(features["Folder"]["v3"], "Filesystem watcher", rep.FolderUsesV3.FsWatcherEnabled)
			inc(features["Folder"]["v3"], "Case sensitive FS", rep.FolderUsesV3.CaseSensitiveFS)
			inc(features["Folder"]["v3"], "Mode, receive encrypted", rep.FolderUsesV3.ReceiveEncrypted)

			add(featureGroups["Folder"]["v3"], "Conflicts", "Disabled", rep.FolderUsesV3.ConflictsDisabled)
			add(featureGroups["Folder"]["v3"], "Conflicts", "Unlimited", rep.FolderUsesV3.ConflictsUnlimited)
			add(featureGroups["Folder"]["v3"], "Conflicts", "Limited", rep.FolderUsesV3.ConflictsOther)

			for key, value := range rep.FolderUsesV3.PullOrder {
				add(featureGroups["Folder"]["v3"], "Pull Order", prettyCase(key), value)
			}

			inc(features["Device"]["v3"], "Untrusted", rep.DeviceUsesV3.Untrusted)

			totals["GUI"] += rep.GUIStats.Enabled

			inc(features["GUI"]["v3"], "Auth Enabled", rep.GUIStats.UseAuth)
			inc(features["GUI"]["v3"], "TLS Enabled", rep.GUIStats.UseTLS)
			inc(features["GUI"]["v3"], "Insecure Admin Access", rep.GUIStats.InsecureAdminAccess)
			inc(features["GUI"]["v3"], "Skip Host check", rep.GUIStats.InsecureSkipHostCheck)
			inc(features["GUI"]["v3"], "Allow Frame loading", rep.GUIStats.InsecureAllowFrameLoading)

			add(featureGroups["GUI"]["v3"], "Listen address", "Local", rep.GUIStats.ListenLocal)
			add(featureGroups["GUI"]["v3"], "Listen address", "Unspecified", rep.GUIStats.ListenUnspecified)
			add(featureGroups["GUI"]["v3"], "Listen address", "Other", rep.GUIStats.Enabled-rep.GUIStats.ListenLocal-rep.GUIStats.ListenUnspecified)

			for theme, count := range rep.GUIStats.Theme {
				add(featureGroups["GUI"]["v3"], "Theme", prettyCase(theme), count)
			}

			for transport, count := range rep.TransportStats {
				add(featureGroups["Connection"]["v3"], "Transport", strings.Title(transport), count)
				if strings.HasSuffix(transport, "4") {
					add(featureGroups["Connection"]["v3"], "IP version", "IPv4", count)
				} else if strings.HasSuffix(transport, "6") {
					add(featureGroups["Connection"]["v3"], "IP version", "IPv6", count)
				} else {
					add(featureGroups["Connection"]["v3"], "IP version", "Unknown", count)
				}
			}
		}
	}

	var categories []category
	categories = append(categories, category{
		Values: statsForInts(totFiles),
		Descr:  "Files Managed per Device",
	})

	categories = append(categories, category{
		Values: statsForInts(maxFiles),
		Descr:  "Files in Largest Folder",
	})

	categories = append(categories, category{
		Values: statsForInt64s(totMiB),
		Descr:  "Data Managed per Device",
		Unit:   "B",
		Type:   NumberBinary,
	})

	categories = append(categories, category{
		Values: statsForInt64s(maxMiB),
		Descr:  "Data in Largest Folder",
		Unit:   "B",
		Type:   NumberBinary,
	})

	categories = append(categories, category{
		Values: statsForInts(numDevices),
		Descr:  "Number of Devices in Cluster",
	})

	categories = append(categories, category{
		Values: statsForInts(numFolders),
		Descr:  "Number of Folders Configured",
	})

	categories = append(categories, category{
		Values: statsForInt64s(memoryUsage),
		Descr:  "Memory Usage",
		Unit:   "B",
		Type:   NumberBinary,
	})

	categories = append(categories, category{
		Values: statsForInt64s(memorySize),
		Descr:  "System Memory",
		Unit:   "B",
		Type:   NumberBinary,
	})

	categories = append(categories, category{
		Values: statsForFloats(sha256Perf),
		Descr:  "SHA-256 Hashing Performance",
		Unit:   "B/s",
		Type:   NumberBinary,
	})

	categories = append(categories, category{
		Values: statsForInts(numCPU),
		Descr:  "Number of CPU cores",
	})

	categories = append(categories, category{
		Values: statsForInts(uptime),
		Descr:  "Uptime (v3)",
		Type:   NumberDuration,
	})

	reportFeatures := make(map[string][]feature)
	for featureType, versions := range features {
		var featureList []feature
		for version, featureMap := range versions {
			// We count totals of the given feature type, for example number of
			// folders or devices, if that doesn't exist, we work out percentage
			// against the total of the version reports. Things like "Various"
			// never have counts.
			total, ok := totals[featureType]
			if !ok {
				total = reports[version]
			}
			for key, count := range featureMap {
				featureList = append(featureList, feature{
					Key:     key,
					Version: version,
					Count:   count,
					Pct:     (100 * float64(count)) / float64(total),
				})
			}
		}
		sort.Sort(sort.Reverse(sortableFeatureList(featureList)))
		reportFeatures[featureType] = featureList
	}

	reportFeatureGroups := make(map[string][]featureGroup)
	for featureType, versions := range featureGroups {
		var featureList []featureGroup
		for version, featureMap := range versions {
			for key, counts := range featureMap {
				featureList = append(featureList, featureGroup{
					Key:     key,
					Version: version,
					Counts:  counts,
				})
			}
		}
		reportFeatureGroups[featureType] = featureList
	}

	var countryList []feature
	for country, count := range countries {
		countryList = append(countryList, feature{
			Key:   country,
			Count: count,
			Pct:   (100 * float64(count)) / float64(countriesTotal),
		})
		sort.Sort(sort.Reverse(sortableFeatureList(countryList)))
	}

	r := make(map[string]interface{})
	r["features"] = reportFeatures
	r["featureGroups"] = reportFeatureGroups
	r["nodes"] = nodes
	r["versionNodes"] = reports
	r["categories"] = categories
	r["versions"] = group(byVersion, analyticsFor(versions, 2000), 10)
	r["versionPenetrations"] = penetrationLevels(analyticsFor(versions, 2000), []float64{50, 75, 90, 95})
	r["platforms"] = group(byPlatform, analyticsFor(platforms, 2000), 10)
	r["compilers"] = group(byCompiler, analyticsFor(compilers, 2000), 5)
	r["builders"] = analyticsFor(builders, 12)
	r["distributions"] = analyticsFor(distributions, len(knownDistributions))
	r["featureOrder"] = featureOrder
	r["locations"] = locations
	r["contries"] = countryList

	return r
}

var (
	plusRe  = regexp.MustCompile(`(\+.*|\.dev\..*)$`)
	plusStr = "(+dev)"
)

// transformVersion returns a version number formatted correctly, with all
// development versions aggregated into one.
func transformVersion(v string) string {
	if v == "unknown-dev" {
		return v
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	v = plusRe.ReplaceAllString(v, " "+plusStr)

	return v
}

type summary struct {
	versions map[string]int   // version string to count index
	max      map[string]int   // version string to max users per day
	rows     map[string][]int // date to list of counts
}

func newSummary() summary {
	return summary{
		versions: make(map[string]int),
		max:      make(map[string]int),
		rows:     make(map[string][]int),
	}
}

func (s *summary) setCount(date, version string, count int) {
	idx, ok := s.versions[version]
	if !ok {
		idx = len(s.versions)
		s.versions[version] = idx
	}

	if s.max[version] < count {
		s.max[version] = count
	}

	row := s.rows[date]
	if len(row) <= idx {
		old := row
		row = make([]int, idx+1)
		copy(row, old)
		s.rows[date] = row
	}

	row[idx] = count
}

func (s *summary) MarshalJSON() ([]byte, error) {
	var versions []string
	for v := range s.versions {
		versions = append(versions, v)
	}
	sort.Slice(versions, func(a, b int) bool {
		return upgrade.CompareVersions(versions[a], versions[b]) < 0
	})

	var filtered []string
	for _, v := range versions {
		if s.max[v] > 50 {
			filtered = append(filtered, v)
		}
	}
	versions = filtered

	headerRow := []interface{}{"Day"}
	for _, v := range versions {
		headerRow = append(headerRow, v)
	}

	var table [][]interface{}
	table = append(table, headerRow)

	var dates []string
	for k := range s.rows {
		dates = append(dates, k)
	}
	sort.Strings(dates)

	for _, date := range dates {
		row := []interface{}{date}
		for _, ver := range versions {
			idx := s.versions[ver]
			if len(s.rows[date]) > idx && s.rows[date][idx] > 0 {
				row = append(row, s.rows[date][idx])
			} else {
				row = append(row, nil)
			}
		}
		table = append(table, row)
	}

	return json.Marshal(table)
}

// filter removes versions that never reach the specified min count.
func (s *summary) filter(min int) {
	// We cheat and just remove the versions from the "index" and leave the
	// data points alone. The version index is used to build the table when
	// we do the serialization, so at that point the data points are
	// filtered out as well.
	for ver := range s.versions {
		if s.max[ver] < min {
			delete(s.versions, ver)
			delete(s.max, ver)
		}
	}
}

func getSummary(db *sql.DB, min int) (summary, error) {
	s := newSummary()

	rows, err := db.Query(`SELECT Day, Version, Count FROM VersionSummary WHERE Day > now() - '2 year'::INTERVAL;`)
	if err != nil {
		return summary{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var day time.Time
		var ver string
		var num int
		err := rows.Scan(&day, &ver, &num)
		if err != nil {
			return summary{}, err
		}

		if ver == "v0.0" {
			// ?
			continue
		}

		// SUPER UGLY HACK to avoid having to do sorting properly
		if len(ver) == 4 && strings.HasPrefix(ver, "v0.") { // v0.x
			ver = ver[:3] + "0" + ver[3:] // now v0.0x
		}

		s.setCount(day.Format("2006-01-02"), ver, num)
	}

	s.filter(min)
	return s, nil
}

func getMovement(db *sql.DB) ([][]interface{}, error) {
	rows, err := db.Query(`SELECT Day, Added, Removed, Bounced FROM UserMovement WHERE Day > now() - '2 year'::INTERVAL ORDER BY Day`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := [][]interface{}{
		{"Day", "Joined", "Left", "Bounced"},
	}

	for rows.Next() {
		var day time.Time
		var added, removed, bounced int
		err := rows.Scan(&day, &added, &removed, &bounced)
		if err != nil {
			return nil, err
		}

		row := []interface{}{day.Format("2006-01-02"), added, -removed, bounced}
		if removed == 0 {
			row[2] = nil
		}
		if bounced == 0 {
			row[3] = nil
		}

		res = append(res, row)
	}

	return res, nil
}

func getPerformance(db *sql.DB) ([][]interface{}, error) {
	rows, err := db.Query(`SELECT Day, TotFiles, TotMiB, SHA256Perf, MemorySize, MemoryUsageMiB FROM Performance WHERE Day > '2014-06-20'::TIMESTAMP ORDER BY Day`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := [][]interface{}{
		{"Day", "TotFiles", "TotMiB", "SHA256Perf", "MemorySize", "MemoryUsageMiB"},
	}

	for rows.Next() {
		var day time.Time
		var sha256Perf float64
		var totFiles, totMiB, memorySize, memoryUsage int
		err := rows.Scan(&day, &totFiles, &totMiB, &sha256Perf, &memorySize, &memoryUsage)
		if err != nil {
			return nil, err
		}

		row := []interface{}{day.Format("2006-01-02"), totFiles, totMiB, float64(int(sha256Perf*10)) / 10, memorySize, memoryUsage}
		res = append(res, row)
	}

	return res, nil
}

func getBlockStats(db *sql.DB) ([][]interface{}, error) {
	rows, err := db.Query(`SELECT Day, Reports, Pulled, Renamed, Reused, CopyOrigin, CopyOriginShifted, CopyElsewhere FROM BlockStats WHERE Day > '2017-10-23'::TIMESTAMP ORDER BY Day`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := [][]interface{}{
		{"Day", "Number of Reports", "Transferred (GiB)", "Saved by renaming files (GiB)", "Saved by resuming transfer (GiB)", "Saved by reusing data from old file (GiB)", "Saved by reusing shifted data from old file (GiB)", "Saved by reusing data from other files (GiB)"},
	}
	blocksToGb := float64(8 * 1024)
	for rows.Next() {
		var day time.Time
		var reports, pulled, renamed, reused, copyOrigin, copyOriginShifted, copyElsewhere float64
		err := rows.Scan(&day, &reports, &pulled, &renamed, &reused, &copyOrigin, &copyOriginShifted, &copyElsewhere)
		if err != nil {
			return nil, err
		}
		row := []interface{}{
			day.Format("2006-01-02"),
			reports,
			pulled / blocksToGb,
			renamed / blocksToGb,
			reused / blocksToGb,
			copyOrigin / blocksToGb,
			copyOriginShifted / blocksToGb,
			copyElsewhere / blocksToGb,
		}
		res = append(res, row)
	}

	return res, nil
}

type sortableFeatureList []feature

func (l sortableFeatureList) Len() int {
	return len(l)
}
func (l sortableFeatureList) Swap(a, b int) {
	l[a], l[b] = l[b], l[a]
}
func (l sortableFeatureList) Less(a, b int) bool {
	if l[a].Pct != l[b].Pct {
		return l[a].Pct < l[b].Pct
	}
	return l[a].Key > l[b].Key
}

func prettyCase(input string) string {
	output := ""
	for i, runeValue := range input {
		if i == 0 {
			runeValue = unicode.ToUpper(runeValue)
		} else if unicode.IsUpper(runeValue) {
			output += " "
		}
		output += string(runeValue)
	}
	return output
}
