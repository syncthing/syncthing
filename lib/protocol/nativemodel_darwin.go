// Copyright (C) 2014 The Protocol Authors.

// +build darwin

package protocol

// Darwin uses NFD normalization

import "golang.org/x/text/unicode/norm"

type nativeModel struct {
	Model
}

func (m nativeModel) Index(deviceID DeviceID, folder string, files []FileInfo, flags uint32, options []Option) {
	for i := range files {
		files[i].Name = norm.NFD.String(files[i].Name)
	}
	m.Model.Index(deviceID, folder, files, flags, options)
}

func (m nativeModel) IndexUpdate(deviceID DeviceID, folder string, files []FileInfo, flags uint32, options []Option) {
	for i := range files {
		files[i].Name = norm.NFD.String(files[i].Name)
	}
	m.Model.IndexUpdate(deviceID, folder, files, flags, options)
}

func (m nativeModel) Request(deviceID DeviceID, folder string, name string, offset int64, hash []byte, flags uint32, options []Option, buf []byte) error {
	name = norm.NFD.String(name)
	return m.Model.Request(deviceID, folder, name, offset, hash, flags, options, buf)
}
