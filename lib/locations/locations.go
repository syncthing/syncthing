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

type LocationationEnum string

// Use strings as keys to make printout and serialization of the locations map
// more meaningful.
const (
	ConfigFileLocation    LocationationEnum = "config"
	CertFileLocation                        = "certFile"
	KeyFileLocation                         = "keyFile"
	HTTPSCertFileLocation                   = "httpsCertFile"
	HTTPSKeyFileLocation                    = "httpsKeyFile"
	DatabaseLocation                        = "database"
	LogFileLocation                         = "logFile"
	CsrfTokensLocation                      = "csrfTokens"
	PanicLogLocation                        = "panicLog"
	AuditLogLocation                        = "auditLog"
	GUIAssetsLocation                       = "GUIAssets"
	DefFolderLocation                       = "defFolder"
)

type BaseDirEnum string

const (
	ConfigBaseDir BaseDirEnum = "config"
	HomeBaseDir               = "home"
)

func init() {
	err := expandLocations()
	if err != nil {
		panic(err)
	}
}

func SetBaseDir(baseDirName BaseDirEnum, path string) error {
	_, ok := baseDirs[baseDirName]
	if !ok {
		return fmt.Errorf("unknown base dir: %s", baseDirName)
	}
	baseDirs[baseDirName] = path
	return expandLocations()
}

func GetLocation(location LocationationEnum) string {
	return locations[location]
}

func GetBaseDir(baseDir BaseDirEnum) string {
	return baseDirs[baseDir]
}

// Platform dependent directories
var baseDirs = map[BaseDirEnum]string{
	ConfigBaseDir: defaultConfigDir(), // Overridden by -home flag
	HomeBaseDir:   homeDir(),          // User's home directory, *not* -home flag
}

// Use the variables from baseDirs here
var locations = map[LocationationEnum]string{
	ConfigFileLocation:    "${config}/config.xml",
	CertFileLocation:      "${config}/cert.pem",
	KeyFileLocation:       "${config}/key.pem",
	HTTPSCertFileLocation: "${config}/https-cert.pem",
	HTTPSKeyFileLocation:  "${config}/https-key.pem",
	DatabaseLocation:      "${config}/index-v0.14.0.db",
	LogFileLocation:       "${config}/syncthing.log", // -logfile on Windows
	CsrfTokensLocation:    "${config}/csrftokens.txt",
	PanicLogLocation:      "${config}/panic-${timestamp}.log",
	AuditLogLocation:      "${config}/audit-${timestamp}.log",
	GUIAssetsLocation:     "${config}/gui",
	DefFolderLocation:     "${home}/Sync",
}

// expandLocations replaces the variables in the locations map with actual
// directory locations.
func expandLocations() error {
	for key, dir := range locations {
		for varName, value := range baseDirs {
			dir = strings.Replace(dir, "${"+string(varName)+"}", value, -1)
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
		if p := os.Getenv("LocationalAppData"); p != "" {
			return filepath.Join(p, "Syncthing")
		}
		return filepath.Join(os.Getenv("AppData"), "Syncthing")

	case "darwin":
		dir, err := fs.ExpandTilde("~/Library/Application Support/Syncthing")
		if err != nil {
			panic(err)
		}
		return dir

	default:
		if xdgCfg := os.Getenv("XDG_CONFIG_HOME"); xdgCfg != "" {
			return filepath.Join(xdgCfg, "syncthing")
		}
		dir, err := fs.ExpandTilde("~/.config/syncthing")
		if err != nil {
			panic(err)
		}
		return dir
	}
}

// homeDir returns the user's home directory, or dies trying.
func homeDir() string {
	home, err := fs.ExpandTilde("~")
	if err != nil {
		panic(err)
	}
	return home
}

func GetTimestampedLocation(key LocationationEnum) string {
	// We take the roundtrip via "${timestamp}" instead of passing the path
	// directly through time.Format() to avoid issues when the path we are
	// expanding contains numbers; otherwise for example
	// /home/user2006/.../panic-20060102-150405.log would get both instances of
	// 2006 replaced by 2015...
	tpl := locations[key]
	now := time.Now().Format("20060102-150405")
	return strings.Replace(tpl, "${timestamp}", now, -1)
}
