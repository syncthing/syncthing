// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"math"
	"time"

	metrics "github.com/rcrowley/go-metrics"
)

const cpuTickRate = 5 * time.Second

type cpuService struct {
	avg  metrics.EWMA
	stop chan struct{}
}

func newCPUService() *cpuService {
	return &cpuService{
		// 10 second average. Magic alpha value comes from looking at EWMA package
		// definitions of EWMA1, EWMA5. The tick rate *must* be five seconds (hard
		// coded in the EWMA package).
		avg:  metrics.NewEWMA(1 - math.Exp(-float64(cpuTickRate)/float64(time.Second)/10.0)),
		stop: make(chan struct{}),
	}
}

func (s *cpuService) Serve() {
	// Initialize prevUsage to an actual value returned by cpuUsage
	// instead of zero, because at least Windows returns a huge negative
	// number here that then slowly increments...
	prevUsage := cpuUsage()
	ticker := time.NewTicker(cpuTickRate)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			curUsage := cpuUsage()
			s.avg.Update(int64((curUsage - prevUsage) / time.Millisecond))
			prevUsage = curUsage
			s.avg.Tick()
		case <-s.stop:
			return
		}
	}
}

func (s *cpuService) Stop() {
	close(s.stop)
}

func (s *cpuService) Rate() float64 {
	return s.avg.Rate()
}
