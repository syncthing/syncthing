// Copyright (C) 2014-2015 Jakob Borg and Contributors (see the CONTRIBUTORS file).

package main

import (
	"bytes"
	"database/sql"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"sync/atomic"
	"time"
)

type stats struct {
	// Incremented atomically
	announces int64
	queries   int64
	answers   int64
	errors    int64
}

func (s *stats) Announce() {
	atomic.AddInt64(&s.announces, 1)
}

func (s *stats) Query() {
	atomic.AddInt64(&s.queries, 1)
}

func (s *stats) Answer() {
	atomic.AddInt64(&s.answers, 1)
}

func (s *stats) Error() {
	atomic.AddInt64(&s.errors, 1)
}

// Reset returns a copy of the current stats and resets the counters to
// zero.
func (s *stats) Reset() stats {
	// Create a copy of the stats using atomic reads
	copy := stats{
		announces: atomic.LoadInt64(&s.announces),
		queries:   atomic.LoadInt64(&s.queries),
		answers:   atomic.LoadInt64(&s.answers),
		errors:    atomic.LoadInt64(&s.errors),
	}

	// Reset the stats by subtracting the values that we copied
	atomic.AddInt64(&s.announces, -copy.announces)
	atomic.AddInt64(&s.queries, -copy.queries)
	atomic.AddInt64(&s.answers, -copy.answers)
	atomic.AddInt64(&s.errors, -copy.errors)

	return copy
}

type statssrv struct {
	intv time.Duration
	file string
	db   *sql.DB
}

func (s *statssrv) Serve() {
	lastReset := time.Now()
	for {
		time.Sleep(next(s.intv))

		stats := globalStats.Reset()
		d := time.Since(lastReset).Seconds()
		lastReset = time.Now()

		log.Printf("Stats: %.02f announces/s, %.02f queries/s, %.02f answers/s, %.02f errors/s",
			float64(stats.announces)/d, float64(stats.queries)/d, float64(stats.answers)/d, float64(stats.errors)/d)

		if s.file != "" {
			s.writeToFile(stats, d)
		}
	}
}

func (s *statssrv) Stop() {
	panic("stop unimplemented")
}

func (s *statssrv) writeToFile(stats stats, secs float64) {
	newLine := []byte("\n")

	var addrs int
	row := s.db.QueryRow("SELECT COUNT(*) FROM Addresses")
	if err := row.Scan(&addrs); err != nil {
		log.Println("stats query:", err)
		return
	}

	fd, err := os.OpenFile(s.file, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		log.Println("stats file:", err)
		return
	}
	defer func() {
		err = fd.Close()
		if err != nil {
			log.Println("stats file:", err)
		}
	}()

	bs, err := ioutil.ReadAll(fd)
	if err != nil {
		log.Println("stats file:", err)
		return
	}
	lines := bytes.Split(bytes.TrimSpace(bs), newLine)
	if len(lines) > 12 {
		lines = lines[len(lines)-12:]
	}

	latest := fmt.Sprintf("%v: %6d addresses, %8.02f announces/s, %8.02f queries/s, %8.02f answers/s, %8.02f errors/s\n",
		time.Now().UTC().Format(time.RFC3339), addrs,
		float64(stats.announces)/secs, float64(stats.queries)/secs, float64(stats.answers)/secs, float64(stats.errors)/secs)
	lines = append(lines, []byte(latest))

	_, err = fd.Seek(0, 0)
	if err != nil {
		log.Println("stats file:", err)
		return
	}
	err = fd.Truncate(0)
	if err != nil {
		log.Println("stats file:", err)
		return
	}

	_, err = fd.Write(bytes.Join(lines, newLine))
	if err != nil {
		log.Println("stats file:", err)
		return
	}
}
