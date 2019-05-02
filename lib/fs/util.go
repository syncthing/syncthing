// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var errNoHome = errors.New("no home directory found - set $HOME (or the platform equivalent)")

func ExpandTilde(path string) (string, error) {
	if path == "~" {
		return getHomeDir()
	}

	path = filepath.FromSlash(path)
	if !strings.HasPrefix(path, fmt.Sprintf("~%c", PathSeparator)) {
		return path, nil
	}

	home, err := getHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, path[2:]), nil
}

func getHomeDir() (string, error) {
	var home string

	switch runtime.GOOS {
	case "windows":
		home = filepath.Join(os.Getenv("HomeDrive"), os.Getenv("HomePath"))
		if home == "" {
			home = os.Getenv("UserProfile")
		}
	default:
		home = os.Getenv("HOME")
	}

	if home == "" {
		return "", errNoHome
	}

	return home, nil
}

var windowsDisallowedCharacters = string([]rune{
	'<', '>', ':', '"', '|', '?', '*',
	0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
	11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
	21, 22, 23, 24, 25, 26, 27, 28, 29, 30,
	31,
})

func WindowsInvalidFilename(name string) bool {
	// None of the path components should end in space
	for _, part := range strings.Split(name, `\`) {
		if len(part) == 0 {
			continue
		}
		if part[len(part)-1] == ' ' {
			// Names ending in space are not valid.
			return true
		}
	}

	// The path must not contain any disallowed characters
	return strings.ContainsAny(name, windowsDisallowedCharacters)
}

// IsParent compares paths purely lexicographically, meaning it returns false
// if path and parent aren't both absolute or relative.
func IsParent(path, parent string) bool {
	if parent == path {
		// Twice the same root on windows would not be caught at the end.
		return false
	}
	if filepath.IsAbs(path) != filepath.IsAbs(parent) {
		return false
	}
	if parent == "" || parent == "." {
		// The empty string is the parent of everything except the empty
		// string and ".". (Avoids panic in the last step.)
		return path != "" && path != "."
	}
	if parent == "/" {
		// The root is the parent of everything except itself, which would
		// not be caught below.
		return path != "/"
	}
	if parent[len(parent)-1] != PathSeparator {
		parent += string(PathSeparator)
	}
	return strings.HasPrefix(path, parent)
}

func CommonPrefix(first, second string) string {
	if filepath.IsAbs(first) != filepath.IsAbs(second) {
		// Whatever
		return ""
	}

	firstParts := strings.Split(filepath.Clean(first), string(PathSeparator))
	secondParts := strings.Split(filepath.Clean(second), string(PathSeparator))

	isAbs := filepath.IsAbs(first) && filepath.IsAbs(second)

	count := len(firstParts)
	if len(secondParts) < len(firstParts) {
		count = len(secondParts)
	}

	common := make([]string, 0, count)
	for i := 0; i < count; i++ {
		if firstParts[i] != secondParts[i] {
			break
		}
		common = append(common, firstParts[i])
	}

	if isAbs {
		if runtime.GOOS == "windows" && isVolumeNameOnly(common) {
			// Because strings.Split strips out path separators, if we're at the volume name, we end up without a separator
			// Wedge an empty element to be joined with.
			common = append(common, "")
		} else if len(common) == 1 {
			// If isAbs on non Windows, first element in both first and second is "", hence joining that returns nothing.
			return string(PathSeparator)
		}
	}

	// This should only be true on Windows when drive letters are different or when paths are relative.
	// In case of UNC paths we should end up with more than a single element hence joining is fine
	if len(common) == 0 {
		return ""
	}

	// This has to be strings.Join, because filepath.Join([]string{"", "", "?", "C:", "Audrius"}...) returns garbage
	result := strings.Join(common, string(PathSeparator))
	return filepath.Clean(result)
}

func isVolumeNameOnly(parts []string) bool {
	isNormalVolumeName := len(parts) == 1 && strings.HasSuffix(parts[0], ":")
	isUNCVolumeName := len(parts) == 4 && strings.HasSuffix(parts[3], ":")
	return isNormalVolumeName || isUNCVolumeName
}
