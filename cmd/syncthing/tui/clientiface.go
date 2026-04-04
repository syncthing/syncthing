// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package tui

import (
	"context"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/model"
)

// APIClient defines the interface used by the TUI to interact with the
// Syncthing REST API. This enables testing with mock implementations.
type APIClient interface {
	Ping() error
	SystemStatusGet() (SystemStatus, error)
	SystemVersionGet() (SystemVersion, error)
	SystemConnectionsGet() (ConnectionsResponse, error)
	SystemErrorsGet() ([]SystemError, error)
	ConfigGet() (config.Configuration, error)
	DBStatusGet(folderID string) (model.FolderSummary, error)
	FolderErrorsGet(folderID string) ([]FolderError, error)
	StatsDeviceGet() (map[string]DeviceStatistics, error)
	StatsFolderGet() (map[string]FolderStatistics, error)
	PendingDevicesGet() (map[string]PendingDevice, error)
	PendingFoldersGet() (map[string]PendingFolderEntry, error)
	DiscoveryGet() (map[string]DiscoveryEntry, error)
	RestartRequiredGet() (bool, error)
	EventsGet(ctx context.Context, since int, timeout int) ([]Event, error)

	ConfigFolderGet(id string) (config.FolderConfiguration, error)
	ConfigDeviceGet(id string) (config.DeviceConfiguration, error)
	FolderAdd(cfg config.FolderConfiguration) error
	FolderUpdate(cfg config.FolderConfiguration) error
	FolderRemove(id string) error
	DeviceAdd(cfg config.DeviceConfiguration) error
	DeviceUpdate(cfg config.DeviceConfiguration) error
	DeviceRemove(id string) error
	PendingDeviceDismiss(deviceID string) error
	PendingFolderDismiss(deviceID, folderID string) error
	Pause(deviceID string) error
	Resume(deviceID string) error
	Scan(folderID string) error
	Override(folderID string) error
	Revert(folderID string) error
	Restart() error
	Shutdown() error
	ErrorsClear() error
	SystemLogGet() ([]LogEntry, error)

	GUIConfigGet() (config.GUIConfiguration, error)
	GUIConfigPatch(gui config.GUIConfiguration) error
}

// Verify Client satisfies APIClient at compile time.
var _ APIClient = (*Client)(nil)
