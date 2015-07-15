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
		log.Fatalln("aggregate:", err)
	}
	log.Println("Inserted", rows, "rows")

	log.Println("Aggregating UserMovement data")
	rows, err = aggregateUserMovement(db)
	if err != nil {
		log.Fatalln("aggregate:", err)
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
		Removed INTEGER NOT NULL
	)`)
	if err != nil {
		return err
	}

	row := db.QueryRow(`SELECT 'UniqueDayVersionIndex'::regclass`)
	if err := row.Scan(nil); err != nil {
		_, err = db.Exec(`CREATE UNIQUE INDEX UniqueDayVersionIndex ON VersionSummary (Day, Version)`)
	}

	row = db.QueryRow(`SELECT 'DayIndex'::regclass`)
	if err := row.Scan(nil); err != nil {
		_, err = db.Exec(`CREATE INDEX DayIndex ON VerionSummary (Day)`)
	}

	row = db.QueryRow(`SELECT 'MovementDayIndex'::regclass`)
	if err := row.Scan(nil); err != nil {
		_, err = db.Exec(`CREATE INDEX MovementDayIndex ON UserMovement (Day)`)
	}

	return err
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
		SUBSTRING(Version FROM '^v\d.\d+') AS Ver,
		COUNT(*) AS Count
		FROM Reports
		WHERE
			DATE_TRUNC('day', Received) > $1
			AND DATE_TRUNC('day', Received) < DATE_TRUNC('day', NOW())
			AND Version like 'v0.%'
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
		UniqueID
		FROM Reports
		WHERE
			DATE_TRUNC('day', Received) < DATE_TRUNC('day', NOW())
			AND Version like 'v0.%'
		ORDER BY Day
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	firstSeen := make(map[string]time.Time)
	lastSeen := make(map[string]time.Time)
	var minTs time.Time

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
	}
	var sumRows []sumRow
	for t := minTs; t.Before(time.Now().Truncate(24 * time.Hour)); t = t.AddDate(0, 0, 1) {
		var added, removed int
		for id, first := range firstSeen {
			last := lastSeen[id]
			if first.Equal(t) {
				added++
			}
			if last == t && t.Before(time.Now().AddDate(0, 0, -14)) {
				removed++
			}
		}
		sumRows = append(sumRows, sumRow{t, added, removed})
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
		if _, err := tx.Exec("INSERT INTO UserMovement (Day, Added, Removed) VALUES ($1, $2, $3)", r.day, r.added, r.removed); err != nil {
			tx.Rollback()
			return 0, err
		}
	}

	return int64(len(sumRows)), tx.Commit()
}
