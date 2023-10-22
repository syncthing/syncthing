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

	"github.com/syncthing/syncthing/lib/build"
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
	GUIAssets     LocationEnum = "guiAssets"
	DefFolder     LocationEnum = "defFolder"
)

type BaseDirEnum string

const (
	// Overridden by --home flag, $STHOMEDIR, --config flag, or $STCONFDIR
	ConfigBaseDir BaseDirEnum = "config"
	// Overridden by --home flag, $STHOMEDIR, --data flag, or $STDATADIR
	DataBaseDir BaseDirEnum = "data"

	// User's home directory, *not* --home flag
	UserHomeBaseDir BaseDirEnum = "userHome"

	LevelDBDir          = "index-v0.14.0.db"
	configFileName      = "config.xml"
	defaultStateDir     = ".local/state/syncthing"
	oldDefaultConfigDir = ".config/syncthing"
)

// Platform dependent directories
var baseDirs = make(map[BaseDirEnum]string, 3)

func init() {
	userHome := userHomeDir()
	config := defaultConfigDir(userHome)
	data := defaultDataDir(userHome, config)

	baseDirs[UserHomeBaseDir] = userHome
	baseDirs[ConfigBaseDir] = config
	baseDirs[DataBaseDir] = data

	if err := expandLocations(); err != nil {
		fmt.Println(err)
		panic("Failed to expand locations at init time")
	}
}

// Set overrides a location to the given path, making sure to it points to an
// absolute path first.  Only the special "-" value will be used verbatim.
func Set(locationName LocationEnum, path string) error {
	if !filepath.IsAbs(path) && path != "-" {
		var err error
		path, err = filepath.Abs(path)
		if err != nil {
			return err
		}
	}
	_, ok := locationTemplates[locationName]
	if !ok {
		return fmt.Errorf("unknown location: %s", locationName)
	}
	locations[locationName] = filepath.Clean(path)
	return nil
}

func SetBaseDir(baseDirName BaseDirEnum, path string) error {
	if !filepath.IsAbs(path) {
		var err error
		path, err = filepath.Abs(path)
		if err != nil {
			return err
		}
	}
	if _, ok := baseDirs[baseDirName]; !ok {
		return fmt.Errorf("unknown base dir: %s", baseDirName)
	}
	baseDirs[baseDirName] = filepath.Clean(path)
	return expandLocations()
}

func Get(location LocationEnum) string {
	return locations[location]
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
	LogFile:       "${data}/syncthing.log", // --logfile on Windows
	CsrfTokens:    "${data}/csrftokens.txt",
	PanicLog:      "${data}/panic-${timestamp}.log",
	AuditLog:      "${data}/audit-${timestamp}.log",
	GUIAssets:     "${config}/gui",
	DefFolder:     "${userHome}/Sync",
}

var locations = make(map[LocationEnum]string)

// expandLocations replaces the variables in the locations map with actual
// directory locations.
func expandLocations() error {
	newLocations := make(map[LocationEnum]string)
	for key, dir := range locationTemplates {
		dir = os.Expand(dir, func(s string) string {
			return baseDirs[BaseDirEnum(s)]
		})
		var err error
		dir, err = fs.ExpandTilde(dir)
		if err != nil {
			return err
		}
		newLocations[key] = filepath.Clean(dir)
	}
	locations = newLocations
	return nil
}

// ListExpandedPaths returns a machine-readable mapping of the currently configured locations.
func ListExpandedPaths() map[string]string {
	res := make(map[string]string, len(locations))
	for key, path := range baseDirs {
		res["baseDir-"+string(key)] = path
	}
	for key, path := range locations {
		res[string(key)] = path
	}
	return res
}

