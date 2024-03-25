// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

type stringBoolTestCases map[string]bool

type filterTestCase struct {
	input    []string
	expected int // length of array of files returned
}

var (
	validPathCases = make(map[FilesystemEncoderType]stringBoolTestCases)

	encodeNameCases = make(map[FilesystemEncoderType]encodeTestCases)
	encodePathCases = make(map[FilesystemEncoderType]encodeTestCases)
	encodeAllCases  = make(map[FilesystemEncoderType]encodeTestCases)

	decodeNameCases = make(encodeTestCases)
	decodePathCases = make(encodeTestCases)
	decodeAllCases  = make(encodeTestCases)

	filterCases      = make(map[FilesystemEncoderType][]filterTestCase)
	wouldEncodeCases = make(map[FilesystemEncoderType]stringBoolTestCases)
)

func testPrepareEncodeVars() {
	if len(encodeAllCases) > 0 {
		return
	}
	for encodeType := range encodeNameCases {
		encodeAllCases[encodeType] = make(encodeTestCases)

		encodeNameCases[encodeType] = prepare(encodeNameCases[encodeType])
		for k, v := range encodeNameCases[encodeType] {
			encodeAllCases[encodeType][k] = v
			decodeNameCases[k] = v
			decodeAllCases[k] = v
		}

		encodePathCases[encodeType] = prepare(encodePathCases[encodeType])
		for k, v := range encodePathCases[encodeType] {
			encodeAllCases[encodeType][k] = v
			decodePathCases[k] = v
			decodeAllCases[k] = v
		}
	}
}

func testEqual(t *testing.T, msg string, args ...interface{}) {
	t.Helper()
	if args[len(args)-2] != args[len(args)-1] {
		t.Errorf(msg, args...)
	}
}

func testErr(t *testing.T, msg string, args ...interface{}) {
	t.Helper()
	if args[len(args)-1] != nil {
		t.Errorf(msg, args...)
	}
}

func TestEncoderCharsToEncode(t *testing.T) {
	fs, _ := setup(t)
	if fs.encoder == nil {
		t.Skipf("Skipping for the %v encoder", fs.encoderType)
	}
	for _, r := range fs.encoder.CharsToEncode() {
		if r < 0 || r >= encoderNumChars {
			t.Errorf("%v encoder: CharsToEncode() contains character %q which cannot be encoded (as its Unicode value (%d) is above %d)", fs.encoderType, string(r), r, encoderNumChars-1)
		}
	}
}

func TestEncoderEncoderRunes(t *testing.T) {
	fs, _ := setup(t)
	if fs.encoder == nil {
		t.Skipf("Skipping for the %v encoder", fs.encoderType)
	}
	encoderType := fs.encoderType
	var sum rune
	for _, r := range fs.encoder.EncoderRunes() {
		sum += r
	}
	if sum == 0 {
		t.Errorf("%v encoder: The EncoderRunes() array has not been initialized", encoderType)
	}
	for i, r := range fs.encoder.EncoderRunes() {
		if r < 0 {
			t.Errorf("%v encoder: EncoderRunes() character %d (%q) has a Unicode value of %d which is negative", encoderType, i, string(r), r)
		}
		if r >= encoderNumChars && r < encoderBaseRune {
			t.Errorf("%v encoder: EncoderRunes() character %d (%q) has a Unicode value of %d which is greater than %d but less than %d", encoderType, i, string(r), r, encoderNumChars, encoderBaseRune)
		}
		if r >= encoderBaseRune+encoderNumChars {
			t.Errorf("%v encoder: EncoderRunes() character %d (%q) has a Unicode value of %d which is greater than %d", encoderType, i, string(r), r, encoderBaseRune+encoderNumChars-1)
		}
	}
}

func TestEncoderEncoderType(t *testing.T) {
	fs, _ := setup(t)
	if fs.encoder == nil {
		t.Skipf("Skipping for the %v encoder", fs.encoderType)
	}

	testEqual(t, "%v encoder: fs.encoder.EncoderType() (%d) != fs.encoderType (%d)", fs.encoderType, fs.encoder.EncoderType(), fs.encoderType)

	for id := range FilesystemEncoderType_name {
		encoderType := FilesystemEncoderType(id)
		if fs.encoder.EncoderType() == encoderType {
			return
		}
	}
	t.Errorf("%v encoder: Unknown fs.encoder.EncoderType() %d", fs.encoderType, fs.encoder.EncoderType())
}

func TestEncoderAllowReservedFilenames(t *testing.T) {
	fs, _ := setup(t)
	if fs.encoder == nil {
		t.Skipf("Skipping for the %v encoder", fs.encoderType)
	}
	if fs.encoder.AllowReservedFilenames() {
		t.Errorf("%v encoder: fs.encoder.AllowReservedFilenames() returned true, expected false", fs.encoderType)
	}
}

