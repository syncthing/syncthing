// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package api

import (
	"net"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/connections"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/model"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/stats"
	"github.com/syncthing/syncthing/lib/versioner"
)

type mockedModel struct{}

func (m *mockedModel) GlobalDirectoryTree(folder, prefix string, levels int, dirsonly bool) map[string]interface{} {
	return nil
}

func (m *mockedModel) Completion(device protocol.DeviceID, folder string) model.FolderCompletion {
	return model.FolderCompletion{}
}

func (m *mockedModel) Override(folder string) {}

func (m *mockedModel) Revert(folder string) {}

func (m *mockedModel) NeedFolderFiles(folder string, page, perpage int) ([]db.FileInfoTruncated, []db.FileInfoTruncated, []db.FileInfoTruncated) {
	return nil, nil, nil
}

func (m *mockedModel) RemoteNeedFolderFiles(device protocol.DeviceID, folder string, page, perpage int) ([]db.FileInfoTruncated, error) {
	return nil, nil
}

func (m *mockedModel) NeedSize(folder string) db.Counts {
	return db.Counts{}
}

func (m *mockedModel) ConnectionStats() map[string]interface{} {
	return nil
}

func (m *mockedModel) DeviceStatistics() map[string]stats.DeviceStatistics {
	return nil
}

func (m *mockedModel) FolderStatistics() map[string]stats.FolderStatistics {
	return nil
}

func (m *mockedModel) CurrentFolderFile(folder string, file string) (protocol.FileInfo, bool) {
	return protocol.FileInfo{}, false
}

func (m *mockedModel) CurrentGlobalFile(folder string, file string) (protocol.FileInfo, bool) {
	return protocol.FileInfo{}, false
}

func (m *mockedModel) ResetFolder(folder string) {
}

func (m *mockedModel) Availability(folder string, file protocol.FileInfo, block protocol.BlockInfo) []model.Availability {
	return nil
}

func (m *mockedModel) GetIgnores(folder string) ([]string, []string, error) {
	return nil, nil, nil
}

func (m *mockedModel) SetIgnores(folder string, content []string) error {
	return nil
}

func (m *mockedModel) GetFolderVersions(folder string) (map[string][]versioner.FileVersion, error) {
	return nil, nil
}

func (m *mockedModel) RestoreFolderVersions(folder string, versions map[string]time.Time) (map[string]string, error) {
	return nil, nil
}

func (m *mockedModel) PauseDevice(device protocol.DeviceID) {
}

func (m *mockedModel) ResumeDevice(device protocol.DeviceID) {}

func (m *mockedModel) DelayScan(folder string, next time.Duration) {}

func (m *mockedModel) ScanFolder(folder string) error {
	return nil
}

func (m *mockedModel) ScanFolders() map[string]error {
	return nil
}

func (m *mockedModel) ScanFolderSubdirs(folder string, subs []string) error {
	return nil
}

func (m *mockedModel) BringToFront(folder, file string) {}

func (m *mockedModel) Connection(deviceID protocol.DeviceID) (connections.Connection, bool) {
	return nil, false
}

func (m *mockedModel) GlobalSize(folder string) db.Counts {
	return db.Counts{}
}

func (m *mockedModel) LocalSize(folder string) db.Counts {
	return db.Counts{}
}

func (m *mockedModel) ReceiveOnlyChangedSize(folder string) db.Counts {
	return db.Counts{}
}

func (m *mockedModel) CurrentSequence(folder string) (int64, bool) {
	return 0, false
}

func (m *mockedModel) RemoteSequence(folder string) (int64, bool) {
	return 0, false
}

func (m *mockedModel) State(folder string) (string, time.Time, error) {
	return "", time.Time{}, nil
}

func (m *mockedModel) UsageReportingStats(version int, preview bool) map[string]interface{} {
	return nil
}

func (m *mockedModel) FolderErrors(folder string) ([]model.FileError, error) {
	return nil, nil
}

func (m *mockedModel) WatchError(folder string) error {
	return nil
}

func (m *mockedModel) LocalChangedFiles(folder string, page, perpage int) []db.FileInfoTruncated {
	return nil
}

func (m *mockedModel) Serve()                                                                     {}
func (m *mockedModel) Stop()                                                                      {}
func (m *mockedModel) Index(deviceID protocol.DeviceID, folder string, files []protocol.FileInfo) {}
func (m *mockedModel) IndexUpdate(deviceID protocol.DeviceID, folder string, files []protocol.FileInfo) {
}

func (m *mockedModel) Request(deviceID protocol.DeviceID, folder, name string, size int32, offset int64, hash []byte, weakHash uint32, fromTemporary bool) (protocol.RequestResponse, error) {
	return nil, nil
}

func (m *mockedModel) ClusterConfig(deviceID protocol.DeviceID, config protocol.ClusterConfig) {}

func (m *mockedModel) Closed(conn protocol.Connection, err error) {}

func (m *mockedModel) DownloadProgress(deviceID protocol.DeviceID, folder string, updates []protocol.FileDownloadProgressUpdate) {
}

func (m *mockedModel) AddConnection(conn connections.Connection, hello protocol.HelloResult) {}

func (m *mockedModel) OnHello(protocol.DeviceID, net.Addr, protocol.HelloResult) error {
	return nil
}

func (m *mockedModel) GetHello(protocol.DeviceID) protocol.HelloIntf {
	return nil
}

func (m *mockedModel) AddFolder(cfg config.FolderConfiguration) {}

func (m *mockedModel) RestartFolder(from, to config.FolderConfiguration) {}

func (m *mockedModel) StartFolder(folder string) {}

func (m *mockedModel) StartDeadlockDetector(timeout time.Duration) {}
