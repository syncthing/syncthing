// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package locations

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/fs"
)

type LocationEnum string

// Use strings as keys to make printout and serialization of the locations map
// more meaningful.
const (
	ConfigFile    LocationEnum = "config"
	CertFile      LocationEnum = "certFile"
	KeyFile       LocationEnum = "keyFile"
	HTTPSCertFile LocationEnum = "httpsCertFile"
	HTTPSKeyFile  LocationEnum = "httpsKeyFile"
	Database      LocationEnum = "database"
	LogFile       LocationEnum = "logFile"
	CsrfTokens    LocationEnum = "csrfTokens"
	PanicLog      LocationEnum = "panicLog"
	AuditLog      LocationEnum = "auditLog"
	GUIAssets     LocationEnum = "GUIAssets"
	DefFolder     LocationEnum = "defFolder"
	FailuresFile  LocationEnum = "FailuresFile"
)

type BaseDirEnum string

const (
	// Overridden by -home flag
	ConfigBaseDir   BaseDirEnum = "config"
	DataBaseDir     BaseDirEnum = "data"
	// User's home directory, *not* -home flag
	UserHomeBaseDir BaseDirEnum = "userHome"

	LevelDBDir = "index-v0.14.0.db"
	BadgerDir  = "indexdb.badger"
)

// Platform dependent directories
var baseDirs = make(map[BaseDirEnum]string, 3)

func init() {
	userHome := "~"
	config := defaultConfigDir(userHome)
	baseDirs[UserHomeBaseDir] = userHome
	baseDirs[ConfigBaseDir] = config
	baseDirs[DataBaseDir] = defaultDataDir(userHome, config)

	err := expandLocations()
	if err != nil {
		fmt.Println(err)
		panic("Failed to expand locations at init time")
	}
}

func SetBaseDir(baseDirName BaseDirEnum, path string) error {
	_, ok := baseDirs[baseDirName]
	if !ok {
		return fmt.Errorf("unknown base dir: %s", baseDirName)
	}
	baseDirs[baseDirName] = filepath.Clean(path)
	return expandLocations()
}

func GetRelative(location LocationEnum) string {
	return locations[location]
}

func Get(location LocationEnum) string {
	relPath := locations[location]
	var err error
	var fullPath string
	fullPath, err = fs.ExpandTilde(relPath)
	if err != nil {
		return relPath
	}
	return fullPath
}

func GetBaseDir(baseDir BaseDirEnum) string {
	return baseDirs[baseDir]
}

// Use the variables from baseDirs here
var locationTemplates = map[LocationEnum]string{
	ConfigFile:    "${config}/config.xml",
	CertFile:      "${config}/cert.pem",
	KeyFile:       "${config}/key.pem",
	HTTPSCertFile: "${config}/https-cert.pem",
	HTTPSKeyFile:  "${config}/https-key.pem",
	Database:      "${data}/" + LevelDBDir,
	LogFile:       "${data}/syncthing.log", // -logfile on Windows
	CsrfTokens:    "${data}/csrftokens.txt",
	PanicLog:      "${data}/panic-${timestamp}.log",
	AuditLog:      "${data}/audit-${timestamp}.log",
	GUIAssets:     "${config}/gui",
	DefFolder:     "Sync",
	FailuresFile:  "${data}/failures-unreported.txt",
}

var locations = make(map[LocationEnum]string)

// expandLocations replaces the variables in the locations map with actual
// directory locations.
func expandLocations() error {
	newLocations := make(map[LocationEnum]string)
	for key, dir := range locationTemplates {
		for varName, value := range baseDirs {
			dir = strings.Replace(dir, "${"+string(varName)+"}", value, -1)
		}
		newLocations[key] = filepath.Clean(dir)
	}
	locations = newLocations
	return nil
}

// defaultConfigDir returns the default configuration directory, as figured
// out by various the environment variables present on each platform, or dies
// trying.
func defaultConfigDir(userHome string) string {
	switch runtime.GOOS {
	case "windows":
		if p := os.Getenv("LocalAppData"); p != "" {
			return filepath.Join(p, "Syncthing")
		}
		return filepath.Join(os.Getenv("AppData"), "Syncthing")

	case "darwin":
		return filepath.Join(userHome, "Library/Application Support/Syncthing")

	default:
		if xdgCfg := os.Getenv("XDG_CONFIG_HOME"); xdgCfg != "" {
			return filepath.Join(xdgCfg, "syncthing")
		}
		return filepath.Join(userHome, ".config/syncthing")
	}
}

// defaultDataDir returns the default data directory, which usually is the
// config directory but might be something else.
func defaultDataDir(userHome, config string) string {
	switch runtime.GOOS {
	case "windows", "darwin":
		return config

	default:
		// If a database exists at the "normal" location, use that anyway.
		// We look for both LevelDB and Badger variants here regardless of
		// what we're currently configured to use, because we might be
		// starting up in Badger mode with only a LevelDB database present
		// (will be converted), or vice versa.
		if _, err := os.Lstat(filepath.Join(config, LevelDBDir)); err == nil {
			return config
		}
		if _, err := os.Lstat(filepath.Join(config, BadgerDir)); err == nil {
			return config
		}
		// Always use this env var, as it's explicitly set by the user
		if xdgHome := os.Getenv("XDG_DATA_HOME"); xdgHome != "" {
			return filepath.Join(xdgHome, "syncthing")
		}
		// Only use the XDG default, if a syncthing specific dir already
		// exists. Existence of ~/.local/share is not deemed enough, as
		// it may also exist erroneously on non-XDG systems.
		xdgDefault := filepath.Join(userHome, ".local/share/syncthing")
		if _, err := os.Lstat(xdgDefault); err == nil {
			return xdgDefault
		}
		// FYI: XDG_DATA_DIRS is not relevant, as it is for system-wide
		// data dirs, not user specific ones.
		return config
	}
}

func GetTimestamped(key LocationEnum) string {
	// We take the roundtrip via "${timestamp}" instead of passing the path
	// directly through time.Format() to avoid issues when the path we are
	// expanding contains numbers; otherwise for example
	// /home/user2006/.../panic-20060102-150405.log would get both instances of
	// 2006 replaced by 2015...
	tpl := locations[key]
	now := time.Now().Format("20060102-150405")
	return strings.Replace(tpl, "${timestamp}", now, -1)
}
