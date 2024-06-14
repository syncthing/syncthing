// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public License,
// v. 2.0. If a copy of the MPL was not distributed with this file, You can
// obtain one at https://mozilla.org/MPL/2.0/.

package aggregate

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	_ "github.com/lib/pq"
	"github.com/syncthing/syncthing/cmd/ursrv/blob"
	"github.com/syncthing/syncthing/lib/geoip"
	"github.com/syncthing/syncthing/lib/ur"
	"github.com/syncthing/syncthing/lib/ur/contract"
)

const (
	countryTag  = "countries"
	locationTag = "locations"
)

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
	DBConn          string `env:"UR_DB_URL" default:"postgres://user:password@localhost/ur?sslmode=disable"`
	GeoIPLicenseKey string `env:"UR_GEOIP_LICENSE_KEY"`
	GeoIPAccountID  int    `env:"UR_GEOIP_ACCOUNT_ID"`
	Migrate         bool   `env:"UR_MIGRATE"`                           // Migration support (to be removed post-migration).
	From            string `env:"UR_MIGRATE_FROM" default:"2014-06-11"` // Migration support (to be removed post-migration).
	To              string `env:"UR_MIGRATE_TO"`                        // Migration support (to be removed post-migration).
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

	// Initialize the geoip provider.
	geoip, err := geoip.NewGeoLite2CityProvider(context.Background(), cli.GeoIPAccountID, cli.GeoIPLicenseKey, os.TempDir())
	if err != nil {
		log.Fatalln("geoip:", err)
	}
	go geoip.Serve(context.TODO())

	// Migration support (to be removed post-migration).
	if cli.Migrate {
		log.Println("Starting migration")
		if err := runMigration(db, store, geoip, cli.From, cli.To); err != nil {
			log.Println("Migration failed:", err)
			return err
		}
		log.Println("Migration complete")

		// Skip the regular aggregation.
		return nil
	}

	for {
		if cli.From != "" {
			var toDate, fromDate time.Time

			// Default is v1.0.0 release date.
			fromDate, err = time.Parse(time.DateOnly, cli.From)
			if err != nil {
				return err
			}
			to := cli.To
			if to == "" {
				// No end-date was set, default is yesterday.
				to = time.Now().UTC().AddDate(0, 0, -1).Format(time.DateOnly)
			}
			toDate, err = time.Parse(time.DateOnly, to)
			if err != nil {
				return err
			}

			// Aggregate the reports of all the days prior to today, as all the
			// usage reports for those days should be put in the db already.
			for fromDate.Before(toDate) {
				runAggregation(store, geoip, fromDate)

				// Continue to the next day.
				fromDate = fromDate.AddDate(0, 0, 1)
			}
		}
		runAggregation(store, geoip, time.Now().UTC().AddDate(0, 0, -1))

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

func runAggregation(store *blob.UrsrvStore, geoip *geoip.Provider, aggregateDate time.Time) {
	log.Println("Aggregating", aggregateDate)

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
func runMigration(db *sql.DB, store *blob.UrsrvStore, geoip *geoip.Provider, from, to string) error {
	var toDate, fromDate time.Time

	// Default is v1.0.0 release date.
	fromDate, err := time.Parse(time.DateOnly, from)
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

	// Select all the rows where the received day is equal to the given
	// timestamp's day.
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
func aggregateUserReports(geoip *geoip.Provider, date time.Time, reps []contract.Report) *ur.Aggregation {
	h := newAggregateHandler()

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
		// Handled separately, being mapped to known distributions, compilers
		// and builders.
		return true
	default:
		return false
	}
}
