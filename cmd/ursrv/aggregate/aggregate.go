// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package aggregate

import (
	"database/sql"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/lib/pq"
	"github.com/oschwald/geoip2-golang"
	"github.com/syncthing/syncthing/cmd/ursrv/blob"
	"github.com/syncthing/syncthing/lib/ur"
	"github.com/syncthing/syncthing/lib/ur/contract"
)

const countryTag = "countries"

var (
	compilerRe         = regexp.MustCompile(`\(([A-Za-z0-9()., -]+) \w+-\w+(?:| android| default)\) ([\w@.-]+)`)
	knownDistributions = []distributionMatch{
		// Maps well known builders to the official distribution method that
		// they represent

		{regexp.MustCompile(`\steamcity@build\.syncthing\.net`), "GitHub"},
		{regexp.MustCompile(`\sjenkins@build\.syncthing\.net`), "GitHub"},
		{regexp.MustCompile(`\sbuilder@github\.syncthing\.net`), "GitHub"},

		{regexp.MustCompile(`\sdeb@build\.syncthing\.net`), "APT"},
		{regexp.MustCompile(`\sdebian@github\.syncthing\.net`), "APT"},

		{regexp.MustCompile(`\sdocker@syncthing\.net`), "Docker Hub"},
		{regexp.MustCompile(`\sdocker@build.syncthing\.net`), "Docker Hub"},
		{regexp.MustCompile(`\sdocker@github.syncthing\.net`), "Docker Hub"},

		{regexp.MustCompile(`\sandroid-builder@github\.syncthing\.net`), "Google Play"},
		{regexp.MustCompile(`\sandroid-.*teamcity@build\.syncthing\.net`), "Google Play"},
		{regexp.MustCompile(`\sandroid-.*vagrant@basebox-stretch64`), "F-Droid"},
		{regexp.MustCompile(`\svagrant@bullseye`), "F-Droid"},
		{regexp.MustCompile(`\sbuilduser@(archlinux|svetlemodry)`), "Arch (3rd party)"},
		{regexp.MustCompile(`@debian`), "Debian (3rd party)"},
		{regexp.MustCompile(`@fedora`), "Fedora (3rd party)"},
		{regexp.MustCompile(`\sbrew@`), "Homebrew (3rd party)"},
		{regexp.MustCompile(`\sroot@buildkitsandbox`), "LinuxServer.io (3rd party)"},
		{regexp.MustCompile(`.`), "Others"},
	}
)

type distributionMatch struct {
	matcher      *regexp.Regexp
	distribution string
}

type CLI struct {
	DBConn    string `env:"UR_DB_URL" default:"postgres://user:password@localhost/ur?sslmode=disable"`
	GeoIPPath string `env:"UR_GEOIP" default:"GeoLite2-City.mmdb"`
	Migrate   bool   `env:"UR_MIGRATE"`                           // Migration support (to be removed post-migration).
	From      string `env:"UR_MIGRATE_FROM" default:"2014-06-11"` // Migration support (to be removed post-migration).
	To        string `env:"UR_MIGRATE_TO"`                        // Migration support (to be removed post-migration).
}

func (cli *CLI) Run(s3Config blob.S3Config) error {
	log.SetFlags(log.Ltime | log.Ldate)
	log.SetOutput(os.Stdout)

	// Deprecated (remove post-migration).
	db, err := sql.Open("postgres", cli.DBConn)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}

	// Initialize the storage and store.
	store := blob.NewUrsrvStore(blob.NewBlobStorage(s3Config))

	// Migration support (to be removed post-migration).
	if cli.Migrate {
		log.Println("Starting migration")
		if err := runMigration(db, store, cli.GeoIPPath, cli.From, cli.To); err != nil {
			log.Println("Migration failed:", err)
			return err
		}
		log.Println("Migration complete")

		// Skip the regular aggregation.
		return nil
	}

	for {
		runAggregation(store, cli.GeoIPPath, time.Now().UTC().AddDate(0, 0, -1))

		// Sleep until one minute past next midnight
		sleepUntilNext(24*time.Hour, 1*time.Minute)
	}
}

func sleepUntilNext(intv, margin time.Duration) {
	now := time.Now().UTC()
	next := now.Truncate(intv).Add(intv).Add(margin)

	log.Println("Sleeping until", next)
	time.Sleep(next.Sub(now))
}

