// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/model"
	"github.com/thejerf/suture"
)

// Current version number of the usage report, for acceptance purposes. If
// fields are added or changed this integer must be incremented so that users
// are prompted for acceptance of the new report.
const usageReportVersion = 1

type usageReportingManager struct {
	model *model.Model
	sup   *suture.Supervisor
}

func newUsageReportingManager(m *model.Model, cfg *config.Wrapper) *usageReportingManager {
	mgr := &usageReportingManager{
		model: m,
	}

	// Start UR if it's enabled.
	mgr.CommitConfiguration(config.Configuration{}, cfg.Raw())

	// Listen to future config changes so that we can start and stop as
	// appropriate.
	cfg.Subscribe(mgr)

	return mgr
}

func (m *usageReportingManager) VerifyConfiguration(from, to config.Configuration) error {
	return nil
}

func (m *usageReportingManager) CommitConfiguration(from, to config.Configuration) bool {
	if to.Options.URAccepted >= usageReportVersion && m.sup == nil {
		// Usage reporting was turned on; lets start it.
		svc := &usageReportingService{
			model: m.model,
		}
		m.sup = suture.NewSimple("usageReporting")
		m.sup.Add(svc)
		m.sup.ServeBackground()
	} else if to.Options.URAccepted < usageReportVersion && m.sup != nil {
		// Usage reporting was turned off
		m.sup.Stop()
		m.sup = nil
	}

	return true
}

func (m *usageReportingManager) String() string {
	return fmt.Sprintf("usageReportingManager@%p", m)
}

// reportData returns the data to be sent in a usage report. It's used in
// various places, so not part of the usageReportingSvc object.
func reportData(m *model.Model) map[string]interface{} {
	res := make(map[string]interface{})
	res["uniqueID"] = cfg.Options().URUniqueID
	res["version"] = Version
	res["longVersion"] = LongVersion
	res["platform"] = runtime.GOOS + "-" + runtime.GOARCH
	res["numFolders"] = len(cfg.Folders())
	res["numDevices"] = len(cfg.Devices())

	var totFiles, maxFiles int
	var totBytes, maxBytes int64
	for folderID := range cfg.Folders() {
		files, _, bytes := m.GlobalSize(folderID)
		totFiles += files
		totBytes += bytes
		if files > maxFiles {
			maxFiles = files
		}
		if bytes > maxBytes {
			maxBytes = bytes
		}
	}

	res["totFiles"] = totFiles
	res["folderMaxFiles"] = maxFiles
	res["totMiB"] = totBytes / 1024 / 1024
	res["folderMaxMiB"] = maxBytes / 1024 / 1024

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	res["memoryUsageMiB"] = (mem.Sys - mem.HeapReleased) / 1024 / 1024

	var perf float64
	for i := 0; i < 5; i++ {
		p := cpuBench()
		if p > perf {
			perf = p
		}
	}
	res["sha256Perf"] = perf

	bytes, err := memorySize()
	if err == nil {
		res["memorySize"] = bytes / 1024 / 1024
	}

	return res
}

type usageReportingService struct {
	model *model.Model
	stop  chan struct{}
}

func (s *usageReportingService) sendUsageReport() error {
	d := reportData(s.model)
	var b bytes.Buffer
	json.NewEncoder(&b).Encode(d)

	var client = http.DefaultClient
	if BuildEnv == "android" {
		// This works around the lack of DNS resolution on Android... :(
		tr := &http.Transport{
			Dial: func(network, addr string) (net.Conn, error) {
				return net.Dial(network, "194.126.249.13:443")
			},
		}
		client = &http.Client{Transport: tr}
	}
	_, err := client.Post("https://data.syncthing.net/newdata", "application/json", &b)
	return err
}

func (s *usageReportingService) Serve() {
	s.stop = make(chan struct{})

	l.Infoln("Starting usage reporting")
	defer l.Infoln("Stopping usage reporting")

	t := time.NewTimer(10 * time.Minute) // time to initial report at start
	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
			err := s.sendUsageReport()
			if err != nil {
				l.Infoln("Usage report:", err)
			}
			t.Reset(24 * time.Hour) // next report tomorrow
		}
	}
}

func (s *usageReportingService) Stop() {
	close(s.stop)
}

// cpuBench returns CPU performance as a measure of single threaded SHA-256 MiB/s
func cpuBench() float64 {
	chunkSize := 100 * 1 << 10
	h := sha256.New()
	bs := make([]byte, chunkSize)
	rand.Reader.Read(bs)

	t0 := time.Now()
	b := 0
	for time.Since(t0) < 125*time.Millisecond {
		h.Write(bs)
		b += chunkSize
	}
	h.Sum(nil)
	d := time.Since(t0)
	return float64(int(float64(b)/d.Seconds()/(1<<20)*100)) / 100
}
