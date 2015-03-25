// Copyright (C) 2014-2015 Jakob Borg and Contributors (see the CONTRIBUTORS file).

package main

import (
	"log"
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
}

func (s *statssrv) Serve() {
	for {
		time.Sleep(next(s.intv))

		stats := globalStats.Reset()
		s := time.Since(stats.reset).Seconds()
		log.Printf("Stats: %.02f announces/s, %.02f queries/s, %.02f answers/s, %.02f errors/s",
			float64(stats.announces)/s, float64(stats.queries)/s, float64(stats.answers)/s, float64(stats.errors)/s)
	}
}

func (s *statssrv) Stop() {
	panic("stop unimplemented")
}
