// Copyright (C) 2014 The Protocol Authors.

// +build windows

package protocol

// Windows uses backslashes as file separator and disallows a bunch of
// characters in the filename

import (
	"path/filepath"
	"strings"
)

var disallowedCharacters = string([]rune{
	'<', '>', ':', '"', '|', '?', '*',
	0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
	11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
	21, 22, 23, 24, 25, 26, 27, 28, 29, 30,
	31,
})

type nativeModel struct {
	next Model
}

func (m nativeModel) Index(deviceID DeviceID, folder string, files []FileInfo, flags uint32, options []Option) {
	fixupFiles(files)
	m.next.Index(deviceID, folder, files, flags, options)
}

func (m nativeModel) IndexUpdate(deviceID DeviceID, folder string, files []FileInfo, flags uint32, options []Option) {
	fixupFiles(files)
	m.next.IndexUpdate(deviceID, folder, files, flags, options)
}

func (m nativeModel) Request(deviceID DeviceID, folder string, name string, offset int64, size int, hash []byte, flags uint32, options []Option) ([]byte, error) {
	name = filepath.FromSlash(name)
	return m.next.Request(deviceID, folder, name, offset, size, hash, flags, options)
}

func (m nativeModel) ClusterConfig(deviceID DeviceID, config ClusterConfigMessage) {
	m.next.ClusterConfig(deviceID, config)
}

func (m nativeModel) Close(deviceID DeviceID, err error) {
	m.next.Close(deviceID, err)
}

func fixupFiles(files []FileInfo) {
	for i, f := range files {
		if strings.ContainsAny(f.Name, disallowedCharacters) {
			if f.IsDeleted() {
				// Don't complain if the file is marked as deleted, since it
				// can't possibly exist here anyway.
				continue
			}
			files[i].Flags |= FlagInvalid
			l.Warnf("File name %q contains invalid characters; marked as invalid.", f.Name)
		}
		files[i].Name = filepath.FromSlash(files[i].Name)
	}
}