func runAggregation(store *blob.UrsrvStore, geoIPPath string, aggregateDate time.Time) {
	// Prepare the geo location database.
	geoip, err := geoip2.Open(geoIPPath)
	if err != nil {
		log.Println("opening geoip db", err)
		geoip = nil
	} else {
		defer geoip.Close()
	}

	// Retreive the usage reports for the given date.
	reps, err := store.ListUsageReportsForDate(aggregateDate)
	if err != nil {
		log.Printf("error while listing reports for the previous day, %v", err)
		return
	}

	if len(reps) == 0 {
		log.Printf("no reports to aggregate")
		return
	}

	// Aggregate the obtained usage reports to a single daily summary.
	ar := aggregateUserReports(geoip, aggregateDate, reps)

	// Store the aggregated report.
	err = store.PutAggregatedReport(ar)
	if err != nil {
		log.Printf("error while storing the aggregated report, skipping cleanup %v", err)
		return
	}
}

// Migration support (to be removed post-migration).
func runMigration(db *sql.DB, store *blob.UrsrvStore, geoIPPath, from, to string) error {
	geoip, err := geoip2.Open(geoIPPath)
	if err != nil {
		log.Println("opening geoip db", err)
		geoip = nil
	} else {
		defer geoip.Close()
	}

	var toDate, fromDate time.Time

	// Default is v1.0.0 release date.
	fromDate, err = time.Parse(time.DateOnly, from)
	if err != nil {
		return err
	}

	if to == "" {
		// No end-date was set, default is yesterday.
		to = time.Now().UTC().AddDate(0, 0, -1).Format(time.DateOnly)
	}
	toDate, err = time.Parse(time.DateOnly, to)
	if err != nil {
		return err
	}

	// Aggregate the reports of all the days prior to today, as all the usage
	// reports for those days should be put in the db already.
	for fromDate.Before(toDate) {
		log.Println("migrating", fromDate.Format(time.DateOnly))

		// Obtain the reports for the given date from the db.
		reports, err := reportsFromDB(db, fromDate)
		if err != nil {
			return fmt.Errorf("error while retrieving reports for date %v: %w", fromDate, err)
		}
		if len(reports) == 0 {
			// No valid reports were obtained for this date.
			log.Println("no reports for", fromDate.Format(time.DateOnly))
			fromDate = fromDate.AddDate(0, 0, 1)
			continue
		}
		log.Println("got", len(reports), "reports for", fromDate.Format(time.DateOnly))

		// Aggregate the reports.
		aggregated := aggregateUserReports(geoip, fromDate, reports)

		// Store the aggregated report in the new storage location.
		err = store.PutAggregatedReport(aggregated)
		if err != nil {
			log.Println("migrate aggregated report failed", fromDate, err)
		}

		// Continue to the next day.
		fromDate = fromDate.AddDate(0, 0, 1)
	}

	return nil
}

// Migration support (to be removed post-migration).
func reportsFromDB(db *sql.DB, date time.Time) ([]contract.Report, error) {
	var reports []contract.Report

	// Select all the rows where the received day is equal to the given timestamp's day.
	date = date.UTC()
	nextDay := date.AddDate(0, 0, 1)
	rows, err := db.Query(`
	SELECT Received, Report FROM ReportsJson WHERE Received >= $1 AND Received < $2`, date, nextDay)
	if err != nil {
		return reports, err
	}

	// Parse the row-data and append it to a slice.
	var rep contract.Report
	for rows.Next() {
		err := rows.Scan(&rep.Received, &rep)
		if err != nil {
			log.Println("sql:", err)
			continue
		}
		if err = rep.Validate(); err != nil {
			continue
		}
		reports = append(reports, rep)
	}
	if rows.Err() != nil {
		return reports, rows.Err()
	}

	return reports, nil
}

// New aggregation...
func aggregateUserReports(geoip *geoip2.Reader, date time.Time, reps []contract.Report) *ur.Aggregation {
	h := newAggregateHelper()

	var wg sync.WaitGroup
	max := make(chan struct{}, 5)
	for _, rep := range reps {
		wg.Add(1)
		max <- struct{}{}
		go func(rep contract.Report) {
			defer func() {
				wg.Done()
				<-max
			}()
			// Prepare the report
			rep.Version = transformVersion(rep.Version)

			// Handle distrubition, compiler and builder info
			h.handleMiscStats(rep.LongVersion)

			// Handle Geo-locations
			h.parseGeoLocation(geoip, rep.Address)

			// Aggregate the rest of the report
			h.aggregateReportData(rep, rep.URVersion, "")

			// Increase the report counter(s)
			h.incReportCounter(rep.URVersion)
		}(rep)
	}
	wg.Wait()

	// Summarise the data to one report
	return h.calculateSummary(date)
}

