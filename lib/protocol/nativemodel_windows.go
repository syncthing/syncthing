// Copyright (C) 2014 The Protocol Authors.

// +build windows

package protocol

// Windows uses backslashes as file separator

import "path/filepath"

type nativeModel struct {
	Model
}

func (m nativeModel) Index(deviceID DeviceID, folder string, files []FileInfo) {
	fixupFiles(folder, files)
	m.Model.Index(deviceID, folder, files)
}

func (m nativeModel) IndexUpdate(deviceID DeviceID, folder string, files []FileInfo) {
	fixupFiles(folder, files)
	m.Model.IndexUpdate(deviceID, folder, files)
}

func (m nativeModel) Request(deviceID DeviceID, folder string, name string, offset int64, hash []byte, fromTemporary bool, buf []byte) error {
	name = filepath.FromSlash(name)
	return m.Model.Request(deviceID, folder, name, offset, hash, fromTemporary, buf)
}

func fixupFiles(folder string, files []FileInfo) {
	for i := range files {
		files[i].Name = filepath.FromSlash(files[i].Name)
	}
}
