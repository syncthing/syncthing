// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package upgrade

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// CompatibilityJson is the name of the file generated to be included in
	// the release bundle (.tar.gz or .zip).
	CompatibilityJson = "compatibility.json"
	// CompatibilityYaml is the name of the file whose content is included in
	// the upgrade server's /meta.json response.
	CompatibilityYaml = "compatibility.yaml"
)

// GetMinOSVersions reads compatibility.yaml and returns the MinOSVersions found.
func GetMinOSVersions(dir string) (MinOSVersions, error) {
	path, err := findCompatibilityYaml(dir)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	minOSVersions := MinOSVersions{}
	if err := yaml.Unmarshal(data, &minOSVersions); err != nil {
		return nil, err
	}

	return fillMinOSVersions(minOSVersions), nil
}

func (mm *MinOSVersions) UnmarshalYAML(value *yaml.Node) error {
	var temp RuntimeInfos
	if err := value.Decode(&temp); err != nil {
		return err
	}

	if *mm == nil {
		*mm = make(MinOSVersions, len(temp))
	}

	for _, entry := range temp {
		if entry.MinOSVersion == nil {
			entry.MinOSVersion = make(MinOSVersion)
		}
		(*mm)[entry.Runtime] = entry.MinOSVersion
	}

	return nil
}

// findCompatibilityYaml searches for compatibility.yaml in startDir, and all
// parent directories of startDir, and returns the first match found.
// If startDir is empty it searches from the current directory up.
// If still not found, it searches from the executable's directory, and up.
func findCompatibilityYaml(startDir string) (string, error) {
	dir := startDir

	var err error

	if dir != "" {
		return findFile(dir, CompatibilityYaml)
	}

	dir, err = os.Getwd()
	if err != nil {
		return "", err
	}

	path, err := findFile(dir, CompatibilityYaml)
	if err == nil {
		return path, nil
	}

	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	dir = filepath.Dir(exe)

	return findFile(dir, CompatibilityYaml)
}

func findFile(startDir string, filename string) (string, error) {
	dir := startDir

	for {
		path := filepath.Join(dir, filename)

		_, err := os.Stat(path)
		if err == nil {
			return path, nil
		}

		parentDir := filepath.Dir(dir)

		if parentDir == dir {
			return "", fmt.Errorf("cannot find %q in, or under, %q", filename, startDir)
		}

		dir = parentDir
	}
}

// fillMinOSVersions copies any settings from each previous runtime entry into
// the next runtime entry, if missing in that entry.
func fillMinOSVersions(minOSVersions MinOSVersions) MinOSVersions {
	keys := make([]string, 0, len(minOSVersions))

	for key := range minOSVersions {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	info0 := make(MinOSVersion)
	for k, v := range minOSVersions[keys[0]] {
		info0[k] = v
	}

	for _, rt := range keys[1:] {
		for k0, v0 := range info0 {
			v, ok := minOSVersions[rt][k0]
			if ok {
				info0[k0] = v
			} else {
				minOSVersions[rt][k0] = v0
			}
		}
	}

	return minOSVersions
}

// saveCompatibilityJson saves a compatibility.json for the runtime version rt.
func saveCompatibilityJson(dir string, minOSVersions MinOSVersions, rt string) error {
	majorMinor, err := normalizeRuntimeVersion(rt)
	if err != nil {
		return err
	}
	minOSVersion, ok := minOSVersions[majorMinor]
	if !ok {
		return fmt.Errorf("runtime %v not found in %q", majorMinor, CompatibilityYaml)
	}

	runtimeInfo := RuntimeInfo{
		Runtime:      majorMinor,
		MinOSVersion: minOSVersion,
	}

	data, err := json.Marshal(runtimeInfo)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, CompatibilityJson)

	return os.WriteFile(path, data, 0o644)
}

// GenerateCompatibilityJson generates compatibility.json for the
// runtime.Version() entry found in compatibility.yaml.
func GenerateCompatibilityJson(dir string) error {
	return genCompatibilityJson(dir, runtime.Version())
}

func genCompatibilityJson(dir string, rt string) error {
	minOSVersions, err := GetMinOSVersions(dir)
	if err != nil {
		return err
	}

	return saveCompatibilityJson(dir, minOSVersions, rt)
}

// normalizeRuntimeVersion strips off the runtime.Version()'s patch level, so
// "go1.22.5" becomes "go1.22".
func normalizeRuntimeVersion(rt string) (string, error) {
	parts := strings.Split(rt, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("go version %q is not in the form gox.y", rt)
	}

	return strings.Join(parts[:2], "."), nil
}

// normalizeKernelVersion strips off anything after the kernel version's patch
// level. So Windows' "10.0.22631.3880 Build 22631.3880" becomes "10.0.22631",
// and Linux's "6.8.0-39-generic" becomes "6.8.0".
func normalizeKernelVersion(ver string) string {
	ver, _, _ = strings.Cut(ver, " ")
	ver, _, _ = strings.Cut(ver, "-")

	parts := strings.Split(ver, ".")
	maxParts := min(len(parts), 3)
	return strings.Join(parts[:maxParts], ".")
}
