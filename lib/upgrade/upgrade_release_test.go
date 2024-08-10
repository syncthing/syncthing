// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build !noupgrade || release
// +build !noupgrade release

package upgrade

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/shirou/gopsutil/v4/host"
)

func TestCompatibilityGenerate(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Error(err)
	}
	baseDir := filepath.Clean(filepath.Join(cwd, "..", ".."))

	compatibilityPath := filepath.Join(baseDir, CompatibilityJson)
	os.Remove(compatibilityPath)
	defer os.Remove(compatibilityPath)

	compInfos, err := loadCompatibilityYaml(baseDir)
	if err != nil {
		t.Error(err)
	}

	for rt := range compInfos {
		err := generateCompatibilityJson(baseDir, rt)
		if err != nil {
			t.Error(err)
		}

		compatibilityPath := filepath.Join(baseDir, CompatibilityJson)
		comp, err := os.ReadFile(compatibilityPath)
		if err != nil {
			t.Error(err)
		}

		var compInfo CompInfo
		err = json.Unmarshal(comp, &compInfo)
		if err != nil {
			t.Error(err)
		}
		if compInfo.Runtime != rt {
			t.Errorf("Got %q, expected %q in %q", compInfo.Runtime, rt, compatibilityPath)
		}

		if testing.Verbose() {
			t.Logf("%v:\n", compInfo.Runtime)
			for k, v := range compInfo.MinOSVersion {
				t.Logf("  %v: %v\n", k, v)
			}
		}
	}
}

func TestCompatibilityJson(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Error(err)
	}
	baseDir := filepath.Clean(filepath.Join(cwd, "..", ".."))

	compatibilityPath := filepath.Join(baseDir, CompatibilityJson)
	os.Remove(compatibilityPath)
	defer os.Remove(compatibilityPath)

	err = GenerateCompatibilityJson(baseDir)
	if err != nil {
		t.Error(err)
	}

	comp, err := os.ReadFile(compatibilityPath)
	if err != nil {
		t.Error(err)
	}

	var compInfo CompInfo
	err = json.Unmarshal(comp, &compInfo)
	if err != nil {
		t.Error(err)
	}
	if compInfo.Runtime != strings.TrimSpace(compInfo.Runtime) {
		t.Errorf("%s: %q contains spaces", CompatibilityJson, compInfo.Runtime)
	}
	// If we're running inside a GitHub action, we need to compare the go
	// version in compatibility.json with both the current runtime version, and
	// the previous one, as build-syncthing.yaml runs our unit tests with
	// both versions.
	want := Equal
	if os.Getenv("CI") != "" {
		want = Newer
	}
	crt := strings.ReplaceAll(compInfo.Runtime, "go", "")
	rt := strings.ReplaceAll(runtime.Version(), "go", "")
	// Strip off the patch level, as go has only ever dropped support when
	// bumping the minor version number.
	parts := strings.Split(rt, ".")
	if len(parts) < 2 {
		t.Errorf("Go version %q is not in the form goX.Y", rt)
	}
	majorMinor := strings.Join(parts[:2], ".")
	cmp := CompareVersions(crt, majorMinor)
	if cmp != Equal && cmp != want {
		t.Errorf("Got go version %q, want %q in %q", crt, majorMinor, CompatibilityJson)
	}

	for hostArch, minOSVersion := range compInfo.MinOSVersion {
		if hostArch != strings.ReplaceAll(hostArch, " ", "") {
			t.Errorf("%s: %q contains spaces", CompatibilityJson, hostArch)
		}
		if minOSVersion != strings.ReplaceAll(minOSVersion, " ", "") {
			t.Errorf("%s: %q contains spaces", CompatibilityJson, minOSVersion)
		}
		if strings.Count(minOSVersion, ".") > 2 {
			t.Errorf("%s: %q contains more than two periods, which is not supported by CompareVersions()", CompatibilityJson, minOSVersion)
		}
	}

	err = verifyCompatibility(comp)
	if err != nil {
		t.Errorf("%s: %s", CompatibilityJson, err)
	}
}

func TestCompatibilityVerify(t *testing.T) {
	currentOSVersion, err := host.KernelVersion()
	if err != nil {
		t.Error(err)
	}
	// KernelVersion() returns '10.0.22631.3880 Build 22631.3880' on Windows
	ver, _, _ := strings.Cut(currentOSVersion, " ")
	compInfo := CompInfo{runtime.Version(), map[string]string{runtime.GOOS: ver}}
	bytes, err := json.Marshal(compInfo)
	if err != nil {
		t.Error(err)
	}

	err = verifyCompatibility(bytes)
	if err != nil {
		t.Errorf("%s: %q", err, string(bytes))
	}

	compInfo = CompInfo{runtime.Version(), map[string]string{runtime.GOOS + "/" + runtime.GOARCH: ver}}
	bytes, err = json.Marshal(compInfo)
	if err != nil {
		t.Error(err)
	}
	err = verifyCompatibility(bytes)
	if err != nil {
		t.Errorf("%s: %q", err, string(bytes))
	}

	before, after, _ := strings.Cut(ver, ".")
	major, err := strconv.Atoi(before)
	if err != nil {
		t.Errorf("Invalid int in %q", currentOSVersion)
	}
	major++
	ver = fmt.Sprintf("%d.%s", major, after)
	compInfo = CompInfo{runtime.Version(), map[string]string{runtime.GOOS: ver}}
	bytes, err = json.Marshal(compInfo)
	if err != nil {
		t.Error(err)
	}
	err = verifyCompatibility(bytes)
	if err == nil {
		t.Errorf("got nil, expected an error, as our OS version is %s: %q", currentOSVersion, string(bytes))
	}

	major -= 2
	ver = fmt.Sprintf("%d.%s", major, after)
	compInfo = CompInfo{runtime.Version(), map[string]string{runtime.GOOS: ver}}
	bytes, err = json.Marshal(compInfo)
	if err != nil {
		t.Error(err)
	}
	err = verifyCompatibility(bytes)
	if err != nil {
		t.Errorf("%s as our kernel is %s: %q", err, currentOSVersion, string(bytes))
	}
}
