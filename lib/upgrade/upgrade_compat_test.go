// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build !noupgrade
// +build !noupgrade

package upgrade

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/shirou/gopsutil/v4/host"
)

func TestCompatCheckOSCompatibility(t *testing.T) {
	currentKernel, err := host.KernelVersion()
	if err != nil {
		t.Fatal(err)
	}
	osVersion := normalizeKernelVersion(currentKernel)
	runtimeReqs := RuntimeReqs{
		Runtime:      runtime.Version(),
		Requirements: Requirements{runtime.GOOS: osVersion},
	}

	err = verifyRuntimeRequirements(runtimeReqs)
	if err != nil {
		t.Errorf("checkOSCompatibility(%+v): got %q, expected nil", runtimeReqs, err)
	}

	runtimeReqs.Requirements = Requirements{runtime.GOOS + "/" + runtime.GOARCH: osVersion}
	err = verifyRuntimeRequirements(runtimeReqs)
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
	err = verifyRuntimeRequirements(runtimeReqs)
	if err == nil {
		t.Errorf("checkOSCompatibility(%+v): got nil, expected an error, as our OS kernel is %q",
			runtimeReqs, currentKernel)
	}

	major -= 2
	runtimeReqs.Requirements = Requirements{runtime.GOOS: fmt.Sprintf("%d.%s", major, after)}
	err = verifyRuntimeRequirements(runtimeReqs)
	if err != nil {
		t.Errorf("checkOSCompatibility(%+v): got %q, expected nil, as our OS kernel is %q",
			runtimeReqs, err, currentKernel)
	}
}

func TestCompatNormalizeKernelVersion(t *testing.T) {
	tests := []struct {
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
