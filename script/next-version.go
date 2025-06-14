//go:build ignore
// +build ignore

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/coreos/go-semver/semver"
)

const suffix = "rc"

func main() {
	pre := flag.Bool("pre", false, "Create a prerelease")
	flag.Parse()

	// Get the latest "v1.22.3" or "v1.22.3-rc.1" style tag.
	latestTag, err := cmd("git", "describe", "--abbrev=0", "--match", "v[0-9].*")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	latest, err := semver.NewVersion(latestTag[1:])
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// Get the latest "v1.22.3" style tag, excludeing prereleases.
	latestStableTag, err := cmd("git", "describe", "--abbrev=0", "--match", "v[0-9].*", "--exclude", "*-*")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	latestStable, err := semver.NewVersion(latestStableTag[1:])
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// Get the commit logs since the latest stable tag.
	logsSinceLatest, err := cmd("git", "log", "--pretty=format:%s", latestStableTag+"..HEAD")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// Check if the next version should be a feature or a patch release
	nextIsFeature := false
	for _, line := range strings.Split(logsSinceLatest, "\n") {
		if strings.HasPrefix(line, "feat") {
			nextIsFeature = true
			break
		}
	}
	next := *latestStable
	if nextIsFeature {
		next.BumpMinor()
	} else {
		next.BumpPatch()
	}

	if latest.PreRelease != "" {
		if !*pre {
			// We want a stable release. Simply remove the prerelease
			// suffix.
			latest.PreRelease = ""
			fmt.Println("v" + latest.String())
			return
		}

		// We want the next prerelease. We are already on a prerelease. If
		// it's the correct prerelease compared to the logs we just got, we
		// should just bump the prerelease counter.
		if next.LessThan(*latest) {
			parts := latest.PreRelease.Slice()
			for i, p := range parts {
				if v, err := strconv.Atoi(p); err == nil {
					parts[i] = strconv.Itoa(v + 1)
					latest.PreRelease = semver.PreRelease(strings.Join(parts, "."))
					fmt.Println("v" + latest.String())
					return
				}
			}
		}

		// Otherwise we generate a new rc.1 for the correct next version.
		next.PreRelease = suffix + ".1"
		fmt.Println("v" + next.String())
		return
	}

	if nextIsFeature {
		latest.BumpMinor()
	} else {
		latest.BumpPatch()
	}
	if *pre {
		latest.PreRelease = suffix + ".1"
	}

	fmt.Println("v" + latest.String())
}

func cmd(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	bs, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(bs)), nil
}
