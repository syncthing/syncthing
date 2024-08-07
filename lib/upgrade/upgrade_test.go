// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build !noupgrade
// +build !noupgrade

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
	"github.com/syncthing/syncthing/lib/build"
)

var versions = []struct {
	a, b string
	r    Relation
}{
	{"0.1.2", "0.1.2", Equal},
	{"0.1.3", "0.1.2", Newer},
	{"0.1.1", "0.1.2", Older},
	{"0.3.0", "0.1.2", Newer},
	{"0.0.9", "0.1.2", Older},
	{"1.3.0", "1.1.2", Newer},
	{"1.0.9", "1.1.2", Older},
	{"2.3.0", "1.1.2", MajorNewer},
	{"1.0.9", "2.1.2", MajorOlder},
	{"1.1.2", "0.1.2", Newer},
	{"0.1.2", "1.1.2", Older},
	{"2.1.2", "0.1.2", MajorNewer},
	{"0.1.2", "2.1.2", MajorOlder},
	{"0.1.10", "0.1.9", Newer},
	{"0.10.0", "0.2.0", Newer},
	{"30.10.0", "4.9.0", MajorNewer},
	{"0.9.0-beta7", "0.9.0-beta6", Newer},
	{"0.9.0-beta7", "1.0.0-alpha", Older},
	{"1.0.0-alpha", "1.0.0-alpha.1", Older},
	{"1.0.0-alpha.1", "1.0.0-alpha.beta", Older},
	{"1.0.0-alpha.beta", "1.0.0-beta", Older},
	{"1.0.0-beta", "1.0.0-beta.2", Older},
	{"1.0.0-beta.2", "1.0.0-beta.11", Older},
	{"1.0.0-beta.11", "1.0.0-rc.1", Older},
	{"1.0.0-rc.1", "1.0.0", Older},
	{"1.0.0+45", "1.0.0+23-dev-foo", Equal},
	{"1.0.0-beta.23+45", "1.0.0-beta.23+23-dev-foo", Equal},
	{"1.0.0-beta.3+99", "1.0.0-beta.24+0", Older},

	{"v1.1.2", "1.1.2", Equal},
	{"v1.1.2", "V1.1.2", Equal},
	{"1.1.2", "V1.1.2", Equal},
}

func TestCompareVersions(t *testing.T) {
	for _, v := range versions {
		if r := CompareVersions(v.a, v.b); r != v.r {
			t.Errorf("compareVersions(%q, %q): %d != %d", v.a, v.b, r, v.r)
		}
	}
}

func TestErrorRelease(t *testing.T) {
	_, err := SelectLatestRelease(nil, "v0.11.0-beta", false)
	if err == nil {
		t.Error("Should return an error when no release were available")
	}
}

func TestSelectedRelease(t *testing.T) {
	testcases := []struct {
		current      string
		upgradeToPre bool
		candidates   []string
		selected     string
	}{
		// Within the same "major" (minor, in this case) select the newest
		{"v0.12.24", false, []string{"v0.12.23", "v0.12.24", "v0.12.25", "v0.12.26"}, "v0.12.26"},
		{"v0.12.24", false, []string{"v0.12.23", "v0.12.24", "v0.12.25", "v0.13.0"}, "v0.13.0"},
		{"v0.12.24", false, []string{"v0.12.23", "v0.12.24", "v0.12.25", "v1.0.0"}, "v1.0.0"},
		// Do no select beta versions when we are not allowed to
		{"v0.12.24", false, []string{"v0.12.26", "v0.12.27-beta.42"}, "v0.12.26"},
		{"v0.12.24-beta.0", false, []string{"v0.12.26", "v0.12.27-beta.42"}, "v0.12.26"},
		// Do select beta versions when we can
		{"v0.12.24", true, []string{"v0.12.26", "v0.12.27-beta.42"}, "v0.12.27-beta.42"},
		{"v0.12.24-beta.0", true, []string{"v0.12.26", "v0.12.27-beta.42"}, "v0.12.27-beta.42"},
		// Select the best within the current major when there is a minor upgrade available
		{"v0.12.24", false, []string{"v1.12.23", "v1.12.24", "v1.14.2", "v2.0.0"}, "v1.14.2"},
		{"v1.12.24", false, []string{"v1.12.23", "v1.12.24", "v1.14.2", "v2.0.0"}, "v1.14.2"},
		// Select the next major when we are at the best minor
		{"v0.12.25", true, []string{"v0.12.23", "v0.12.24", "v0.12.25", "v0.13.0"}, "v0.13.0"},
		{"v1.14.2", true, []string{"v0.12.23", "v0.12.24", "v1.14.2", "v2.0.0"}, "v2.0.0"},
	}

	for i, tc := range testcases {
		// Prepare a list of candidate releases
		var rels []Release
		for _, c := range tc.candidates {
			rels = append(rels, Release{
				Tag:        c,
				Prerelease: strings.Contains(c, "-"),
				Assets: []Asset{
					// There must be a matching asset or it will not get selected
					{Name: releaseNames(c)[0]},
				},
			})
		}

		// Check the selection
		sel, err := SelectLatestRelease(rels, tc.current, tc.upgradeToPre)
		if err != nil {
			t.Fatal("Unexpected error:", err)
		}
		if sel.Tag != tc.selected {
			t.Errorf("Test case %d: expected %s to be selected, but got %s", i, tc.selected, sel.Tag)
		}
	}
}

