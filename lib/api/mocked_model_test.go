// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package api

import (
	"context"
	"net"
	"time"

	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/model"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/stats"
	"github.com/syncthing/syncthing/lib/ur/contract"
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

func (m *mockedModel) NeedFolderFiles(folder string, page, perpage int) ([]db.FileInfoTruncated, []db.FileInfoTruncated, []db.FileInfoTruncated, error) {
	return nil, nil, nil, nil
}

func (*mockedModel) RemoteNeedFolderFiles(folder string, device protocol.DeviceID, page, perpage int) ([]db.FileInfoTruncated, error) {
	return nil, nil
}

func (*mockedModel) LocalChangedFolderFiles(folder string, page, perpage int) ([]db.FileInfoTruncated, error) {
	return nil, nil
}

func (m *mockedModel) FolderProgressBytesCompleted(_ string) int64 {
	return 0
}

func (m *mockedModel) NumConnections() int {
	return 0
}

func (m *mockedModel) ConnectionStats() map[string]interface{} {
	return nil
}

func (m *mockedModel) DeviceStatistics() (map[protocol.DeviceID]stats.DeviceStatistics, error) {
	return nil, nil
}

func (m *mockedModel) FolderStatistics() (map[string]stats.FolderStatistics, error) {
	return nil, nil
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

func (m *mockedModel) LoadIgnores(folder string) ([]string, []string, error) {
	return nil, nil, nil
}

func (m *mockedModel) CurrentIgnores(folder string) ([]string, []string, error) {
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

func (m *mockedModel) Connection(deviceID protocol.DeviceID) (protocol.Connection, bool) {
	return nil, false
}

func (m *mockedModel) State(folder string) (string, time.Time, error) {
	return "", time.Time{}, nil
}

func (m *mockedModel) UsageReportingStats(r *contract.Report, version int, preview bool) {
}

func (m *mockedModel) PendingDevices() (map[protocol.DeviceID]db.ObservedDevice, error) {
	return nil, nil
}

func (m *mockedModel) PendingFolders(device protocol.DeviceID) (map[string]db.PendingFolder, error) {
	return nil, nil
}

func (m *mockedModel) CandidateDevices(folder string) (map[protocol.DeviceID]db.CandidateDevice, error) {
	return nil, nil
}

func (m *mockedModel) CandidateFolders(device protocol.DeviceID) (map[string]db.CandidateFolder, error) {
	return nil, nil
}

func (m *mockedModel) FolderErrors(folder string) ([]model.FileError, error) {
	return nil, nil
}

func (m *mockedModel) WatchError(folder string) error {
	return nil
}

func (m *mockedModel) Serve(ctx context.Context) error { return nil }

func (m *mockedModel) Index(deviceID protocol.DeviceID, folder string, files []protocol.FileInfo) error {
	return nil
}

func (m *mockedModel) IndexUpdate(deviceID protocol.DeviceID, folder string, files []protocol.FileInfo) error {
	return nil
}

func (m *mockedModel) Request(deviceID protocol.DeviceID, folder, name string, blockNo, size int32, offset int64, hash []byte, weakHash uint32, fromTemporary bool) (protocol.RequestResponse, error) {
	return nil, nil
}

func (m *mockedModel) ClusterConfig(deviceID protocol.DeviceID, config protocol.ClusterConfig) error {
	return nil
}

func (m *mockedModel) Closed(conn protocol.Connection, err error) {}

func (m *mockedModel) DownloadProgress(deviceID protocol.DeviceID, folder string, updates []protocol.FileDownloadProgressUpdate) error {
	return nil
}

func (m *mockedModel) AddConnection(conn protocol.Connection, hello protocol.Hello) {}

func (m *mockedModel) OnHello(protocol.DeviceID, net.Addr, protocol.Hello) error {
	return nil
}

func (m *mockedModel) GetHello(protocol.DeviceID) protocol.HelloIntf {
	return nil
}

func (m *mockedModel) StartDeadlockDetector(timeout time.Duration) {}

func (m *mockedModel) DBSnapshot(_ string) (*db.Snapshot, error) {
	return nil, nil
}

type mockedFolderSummaryService struct{}

func (m *mockedFolderSummaryService) Serve(context.Context) error { return nil }

func (m *mockedFolderSummaryService) Summary(folder string) (map[string]interface{}, error) {
	return map[string]interface{}{"mocked": true}, nil
}

func (m *mockedFolderSummaryService) OnEventRequest() {}
