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

func baseDir(t *testing.T) string {
	t.Helper()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	return filepath.Clean(filepath.Join(cwd, "..", ".."))
}

func TestCompatGenerateCompatJson(t *testing.T) {
	baseDir := baseDir(t)

	path := filepath.Join(baseDir, CompatJson)
	defer os.Remove(path)

	err := GenerateCompatJson(baseDir)
	if err != nil {
		t.Fatal(err)
	}

	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var runtimeReqs RuntimeReqs
	err = json.Unmarshal(bytes, &runtimeReqs)
	if err != nil {
		t.Fatal(err)
	}
	if runtimeReqs.Runtime != strings.TrimSpace(runtimeReqs.Runtime) {
		t.Errorf("%q: %q contains spaces", CompatJson, runtimeReqs.Runtime)
	}
	if runtimeReqs.Requirements == nil {
		t.Errorf("%q: runtimeReqs.Requirements is nil", CompatJson)
	}

	// If we're running inside a GitHub action, we need to compare the go
	// version in compat.json with both the current runtime version, and
	// the previous one, as build-syncthing.yaml runs our unit tests with
	// both versions.
	want := Equal
	if os.Getenv("CI") != "" {
		want = Newer
	}
	reqRuntime := strings.ReplaceAll(runtimeReqs.Runtime, "go", "")
	myRuntime := strings.ReplaceAll(runtime.Version(), "go", "")
	// Strip off the patch level, as go has only ever dropped support when
	// bumping the minor version number.
	majorMinor := normalizeRuntimeVersion(myRuntime)
	cmp := CompareVersions(reqRuntime, majorMinor)
	if cmp != Equal && cmp != want {
		t.Errorf("Got go version %q, want %q in %q", reqRuntime, majorMinor, CompatJson)
	}

	for hostArch, requirements := range runtimeReqs.Requirements {
		if hostArch != strings.ReplaceAll(hostArch, " ", "") {
			t.Errorf("%s: %q contains spaces", CompatJson, hostArch)
		}
		if requirements != strings.ReplaceAll(requirements, " ", "") {
			t.Errorf("%s: %q contains spaces", CompatJson, requirements)
		}
		if strings.Count(requirements, ".") > 2 {
			t.Errorf("%s: %q contains more than two periods, which is not supported by CompareVersions()", CompatJson, requirements)
		}
	}
}

func TestCompatGetRequirementsMap(t *testing.T) {
	baseDir := baseDir(t)

	requirementsMap, err := getRequirementsMap(baseDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(requirementsMap) == 0 {
		t.Fatal("No entries in " + CompatYaml)
	}

	for goRuntime, requirements := range requirementsMap {
		if goRuntime != strings.ReplaceAll(goRuntime, " ", "") {
			t.Errorf("%s: %q contains spaces", CompatYaml, goRuntime)
		}
		for hostArch, osVersion := range requirements {
			if hostArch != strings.ReplaceAll(hostArch, " ", "") {
				t.Errorf("%s: %q: %q contains spaces", CompatYaml, goRuntime, hostArch)
			}
			if strings.Count(hostArch, "/") > 1 {
				t.Errorf("%s: %q: %q contains more than one slash", CompatYaml, goRuntime, hostArch)
			}
			if osVersion != strings.ReplaceAll(osVersion, " ", "") {
				t.Errorf("%s: %q: %q contains spaces", CompatYaml, goRuntime, osVersion)
			}
			if strings.Count(osVersion, ".") > 2 {
				t.Errorf("%s: %q: %q contains more than two periods, which is not supported by CompareVersions()",
					CompatYaml, goRuntime, osVersion)
			}
		}
	}
}

func TestCompatGenerateCompatJsons(t *testing.T) {
	baseDir := baseDir(t)

	path := filepath.Join(baseDir, CompatJson)
	defer os.Remove(path)

	requirementsMap, err := getRequirementsMap(baseDir)
	if err != nil {
		t.Fatal(err)
	}

	for goRuntime := range requirementsMap {
		err := genCompatJson(baseDir, goRuntime)
		if err != nil {
			t.Error(err)
		}

		path := filepath.Join(baseDir, CompatJson)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Error(err)
		}

		var runtimeReqs RuntimeReqs
		err = json.Unmarshal(data, &runtimeReqs)
		if err != nil {
			t.Error(err)
		}
		if runtimeReqs.Runtime != goRuntime {
			t.Errorf("Got %q, expected %q in %q", runtimeReqs.Runtime, goRuntime, path)
		}

		if testing.Verbose() {
			t.Logf("%v:\n", runtimeReqs.Runtime)
			for k, v := range runtimeReqs.Requirements {
				t.Logf("  %v: %v\n", k, v)
			}
		}
	}
}

