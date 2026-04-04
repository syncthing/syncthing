// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package tui

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/syncthing/syncthing/lib/config"
)

const (
	eventPollTimeout    = 60 // seconds
	maxEventLogSize     = 500
	connectionsInterval = 5 * time.Second
	reconnectMin        = 1 * time.Second
	reconnectMax        = 30 * time.Second
)

// --- Messages ---

// eventsMsg carries events from the long-poll.
type eventsMsg struct {
	events []Event
	lastID int
}

// fullStateMsg carries the initial full state (starts poll/tick).
type fullStateMsg struct {
	state *AppState
	err   error
}

// connectionsTickMsg triggers a periodic connections fetch for bandwidth rates.
type connectionsTickMsg time.Time

// actionResultMsg carries the result of a write action.
type actionResultMsg struct {
	action string
	err    error
}

// connectionsMsg carries updated connection data for bandwidth rate calculation.
type connectionsMsg struct {
	conns *ConnectionsResponse
}

// folderErrorsMsg carries folder errors fetched in the background.
type folderErrorsMsg struct {
	folderID string
	errors   []FolderError
}

// pendingDevicesMsg carries updated pending devices list.
type pendingDevicesMsg struct {
	pending map[string]PendingDevice
}

// pendingMsg carries both updated pending devices and folders.
type pendingMsg struct {
	devices map[string]PendingDevice
	folders map[string]PendingFolderEntry
}

// folderConfigMsg carries a fetched folder configuration for editing.
type folderConfigMsg struct {
	cfg config.FolderConfiguration
	err error
}

// deviceConfigMsg carries a fetched device configuration for editing.
type deviceConfigMsg struct {
	cfg config.DeviceConfiguration
	err error
}

// logsMsg carries fetched log entries.
type logsMsg struct {
	entries []LogEntry
	err     error
}

// reconnectedMsg indicates successful reconnection.
type reconnectedMsg struct{}

// errMsg carries an error from any background operation.
type errMsg struct {
	err    error
	source string // "events", "refresh", "action"
}

// --- Commands ---

// pollEvents long-polls for events and returns them as a message.
func pollEvents(client APIClient, lastID int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(eventPollTimeout+5)*time.Second)
		defer cancel()

		evts, err := client.EventsGet(ctx, lastID, eventPollTimeout)
		if err != nil {
			return errMsg{err: err, source: "events"}
		}

		maxID := lastID
		for _, e := range evts {
			if e.ID > maxID {
				maxID = e.ID
			}
		}
		return eventsMsg{events: evts, lastID: maxID}
	}
}

// fetchFullState fetches all data needed to build the initial state.
func fetchFullState(client APIClient) tea.Cmd {
	return func() tea.Msg {
		state := &AppState{Connected: true}

		// Version
		ver, err := client.SystemVersionGet()
		if err != nil {
			return fullStateMsg{err: err}
		}
		state.UpdateSystemVersion(ver)

		// Status
		status, err := client.SystemStatusGet()
		if err != nil {
			return fullStateMsg{err: err}
		}
		state.UpdateSystemStatus(status)

		// Config
		cfg, err := client.ConfigGet()
		if err != nil {
			return fullStateMsg{err: err}
		}
		state.InitFromConfig(cfg)

		// Connections
		conns, err := client.SystemConnectionsGet()
		if err != nil {
			return fullStateMsg{err: err}
		}
		state.UpdateConnections(conns)

		// Folder statuses
		for i, f := range state.Folders {
			summary, err := client.DBStatusGet(f.ID)
			if err == nil {
				state.Folders[i].Summary = &summary
				state.Folders[i].State = summary.State
				state.Folders[i].Error = summary.Error
			}
		}

		// Device stats
		devStats, err := client.StatsDeviceGet()
		if err == nil {
			state.UpdateDeviceStats(devStats)
		}

		// Folder stats
		folderStats, err := client.StatsFolderGet()
		if err == nil {
			state.UpdateFolderStats(folderStats)
		}

		// System errors
		sysErrors, err := client.SystemErrorsGet()
		if err == nil {
			state.SystemErrors = sysErrors
		}

		// Pending devices
		pendingDevs, err := client.PendingDevicesGet()
		if err == nil {
			state.UpdatePendingDevices(pendingDevs)
		}

		// Pending folders
		pendingFldrs, err := client.PendingFoldersGet()
		if err == nil {
			state.UpdatePendingFoldersFromAPI(pendingFldrs)
		}

		// Discovery (nearby devices not yet in config)
		discovery, err := client.DiscoveryGet()
		if err == nil {
			state.UpdateDiscovery(discovery)
		}

		// Restart required
		restart, err := client.RestartRequiredGet()
		if err == nil {
			state.RestartRequired = restart
		}

		state.LastUpdated = time.Now()
		return fullStateMsg{state: state}
	}
}

