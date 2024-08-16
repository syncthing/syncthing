// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package upgrade

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/shirou/gopsutil/v4/host"
	"gopkg.in/yaml.v3"
)

const (
	// CompatJson is the name of the file generated to be included in
	// the release bundle (.tar.gz or .zip).
	CompatJson = "compat.json"
	// CompatYaml is the name of the file containing all of the minimum OS
	// versions (aka kernel version) for each version of go that is released.
	CompatYaml = "compat.yaml"

	osTooOldErr = "cannot upgrade, as the upgrade was compiled with %v " +
		"which requires OS version %v or later, but this system is currently " +
		"running version %v"
)

type tagRuntime struct {
	tag     string
	runtime string
}

// tagRuntimes defines the go version used to build each release, and all
// future releases, if not listed.
// It's only used when the STUPGRADETEST_RELEASESURL environment variable is set
// so our unit tests pass.
// Once we include compat.json in our release bundles, we can remove this logic.
var tagRuntimes = []tagRuntime{
	{"1.27.10", "go1.22"},
	{"1.27.12", "go1.23"}, // guesstimate
}

// FetchCompatJson downloads the compat.json file contained in a release bundle
// (.tar.gz or .zip) that's been published on GitHub. The "current" parameter is
// used for setting the User-Agent only.
func FetchCompatJson(rel Release, current string) (RuntimeReqs, error) {
	var err error
	runtimeReqs := RuntimeReqs{}

	for {
		var url string
		url, err = getCompatJsonURL(rel)
		if err != nil {
			break
		}

		var resp *http.Response
		resp, err = insecureGet(url, current)
		if err != nil {
			err = fmt.Errorf("couldn't fetch %s: %w", url, err)
			break
		}

		if resp.StatusCode > 299 {
			err = fmt.Errorf("error %v downloading %s", resp.Status, url)
		} else {
			err = json.NewDecoder(io.LimitReader(resp.Body, maxCompatJsonSize)).Decode(&runtimeReqs)
		}

		resp.Body.Close()

		break
	}

	if err != nil {
		// Once we include compat.json in our release bundles, we can remove this logic.
		if os.Getenv(releasesURLEnvVar) != "" {
			var err2 error
			runtimeReqs, err2 = getRuntimeReqsByReleaseTag(rel)
			if err2 == nil {
				err = nil
			}
		}
	}

	if err != nil {
		l.Infoln(err)
	}

	return runtimeReqs, err
}

// getCompatJsonURL finds the URL of the compat.json file in a release bundle.
func getCompatJsonURL(rel Release) (string, error) {
	for _, asset := range rel.Assets {
		assetName := path.Base(asset.Name)
		if assetName == CompatJson {
			return asset.URL, nil
		}
	}
	return "", fmt.Errorf(CompatJson + " not found in release")
}

// GenerateCompatJson generates compat.json for the runtime.Version() entry
// found in compat.yaml.
func GenerateCompatJson(dir string) error {
	return genCompatJson(dir, runtime.Version())
}

func genCompatJson(dir string, runtimeVer string) error {
	requirementsMap, err := getRequirementsMap(dir)
	if err != nil {
		return err
	}

	return saveCompatJson(dir, requirementsMap, runtimeVer)
}

// getRequirementsMap search dir for compat.yaml and returns the RequirementsMap
// found in it, after filling in any missing entries.
func getRequirementsMap(dir string) (RequirementsMap, error) {
	path, err := findCompatYaml(dir)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	requirementsMap := RequirementsMap{}
	if err := yaml.Unmarshal(data, &requirementsMap); err != nil {
		return nil, err
	}

	return fillRequirementsMap(requirementsMap), nil
}

// UnmarshalYAML converts a RuntimeReqsArray to a RequirementsMap.
func (mm *RequirementsMap) UnmarshalYAML(value *yaml.Node) error {
	var temp RuntimeReqsArray
	if err := value.Decode(&temp); err != nil {
		return err
	}

	if *mm == nil {
		*mm = make(RequirementsMap, len(temp))
	}

	for _, entry := range temp {
		if entry.Requirements == nil {
			entry.Requirements = make(Requirements)
		}
		(*mm)[entry.Runtime] = entry.Requirements
	}

	return nil
}

// findCompatYaml searches for compat.yaml in startDir, and all
// parent directories of startDir, and returns the first match found.
// If startDir is empty it searches from the current directory up.
// If still not found, it searches from the executable's directory, and up.
func findCompatYaml(startDir string) (string, error) {
	dir := startDir

	var err error

	if dir != "" {
		return findFile(dir, CompatYaml)
	}

	dir, err = os.Getwd()
	if err != nil {
		return "", err
	}

	path, err := findFile(dir, CompatYaml)
	if err == nil {
		return path, nil
	}

	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	dir = filepath.Dir(exe)

	return findFile(dir, CompatYaml)
}

// findFile searches for filename in startDir, and all parent directories of
// startDir, and returns the first match found.
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

