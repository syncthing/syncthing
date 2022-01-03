// Copyright (C) 2014 The Protocol Authors.

//go:build darwin
// +build darwin

package protocol

// Darwin uses NFD normalization

import "golang.org/x/text/unicode/norm"

func makeNative(m Model) Model { return nativeModel{m} }

type nativeModel struct {
	Model
}

func (m nativeModel) Index(deviceID DeviceID, folder string, files []FileInfo) error {
	for i := range files {
		files[i].Name = norm.NFD.String(files[i].Name)
	}
	return m.Model.Index(deviceID, folder, files)
}

func (m nativeModel) IndexUpdate(deviceID DeviceID, folder string, files []FileInfo) error {
	for i := range files {
		files[i].Name = norm.NFD.String(files[i].Name)
	}
	return m.Model.IndexUpdate(deviceID, folder, files)
}

func (m nativeModel) Request(deviceID DeviceID, folder, name string, blockNo, size int32, offset int64, hash []byte, weakHash uint32, fromTemporary bool) (RequestResponse, error) {
	name = norm.NFD.String(name)
	return m.Model.Request(deviceID, folder, name, blockNo, size, offset, hash, weakHash, fromTemporary)
}
