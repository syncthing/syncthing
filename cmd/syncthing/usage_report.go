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
	"net"
	"net/http"
	"runtime"
	"time"

	"github.com/syncthing/syncthing/internal/model"
)

// Current version number of the usage report, for acceptance purposes. If
// fields are added or changed this integer must be incremented so that users
// are prompted for acceptance of the new report.
const usageReportVersion = 1

var stopUsageReportingCh = make(chan struct{})

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

func sendUsageReport(m *model.Model) error {
	d := reportData(m)
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

func usageReportingLoop(m *model.Model) {
	l.Infoln("Starting usage reporting")
	t := time.NewTicker(86400 * time.Second)
loop:
	for {
		select {
		case <-stopUsageReportingCh:
			break loop
		case <-t.C:
			err := sendUsageReport(m)
			if err != nil {
				l.Infoln("Usage report:", err)
			}
		}
	}
	l.Infoln("Stopping usage reporting")
}

func stopUsageReporting() {
	select {
	case stopUsageReportingCh <- struct{}{}:
	default:
	}
}

// Returns CPU performance as a measure of single threaded SHA-256 MiB/s
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
