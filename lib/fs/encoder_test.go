// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"
)

type encodeTestCases map[string]string

func TestEncoderGetEncoder(t *testing.T) {
	for id, name := range FilesystemEncoderType_name {
		encoderType := FilesystemEncoderType(id)
		res := GetEncoder(encoderType)
		if encoderType == FilesystemEncoderTypePassthrough {
			if res == nil {
				continue
			}
			t.Errorf("GetEncoder(), encoderType %v (%d) (%v): expected nil, got %v", encoderType, encoderType, name, res)
		}
		if res == nil {
			t.Errorf("GetEncoder(), expected encoderType %v (%d) (%v)", encoderType, encoderType, name)
		}
	}
}

func TestEncoderGetEncoderOption(t *testing.T) {
	for id, name := range FilesystemEncoderType_name {
		encoderType := FilesystemEncoderType(id)
		res := GetEncoderOption(encoderType)
		if res == nil {
			t.Errorf("GetEncoderOption(), expected encoderType %v (%d) (%v)", encoderType, encoderType, name)
		}
	}
}

func TestEncoderGetEncoders(t *testing.T) {
	encoders := GetEncoders()
	if len(encoders) != len(FilesystemEncoderType_name) {
		t.Errorf("GetEncoders(), expected %d, but got %d", len(FilesystemEncoderType_name), len(encoders))
	}

	for id, name := range FilesystemEncoderType_name {
		encoderType := FilesystemEncoderType(id)
		_, ok := encoders[encoderType]
		if !ok {
			t.Errorf("GetEncoders(), expected encoderType %v (%d) (%v)", encoderType, encoderType, name)
		}
	}
}

func TestEncoderDecodeNames(t *testing.T) {
	testPrepareEncodeVars()
	testDecodeCases(t, decodeNameCases, "TestEncoderDecodeNames: decode")
}

func TestEncoderDecodePaths(t *testing.T) {
	testPrepareEncodeVars()
	testDecodeCases(t, decodePathCases, "TestEncoderDecodePaths: decode")
}

func TestEncoderDecodeChars(t *testing.T) {
	var cases = make(encodeTestCases)
	for _, r := range encodedChars {
		// flip expected and input to test decode:
		cases[string(r&^0xf000)] = string(r)
	}
	testDecodeCases(t, cases, "TestEncoderDecodeChars: decode")
}

var decodePatternCases = encodeTestCases{
	// input, expected
	"*":                  "*",
	"?":                  "?",
	"\uf02a":             "*",
	"\uf03f":             "?",
	"\uf000":             "\uf000",
	"\uf000\uf02a":       "\uf000*",
	"\uf000\uf03f":       "\uf000?",
	"\uf02a\uf000":       "*\uf000",
	"\uf03f\uf000":       "?\uf000",
	"\uf000\uf02a\uf000": "\uf000*\uf000",
	"\uf000\uf03f\uf000": "\uf000?\uf000",
}

func TestEncoderDecodePattern(t *testing.T) {
	testEncoderCases(t, decodePatternCases, decodePattern, "TestEncoderDecodePattern: decodePattern")
}
func TestEncoderIsEncoded(t *testing.T) {
	testPrepareEncodeVars()

	var cases = make(map[string]bool)

	for input, expected := range decodeAllCases {
		if input == expected {
			cases[input] = false
		} else {
			cases[input] = false
			cases[expected] = true
		}
	}
	for input, expected := range cases {
		res := isEncoded(input)
		if res != expected {
			t.Errorf("isEncoded(%q), expected %v, but got %v", input, expected, res)
		}
	}
}

func testDecodeCases(t *testing.T, cases encodeTestCases, name string) {
	var flipped = make(encodeTestCases)
	for input, expected := range cases {
		flipped[expected] = input
	}
	testEncoderCases(t, flipped, decode, name)
}

type stringStringFunc func(string) string

func testEncodeCases(t *testing.T, cases encodeTestCases, name string, fn stringStringFunc, fs *BasicFilesystem) {
	testEncoderCases(t, cases, fn, fmt.Sprintf("%q encoder: %s", fs.encoderType, name))
}

func testEncoderCases(t *testing.T, cases encodeTestCases, fn stringStringFunc, name string) {
	for input, expected := range cases {
		res := fn(input)
		testEqual(t, "%s(%q), expected %q, but got %q", name, input, expected, res)
	}
}

func prepare(cases encodeTestCases) encodeTestCases {
	return toSlash(filter(cases))
}

// Filter out Windows-specific paths on non-Windows platforms.
func filter(cases encodeTestCases) encodeTestCases {
	if runtime.GOOS == "windows" {
		return cases
	}
	var filtered = make(encodeTestCases)
	for input, expected := range cases {
		if isWindowsPath(input) {
			continue
		}
		filtered[input] = expected
	}
	return filtered
}

// Change \ to / on non-Windows platforms.
func toSlash(cases encodeTestCases) encodeTestCases {
	if runtime.GOOS == "windows" {
		return cases
	}

	var slashed = make(encodeTestCases, len(cases))
	for input, expected := range cases {
		slashed[filepath.ToSlash(input)] = filepath.ToSlash(expected)
	}
	return slashed
}

// isWindowsPath returns true if path is a Windows-specific path.
func isWindowsPath(path string) bool {
	path = filepath.FromSlash(path)
	if strings.HasPrefix(path, ntNamespacePrefix) {
		return true
	}

	if utf8.RuneCountInString(path) > 1 {
		runes := []rune(path)
		return runes[1] == ':' && strings.ContainsRune(validDrives, runes[0])
	}

	return false
}
