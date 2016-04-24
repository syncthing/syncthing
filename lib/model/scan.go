// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"math/rand"
	"time"
)

type rescanRequest struct {
	subdirs []string
	err     chan error
}

// bundle all folder scan activity
type scan struct {
	scanInterval time.Duration
	scanTimer    *time.Timer
	scanNow      chan rescanRequest
	scanDelay    chan time.Duration
}

func (s *scan) rescheduleScan() {
	if s.scanInterval == 0 {
		return
	}
	// Sleep a random time between 3/4 and 5/4 of the configured interval.
	sleepNanos := (s.scanInterval.Nanoseconds()*3 + rand.Int63n(2*s.scanInterval.Nanoseconds())) / 4
	interval := time.Duration(sleepNanos) * time.Nanosecond
	l.Debugln(s, "next rescan in", interval)
	s.scanTimer.Reset(interval)
}

func (s *scan) Scan(subdirs []string) error {
	req := rescanRequest{
		subdirs: subdirs,
		err:     make(chan error),
	}
	s.scanNow <- req
	return <-req.err
}

func (s *scan) DelayScan(next time.Duration) {
	s.scanDelay <- next
}
