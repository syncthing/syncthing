// Copyright (C) 2014 The Protocol Authors.

// +build !windows,!darwin

package protocol

// Normal Unixes uses NFC and slashes, which is the wire format.

type nativeModel struct {
	next Model
}

func (m nativeModel) Index(deviceID DeviceID, folder string, files []FileInfo) {
	m.next.Index(deviceID, folder, files)
}

func (m nativeModel) IndexUpdate(deviceID DeviceID, folder string, files []FileInfo) {
	m.next.IndexUpdate(deviceID, folder, files)
}

func (m nativeModel) Request(deviceID DeviceID, folder string, name string, offset int64, size int) ([]byte, error) {
	return m.next.Request(deviceID, folder, name, offset, size)
}

func (m nativeModel) ClusterConfig(deviceID DeviceID, config ClusterConfigMessage) {
	m.next.ClusterConfig(deviceID, config)
}

func (m nativeModel) Close(deviceID DeviceID, err error) {
	m.next.Close(deviceID, err)
}