// PrettyPaths returns a nicely formatted, human-readable listing
func PrettyPaths() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Configuration file:\n\t%s\n\n", Get(ConfigFile))
	fmt.Fprintf(&b, "Device private key & certificate files:\n\t%s\n\t%s\n\n", Get(KeyFile), Get(CertFile))
	fmt.Fprintf(&b, "GUI / API HTTPS private key & certificate files:\n\t%s\n\t%s\n\n", Get(HTTPSKeyFile), Get(HTTPSCertFile))
	fmt.Fprintf(&b, "Database location:\n\t%s\n\n", Get(Database))
	fmt.Fprintf(&b, "Log file:\n\t%s\n\n", Get(LogFile))
	fmt.Fprintf(&b, "GUI override directory:\n\t%s\n\n", Get(GUIAssets))
	fmt.Fprintf(&b, "CSRF tokens file:\n\t%s\n\n", Get(CsrfTokens))
	fmt.Fprintf(&b, "Default sync folder directory:\n\t%s\n\n", Get(DefFolder))
	return b.String()
}

// defaultConfigDir returns the default configuration directory, as figured
// out by various the environment variables present on each platform, or dies
// trying.
func defaultConfigDir(userHome string) string {
	switch runtime.GOOS {
	case build.Windows:
		if p := os.Getenv("LocalAppData"); p != "" {
			return filepath.Join(p, "Syncthing")
		}
		return filepath.Join(os.Getenv("AppData"), "Syncthing")

	case build.Darwin:
		return filepath.Join(userHome, "Library/Application Support/Syncthing")

	default:
		// Legacy: if our config exists under $XDG_CONFIG_HOME/syncthing, use that
		if xdgCfg := os.Getenv("XDG_CONFIG_HOME"); xdgCfg != "" {
			candidate := filepath.Join(xdgCfg, "syncthing")
			if _, err := os.Lstat(filepath.Join(candidate, configFileName)); err == nil {
				return candidate
			}
		}
		// Legacy: if our config exists under ~/.config/syncthing, use that
		candidate := filepath.Join(userHome, oldDefaultConfigDir)
		if _, err := os.Lstat(filepath.Join(candidate, configFileName)); err == nil {
			return candidate
		}
		// If XDG_STATE_HOME is set, use that
		if xdgState := os.Getenv("XDG_STATE_HOME"); xdgState != "" {
			return filepath.Join(xdgState, "syncthing")
		}
		// Use our default
		return filepath.Join(userHome, defaultStateDir)
	}
}

// defaultDataDir returns the default data directory, where we store the
// database, log files, etc.
func defaultDataDir(userHome, config string) string {
	if build.IsWindows || build.IsDarwin {
		return config
	}

	// Legacy: if a database exists at the config location, use that.
	if _, err := os.Lstat(filepath.Join(config, LevelDBDir)); err == nil {
		return config
	}
	// Legacy: if a database exists under $XDG_DATA_HOME/syncthing, use that
	if xdgData := os.Getenv("XDG_DATA_HOME"); xdgData != "" {
		candidate := filepath.Join(xdgData, "syncthing")
		if _, err := os.Lstat(filepath.Join(candidate, LevelDBDir)); err == nil {
			return candidate
		}
	}
	// Legacy: if a database exists under ~/.config/syncthing, use that
	candidate := filepath.Join(userHome, oldDefaultConfigDir)
	if _, err := os.Lstat(filepath.Join(candidate, LevelDBDir)); err == nil {
		return candidate
	}
	// If XDG_STATE_HOME is set, use that
	if xdgState := os.Getenv("XDG_STATE_HOME"); xdgState != "" {
		return filepath.Join(xdgState, "syncthing")
	}
	// Use our default
	return filepath.Join(userHome, defaultStateDir)
}

// userHomeDir returns the user's home directory, or dies trying.
func userHomeDir() string {
	userHome, err := fs.ExpandTilde("~")
	if err != nil {
		fmt.Println(err)
		panic("Failed to get user home dir")
	}
	return userHome
}

func GetTimestamped(key LocationEnum) string {
	// We take the roundtrip via "${timestamp}" instead of passing the path
	// directly through time.Format() to avoid issues when the path we are
	// expanding contains numbers; otherwise for example
	// /home/user2006/.../panic-20060102-150405.log would get both instances of
	// 2006 replaced by 2015...
	tpl := locations[key]
	now := time.Now().Format("20060102-150405")
	return strings.ReplaceAll(tpl, "${timestamp}", now)
}
