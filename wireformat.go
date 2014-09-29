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

package protocol

import (
	"path/filepath"

	"code.google.com/p/go.text/unicode/norm"
)

type wireFormatConnection struct {
	next Connection
}

func (c wireFormatConnection) ID() DeviceID {
	return c.next.ID()
}

func (c wireFormatConnection) Name() string {
	return c.next.Name()
}

func (c wireFormatConnection) Index(folder string, fs []FileInfo) error {
	var myFs = make([]FileInfo, len(fs))
	copy(myFs, fs)

	for i := range fs {
		myFs[i].Name = norm.NFC.String(filepath.ToSlash(myFs[i].Name))
	}

	return c.next.Index(folder, myFs)
}

func (c wireFormatConnection) IndexUpdate(folder string, fs []FileInfo) error {
	var myFs = make([]FileInfo, len(fs))
	copy(myFs, fs)

	for i := range fs {
		myFs[i].Name = norm.NFC.String(filepath.ToSlash(myFs[i].Name))
	}

	return c.next.IndexUpdate(folder, myFs)
}

func (c wireFormatConnection) Request(folder, name string, offset int64, size int) ([]byte, error) {
	name = norm.NFC.String(filepath.ToSlash(name))
	return c.next.Request(folder, name, offset, size)
}

func (c wireFormatConnection) ClusterConfig(config ClusterConfigMessage) {
	c.next.ClusterConfig(config)
}

func (c wireFormatConnection) Statistics() Statistics {
	return c.next.Statistics()
}
