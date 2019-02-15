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

func IsParent(path, parent string) bool {
	if len(parent) == 0 {
		// The empty string is the parent of everything except the empty
		// string. (Avoids panic in the next step.)
		return len(path) > 0
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
	fmt.Println("input", first, second, isAbs)
	i := 0
	for ; i < count; i++ {
		if firstParts[i] != secondParts[i] {
			break
		}
	}

	fmt.Println("got", firstParts[:i])

	// If isAbs on Linux, first element in both first and second is "", hence joining that returns nothing.
	if i == 1 && isAbs && runtime.GOOS != "windows" {
		fmt.Println("Returning one")
		return string(PathSeparator)
	}

	// This should only be true on Windows when drive letters are different or when paths are relative.
	// In case of UNC paths we should end up with more than a single element hence joining is fine
	if i == 0 {
		fmt.Println("returning zero")
		return ""
	}

	fmt.Println("pre join", i, firstParts[:i])
	// This has to be strings.Join, because joining ["", "", "?", "C:", "Audrius"] returns garbage
	result := strings.Join(firstParts[:i], string(PathSeparator))
	fmt.Println("pre clean", result)
	result = filepath.Clean(result)
	fmt.Println("post clean", result)
	if runtime.GOOS == "windows" {
		if len(result) == 3 && strings.HasSuffix(result, ":.") {
			// filepath.Clean("C:\") return "C:.", fix that up.
			bytes := []byte(result)
			bytes[len(bytes)-1 ] = PathSeparator
			result = string(bytes)
		} else if len(result) == 6 && strings.HasPrefix(result, `\\?\`) {
			// filepath.Clean("\\?\C:\") return "\\?\C:", fix that up.
			result += string(PathSeparator)
		}
	}
	return result
}
