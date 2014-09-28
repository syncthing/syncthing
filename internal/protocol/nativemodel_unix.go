// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

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
