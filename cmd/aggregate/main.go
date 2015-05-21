package main

import (
	"database/sql"
	_ "github.com/lib/pq"
	"log"
	"os"
	"time"
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
	since := maxIndexedDay(db)
	log.Println("Aggregating data since", since)
	rows, err := aggregate(db, since)
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

	row := db.QueryRow(`SELECT 'UniqueDayVersionIndex'::regclass`)
	if err := row.Scan(nil); err != nil {
		_, err = db.Exec(`CREATE UNIQUE INDEX UniqueDayVersionIndex ON VersionSummary (Day, Version)`)
	}

	row = db.QueryRow(`SELECT 'DayIndex'::regclass`)
	if err := row.Scan(nil); err != nil {
		_, err = db.Exec(`CREATE INDEX DayIndex ON VerionSummary (Day)`)
	}

	return err
}

func maxIndexedDay(db *sql.DB) time.Time {
	var t time.Time
	row := db.QueryRow("SELECT MAX(Day) FROM VersionSummary")
	err := row.Scan(&t)
	if err != nil {
		return time.Time{}
	}
	return t
}

func aggregate(db *sql.DB, since time.Time) (int64, error) {
	res, err := db.Exec(`INSERT INTO VersionSummary (
	SELECT
		DATE_TRUNC('day', Received) AS Day,
		SUBSTRING(Version FROM '^v\d.\d+') AS Ver,
		COUNT(*) AS Count
		FROM Reports
		WHERE
			DATE_TRUNC('day', Received) > $1
			AND DATE_TRUNC('day', Received) < DATE_TRUNC('day', NOW() - '1 day'::INTERVAL)
			AND Version like 'v0.%'
		GROUP BY Day, Ver
		);
	`, since)
	if err != nil {
		return 0, err
	}

	return res.RowsAffected()
}
