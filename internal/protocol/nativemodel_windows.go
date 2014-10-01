// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

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

func (m nativeModel) Index(deviceID DeviceID, folder string, files []FileInfo) {
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
		files[i].Name = filepath.FromSlash(f.Name)
	}
	m.next.Index(deviceID, folder, files)
}

func (m nativeModel) IndexUpdate(deviceID DeviceID, folder string, files []FileInfo) {
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
	m.next.IndexUpdate(deviceID, folder, files)
}

func (m nativeModel) Request(deviceID DeviceID, folder string, name string, offset int64, size int) ([]byte, error) {
	name = filepath.FromSlash(name)
	return m.next.Request(deviceID, folder, name, offset, size)
}

func (m nativeModel) ClusterConfig(deviceID DeviceID, config ClusterConfigMessage) {
	m.next.ClusterConfig(deviceID, config)
}

func (m nativeModel) Close(deviceID DeviceID, err error) {
	m.next.Close(deviceID, err)
}
