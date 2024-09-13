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
	"os"
	"time"

	_ "github.com/lib/pq"
)

type CLI struct {
	DBConn string `env:"UR_DB_URL" default:"postgres://user:password@localhost/ur?sslmode=disable"`
}

func (cli *CLI) Run() error {
	log.SetFlags(log.Ltime | log.Ldate)
	log.SetOutput(os.Stdout)

	db, err := sql.Open("postgres", cli.DBConn)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	err = setupDB(db)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}

	for {
		runAggregation(db)
		// Sleep until one minute past next midnight
		sleepUntilNext(24*time.Hour, 1*time.Minute)
	}
}

func runAggregation(db *sql.DB) {
	since := maxIndexedDay(db, "VersionSummary")
	log.Println("Aggregating VersionSummary data since", since)
	rows, err := aggregateVersionSummary(db, since.Add(24*time.Hour))
	if err != nil {
		log.Println("aggregate:", err)
	}
	log.Println("Inserted", rows, "rows")

	since = maxIndexedDay(db, "Performance")
	log.Println("Aggregating Performance data since", since)
	rows, err = aggregatePerformance(db, since.Add(24*time.Hour))
	if err != nil {
		log.Println("aggregate:", err)
	}
	log.Println("Inserted", rows, "rows")

	since = maxIndexedDay(db, "BlockStats")
	log.Println("Aggregating BlockStats data since", since)
	rows, err = aggregateBlockStats(db, since.Add(24*time.Hour))
	if err != nil {
		log.Println("aggregate:", err)
	}
	log.Println("Inserted", rows, "rows")
}

func sleepUntilNext(intv, margin time.Duration) {
	now := time.Now().UTC()
	next := now.Truncate(intv).Add(intv).Add(margin)
	log.Println("Sleeping until", next)
	time.Sleep(next.Sub(now))
}

func setupDB(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS VersionSummary (
		Day TIMESTAMP NOT NULL,
		Version VARCHAR(8) NOT NULL,
		Count INTEGER NOT NULL
	)`)
	if err != nil {
		return err
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS Performance (
		Day TIMESTAMP NOT NULL,
		TotFiles INTEGER NOT NULL,
		TotMiB INTEGER NOT NULL,
		SHA256Perf DOUBLE PRECISION NOT NULL,
		MemorySize INTEGER NOT NULL,
		MemoryUsageMiB INTEGER NOT NULL
	)`)
	if err != nil {
		return err
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS BlockStats (
		Day TIMESTAMP NOT NULL,
		Reports INTEGER NOT NULL,
		Total BIGINT NOT NULL,
		Renamed BIGINT NOT NULL,
		Reused BIGINT NOT NULL,
		Pulled BIGINT NOT NULL,
		CopyOrigin BIGINT NOT NULL,
		CopyOriginShifted BIGINT NOT NULL,
		CopyElsewhere BIGINT NOT NULL
	)`)
	if err != nil {
		return err
	}

	var t string

	row := db.QueryRow(`SELECT 'UniqueDayVersionIndex'::regclass`)
	if err := row.Scan(&t); err != nil {
		_, _ = db.Exec(`CREATE UNIQUE INDEX UniqueDayVersionIndex ON VersionSummary (Day, Version)`)
	}

	row = db.QueryRow(`SELECT 'VersionDayIndex'::regclass`)
	if err := row.Scan(&t); err != nil {
		_, _ = db.Exec(`CREATE INDEX VersionDayIndex ON VersionSummary (Day)`)
	}

	row = db.QueryRow(`SELECT 'PerformanceDayIndex'::regclass`)
	if err := row.Scan(&t); err != nil {
		_, _ = db.Exec(`CREATE INDEX PerformanceDayIndex ON Performance (Day)`)
	}

	row = db.QueryRow(`SELECT 'BlockStatsDayIndex'::regclass`)
	if err := row.Scan(&t); err != nil {
		_, _ = db.Exec(`CREATE INDEX BlockStatsDayIndex ON BlockStats (Day)`)
	}

	return nil
}

func maxIndexedDay(db *sql.DB, table string) time.Time {
	var t time.Time
	row := db.QueryRow("SELECT MAX(DATE_TRUNC('day', Day)) FROM " + table)
	err := row.Scan(&t)
	if err != nil {
		return time.Time{}
	}
	return t
}

func aggregateVersionSummary(db *sql.DB, since time.Time) (int64, error) {
	res, err := db.Exec(`INSERT INTO VersionSummary (
	SELECT
		DATE_TRUNC('day', Received) AS Day,
		SUBSTRING(Report->>'version' FROM '^v\d.\d+') AS Ver,
		COUNT(*) AS Count
		FROM ReportsJson
		WHERE
			Received > $1
			AND Received < DATE_TRUNC('day', NOW())
			AND Report->>'version' like 'v_.%'
		GROUP BY Day, Ver
		);
	`, since)
	if err != nil {
		return 0, err
	}

	return res.RowsAffected()
}

func aggregatePerformance(db *sql.DB, since time.Time) (int64, error) {
	res, err := db.Exec(`INSERT INTO Performance (
	SELECT
		DATE_TRUNC('day', Received) AS Day,
		AVG((Report->>'totFiles')::numeric) As TotFiles,
		AVG((Report->>'totMiB')::numeric) As TotMiB,
		AVG((Report->>'sha256Perf')::numeric) As SHA256Perf,
		AVG((Report->>'memorySize')::numeric) As MemorySize,
		AVG((Report->>'memoryUsageMiB')::numeric) As MemoryUsageMiB
		FROM ReportsJson
		WHERE
			Received > $1
			AND Received < DATE_TRUNC('day', NOW())
			AND Report->>'version' like 'v_.%'
			/* Some custom implementation reported bytes when we expect megabytes, cap at petabyte */
			AND (Report->>'memorySize')::numeric < 1073741824
		GROUP BY Day
		);
	`, since)
	if err != nil {
		return 0, err
	}

	return res.RowsAffected()
}

func aggregateBlockStats(db *sql.DB, since time.Time) (int64, error) {
	// Filter out anything prior 0.14.41 as that has sum aggregations which
	// made no sense.
	res, err := db.Exec(`INSERT INTO BlockStats (
	SELECT
		DATE_TRUNC('day', Received) AS Day,
		COUNT(1) As Reports,
		SUM((Report->'blockStats'->>'total')::numeric)::bigint AS Total,
		SUM((Report->'blockStats'->>'renamed')::numeric)::bigint AS Renamed,
		SUM((Report->'blockStats'->>'reused')::numeric)::bigint AS Reused,
		SUM((Report->'blockStats'->>'pulled')::numeric)::bigint AS Pulled,
		SUM((Report->'blockStats'->>'copyOrigin')::numeric)::bigint AS CopyOrigin,
		SUM((Report->'blockStats'->>'copyOriginShifted')::numeric)::bigint AS CopyOriginShifted,
		SUM((Report->'blockStats'->>'copyElsewhere')::numeric)::bigint AS CopyElsewhere
		FROM ReportsJson
		WHERE
			Received > $1
			AND Received < DATE_TRUNC('day', NOW())
			AND (Report->>'urVersion')::numeric >= 3
			AND Report->>'version' like 'v_.%'
			AND Report->>'version' NOT LIKE 'v0.14.40%'
			AND Report->>'version' NOT LIKE 'v0.14.39%'
			AND Report->>'version' NOT LIKE 'v0.14.38%'
		GROUP BY Day
	);
	`, since)
	if err != nil {
		return 0, err
	}

	return res.RowsAffected()
}
