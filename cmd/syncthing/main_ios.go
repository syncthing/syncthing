// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package toplevel

import (
	"github.com/syncthing/syncthing/lib/locations"
	"github.com/syncthing/syncthing/lib/logger"
	"github.com/syncthing/syncthing/lib/syncthing"
)

func SyncthingIsRunning() bool {
	return runningApp != nil
}

func SyncthingStart() int {

	// The below is forked from main.go so needs to be maintained manually
	options := RuntimeOptions{
		Options: syncthing.Options{
			AssetDir:    locations.Get(locations.GUIAssets),
			NoUpgrade:   false,   // os.Getenv("STNOUPGRADE") != ""
			ProfilerURL: "",      // os.Getenv("STPROFILER")
			Verbose:     true,
		},
		noRestart:    false,    // os.Getenv("STNORESTART") != ""
		cpuProfile:   false,    // os.Getenv("STCPUPROFILE") != ""
		stRestarting: false,    // os.Getenv("STRESTART") != ""
		logFile:      "-",
		logFlags:     logger.DebugFlags,
		logMaxSize:   10 << 20, // 10 MiB
		logMaxFiles:  3,        // plus the current one
	}

	l.SetFlags(options.logFlags)

	// Ensure that our home directory exists.
	if err := ensureDir(locations.GetBaseDir(locations.ConfigBaseDir), 0700); err != nil {
		l.Warnln("Failure on home directory:", err)
		return syncthing.ExitError.AsInt()
	}

	innerProcess = true

	return syncthingMain(options)
}