// refreshState fetches updated status data (not full config).
// fetchFolderErrors fetches folder errors in the background and returns
// them as a folderErrorsMsg, avoiding direct mutation of shared state.
func fetchFolderErrors(client APIClient, folderID string) tea.Cmd {
	return func() tea.Msg {
		errs, err := client.FolderErrorsGet(folderID)
		if err != nil {
			return folderErrorsMsg{folderID: folderID, errors: nil}
		}
		return folderErrorsMsg{folderID: folderID, errors: errs}
	}
}

// fetchConnections fetches current connection data for bandwidth rate updates.
func fetchConnections(client APIClient) tea.Cmd {
	return func() tea.Msg {
		conns, err := client.SystemConnectionsGet()
		if err != nil {
			return connectionsMsg{conns: nil}
		}
		return connectionsMsg{conns: &conns}
	}
}

// fetchPendingDevices fetches the current pending devices list.
func fetchPendingDevices(client APIClient) tea.Cmd {
	return func() tea.Msg {
		pending, err := client.PendingDevicesGet()
		if err != nil {
			return pendingDevicesMsg{pending: nil}
		}
		return pendingDevicesMsg{pending: pending}
	}
}

// fetchPending fetches both pending devices and pending folders.
func fetchPending(client APIClient) tea.Cmd {
	return func() tea.Msg {
		devs, _ := client.PendingDevicesGet()
		fldrs, _ := client.PendingFoldersGet()
		return pendingMsg{devices: devs, folders: fldrs}
	}
}

// tryReconnect attempts to reconnect to the daemon.
func tryReconnect(client APIClient, delay time.Duration) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(delay)
		if err := client.Ping(); err != nil {
			nextDelay := delay * 2
			if nextDelay > reconnectMax {
				nextDelay = reconnectMax
			}
			return errMsg{err: err, source: "reconnect"}
		}
		return reconnectedMsg{}
	}
}

// doAction executes a write action and returns the result.
func doAction(name string, fn func() error) tea.Cmd {
	return func() tea.Msg {
		return actionResultMsg{
			action: name,
			err:    fn(),
		}
	}
}

// tickConnections returns a command that triggers a connections fetch after the interval.
func tickConnections() tea.Cmd {
	return tea.Tick(connectionsInterval, func(t time.Time) tea.Msg {
		return connectionsTickMsg(t)
	})
}

// fetchFolderConfig fetches the full folder configuration for editing.
func fetchFolderConfig(client APIClient, folderID string) tea.Cmd {
	return func() tea.Msg {
		cfg, err := client.ConfigFolderGet(folderID)
		if err != nil {
			return folderConfigMsg{err: fmt.Errorf("folder %s: %w", folderID, err)}
		}
		return folderConfigMsg{cfg: cfg}
	}
}

// fetchDeviceConfig fetches the full device configuration for editing.
func fetchDeviceConfig(client APIClient, deviceID string) tea.Cmd {
	return func() tea.Msg {
		cfg, err := client.ConfigDeviceGet(deviceID)
		if err != nil {
			return deviceConfigMsg{err: fmt.Errorf("device %s: %w", deviceID, err)}
		}
		return deviceConfigMsg{cfg: cfg}
	}
}

// fetchLogs fetches the system log from the daemon.
func fetchLogs(client APIClient) tea.Cmd {
	return func() tea.Msg {
		entries, err := client.SystemLogGet()
		if err != nil {
			return logsMsg{err: err}
		}
		return logsMsg{entries: entries}
	}
}
