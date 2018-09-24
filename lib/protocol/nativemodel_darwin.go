// Copyright (C) 2014 The Protocol Authors.

// +build darwin

package protocol

// Darwin uses NFD normalization

import "golang.org/x/text/unicode/norm"

type nativeModel struct {
	Model
}

func (m nativeModel) Index(deviceID DeviceID, folder string, files []FileInfo) {
	for i := range files {
		files[i].Name = norm.NFD.String(files[i].Name)
	}
	m.Model.Index(deviceID, folder, files)
}

func (m nativeModel) IndexUpdate(deviceID DeviceID, folder string, files []FileInfo) {
	for i := range files {
		files[i].Name = norm.NFD.String(files[i].Name)
	}
	m.Model.IndexUpdate(deviceID, folder, files)
}

func (m nativeModel) Request(requestID int32, deviceID DeviceID, folder, name string, size int32, offset int64, hash []byte, weakHash uint32, fromTemporary bool) {
	m.Model.Request(requestID, deviceID, folder, norm.NFD.String(name), size, offset, hash, weakHash, fromTemporary)
}
