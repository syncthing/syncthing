// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build !noupgrade && release
// +build !noupgrade,release

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

	path := filepath.Join(baseDir, CompatibilityJson)
	os.Remove(path)
	defer os.Remove(path)

	minOSVersions, err := GetMinOSVersions(baseDir)
	if err != nil {
		t.Error(err)
	}

	for rt := range minOSVersions {
		err := genCompatibilityJson(baseDir, rt)
		if err != nil {
			t.Error(err)
		}

		path := filepath.Join(baseDir, CompatibilityJson)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Error(err)
		}

		var runtimeInfo RuntimeInfo
		err = json.Unmarshal(data, &runtimeInfo)
		if err != nil {
			t.Error(err)
		}
		if runtimeInfo.Runtime != rt {
			t.Errorf("Got %q, expected %q in %q", runtimeInfo.Runtime, rt, path)
		}

		if testing.Verbose() {
			t.Logf("%v:\n", runtimeInfo.Runtime)
			for k, v := range runtimeInfo.MinOSVersion {
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

	path := filepath.Join(baseDir, CompatibilityJson)
	os.Remove(path)
	defer os.Remove(path)

	err = GenerateCompatibilityJson(baseDir)
	if err != nil {
		t.Error(err)
	}

	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Error(err)
	}

	var runtimeInfo RuntimeInfo
	err = json.Unmarshal(bytes, &runtimeInfo)
	if err != nil {
		t.Error(err)
	}
	if runtimeInfo.Runtime != strings.TrimSpace(runtimeInfo.Runtime) {
		t.Errorf("%s: %q contains spaces", CompatibilityJson, runtimeInfo.Runtime)
	}
	// If we're running inside a GitHub action, we need to compare the go
	// version in compatibility.json with both the current runtime version, and
	// the previous one, as build-syncthing.yaml runs our unit tests with
	// both versions.
	want := Equal
	if os.Getenv("CI") != "" {
		want = Newer
	}
	crt := strings.ReplaceAll(runtimeInfo.Runtime, "go", "")
	rt := strings.ReplaceAll(runtime.Version(), "go", "")
	// Strip off the patch level, as go has only ever dropped support when
	// bumping the minor version number.
	majorMinor, err := normalizeRuntimeVersion(rt)
	if err != nil {
		t.Fatal(err)
	}
	cmp := CompareVersions(crt, majorMinor)
	if cmp != Equal && cmp != want {
		t.Errorf("Got go version %q, want %q in %q", crt, majorMinor, CompatibilityJson)
	}

	for hostArch, minOSVersion := range runtimeInfo.MinOSVersion {
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

	// rel := Release{MinOSVersions: MinOSVersions{runtimeInfo.MinOSVersion}}
	// err = verifyCompatibility(rel, runtimeInfo.Runtime)
	// if err != nil {
	// 	t.Errorf("%s: %s", CompatibilityJson, err)
	// }
}

func TestCompatibilityVerify(t *testing.T) {
	majorMinor, err := normalizeRuntimeVersion(runtime.Version())
	if err != nil {
		t.Fatal(err)
	}

	currentKernel, err := host.KernelVersion()
	if err != nil {
		t.Error(err)
	}
	ver := normalizeKernelVersion(currentKernel)

	rel := Release{MinOSVersions: MinOSVersions{
		majorMinor: MinOSVersion{runtime.GOOS: ver},
	}}
	err = verifyCompatibility(rel, majorMinor)
	if err != nil {
		t.Errorf("verifyCompatibility(%+v, %s): got %q, expected nil", rel.MinOSVersions, majorMinor, err)
	}

	rel.MinOSVersions = MinOSVersions{
		majorMinor: MinOSVersion{runtime.GOOS + "/" + runtime.GOARCH: ver},
	}
	err = verifyCompatibility(rel, majorMinor)
	if err != nil {
		t.Errorf("verifyCompatibility(%+v, %s): got %q, expected nil", rel.MinOSVersions, majorMinor, err)
	}

	before, after, _ := strings.Cut(ver, ".")
	major, err := strconv.Atoi(before)
	if err != nil {
		t.Errorf("Invalid int in %q", currentKernel)
	}
	major++
	rel.MinOSVersions = MinOSVersions{
		majorMinor: MinOSVersion{runtime.GOOS: fmt.Sprintf("%d.%s", major, after)},
	}
	err = verifyCompatibility(rel, majorMinor)
	if err == nil {
		t.Errorf("verifyCompatibility(%+v, %s): got nil, expected an error, as our OS kernel is %q",
			rel.MinOSVersions, majorMinor, currentKernel)
	}

	major -= 2
	rel.MinOSVersions = MinOSVersions{
		majorMinor: MinOSVersion{runtime.GOOS: fmt.Sprintf("%d.%s", major, after)},
	}
	err = verifyCompatibility(rel, majorMinor)
	if err != nil {
		t.Errorf("verifyCompatibility(%+v, %s): got %q, expected nil, as our OS kernel is %q",
			rel.MinOSVersions, majorMinor, err, currentKernel)
	}
}
