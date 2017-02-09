// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// This doesn't build on Windows due to the Rusage stuff.

// +build !windows

package main

import (
	"flag"
	"fmt"
	"log"
	"runtime"
	"syscall"
	"time"

	"github.com/syncthing/syncthing/lib/rc"
)

var homeDir = "h1"
var syncthingBin = "./bin/syncthing"
var test = "scan"

func main() {
	flag.StringVar(&homeDir, "home", homeDir, "Home directory location")
	flag.StringVar(&syncthingBin, "bin", syncthingBin, "Binary location")
	flag.StringVar(&test, "test", test, "Test to run")
	flag.Parse()

	switch test {
	case "scan":
		// scan measures the resource usage required to perform the initial
		// scan, without cleaning away the database first.
		testScan()
	}
}

// testScan starts a process and reports on the resource usage required to
// perform the initial scan.
func testScan() {
	log.Println("Starting...")
	p := rc.NewProcess("127.0.0.1:8081")
	if err := p.Start(syncthingBin, "-home", homeDir, "-no-browser"); err != nil {
		log.Println(err)
		return
	}
	defer p.Stop()

	wallTime := awaitScanComplete(p)

	report(p, wallTime)
}

// awaitScanComplete waits for a folder to transition idle->scanning and
// then scanning->idle and returns the time taken for the scan.
func awaitScanComplete(p *rc.Process) time.Duration {
	log.Println("Awaiting scan completion...")
	var t0, t1 time.Time
	lastEvent := 0
loop:
	for {
		evs, err := p.Events(lastEvent)
		if err != nil {
			continue
		}

		for _, ev := range evs {
			if ev.Type == "StateChanged" {
				data := ev.Data.(map[string]interface{})
				log.Println(ev)

				if data["to"].(string) == "scanning" {
					t0 = ev.Time
					continue
				}

				if !t0.IsZero() && data["to"].(string) == "idle" {
					t1 = ev.Time
					break loop
				}
			}
			lastEvent = ev.ID
		}

		time.Sleep(250 * time.Millisecond)
	}

	return t1.Sub(t0)
}

// report stops the given process and reports on it's resource usage in two
// ways: human readable to stderr, and CSV to stdout.
func report(p *rc.Process, wallTime time.Duration) {
	sv, err := p.SystemVersion()
	if err != nil {
		log.Println(err)
		return
	}

	ss, err := p.SystemStatus()
	if err != nil {
		log.Println(err)
		return
	}

	proc, err := p.Stop()
	if err != nil {
		return
	}

	rusage, ok := proc.SysUsage().(*syscall.Rusage)
	if !ok {
		return
	}

	log.Println("Version:", sv.Version)
	log.Println("Alloc:", ss.Alloc/1024, "KiB")
	log.Println("Sys:", ss.Sys/1024, "KiB")
	log.Println("Goroutines:", ss.Goroutines)
	log.Println("Wall time:", wallTime)
	log.Println("Utime:", time.Duration(rusage.Utime.Nano()))
	log.Println("Stime:", time.Duration(rusage.Stime.Nano()))
	if runtime.GOOS == "darwin" {
		// Darwin reports in bytes, Linux seems to report in KiB even
		// though the manpage says otherwise.
		rusage.Maxrss /= 1024
	}
	log.Println("MaxRSS:", rusage.Maxrss, "KiB")

	fmt.Printf("%s,%d,%d,%d,%.02f,%.02f,%.02f,%d\n",
		sv.Version,
		ss.Alloc/1024,
		ss.Sys/1024,
		ss.Goroutines,
		wallTime.Seconds(),
		time.Duration(rusage.Utime.Nano()).Seconds(),
		time.Duration(rusage.Stime.Nano()).Seconds(),
		rusage.Maxrss)
}