// fillRequirementsMap copies any settings from each previous runtime entry into
// the next runtime entry, if missing in that entry.
func fillRequirementsMap(requirementsMap RequirementsMap) RequirementsMap {
	goRuntimes := make([]string, 0, len(requirementsMap))

	for goRuntime := range requirementsMap {
		goRuntimes = append(goRuntimes, goRuntime)
	}
	sort.Strings(goRuntimes)

	info0 := make(Requirements)
	for k, v := range requirementsMap[goRuntimes[0]] {
		info0[k] = v
	}

	for _, goRuntime := range goRuntimes[1:] {
		for k0, v0 := range info0 {
			v, ok := requirementsMap[goRuntime][k0]
			if ok {
				info0[k0] = v
			} else {
				requirementsMap[goRuntime][k0] = v0
			}
		}
	}

	return requirementsMap
}

// saveCompatJson saves a compat.json in dir for the runtime.Version() entry in
// compat.yaml.
func saveCompatJson(dir string, requirementsMap RequirementsMap, goRuntime string) error {
	majorMinor := normalizeRuntimeVersion(goRuntime)
	requirements, ok := requirementsMap[majorMinor]
	if !ok {
		return fmt.Errorf("runtime %v not found in %q", majorMinor, CompatYaml)
	}

	runtimeReqs := RuntimeReqs{
		Runtime:      majorMinor,
		Requirements: requirements,
	}

	data, err := json.Marshal(runtimeReqs)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, CompatJson)

	return os.WriteFile(path, data, 0o644)
}

// checkOSCompatibility returns an error if the OS's version (aka kernel version)
// is incompatible with the version of go used to build the release.
func checkOSCompatibility(runtimeReqs RuntimeReqs) error {
	l.Debugln("checking if OS is compatiable")

	var err error

	for {
		var currentKernel string
		currentKernel, err = host.KernelVersion()
		if err != nil {
			break
		}
		currentKernel = normalizeKernelVersion(currentKernel)

		if runtimeReqs.Requirements == nil {
			err = fmt.Errorf("Runtime info not provided in the upgrade server's response")
			break
		}

		for hostArch, minKernel := range runtimeReqs.Requirements {
			host, arch, found := strings.Cut(hostArch, "/")
			if host != runtime.GOOS {
				continue
			}
			if found {
				if arch != runtime.GOARCH {
					continue
				}
			}
			if CompareVersions(minKernel, currentKernel) > Equal {
				err = fmt.Errorf(osTooOldErr, runtimeReqs.Runtime, minKernel, currentKernel)
				break
			}
		}

		break
	}

	if err != nil {
		l.Warnln(err)
	}

	return err
}

// normalizeRuntimeVersion strips off the runtime.Version()'s patch level, so
// "go1.22.5" becomes "go1.22".
func normalizeRuntimeVersion(goRuntime string) string {
	parts := strings.Split(goRuntime, ".")
	if len(parts) < 2 {
		return goRuntime
	}
	return strings.Join(parts[:2], ".")
}

// normalizeKernelVersion strips off anything after the kernel version's patch
// level. So Windows' "10.0.22631.3880 Build 22631.3880" becomes "10.0.22631",
// and Linux's "6.8.0-39-generic" becomes "6.8.0".
func normalizeKernelVersion(version string) string {
	version, _, _ = strings.Cut(version, " ")
	version, _, _ = strings.Cut(version, "-")

	parts := strings.Split(version, ".")
	maxParts := min(len(parts), 3)
	return strings.Join(parts[:maxParts], ".")
}

// getRuntimeReqsByReleaseTag searches compat.yaml for the go version used to
// build the release, and returns the runtime requirements for that go version.
// It's only used when the STUPGRADETEST_RELEASESURL environment variable is set
// so our unit tests pass.
// Once we include compat.json in our release bundles, we can remove this function.
func getRuntimeReqsByReleaseTag(rel Release) (RuntimeReqs, error) {
	goRuntime := ""
	parts := strings.Split(rel.HTMLURL, "/")
	tag := strings.ReplaceAll(parts[len(parts)-1], "v", "")
	// Strip off -rc1 which CompareVersions sees as older.
	tag = normalizeKernelVersion(tag)
	for _, tagRuntime := range tagRuntimes {
		if CompareVersions(tag, tagRuntime.tag) >= Equal {
			goRuntime = tagRuntime.runtime
		}
	}

	if goRuntime == "" {
		return RuntimeReqs{}, fmt.Errorf("Cannot determine runtime for release %q", tag)
	}

	// Search for compat.yaml in the current directory and above.
	requirementsMap, err := getRequirementsMap("")
	if err != nil {
		return RuntimeReqs{}, err
	}

	majorMinor := normalizeRuntimeVersion(goRuntime)

	requirements, ok := requirementsMap[majorMinor]
	if !ok {
		return RuntimeReqs{}, fmt.Errorf("runtime %v not found in %q", majorMinor, CompatYaml)
	}

	return RuntimeReqs{
		Runtime:      majorMinor,
		Requirements: requirements,
	}, nil
}
