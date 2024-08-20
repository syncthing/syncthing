// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package upgrade

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/shirou/gopsutil/v4/host"
)

// verifyRuntimeRequirements returns an error if the OS's version (aka
// kernel version) is incompatible with the version of go used to build the
// release. If there is no requirement for the current OS and architecture,
// no error is returned.
func verifyRuntimeRequirements(runtimeReqs RuntimeReqs) error {
	currentKernel, err := host.KernelVersion()
	if err != nil {
		return err
	}
	currentKernel = normalizeKernelVersion(currentKernel)

	for hostArch, minKernel := range runtimeReqs.Requirements {
		host, arch, found := strings.Cut(hostArch, "/")
		if host != runtime.GOOS {
			continue
		}
		if found && arch != runtime.GOARCH {
			continue
		}
		if CompareVersions(minKernel, currentKernel) > Equal {
			return fmt.Errorf("runtime %v requires OS version %v but this system is version %v", runtimeReqs.Runtime, minKernel, currentKernel)
		}
	}

	return nil
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
