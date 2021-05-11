// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"database/sql"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq"
)

var dbConn = getEnvDefault("UR_DB_URL", "postgres://user:password@localhost/ur?sslmode=disable")

func getEnvDefault(key, def string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return def
}

func main() {
	log.SetFlags(log.Ltime | log.Ldate)
	log.SetOutput(os.Stdout)

	db, err := sql.Open("postgres", dbConn)
	if err != nil {
		log.Fatalln("database:", err)
	}
	err = setupDB(db)
	if err != nil {
		log.Fatalln("database:", err)
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
	rows, err := aggregateVersionSummary(db, since)
	if err != nil {
		log.Println("aggregate:", err)
	}
	log.Println("Inserted", rows, "rows")

	log.Println("Aggregating UserMovement data")
	rows, err = aggregateUserMovement(db)
	if err != nil {
		log.Println("aggregate:", err)
	}
	log.Println("Inserted", rows, "rows")

	since = maxIndexedDay(db, "Performance")
	log.Println("Aggregating Performance data since", since)
	rows, err = aggregatePerformance(db, since)
	if err != nil {
		log.Println("aggregate:", err)
	}
	log.Println("Inserted", rows, "rows")

	since = maxIndexedDay(db, "BlockStats")
	log.Println("Aggregating BlockStats data since", since)
	rows, err = aggregateBlockStats(db, since)
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

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS UserMovement (
		Day TIMESTAMP NOT NULL,
		Added INTEGER NOT NULL,
		Bounced INTEGER NOT NULL,
		Removed INTEGER NOT NULL
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
		Total INTEGER NOT NULL,
		Renamed INTEGER NOT NULL,
		Reused INTEGER NOT NULL,
		Pulled INTEGER NOT NULL,
		CopyOrigin INTEGER NOT NULL,
		CopyOriginShifted INTEGER NOT NULL,
		CopyElsewhere INTEGER NOT NULL
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

	row = db.QueryRow(`SELECT 'MovementDayIndex'::regclass`)
	if err := row.Scan(&t); err != nil {
		_, _ = db.Exec(`CREATE INDEX MovementDayIndex ON UserMovement (Day)`)
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
	row := db.QueryRow("SELECT MAX(Day) FROM " + table)
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
			DATE_TRUNC('day', Received) > $1
			AND DATE_TRUNC('day', Received) < DATE_TRUNC('day', NOW())
			AND Report->>'version' like 'v_.%'
		GROUP BY Day, Ver
		);
	`, since)
	if err != nil {
		return 0, err
	}

	return res.RowsAffected()
}

func aggregateUserMovement(db *sql.DB) (int64, error) {
	rows, err := db.Query(`SELECT
		DATE_TRUNC('day', Received) AS Day,
		Report->>'uniqueID'
		FROM ReportsJson
		WHERE
			Report->>'uniqueID' IS NOT NULL
			AND DATE_TRUNC('day', Received) < DATE_TRUNC('day', NOW())
			AND Report->>'version' like 'v_.%'
		ORDER BY Day
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	firstSeen := make(map[string]time.Time)
	lastSeen := make(map[string]time.Time)
	var minTs time.Time
	minTs = minTs.In(time.UTC)

	for rows.Next() {
		var ts time.Time
		var id string
		if err := rows.Scan(&ts, &id); err != nil {
			return 0, err
		}

		if minTs.IsZero() {
			minTs = ts
		}
		if _, ok := firstSeen[id]; !ok {
			firstSeen[id] = ts
		}
		lastSeen[id] = ts
	}

	type sumRow struct {
		day     time.Time
		added   int
		removed int
		bounced int
	}
	var sumRows []sumRow
	for t := minTs; t.Before(time.Now().Truncate(24 * time.Hour)); t = t.AddDate(0, 0, 1) {
		var added, removed, bounced int
		old := t.Before(time.Now().AddDate(0, 0, -30))
		for id, first := range firstSeen {
			last := lastSeen[id]
			if first.Equal(t) && last.Equal(t) && old {
				bounced++
				continue
			}
			if first.Equal(t) {
				added++
			}
			if last == t && old {
				removed++
			}
		}
		sumRows = append(sumRows, sumRow{t, added, removed, bounced})
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec("DELETE FROM UserMovement"); err != nil {
		tx.Rollback()
		return 0, err
	}
	for _, r := range sumRows {
		if _, err := tx.Exec("INSERT INTO UserMovement (Day, Added, Removed, Bounced) VALUES ($1, $2, $3, $4)", r.day, r.added, r.removed, r.bounced); err != nil {
			tx.Rollback()
			return 0, err
		}
	}

	return int64(len(sumRows)), tx.Commit()
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
			DATE_TRUNC('day', Received) > $1
			AND DATE_TRUNC('day', Received) < DATE_TRUNC('day', NOW())
			AND Report->>'version' like 'v_.%'
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
		SUM((Report->'blockStats'->>'total')::numeric) AS Total,
		SUM((Report->'blockStats'->>'renamed')::numeric) AS Renamed,
		SUM((Report->'blockStats'->>'reused')::numeric) AS Reused,
		SUM((Report->'blockStats'->>'pulled')::numeric) AS Pulled,
		SUM((Report->'blockStats'->>'copyOrigin')::numeric) AS CopyOrigin,
		SUM((Report->'blockStats'->>'copyOriginShifted')::numeric) AS CopyOriginShifted,
		SUM((Report->'blockStats'->>'copyElsewhere')::numeric) AS CopyElsewhere
		FROM ReportsJson
		WHERE
			DATE_TRUNC('day', Received) > $1
			AND DATE_TRUNC('day', Received) < DATE_TRUNC('day', NOW())
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
