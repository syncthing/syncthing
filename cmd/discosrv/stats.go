// Copyright (C) 2014-2015 Jakob Borg and Contributors (see the CONTRIBUTORS file).

package main

import (
	"bytes"
	"database/sql"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"sync"
	"time"
)

type stats struct {
	mut       sync.Mutex
	reset     time.Time
	announces int64
	queries   int64
	answers   int64
	errors    int64
}

func (s *stats) Announce() {
	s.mut.Lock()
	s.announces++
	s.mut.Unlock()
}

func (s *stats) Query() {
	s.mut.Lock()
	s.queries++
	s.mut.Unlock()
}

func (s *stats) Answer() {
	s.mut.Lock()
	s.answers++
	s.mut.Unlock()
}

func (s *stats) Error() {
	s.mut.Lock()
	s.errors++
	s.mut.Unlock()
}

func (s *stats) Reset() stats {
	s.mut.Lock()
	ns := *s
	s.announces, s.queries, s.answers = 0, 0, 0
	s.reset = time.Now()
	s.mut.Unlock()
	return ns
}

type statssrv struct {
	intv time.Duration
	file string
	db   *sql.DB
}

func (s *statssrv) Serve() {
	for {
		time.Sleep(next(s.intv))

		stats := globalStats.Reset()
		d := time.Since(stats.reset).Seconds()
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

	err = fd.Close()
	if err != nil {
		log.Println("stats file:", err)
		return
	}
}
