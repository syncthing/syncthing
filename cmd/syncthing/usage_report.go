// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"net"
	"net/http"
	"runtime"
	"strings"
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
	res["uniqueID"] = strings.ToLower(myID.String()[:6])
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