func TestEncoderEncodeNames(t *testing.T) {
	fs, _ := setup(t)
	if fs.encoder == nil {
		t.Skipf("Skipping for the %v encoder", fs.encoderType)
	}
	cases := encodeNameCases[fs.encoderType]
	testEncodeCases(t, cases, "TestEncoderEncodeNames", fs.encoder.encode, fs)
}

func TestEncoderEncodePaths(t *testing.T) {
	fs, _ := setup(t)
	if fs.encoder == nil {
		t.Skipf("Skipping for the %v encoder", fs.encoderType)
	}
	cases := encodePathCases[fs.encoderType]
	testEncodeCases(t, cases, "TestEncoderEncodePaths", fs.encoder.encode, fs)
}

func TestEncoderEncodeChars(t *testing.T) {
	fs, _ := setup(t)
	if fs.encoder == nil {
		t.Skipf("Skipping for the %v encoder", fs.encoderType)
	}

	sep := pathSeparatorString

	var cases = make(encodeTestCases)
	for _, r := range fs.encoder.CharsToEncode() {
		s := string(r)
		encoded := string(r | 0xf000)
		cases[s] = encoded
		cases[sep+s] = sep + encoded
		cases[s+sep] = encoded + sep
		cases[sep+s+sep] = sep + encoded + sep
	}
	testEncodeCases(t, cases, "TestEncoderEncodeChars", fs.encoder.encode, fs)
}

func TestEncoderEncodeNameNames(t *testing.T) {
	fs, _ := setup(t)
	if fs.encoder == nil {
		t.Skipf("Skipping for the %v encoder", fs.encoderType)
	}
	cases := encodeNameCases[fs.encoderType]
	testEncodeCases(t, cases, "TestEncoderEncodeNameNames", fs.encoder.encodeName, fs)
}

func TestEncoderEncodeNameChars(t *testing.T) {
	fs, _ := setup(t)
	if fs.encoder == nil {
		t.Skipf("Skipping for the %v encoder", fs.encoderType)
	}

	var cases = make(encodeTestCases)
	for _, r := range fs.encoder.CharsToEncode() {
		cases[string(r)] = string(r | 0xf000)
	}

	testEncodeCases(t, cases, "TestEncoderEncodeNameChars", fs.encoder.encodeName, fs)
}

func TestEncoderEncodeToRunes(t *testing.T) {
	fs, _ := setup(t)
	if fs.encoder == nil {
		t.Skipf("Skipping for the %v encoder", fs.encoderType)
	}

	for input, expected := range encodeNameCases[fs.encoderType] {
		res := fs.encoder.encodeToRunes(input)
		testEqual(t, "%v encoder: encodeToRunes(%q), expected %q, but got %q", fs.encoderType, input, expected, string(res))
	}
}

func TestEncoderFilter(t *testing.T) {
	fs, _ := setup(t)
	if fs.encoder == nil {
		t.Skipf("Skipping for the %v encoder", fs.encoderType)
	}

	cases, ok := filterCases[fs.encoderType]
	if !ok {
		panic(fmt.Sprintf("%v encoder: No test cases defined for filter()", fs.encoderType))
	}

	for _, tc := range cases {
		res := fs.encoder.filter(tc.input)
		testEqual(t, "%v encoder: filter(%q), expected %d, but got %d", fs.encoderType, tc.input, tc.expected, len(res))
	}
}

func TestEncoderWouldEncode(t *testing.T) {
	fs, _ := setup(t)
	if fs.encoder == nil {
		t.Skipf("Skipping for the %v encoder", fs.encoderType)
	}

	cases, ok := wouldEncodeCases[fs.encoderType]
	if !ok {
		panic(fmt.Sprintf("%v encoder: No test cases defined for wouldEncode()", fs.encoderType))
	}

	for input, expected := range cases {
		res := fs.encoder.wouldEncode(input)
		testEqual(t, "%v encoder: wouldEncode(%q), expected %v, but got %v", fs.encoderType, input, expected, res)
	}

	testPrepareEncodeVars()

	for input, expected := range encodeAllCases[fs.encoderType] {
		res := fs.encoder.wouldEncode(input)
		b := input != expected
		testEqual(t, "%v encoder: wouldEncode(%q), expected %v, but got %v", fs.encoderType, input, b, res)
	}
}

type encoderIntMap map[FilesystemEncoderType]int

var onceTestEncodersDirNames bool

