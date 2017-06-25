// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/fs"
)

type locationEnum string

// Use strings as keys to make printout and serialization of the locations map
// more meaningful.
const (
	locConfigFile    locationEnum = "config"
	locCertFile                   = "certFile"
	locKeyFile                    = "keyFile"
	locHTTPSCertFile              = "httpsCertFile"
	locHTTPSKeyFile               = "httpsKeyFile"
	locDatabase                   = "database"
	locLogFile                    = "logFile"
	locCsrfTokens                 = "csrfTokens"
	locPanicLog                   = "panicLog"
	locAuditLog                   = "auditLog"
	locGUIAssets                  = "GUIAssets"
	locDefFolder                  = "defFolder"
)

// Platform dependent directories
var baseDirs = map[string]string{
	"config": defaultConfigDir(), // Overridden by -home flag
	"home":   homeDir(),          // User's home directory, *not* -home flag
}

// Use the variables from baseDirs here
var locations = map[locationEnum]string{
	locConfigFile:    "${config}/config.xml",
	locCertFile:      "${config}/cert.pem",
	locKeyFile:       "${config}/key.pem",
	locHTTPSCertFile: "${config}/https-cert.pem",
	locHTTPSKeyFile:  "${config}/https-key.pem",
	locDatabase:      "${config}/index-v0.14.0.db",
	locLogFile:       "${config}/syncthing.log", // -logfile on Windows
	locCsrfTokens:    "${config}/csrftokens.txt",
	locPanicLog:      "${config}/panic-${timestamp}.log",
	locAuditLog:      "${config}/audit-${timestamp}.log",
	locGUIAssets:     "${config}/gui",
	locDefFolder:     "${home}/Sync",
}

// expandLocations replaces the variables in the location map with actual
// directory locations.
func expandLocations() error {
	for key, dir := range locations {
		for varName, value := range baseDirs {
			dir = strings.Replace(dir, "${"+varName+"}", value, -1)
		}
		var err error
		dir, err = fs.ExpandTilde(dir)
		if err != nil {
			return err
		}
		locations[key] = dir
	}
	return nil
}

// defaultConfigDir returns the default configuration directory, as figured
// out by various the environment variables present on each platform, or dies
// trying.
func defaultConfigDir() string {
	switch runtime.GOOS {
	case "windows":
		if p := os.Getenv("LocalAppData"); p != "" {
			return filepath.Join(p, "Syncthing")
		}
		return filepath.Join(os.Getenv("AppData"), "Syncthing")

	case "darwin":
		dir, err := fs.ExpandTilde("~/Library/Application Support/Syncthing")
		if err != nil {
			l.Fatalln(err)
		}
		return dir

	default:
		if xdgCfg := os.Getenv("XDG_CONFIG_HOME"); xdgCfg != "" {
			return filepath.Join(xdgCfg, "syncthing")
		}
		dir, err := fs.ExpandTilde("~/.config/syncthing")
		if err != nil {
			l.Fatalln(err)
		}
		return dir
	}
}

// homeDir returns the user's home directory, or dies trying.
func homeDir() string {
	home, err := fs.ExpandTilde("~")
	if err != nil {
		l.Fatalln(err)
	}
	return home
}

func timestampedLoc(key locationEnum) string {
	// We take the roundtrip via "${timestamp}" instead of passing the path
	// directly through time.Format() to avoid issues when the path we are
	// expanding contains numbers; otherwise for example
	// /home/user2006/.../panic-20060102-150405.log would get both instances of
	// 2006 replaced by 2015...
	tpl := locations[key]
	now := time.Now().Format("20060102-150405")
	return strings.Replace(tpl, "${timestamp}", now, -1)
}
