// Copyright (C) 2014 The Protocol Authors.

// +build darwin

package protocol

// Darwin uses NFD normalization

import "golang.org/x/text/unicode/norm"

type nativeModel struct {
	next Model
}

func (m nativeModel) Index(deviceID DeviceID, folder string, files []FileInfo, flags uint32, options []Option) {
	for i := range files {
		files[i].Name = norm.NFD.String(files[i].Name)
	}
	m.next.Index(deviceID, folder, files, flags, options)
}

func (m nativeModel) IndexUpdate(deviceID DeviceID, folder string, files []FileInfo, flags uint32, options []Option) {
	for i := range files {
		files[i].Name = norm.NFD.String(files[i].Name)
	}
	m.next.IndexUpdate(deviceID, folder, files, flags, options)
}

func (m nativeModel) Request(deviceID DeviceID, folder string, name string, offset int64, size int, hash []byte, flags uint32, options []Option) ([]byte, error) {
	name = norm.NFD.String(name)
	return m.next.Request(deviceID, folder, name, offset, size, hash, flags, options)
}

func (m nativeModel) ClusterConfig(deviceID DeviceID, config ClusterConfigMessage) {
	m.next.ClusterConfig(deviceID, config)
}

func (m nativeModel) Close(deviceID DeviceID, err error) {
	m.next.Close(deviceID, err)
}
