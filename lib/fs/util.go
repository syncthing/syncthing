// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
)

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
	if runtime.GOOS == "windows" {
		// Legacy -- we prioritize this for historical reasons, whereas
		// os.UserHomeDir uses %USERPROFILE% always.
		home := filepath.Join(os.Getenv("HomeDrive"), os.Getenv("HomePath"))
		if home != "" {
			return home, nil
		}
	}

	return os.UserHomeDir()
}

var (
	windowsDisallowedCharacters = string([]rune{
		'<', '>', ':', '"', '|', '?', '*',
		0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
		11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
		21, 22, 23, 24, 25, 26, 27, 28, 29, 30,
		31,
	})
	windowsDisallowedNames = []string{"CON", "PRN", "AUX", "NUL",
		"COM0", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT0", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9",
	}
)

func WindowsInvalidFilename(name string) error {
	// None of the path components should end in space or period, or be a
	// reserved name. COM0 and LPT0 are missing from the Microsoft docs,
	// but Windows Explorer treats them as invalid too.
	// (https://docs.microsoft.com/en-us/windows/win32/fileio/naming-a-file)
	for _, part := range strings.Split(name, `\`) {
		if len(part) == 0 {
			continue
		}
		switch part[len(part)-1] {
		case ' ', '.':
			// Names ending in space or period are not valid.
			return errInvalidFilenameWindowsSpacePeriod
		}
		if windowsIsReserved(part) {
			return errInvalidFilenameWindowsReservedName
		}
	}

	// The path must not contain any disallowed characters
	if strings.ContainsAny(name, windowsDisallowedCharacters) {
		return errInvalidFilenameWindowsReservedChar
	}

	return nil
}

// SanitizePath takes a string that might contain all kinds of special
// characters and makes a valid, similar, path name out of it.
//
// Spans of invalid characters, whitespace and/or non-UTF-8 sequences are
// replaced by a single space. The result is always UTF-8 and contains only
// printable characters, as determined by unicode.IsPrint.
//
// Invalid characters are non-printing runes, things not allowed in file names
// in Windows, and common shell metacharacters. Even if asterisks and pipes
// and stuff are allowed on Unixes in general they might not be allowed by
// the filesystem and may surprise the user and cause shell oddness. This
// function is intended for file names we generate on behalf of the user,
// and surprising them with odd shell characters in file names is unkind.
//
// We include whitespace in the invalid characters so that multiple
// whitespace is collapsed to a single space. Additionally, whitespace at
// either end is removed.
//
// If the result is a name disallowed on windows, a hyphen is prepended.
func SanitizePath(path string) string {
	var b strings.Builder

	disallowed := `<>:"'/\|?*[]{};:!@$%&^#` + windowsDisallowedCharacters
	prev := ' '
	for _, c := range path {
		if !unicode.IsPrint(c) || c == unicode.ReplacementChar ||
			strings.ContainsRune(disallowed, c) {
			c = ' '
		}

		if !(c == ' ' && prev == ' ') {
			b.WriteRune(c)
		}
		prev = c
	}

	path = strings.TrimSpace(b.String())
	if windowsIsReserved(path) {
		path = "-" + path
	}
	return path
}

func windowsIsReserved(part string) bool {
	upperCased := strings.ToUpper(part)
	for _, disallowed := range windowsDisallowedNames {
		if upperCased == disallowed {
			return true
		}
		if strings.HasPrefix(upperCased, disallowed+".") {
			// nul.txt.jpg is also disallowed
			return true
		}
	}
	return false
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