type SyncMap struct {
}

// CounterHelper is a helper object. It collects the values from the usage
// reports and stores it in a structural manner, making it easy to calculate the
// average, median, sum, min, max, percentile values.
type AggregateHelper struct {
	mutex      sync.Mutex
	floats     map[string][]float64          // fieldName -> data for float statistics
	ints       map[string][]int64            // fieldName -> data for int statistics
	mapStrings map[string]map[string]int64   // fieldName -> mapped value -> data for histogram
	mapInts    map[string]map[string][]int64 // fieldName -> mapped value -> data for int statistics

	totalReports   int // All handled reports counter
	totalV2Reports int
	totalV3Reports int
}

func newAggregateHelper() *AggregateHelper {
	return &AggregateHelper{
		floats:     make(map[string][]float64),
		ints:       make(map[string][]int64),
		mapStrings: make(map[string]map[string]int64),
		mapInts:    make(map[string]map[string][]int64),
	}
}

func (h *AggregateHelper) addFloat(label string, value float64) {
	if value == 0.0 || math.IsNaN(value) {
		return
	}

	h.mutex.Lock()
	defer h.mutex.Unlock()

	res := h.floats[label]
	if res == nil {
		res = make([]float64, 0)
	}
	res = append(res, value)
	h.floats[label] = res
}

func (h *AggregateHelper) addInt(label string, value int) {
	if value == 0 {
		return
	}

	h.mutex.Lock()
	defer h.mutex.Unlock()

	res := h.ints[label]
	if res == nil {
		res = make([]int64, 0)
	}
	res = append(res, int64(value))
	h.ints[label] = res
}

func (h *AggregateHelper) addMapStrings(label, key string, value int64) {
	if value == 0 || key == "" {
		return
	}

	h.mutex.Lock()
	defer h.mutex.Unlock()

	res := h.mapStrings[label]
	if res == nil {
		res = make(map[string]int64)
	}
	res[key] += value
	h.mapStrings[label] = res
}

func (h *AggregateHelper) addIntArr(label string, value []int) {
	for _, v := range value {
		h.addInt(label, v)
	}
}

func (h *AggregateHelper) addMapInts(label string, value map[string]int) {
	if len(value) == 0 {
		return
	}

	h.mutex.Lock()
	defer h.mutex.Unlock()

	res := h.mapInts[label]
	if res == nil {
		res = make(map[string][]int64)
	}
	for k, v := range value {
		if k == "" || v == 0 {
			continue
		}

		res[k] = append(res[k], int64(v))
	}
	h.mapInts[label] = res
}

func (h *AggregateHelper) incReportCounter(version int) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	h.totalReports++
	if version == 2 {
		h.totalV2Reports++
	} else if version == 3 {
		h.totalV3Reports++
	}
}

