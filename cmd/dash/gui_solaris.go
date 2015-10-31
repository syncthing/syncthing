// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

//+build solaris

package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"time"
)

type id_t int32
type ulong_t uint32

type timestruc_t struct {
	Tv_sec  int64
	Tv_nsec int64
}

func (tv timestruc_t) Nano() int64 {
	return tv.Tv_sec*1e9 + tv.Tv_nsec
}

type prusage_t struct {
	Pr_lwpid    id_t        /* lwp id.  0: process or defunct */
	Pr_count    int32       /* number of contributing lwps */
	Pr_tstamp   timestruc_t /* real time stamp, time of read() */
	Pr_create   timestruc_t /* process/lwp creation time stamp */
	Pr_term     timestruc_t /* process/lwp termination time stamp */
	Pr_rtime    timestruc_t /* total lwp real (elapsed) time */
	Pr_utime    timestruc_t /* user level CPU time */
	Pr_stime    timestruc_t /* system call CPU time */
	Pr_ttime    timestruc_t /* other system trap CPU time */
	Pr_tftime   timestruc_t /* text page fault sleep time */
	Pr_dftime   timestruc_t /* data page fault sleep time */
	Pr_kftime   timestruc_t /* kernel page fault sleep time */
	Pr_ltime    timestruc_t /* user lock wait sleep time */
	Pr_slptime  timestruc_t /* all other sleep time */
	Pr_wtime    timestruc_t /* wait-cpu (latency) time */
	Pr_stoptime timestruc_t /* stopped time */
	Pr_minf     ulong_t     /* minor page faults */
	Pr_majf     ulong_t     /* major page faults */
	Pr_nswap    ulong_t     /* swaps */
	Pr_inblk    ulong_t     /* input blocks */
	Pr_oublk    ulong_t     /* output blocks */
	Pr_msnd     ulong_t     /* messages sent */
	Pr_mrcv     ulong_t     /* messages received */
	Pr_sigs     ulong_t     /* signals received */
	Pr_vctx     ulong_t     /* voluntary context switches */
	Pr_ictx     ulong_t     /* involuntary context switches */
	Pr_sysc     ulong_t     /* system calls */
	Pr_ioch     ulong_t     /* chars read and written */

}

func solarisPrusage(pid int, rusage *prusage_t) error {
	fd, err := os.Open(fmt.Sprintf("/proc/%d/usage", pid))
	if err != nil {
		return err
	}
	err = binary.Read(fd, binary.LittleEndian, rusage)
	fd.Close()
	return err
}

func init() {
	go trackCPUUsage()
}

func trackCPUUsage() {
	var prevUsage int64
	var prevTime = time.Now().UnixNano()
	var rusage prusage_t
	var pid = os.Getpid()
	for _ = range time.NewTicker(time.Second).C {
		err := solarisPrusage(pid, &rusage)
		if err != nil {
			l.Warnln("getting prusage:", err)
			continue
		}
		curTime := time.Now().UnixNano()
		timeDiff := curTime - prevTime
		curUsage := rusage.Pr_utime.Nano() + rusage.Pr_stime.Nano()
		usageDiff := curUsage - prevUsage
		cpuUsageLock.Lock()
		copy(cpuUsagePercent[1:], cpuUsagePercent[0:])
		cpuUsagePercent[0] = 100 * float64(usageDiff) / float64(timeDiff)
		cpuUsageLock.Unlock()
		prevTime = curTime
		prevUsage = curUsage
	}
}
