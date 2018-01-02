// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"math/rand"
	"time"

	"github.com/abiosoft/semaphore"
	"github.com/syncthing/syncthing/lib/config"
)

type rescanRequest struct {
	subdirs []string
	err     chan error
}

// bundle all folder scan activity
type folderScanner struct {
	interval time.Duration
	timer    *time.Timer
	now      chan rescanRequest
	delay    chan time.Duration
	limiter  folderScannerLimiter
}

func newFolderScanner(config config.FolderConfiguration, limiter folderScannerLimiter) folderScanner {
	return folderScanner{
		interval: time.Duration(config.RescanIntervalS) * time.Second,
		timer:    time.NewTimer(time.Millisecond), // The first scan should be done immediately.
		now:      make(chan rescanRequest),
		delay:    make(chan time.Duration),
		limiter:  limiter,
	}
}

func (f *folderScanner) Reschedule() {
	if f.interval == 0 {
		return
	}
	// Sleep a random time between 3/4 and 5/4 of the configured interval.
	sleepNanos := (f.interval.Nanoseconds()*3 + rand.Int63n(2*f.interval.Nanoseconds())) / 4
	interval := time.Duration(sleepNanos) * time.Nanosecond
	l.Debugln(f, "next rescan in", interval)
	f.timer.Reset(interval)
}

func (f *folderScanner) Scan(subdirs []string) error {

	f.limiter.Aquire()
	defer f.limiter.Release()
	l.Infoln("DEBUG scan request: %v ", subdirs)

	req := rescanRequest{
		subdirs: subdirs,
		err:     make(chan error),
	}
	f.now <- req
	return <-req.err
}

func (f *folderScanner) Delay(next time.Duration) {
	f.delay <- next
}

type folderScannerLimiter interface {
	Aquire()
	Release()
}

func newFolderScannerLimiter(single bool) folderScannerLimiter {
	if single {
		l.Infoln("DEBUG single global folderScanner limit ")
		return &singleGlobalFolderScannerLimiter{
			sem: semaphore.New(1),
		}
	}
	l.Infoln("DEBUG no global folderScanner limit ")
	return &noGlobalFolderScannerLimiter{}
}

type singleGlobalFolderScannerLimiter struct {
	sem *semaphore.Semaphore
}

func (fsf *singleGlobalFolderScannerLimiter) Aquire() {
	l.Infoln("DEBUG Aquire " + " global scan request")
	fsf.sem.Acquire()
}

func (fsf *singleGlobalFolderScannerLimiter) Release() {
	l.Infoln("DEBUG Release" + " global scan request")
	fsf.sem.Release()
}

type noGlobalFolderScannerLimiter struct {
}

func (fsf *noGlobalFolderScannerLimiter) Aquire() {
	l.Infoln("DEBUG Aquire " + " individual scan request")
}

func (fsf *noGlobalFolderScannerLimiter) Release() {
	l.Infoln("DEBUG Release" + " individual scan request")
}