func (h *AggregateHelper) calculateSummary(date time.Time) *ur.Aggregation {
	// Summarises the data to a single report. This includes calculating the
	// required values, like average, median, sum, etc.
	ap := &ur.Aggregation{
		Date:       date.UTC().Unix(),
		Count:      int64(h.totalReports),   // all reports
		CountV2:    int64(h.totalV2Reports), // v2 repots
		CountV3:    int64(h.totalV3Reports), // v3 reports
		Statistics: make(map[string]ur.Statistic),
	}

	res := make(map[string]ur.Statistic)
	var wg sync.WaitGroup
	var mutex sync.Mutex
	wg.Add(1)
	go func() {
		defer wg.Done()
		mutex.Lock()
		defer mutex.Unlock()

		for label, v := range h.floats {
			res[label] =
				ur.Statistic{
					Key:       label,
					Statistic: &ur.Statistic_Float{Float: floatStats(v)},
				}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for label, v := range h.ints {
			mutex.Lock()
			res[label] =
				ur.Statistic{
					Key:       label,
					Statistic: &ur.Statistic_Integer{Integer: intStats(v)},
				}
			mutex.Unlock()
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for label, child := range h.mapInts {
			mapStats := &ur.MapIntegerStatistic{Map: make(map[string]ur.IntegerStatistic)}
			for k, v := range child {
				mapStats.Map[k] = *intStats(v)
			}

			mutex.Lock()
			res[label] = ur.Statistic{
				Key:       label,
				Statistic: &ur.Statistic_MappedInteger{MappedInteger: mapStats},
			}
			mutex.Unlock()
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for label, child := range h.mapStrings {
			stats := &ur.MapHistogram{Map: make(map[string]int64)}
			for k, v := range child {
				stats.Map[k] = v
			}
			mutex.Lock()
			res[label] = ur.Statistic{
				Key:       label,
				Statistic: &ur.Statistic_Histogram{Histogram: stats},
			}
			mutex.Unlock()
		}
	}()
	wg.Wait()

	ap.Statistics = res

	return ap
}

func (h *AggregateHelper) handleMiscStats(longVersion string) {
	if m := compilerRe.FindStringSubmatch(longVersion); len(m) == 3 {
		h.addMapStrings("compiler", m[1], 1)
		h.addMapStrings("builder", m[2], 1)
	loop:
		for _, d := range knownDistributions {
			if d.matcher.MatchString(longVersion) {
				h.addMapStrings("distribution", d.distribution, 1)
				break loop
			}
		}
	}
}

func (h *AggregateHelper) parseGeoLocation(geoip *geoip2.Reader, addr string) {
	if addr == "" || geoip == nil {
		return
	}

	if addr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(addr, "0")); err == nil {
		city, err := geoip.City(addr.IP)
		if err == nil {
			country := city.Country.Names["en"]
			if country == "" {
				country = "Unkown"
			}
			h.addMapStrings(countryTag, country, 1)
		}
	}
}

func (h *AggregateHelper) aggregateReportData(v any, urVersion int, parent string) {
	s := reflect.ValueOf(v)

	if s.Kind() != reflect.Struct {
		// Sanity check, this will otherwise cause a panic.
		return
	}

	for i := 0; i < s.NumField(); i++ {
		f := s.Field(i)
		tag := s.Type().Field(i).Tag
		h.handleValue(f, tag, urVersion, parent)
	}
}

func (h *AggregateHelper) handleValue(value reflect.Value, tag reflect.StructTag, urVersion int, parent string) {
	parsedTag := parseTag(tag.Get("json"), parent)
	sinceInt, err := strconv.Atoi(tag.Get("since"))
	if err != nil || urVersion < sinceInt {
		return
	}
	// Some fields are either handled separately or not relevant, we should skip
	// those when iterating over the report fields.
	if shouldSkipField(parsedTag) {
		return
	}
	if value.Kind() == reflect.Struct {
		// Walk through the child-struct, append this field's label as
		// parent-label.
		h.aggregateReportData(value.Interface(), urVersion, parsedTag)

		// This field itself has nothing relevant to analyse.
		return
	}

	// Handle the content of the field depending on the type.
	switch t := value.Interface().(type) {
	case string:
		if t == "" {
			return
		}
		h.addMapStrings(parsedTag, t, 1)
	case int:
		if t == 0 {
			return
		}
		h.addInt(parsedTag, t)
	case float64:
		if t == 0 || math.IsNaN(t) {
			return
		}
		h.addFloat(parsedTag, t)
	case bool:
		if !t {
			return
		}
		h.addInt(parsedTag, 1)
	case []int:
		if len(t) == 0 {
			return
		}
		h.addIntArr(parsedTag, t)
	case map[string]int:
		if len(t) == 0 {
			return
		}
		h.addMapInts(parsedTag, t)
	default:
		return
	}
}

func parseTag(tag, parent string) string {
	// If a parent tag is present it gets appended before the current tag,
	// separated with a dot. This prevents potential non-unique field-names
	// conflicts as we iterate through sub-structs as well. E.g.
	// "GUIStats.Enabled" and "Relays.Enabled" would otherwise bothbecome
	// "Enabled".
	split := strings.Split(tag, ",")
	if len(split) > 0 {
		tag = split[0]
	}
	if parent != "" {
		tag = fmt.Sprintf("%s.%s", parent, tag)
	}

	return tag
}

func shouldSkipField(tag string) bool {
	switch tag {
	case "-":
		// Irrelevant.
		return true
	case "uniqueID":
		// Irrelevant.
		return true
	case "date":
		// Irrelevant.
		return true
	case "urVersion":
		// Irrelevant. It is extracted from the report and used where required.
		return true
	case "address":
		// Handled separately, being mapped to the geo-location and country.
		return true
	case "longVersion":
		// Handled separately, being mapped to known distributions, compilers and
		// builders.
		return true
	default:
		return false
	}
}
