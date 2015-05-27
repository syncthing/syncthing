// Copyright (C) 2014 The Protocol Authors.

// +build !windows,!darwin

package protocol

// Normal Unixes uses NFC and slashes, which is the wire format.

type nativeModel struct {
	next Model
}

func (m nativeModel) Index(deviceID DeviceID, folder string, files []FileInfo, flags uint32, options []Option) {
	m.next.Index(deviceID, folder, files, flags, options)
}

func (m nativeModel) IndexUpdate(deviceID DeviceID, folder string, files []FileInfo, flags uint32, options []Option) {
	m.next.IndexUpdate(deviceID, folder, files, flags, options)
}

func (m nativeModel) Request(deviceID DeviceID, folder string, name string, offset int64, size int, hash []byte, flags uint32, options []Option) ([]byte, error) {
	return m.next.Request(deviceID, folder, name, offset, size, hash, flags, options)
}

func (m nativeModel) ClusterConfig(deviceID DeviceID, config ClusterConfigMessage) {
	m.next.ClusterConfig(deviceID, config)
}

func (m nativeModel) Close(deviceID DeviceID, err error) {
	m.next.Close(deviceID, err)
}
