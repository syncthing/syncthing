// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"strings"
	"unicode/utf8"
)

type WindowsEncoderFilesystem struct {
	EncoderFilesystem
}

var windowsReservedChars = string([]rune{
	// 0x00 is disallowed but we should never see it in a filename
	0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
	0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
	0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17,
	0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
	//'"' '*'   ':'   '<'   '>'   '?'   '|'
	0x22, 0x2a, 0x3a, 0x3c, 0x3e, 0x3f, 0x7c,
	// 0x2f (/) is disallowed, but we never see it in a filename
	// 0x5c (\) is disallowed, but we never see it in a filename
})

const windowsReservedStartChars = ""
const windowsReservedEndChars = " ."

// A NewWindowsEncoderFilesystem ensures that paths that contain characters
// that are reserved in NTFS/exFAT/FAT32/reFS filesystems (<>:"|?*), and files
// that end in a period or space, can be safety stored. It does this by
// replacing the reserved characters with UNICODE characters in the private
// use area \uf000-\uf07f. This conversion is compatible with Cygwin,
// Git-Bash, Msys2, Windows Subsystem for Linux (WSL), and other platforms.
//
// For reference, see:
// https://cygwin.com/cygwin-ug-net/using-specialnames.html
// http://msdn.microsoft.com/en-us/library/aa365247%28VS.85%29.aspx
// https://en.wikipedia.org/wiki/Filename#In_Windows
// https://docs.microsoft.com/en-us/windows/win32/fileio/naming-a-file
//
// For implementations, see
// https://github.com/mirror/newlib-cygwin/blob/fb01286fab9b370c86323f84a46285cfbebfe4ff/winsup/cygwin/path.cc#L435
// https://github.com/billziss-gh/winfsp/blob/6e3a8f70b2bd958960012447544d492fc6a2f1af/src/shared/ku/posix.c#L1250
func NewWindowsEncoderFilesystem(fs Filesystem) Filesystem {
	return wrapFilesystem(fs, func(underlying Filesystem) Filesystem {
		efs := EncoderFilesystem{
			Filesystem:         underlying,
			reservedChars:      windowsReservedChars,
			reservedStartChars: windowsReservedStartChars,
			reservedEndChars:   windowsReservedEndChars,
		}
		efs.init()
		return &WindowsEncoderFilesystem{efs}
	})
}

func newWindowsEncoderFilesystem(fs Filesystem) *WindowsEncoderFilesystem {
	return NewWindowsEncoderFilesystem(fs).(*WindowsEncoderFilesystem)
}

const ntNamespacePrefix = `\\?\`
const validDrives = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcedfghijklmnopqrstuvwxyz"

func (f *WindowsEncoderFilesystem) encodedPath(path string) string {
	encodedPath := ""

	// handle `\\?\` prefix as a question mark is a reserved character
	if strings.HasPrefix(path, ntNamespacePrefix) {
		encodedPath = ntNamespacePrefix
		path = strings.TrimPrefix(path, ntNamespacePrefix)
	}

	// handle `c:` prefix as a colon is a reserved character otherwise
	if strings.Index(path, ":") == 1 {
		firstChar, _ := utf8.DecodeRuneInString(path)
		if strings.ContainsRune(validDrives, firstChar) {
			driveColon := string([]rune(path)[:2])
			encodedPath += driveColon
			path = strings.TrimPrefix(path, driveColon)
		}
	}

	return encodedPath + f._encodedPath(path)

	// A BasicFilesystem already implements this logic at
	// https://github.com/syncthing/syncthing/blob/cb26552440ebf60c366d2f9299e7d46e4850ddac/lib/fs/basicfs.go#L82
	// so this code is unneeded
	// if !strings.HasPrefix(encodedPath, ntNamespacePrefix) {
	// 	upperCased := strings.ToUpper(encodedPart)
	// 	for _, disallowed := range windowsDisallowedNames {
	// 		if upperCased == disallowed || strings.HasPrefix(upperCased, disallowed+".") {
	// 			encodedPath = fixLongPath(encodedPath, true)
	// 			break
	// 		}
	// 	}
	// }
}

func (f *WindowsEncoderFilesystem) EncoderType() FilesystemEncoderType {
	return FilesystemEncoderTypeWindows
}