func TestEncodersDirNames(t *testing.T) {
	_, dir := setup(t)
	if onceTestEncodersDirNames {
		// Only run this test once as we are testing the interaction of all the encoders
		t.Skipf("Skipping as we only run once")
	}
	onceTestEncodersDirNames = true

	encfs := getEncoderFilesystems(dir)

	files := saveEncodedFiles(t, encfs[FilesystemEncoderTypeFat])

	names, err := dirNames(dir)
	testErr(t, "os.DirNames(%s) failed: %s", dir, err)
	testEqual(t, "os.DirNames(%s): Expecting %d files, but got %d", dir, files, len(names))

	cases := encoderIntMap{
		FilesystemEncoderTypePassthrough: files,
		FilesystemEncoderTypeStandard:    0,
		FilesystemEncoderTypeFat:         files,
	}

	relativeDir := "."

	for et, found := range cases {
		names, err = encfs[et].DirNames(relativeDir)
		testErr(t, "%v encoder: DirNames(%s) failed: %s", et, relativeDir, err)
		testEqual(t, "%v encoder: DirNames(%s): Expecting %d files, but got %d", et, relativeDir, found, len(names))
	}
}

var onceTestEncodersGlob bool

func TestEncodersGlob(t *testing.T) {
	_, dir := setup(t)
	if onceTestEncodersGlob {
		// Only run this test once as we are testing the interaction of all the encoders
		t.Skipf("Skipping as we only run once")
	}
	onceTestEncodersGlob = true

	encfs := getEncoderFilesystems(dir)

	files := saveEncodedFiles(t, encfs[FilesystemEncoderTypeFat])

	pattern := filepath.Join(dir, "*")

	names, err := filepath.Glob(pattern)
	testErr(t, "filepath.Glob(%s) failed: %s", pattern, err)
	testEqual(t, "filepath.Glob(%s): Expecting %d files, but got %d", pattern, files, len(names))

	pattern = filepath.Base(pattern)

	cases := encoderIntMap{
		FilesystemEncoderTypePassthrough: files,
		FilesystemEncoderTypeStandard:    0,
		FilesystemEncoderTypeFat:         files,
	}

	for et, expected := range cases {
		names, err = encfs[et].Glob(pattern)
		testErr(t, "%v encoder: Glob(%s) failed: %s", et, pattern, err)
		testEqual(t, "%v encoder: Glob(%s): Expecting %d files, but got %d", et, pattern, expected, len(names))
	}
}

var onceTestEncodersWalk bool

func TestEncodersWalk(t *testing.T) {
	_, dir := setup(t)
	if onceTestEncodersWalk {
		// Only run this test once as we are testing the interaction of all the encoders
		t.Skipf("Skipping as we only run once")
	}
	onceTestEncodersWalk = true

	encfs := getEncoderFilesystems(dir)

	files := saveEncodedFiles(t, encfs[FilesystemEncoderTypeFat])

	names, err := dirNames(dir)
	testErr(t, "os.DirNames(%s) failed: %s", dir, err)
	testEqual(t, "os.DirNames(%s): Expecting %d files, but got %d", dir, files, len(names))

	cases := encoderIntMap{
		FilesystemEncoderTypePassthrough: files,
		FilesystemEncoderTypeStandard:    0,
		FilesystemEncoderTypeFat:         files,
	}

	relativeDir := "."

	for et, expected := range cases {
		var names []string
		err = encfs[et].Walk(relativeDir, func(path string, info FileInfo, err error) error {
			if err == nil {
				if !info.IsDir() {
					names = append(names, info.Name())
				}
			}
			return err
		})

		if expected > 0 && err != nil {
			testErr(t, "%v encoder: Walk(%s) failed: %s", et, relativeDir, err)
		}
		testEqual(t, "%v encoder: Walk(%s): Expecting %d files, but got %d", et, relativeDir, expected, len(names))
	}
}

func getEncoderFilesystems(dir string) map[FilesystemEncoderType]Filesystem {
	var encfs = make(map[FilesystemEncoderType]Filesystem)
	for encoderType := range GetEncoders() {
		var testOpts = make([]Option, 1)
		testOpts[0] = GetEncoderOption(encoderType)
		encfs[encoderType] = NewFilesystem(FilesystemTypeBasic, dir, testOpts...)
	}
	return encfs
}

func saveEncodedFiles(t *testing.T, fs Filesystem) int {
	basicFS := basicFilesystem(fs)

	files := 0
	for _, r := range basicFS.encoder.CharsToEncode() {
		filename := fmt.Sprintf("%02x%s", r, string(r))
		fh, err := fs.Create(filename)
		testErr(t, "%s encoder: Cannot create filename %q: %s", basicFS.encoderType, filename, err)
		fh.Write([]byte(filename))
		fh.Close()
		files++
		break // @TODO REMOVEME
	}
	return files
}

func dirNames(name string) ([]string, error) {
	fd, err := os.OpenFile(name, OptReadOnly, 0750)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	names, err := fd.Readdirnames(-1)
	if err != nil {
		return nil, err
	}
	return names, nil
}

func basicFilesystem(fs Filesystem) *BasicFilesystem {
	for {
		if basicFS, ok := fs.(*BasicFilesystem); ok {
			return basicFS
		}
		var b bool
		if fs, b = fs.underlying(); !b {
			return nil
		}
	}
}