func TestSelectedReleaseMacOS(t *testing.T) {
	if !build.IsDarwin {
		t.Skip("macOS only")
	}

	// The alternatives that we expect should work
	assetNames := []string{
		fmt.Sprintf("syncthing-macos-%s-v0.14.47.tar.gz", runtime.GOARCH),
		fmt.Sprintf("syncthing-macosx-%s-v0.14.47.tar.gz", runtime.GOARCH),
	}

	for _, assetName := range assetNames {
		// Provide one release with the given asset name
		rels := []Release{
			{
				Tag:        "v0.14.47",
				Prerelease: false,
				Assets: []Asset{
					{Name: assetName},
				},
			},
		}

		// Check that it is selected and the asset is as expected
		sel, err := SelectLatestRelease(rels, "v0.14.46", false)
		if err != nil {
			t.Fatal("Unexpected error:", err)
		}
		if sel.Tag != "v0.14.47" {
			t.Error("wrong tag selected:", sel.Tag)
		}
		if sel.Assets[0].Name != assetName {
			t.Error("wrong asset selected:", sel.Assets[0].Name)
		}
	}
}

func TestCompatibilityJson(t *testing.T) {
	compatibilityPath := filepath.Join("..", "..", compatibilityJson)
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
		t.Errorf("%s: %q contains spaces", compatibilityJson, compInfo.Runtime)
	}
	// If we're running inside a GitHub action, we need to compare the go
	// version in compatibility.json with both the current runtime version, and
	// the previous one, as build-syncthing.yaml runs our unit tests with
	// both versions.
	want := Newer
	if os.Getenv("CI") != "" {
		want = Older
	}
	crt := strings.ReplaceAll(compInfo.Runtime, "go", "")
	rt := strings.ReplaceAll(runtime.Version(), "go", "")
	// Strip off the patch level, as go has only ever dropped support when
	// bumping the minor version number.
	rt = rt[:4]
	cmp := CompareVersions(crt, rt)
	if cmp != Equal && cmp != want {
		t.Errorf("Got go version %s, want %s in %s", crt, rt, compatibilityJson)
	}

	for hostArch, minOSVersion := range compInfo.MinOSVersion {
		if hostArch != strings.ReplaceAll(hostArch, " ", "") {
			t.Errorf("%s: %q contains spaces", compatibilityJson, hostArch)
		}
		if minOSVersion != strings.ReplaceAll(minOSVersion, " ", "") {
			t.Errorf("%s: %q contains spaces", compatibilityJson, minOSVersion)
		}
		if strings.Count(minOSVersion, ".") > 2 {
			t.Errorf("%s: %q contains more than two periods, which is not supported by CompareVersions()", compatibilityJson, minOSVersion)
		}
	}

	err = verifyCompatibility(comp)
	if err != nil {
		t.Errorf("%s: %s", compatibilityJson, err)
	}
}

func TestCompatibilityVerify(t *testing.T) {
	currentOSVersion, err := host.KernelVersion()
	if err != nil {
		t.Error(err)
	}
	// KernelVersion() returns '10.0.22631.3880 Build 22631.3880' on Windows
	ver, _, _ := strings.Cut(currentOSVersion, " ")
	tpl := `{"runtime": "%s", "minOSVersion": {"%s": "%s"}}`

	comp := fmt.Sprintf(tpl, runtime.Version(), runtime.GOOS, ver)
	err = verifyCompatibility([]byte(comp))
	if err != nil {
		t.Errorf("%s: %q", err, comp)
	}

	comp = fmt.Sprintf(tpl, runtime.Version(), runtime.GOOS+"/"+runtime.GOARCH, ver)
	err = verifyCompatibility([]byte(comp))
	if err != nil {
		t.Errorf("%s: %q", err, comp)
	}

	before, after, _ := strings.Cut(ver, ".")
	major, err := strconv.Atoi(before)
	if err != nil {
		t.Errorf("Invalid int in %q", currentOSVersion)
	}
	major++
	ver = fmt.Sprintf("%d.%s", major, after)
	comp = fmt.Sprintf(tpl, runtime.Version(), runtime.GOOS, ver)
	err = verifyCompatibility([]byte(comp))
	if err == nil {
		t.Errorf("got nil, expected an error, as our OS version is %s: %q", currentOSVersion, comp)
	}

	major -= 2
	ver = fmt.Sprintf("%d.%s", major, after)
	comp = fmt.Sprintf(tpl, runtime.Version(), runtime.GOOS, ver)
	err = verifyCompatibility([]byte(comp))
	if err != nil {
		t.Errorf("%s as our kernel is %s: %q", err, currentOSVersion, comp)
	}
}
