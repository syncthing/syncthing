// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package tui

import (
	"context"
	"sync"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/model"
)

// mockClient implements APIClient for testing. All methods are no-ops by
// default, returning zero values. Override individual fields to customize.
type mockClient struct {
	mu    sync.Mutex
	calls []string // records method names in call order

	// Override return values for specific methods
	pingErr               error
	configGetResult       config.Configuration
	configGetErr          error
	configFolderGetResult config.FolderConfiguration
	configFolderGetErr    error
	configDeviceGetResult config.DeviceConfiguration
	configDeviceGetErr    error
	folderAddErr          error
	folderUpdateErr       error
	folderRemoveErr       error
	deviceAddErr          error
	deviceUpdateErr       error
	deviceRemoveErr       error
	pauseErr              error
	resumeErr             error
	scanErr               error
	overrideErr           error
	revertErr             error
	restartErr            error
	shutdownErr           error
	errorsClearErr        error
	pendingDismissErr     error
}

func (m *mockClient) record(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, name)
}

func (m *mockClient) getCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.calls))
	copy(result, m.calls)
	return result
}

func (m *mockClient) Ping() error {
	m.record("Ping")
	return m.pingErr
}

func (m *mockClient) SystemStatusGet() (SystemStatus, error) {
	m.record("SystemStatusGet")
	return SystemStatus{}, nil
}

func (m *mockClient) SystemVersionGet() (SystemVersion, error) {
	m.record("SystemVersionGet")
	return SystemVersion{}, nil
}

func (m *mockClient) SystemConnectionsGet() (ConnectionsResponse, error) {
	m.record("SystemConnectionsGet")
	return ConnectionsResponse{}, nil
}

func (m *mockClient) SystemErrorsGet() ([]SystemError, error) {
	m.record("SystemErrorsGet")
	return nil, nil
}

func (m *mockClient) ConfigGet() (config.Configuration, error) {
	m.record("ConfigGet")
	return m.configGetResult, m.configGetErr
}

func (m *mockClient) DBStatusGet(folderID string) (model.FolderSummary, error) {
	m.record("DBStatusGet")
	return model.FolderSummary{}, nil
}

func (m *mockClient) FolderErrorsGet(folderID string) ([]FolderError, error) {
	m.record("FolderErrorsGet")
	return nil, nil
}

func (m *mockClient) StatsDeviceGet() (map[string]DeviceStatistics, error) {
	m.record("StatsDeviceGet")
	return nil, nil
}

func (m *mockClient) StatsFolderGet() (map[string]FolderStatistics, error) {
	m.record("StatsFolderGet")
	return nil, nil
}

func (m *mockClient) PendingDevicesGet() (map[string]PendingDevice, error) {
	m.record("PendingDevicesGet")
	return nil, nil
}

func (m *mockClient) PendingFoldersGet() (map[string]PendingFolderEntry, error) {
	m.record("PendingFoldersGet")
	return nil, nil
}

func (m *mockClient) DiscoveryGet() (map[string]DiscoveryEntry, error) {
	m.record("DiscoveryGet")
	return nil, nil
}

func (m *mockClient) RestartRequiredGet() (bool, error) {
	m.record("RestartRequiredGet")
	return false, nil
}

func (m *mockClient) EventsGet(_ context.Context, _ int, _ int) ([]Event, error) {
	m.record("EventsGet")
	return nil, nil
}

func (m *mockClient) ConfigFolderGet(id string) (config.FolderConfiguration, error) {
	m.record("ConfigFolderGet")
	return m.configFolderGetResult, m.configFolderGetErr
}

func (m *mockClient) ConfigDeviceGet(id string) (config.DeviceConfiguration, error) {
	m.record("ConfigDeviceGet")
	return m.configDeviceGetResult, m.configDeviceGetErr
}

func (m *mockClient) FolderAdd(cfg config.FolderConfiguration) error {
	m.record("FolderAdd")
	return m.folderAddErr
}

func (m *mockClient) FolderUpdate(cfg config.FolderConfiguration) error {
	m.record("FolderUpdate")
	return m.folderUpdateErr
}

func (m *mockClient) FolderRemove(id string) error {
	m.record("FolderRemove")
	return m.folderRemoveErr
}

func (m *mockClient) DeviceAdd(cfg config.DeviceConfiguration) error {
	m.record("DeviceAdd")
	return m.deviceAddErr
}

func (m *mockClient) DeviceUpdate(cfg config.DeviceConfiguration) error {
	m.record("DeviceUpdate")
	return m.deviceUpdateErr
}

func (m *mockClient) DeviceRemove(id string) error {
	m.record("DeviceRemove")
	return m.deviceRemoveErr
}

func (m *mockClient) PendingDeviceDismiss(deviceID string) error {
	m.record("PendingDeviceDismiss")
	return m.pendingDismissErr
}

func (m *mockClient) PendingFolderDismiss(deviceID, folderID string) error {
	m.record("PendingFolderDismiss")
	return m.pendingDismissErr
}

func (m *mockClient) Pause(deviceID string) error {
	m.record("Pause")
	return m.pauseErr
}

func (m *mockClient) Resume(deviceID string) error {
	m.record("Resume")
	return m.resumeErr
}

func (m *mockClient) Scan(folderID string) error {
	m.record("Scan")
	return m.scanErr
}

func (m *mockClient) Override(folderID string) error {
	m.record("Override")
	return m.overrideErr
}

func (m *mockClient) Revert(folderID string) error {
	m.record("Revert")
	return m.revertErr
}

func (m *mockClient) Restart() error {
	m.record("Restart")
	return m.restartErr
}

func (m *mockClient) Shutdown() error {
	m.record("Shutdown")
	return m.shutdownErr
}

func (m *mockClient) ErrorsClear() error {
	m.record("ErrorsClear")
	return m.errorsClearErr
}

func (m *mockClient) SystemLogGet() ([]LogEntry, error) {
	m.record("SystemLogGet")
	return nil, nil
}

func (m *mockClient) GUIConfigGet() (config.GUIConfiguration, error) {
	m.record("GUIConfigGet")
	return config.GUIConfiguration{}, nil
}

func (m *mockClient) GUIConfigPatch(gui config.GUIConfiguration) error {
	m.record("GUIConfigPatch")
	return nil
}
