// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package protocol

import (
	"path/filepath"

	"code.google.com/p/go.text/unicode/norm"
)

type wireFormatConnection struct {
	next Connection
}

func (c wireFormatConnection) ID() NodeID {
	return c.next.ID()
}

func (c wireFormatConnection) Name() string {
	return c.next.Name()
}

func (c wireFormatConnection) Index(repo string, fs []FileInfo) error {
	var myFs = make([]FileInfo, len(fs))
	copy(myFs, fs)

	for i := range fs {
		myFs[i].Name = norm.NFC.String(filepath.ToSlash(myFs[i].Name))
	}

	return c.next.Index(repo, myFs)
}

func (c wireFormatConnection) IndexUpdate(repo string, fs []FileInfo) error {
	var myFs = make([]FileInfo, len(fs))
	copy(myFs, fs)

	for i := range fs {
		myFs[i].Name = norm.NFC.String(filepath.ToSlash(myFs[i].Name))
	}

	return c.next.IndexUpdate(repo, myFs)
}

func (c wireFormatConnection) Request(repo, name string, offset int64, size int) ([]byte, error) {
	name = norm.NFC.String(filepath.ToSlash(name))
	return c.next.Request(repo, name, offset, size)
}

func (c wireFormatConnection) ClusterConfig(config ClusterConfigMessage) {
	c.next.ClusterConfig(config)
}

func (c wireFormatConnection) Statistics() Statistics {
	return c.next.Statistics()
}