func TestCompatCheckOSCompatibility(t *testing.T) {
	currentKernel, err := host.KernelVersion()
	if err != nil {
		t.Fatal(err)
	}
	osVersion := normalizeKernelVersion(currentKernel)
	runtimeReqs := RuntimeReqs{
		Runtime:      normalizeRuntimeVersion(runtime.Version()),
		Requirements: Requirements{runtime.GOOS: osVersion},
	}

	err = checkOSCompatibility(runtimeReqs)
	if err != nil {
		t.Errorf("checkOSCompatibility(%+v): got %q, expected nil", runtimeReqs, err)
	}

	runtimeReqs.Requirements = Requirements{runtime.GOOS + "/" + runtime.GOARCH: osVersion}
	err = checkOSCompatibility(runtimeReqs)
	if err != nil {
		t.Errorf("checkOSCompatibility(%+v): got %q, expected nil", runtimeReqs, err)
	}

	before, after, _ := strings.Cut(osVersion, ".")
	major, err := strconv.Atoi(before)
	if err != nil {
		t.Errorf("Invalid int in %q", currentKernel)
	}
	major++
	runtimeReqs.Requirements = Requirements{runtime.GOOS: fmt.Sprintf("%d.%s", major, after)}
	err = checkOSCompatibility(runtimeReqs)
	if err == nil {
		t.Errorf("checkOSCompatibility(%+v): got nil, expected an error, as our OS kernel is %q",
			runtimeReqs, currentKernel)
	}

	major -= 2
	runtimeReqs.Requirements = Requirements{runtime.GOOS: fmt.Sprintf("%d.%s", major, after)}
	err = checkOSCompatibility(runtimeReqs)
	if err != nil {
		t.Errorf("checkOSCompatibility(%+v): got %q, expected nil, as our OS kernel is %q",
			runtimeReqs, err, currentKernel)
	}
}

func TestCompatNormalizeRuntimeVersion(t *testing.T) {
	var tests = []struct {
		input, want string
	}{
		{"go1.22", "go1.22"},
		{"go1.22.0", "go1.22"},
		{"go1.22.1", "go1.22"},
		{"go1.22.2.3", "go1.22"},
		{"go1.23rc1", "go1.23rc1"},
		{"go2", "go2"},
	}

	for _, test := range tests {
		got := normalizeRuntimeVersion(test.input)
		if got != test.want {
			t.Errorf("normalizeRuntimeVersion(%q): got %q, want %q", test.input, got, test.want)
		}
	}
}

func TestCompatNormalizeKernelVersion(t *testing.T) {
	var tests = []struct {
		input, want string
	}{
		{"1", "1"},
		{"1.2", "1.2"},
		{"1.2.3", "1.2.3"},
		{"1.2.3.4", "1.2.3"},
		{"1.2.3-4", "1.2.3"},
		{"10.0.22631.3880 Build 22631.3880", "10.0.22631"},
		{"6.8.0-39-generic", "6.8.0"},
	}

	for _, test := range tests {
		got := normalizeKernelVersion(test.input)
		if got != test.want {
			t.Errorf("normalizeKernelVersion(%q): got %q, want %q", test.input, got, test.want)
		}
	}
}
