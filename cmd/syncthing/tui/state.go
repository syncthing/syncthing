// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package tui

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/model"
)

// AppState holds the entire TUI application state. Methods on AppState are
// pure state transformations with no I/O, making them highly testable.
type AppState struct {
	MyID            string
	Version         string
	StartTime       time.Time // daemon start time; uptime = time.Since(StartTime)
	Connected       bool
	RestartRequired bool

	ListenersTotal   int
	ListenersRunning int
	DiscoveryTotal   int
	DiscoveryRunning int

	Folders           []FolderState
	Devices           []DeviceState
	PendingDevs       []PendingDeviceState
	PendingFldrs      []PendingFolderState
	SystemErrors      []SystemError
	EventLog          []EventEntry
	DiscoveredDevices []DiscoveredDevice
	RecentChanges     []RecentChange

	ListenerDetails  []ListenerDetail
	DiscoveryDetails []DiscoveryDetail

	LastUpdated time.Time
}

// DiscoveredDevice holds a discovered device that is not yet in the config.
type DiscoveredDevice struct {
	DeviceID  string
	Name      string // may be empty for pure discovery entries
	Addresses []string
}

// ListenerDetail holds per-listener status details.
type ListenerDetail struct {
	Name         string
	Error        string
	LANAddresses []string
	WANAddresses []string
}

// DiscoveryDetail holds per-discovery-method status details.
type DiscoveryDetail struct {
	Name  string
	Error string
}

// RecentChange represents a recently synced file.
type RecentChange struct {
	Time   time.Time
	Folder string
	Path   string
	Action string // "update", "delete", "metadata", etc.
	Type   string // "file", "dir", "symlink"
}

// FolderState holds the combined config and runtime status for a folder.
type FolderState struct {
	ID               string
	Label            string
	Path             string
	Type             string
	Paused           bool
	State            string
	Error            string
	Summary          *model.FolderSummary
	Errors           []FolderError
	Completions      map[string]float64 // deviceID -> completion %
	DeviceIDs        []string           // devices sharing this folder
	LastScan         time.Time
	LastFile         string
	LastFileAt       time.Time
	RescanIntervalS  int
	FSWatcherEnabled bool
	PullOrder        string
}

// DeviceState holds the combined config and runtime status for a device.
type DeviceState struct {
	ID             string
	Name           string
	Connected      bool
	Paused         bool
	ClientVersion  string
	Address        string
	ConnectionType string
	InBytesTotal   int64
	OutBytesTotal  int64
	LastSeen       time.Time
	Completion     float64  // aggregate across shared folders
	FolderIDs      []string // folders shared with this device
	Compression    string
	NumConnections int

	// Bandwidth rate tracking (bytes per second), computed from successive polls.
	prevInBytes  int64
	prevOutBytes int64
	prevTime     time.Time
	InBytesRate  float64
	OutBytesRate float64
}

// PendingDeviceState holds a pending device introduction.
type PendingDeviceState struct {
	DeviceID string
	Name     string
	Address  string
	Time     time.Time
}

// PendingFolderState holds a pending folder share offer.
type PendingFolderState struct {
	FolderID   string
	Label      string
	DeviceID   string
	DeviceName string
	Time       time.Time
}

// EventEntry is a human-readable event for the event log.
type EventEntry struct {
	Time    time.Time
	Type    string
	Summary string
}

func (s *AppState) findDevice(id string) *DeviceState {
	for i := range s.Devices {
		if s.Devices[i].ID == id {
			return &s.Devices[i]
		}
	}
	return nil
}

func (s *AppState) findFolder(id string) *FolderState {
	for i := range s.Folders {
		if s.Folders[i].ID == id {
			return &s.Folders[i]
		}
	}
	return nil
}

// InitFromConfig populates folders and devices from a full configuration.
func (s *AppState) InitFromConfig(cfg config.Configuration) {
	// Build device name map
	deviceNames := make(map[string]string)
	for _, dev := range cfg.Devices {
		deviceNames[dev.DeviceID.String()] = dev.Name
	}

	s.Folders = make([]FolderState, 0, len(cfg.Folders))
	for _, f := range cfg.Folders {
		devIDs := make([]string, 0, len(f.Devices))
		for _, d := range f.Devices {
			devIDs = append(devIDs, d.DeviceID.String())
		}
		s.Folders = append(s.Folders, FolderState{
			ID:               f.ID,
			Label:            f.Label,
			Path:             f.Path,
			Type:             f.Type.String(),
			Paused:           f.Paused,
			Completions:      make(map[string]float64),
			DeviceIDs:        devIDs,
			RescanIntervalS:  f.RescanIntervalS,
			FSWatcherEnabled: f.FSWatcherEnabled,
			PullOrder:        f.Order.String(),
		})
	}

	s.Devices = make([]DeviceState, 0, len(cfg.Devices))
	for _, d := range cfg.Devices {
		// Find folders shared with this device
		var folderIDs []string
		for _, f := range cfg.Folders {
			for _, fd := range f.Devices {
				if fd.DeviceID == d.DeviceID {
					folderIDs = append(folderIDs, f.ID)
					break
				}
			}
		}
		compText, _ := d.Compression.MarshalText()
		s.Devices = append(s.Devices, DeviceState{
			ID:             d.DeviceID.String(),
			Name:           d.Name,
			Paused:         d.Paused,
			FolderIDs:      folderIDs,
			Compression:    string(compText),
			NumConnections: d.NumConnections(),
		})
	}
}

// UpdateSystemStatus updates system-level fields.
func (s *AppState) UpdateSystemStatus(status SystemStatus) {
	s.MyID = status.MyID
	if t, err := time.Parse(time.RFC3339, status.StartTime); err == nil {
		s.StartTime = t
	}
	s.ListenersTotal = len(status.ConnectionServiceStatus)
	s.ListenersRunning = 0
	s.ListenerDetails = nil
	for name, svc := range status.ConnectionServiceStatus {
		errStr := connectionSvcErrorString(svc.Error)
		if errStr == "" {
			s.ListenersRunning++
		}
		s.ListenerDetails = append(s.ListenerDetails, ListenerDetail{
			Name:         name,
			Error:        errStr,
			LANAddresses: svc.LANAddresses,
			WANAddresses: svc.WANAddresses,
		})
	}
	s.DiscoveryTotal = len(status.DiscoveryStatus)
	s.DiscoveryRunning = 0
	s.DiscoveryDetails = nil
	for name, svc := range status.DiscoveryStatus {
		errStr := connectionSvcErrorString(svc.Error)
		if errStr == "" {
			s.DiscoveryRunning++
		}
		s.DiscoveryDetails = append(s.DiscoveryDetails, DiscoveryDetail{
			Name:  name,
			Error: errStr,
		})
	}
}

// connectionSvcErrorString extracts an error string from the error field
// which can be nil (no error), a string, or another type.
func connectionSvcErrorString(err interface{}) string {
	if err == nil {
		return ""
	}
	if s, ok := err.(string); ok {
		if s == "" || s == "<nil>" {
			return ""
		}
		return s
	}
	return fmt.Sprintf("%v", err)
}

// UpdateSystemVersion updates version info.
func (s *AppState) UpdateSystemVersion(ver SystemVersion) {
	s.Version = ver.Version
}

// UpdateConnections updates device connection info from the connections response.
func (s *AppState) UpdateConnections(conns ConnectionsResponse) {
	now := time.Now()
	for id, info := range conns.Connections {
		if d := s.findDevice(id); d != nil {
			// Calculate bandwidth rates from byte deltas.
			if !d.prevTime.IsZero() && info.Connected {
				dt := now.Sub(d.prevTime).Seconds()
				if dt > 0 {
					d.InBytesRate = float64(info.InBytesTotal-d.prevInBytes) / dt
					d.OutBytesRate = float64(info.OutBytesTotal-d.prevOutBytes) / dt
					if d.InBytesRate < 0 {
						d.InBytesRate = 0
					}
					if d.OutBytesRate < 0 {
						d.OutBytesRate = 0
					}
				}
			}
			if !info.Connected {
				d.InBytesRate = 0
				d.OutBytesRate = 0
			}
			d.prevInBytes = info.InBytesTotal
			d.prevOutBytes = info.OutBytesTotal
			d.prevTime = now

			d.Connected = info.Connected
			d.Paused = info.Paused
			d.ClientVersion = info.ClientVersion
			d.Address = info.Address
			d.ConnectionType = info.Type
			d.InBytesTotal = info.InBytesTotal
			d.OutBytesTotal = info.OutBytesTotal
		}
	}
}

// UpdateFolderStatus updates a single folder's summary.
func (s *AppState) UpdateFolderStatus(folderID string, summary model.FolderSummary) {
	if f := s.findFolder(folderID); f != nil {
		f.Summary = &summary
		f.State = summary.State
		f.Error = summary.Error
	}
}

// UpdateFolderErrors updates a folder's error list.
func (s *AppState) UpdateFolderErrors(folderID string, errors []FolderError) {
	if f := s.findFolder(folderID); f != nil {
		f.Errors = errors
	}
}

// UpdateCompletion updates a device's completion for a specific folder.
func (s *AppState) UpdateCompletion(deviceID, folderID string, pct float64) {
	if f := s.findFolder(folderID); f != nil {
		f.Completions[deviceID] = pct
	}
	// Recompute device aggregate
	s.recomputeDeviceCompletion(deviceID)
}

func (s *AppState) recomputeDeviceCompletion(deviceID string) {
	d := s.findDevice(deviceID)
	if d == nil {
		return
	}
	var total float64
	var count int
	for _, fid := range d.FolderIDs {
		if f := s.findFolder(fid); f != nil {
			if pct, ok := f.Completions[deviceID]; ok {
				total += pct
				count++
			}
		}
	}
	if count > 0 {
		d.Completion = total / float64(count)
	}
}

// UpdateFolderStats updates folder statistics (last scan, last file).
func (s *AppState) UpdateFolderStats(stats map[string]FolderStatistics) {
	for i := range s.Folders {
		if st, ok := stats[s.Folders[i].ID]; ok {
			s.Folders[i].LastScan = st.LastScan
			s.Folders[i].LastFile = st.LastFile.Filename
			s.Folders[i].LastFileAt = st.LastFile.At
		}
	}
}

// UpdateDeviceStats updates device lastSeen from stats.
func (s *AppState) UpdateDeviceStats(stats map[string]DeviceStatistics) {
	for id, st := range stats {
		if d := s.findDevice(id); d != nil {
			d.LastSeen = st.LastSeen
		}
	}
}

// UpdatePendingDevices replaces the pending devices list.
func (s *AppState) UpdatePendingDevices(pending map[string]PendingDevice) {
	s.PendingDevs = make([]PendingDeviceState, 0, len(pending))
	for id, p := range pending {
		s.PendingDevs = append(s.PendingDevs, PendingDeviceState{
			DeviceID: id,
			Name:     p.Name,
			Address:  p.Address,
			Time:     p.Time,
		})
	}
	slices.SortFunc(s.PendingDevs, func(a, b PendingDeviceState) int {
		return strings.Compare(a.DeviceID, b.DeviceID)
	})
}

// UpdatePendingFoldersFromAPI replaces the pending folders list from the API response.
func (s *AppState) UpdatePendingFoldersFromAPI(pending map[string]PendingFolderEntry) {
	s.PendingFldrs = nil
	for folderID, entry := range pending {
		for deviceID, offer := range entry.OfferedBy {
			devName := ""
			if d := s.findDevice(deviceID); d != nil {
				devName = d.DisplayName()
			}
			s.PendingFldrs = append(s.PendingFldrs, PendingFolderState{
				FolderID:   folderID,
				Label:      offer.Label,
				DeviceID:   deviceID,
				DeviceName: devName,
				Time:       offer.Time,
			})
		}
	}
	slices.SortFunc(s.PendingFldrs, func(a, b PendingFolderState) int {
		if c := strings.Compare(a.FolderID, b.FolderID); c != 0 {
			return c
		}
		return strings.Compare(a.DeviceID, b.DeviceID)
	})
}

// UpdateDiscovery updates the list of discovered devices, filtering out those
// already present in the configuration.
func (s *AppState) UpdateDiscovery(discovered map[string]DiscoveryEntry) {
	s.DiscoveredDevices = nil
	known := make(map[string]bool, len(s.Devices))
	for _, d := range s.Devices {
		known[d.ID] = true
	}
	for id, entry := range discovered {
		if !known[id] {
			s.DiscoveredDevices = append(s.DiscoveredDevices, DiscoveredDevice{
				DeviceID:  id,
				Addresses: entry.Addresses,
			})
		}
	}
	slices.SortFunc(s.DiscoveredDevices, func(a, b DiscoveredDevice) int {
		return strings.Compare(a.DeviceID, b.DeviceID)
	})
}

// ProcessEvent applies a single event to the state and returns a log entry.
func (s *AppState) ProcessEvent(evt Event) *EventEntry {
	entry := &EventEntry{
		Time: evt.Time,
		Type: evt.Type,
	}

	switch evt.Type {
	case "FolderSummary":
		var data model.FolderSummaryEventData
		if json.Unmarshal(evt.Data, &data) == nil && data.Summary != nil {
			s.UpdateFolderStatus(data.Folder, *data.Summary)
			entry.Summary = "Folder " + data.Folder + " updated"
		}

	case "StateChanged":
		var data struct {
			Folder string `json:"folder"`
			From   string `json:"from"`
			To     string `json:"to"`
		}
		if json.Unmarshal(evt.Data, &data) == nil {
			if f := s.findFolder(data.Folder); f != nil {
				f.State = data.To
			}
			entry.Summary = "Folder " + data.Folder + ": " + data.From + " -> " + data.To
		}

	case "FolderCompletion":
		var data struct {
			Folder     string  `json:"folder"`
			Device     string  `json:"device"`
			Completion float64 `json:"completion"`
		}
		if json.Unmarshal(evt.Data, &data) == nil {
			s.UpdateCompletion(data.Device, data.Folder, data.Completion)
			entry.Summary = "Completion " + data.Folder + " @ " + shortDeviceID(data.Device) + ": " + formatPercent(data.Completion)
		}

	case "DeviceConnected":
		var data struct {
			ID   string `json:"id"`
			Addr string `json:"addr"`
		}
		if json.Unmarshal(evt.Data, &data) == nil {
			if d := s.findDevice(data.ID); d != nil {
				d.Connected = true
				d.Address = data.Addr
			}
			entry.Summary = "Device " + shortDeviceID(data.ID) + " connected"
		}

	case "DeviceDisconnected":
		var data struct {
			ID    string `json:"id"`
			Error string `json:"error"`
		}
		if json.Unmarshal(evt.Data, &data) == nil {
			if d := s.findDevice(data.ID); d != nil {
				d.Connected = false
				d.InBytesRate = 0
				d.OutBytesRate = 0
			}
			entry.Summary = "Device " + shortDeviceID(data.ID) + " disconnected"
		}

	case "DevicePaused":
		var data struct {
			Device string `json:"device"`
		}
		if json.Unmarshal(evt.Data, &data) == nil {
			if d := s.findDevice(data.Device); d != nil {
				d.Paused = true
			}
			entry.Summary = "Device " + shortDeviceID(data.Device) + " paused"
		}

	case "DeviceResumed":
		var data struct {
			Device string `json:"device"`
		}
		if json.Unmarshal(evt.Data, &data) == nil {
			if d := s.findDevice(data.Device); d != nil {
				d.Paused = false
			}
			entry.Summary = "Device " + shortDeviceID(data.Device) + " resumed"
		}

	case "FolderErrors":
		var data struct {
			Folder string        `json:"folder"`
			Errors []FolderError `json:"errors"`
		}
		if json.Unmarshal(evt.Data, &data) == nil {
			s.UpdateFolderErrors(data.Folder, data.Errors)
			entry.Summary = "Folder " + data.Folder + ": " + pluralize(len(data.Errors), "error", "errors")
		}

	case "FolderPaused":
		var data struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(evt.Data, &data) == nil {
			if f := s.findFolder(data.ID); f != nil {
				f.Paused = true
			}
			entry.Summary = "Folder " + data.ID + " paused"
		}

	case "FolderResumed":
		var data struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(evt.Data, &data) == nil {
			if f := s.findFolder(data.ID); f != nil {
				f.Paused = false
			}
			entry.Summary = "Folder " + data.ID + " resumed"
		}

	case "ItemFinished":
		var data struct {
			Folder string `json:"folder"`
			Item   string `json:"item"`
			Error  string `json:"error"`
			Type   string `json:"type"`
			Action string `json:"action"`
		}
		if json.Unmarshal(evt.Data, &data) == nil {
			if data.Error != "" {
				entry.Summary = "Error syncing " + data.Item + " in " + data.Folder + ": " + data.Error
			} else {
				entry.Summary = "Synced " + data.Item + " in " + data.Folder
				// Track successfully synced files as recent changes
				s.AddRecentChange(RecentChange{
					Time:   evt.Time,
					Folder: data.Folder,
					Path:   data.Item,
					Action: data.Action,
					Type:   data.Type,
				})
			}
		}

	case "ItemStarted":
		var data struct {
			Folder string `json:"folder"`
			Item   string `json:"item"`
			Type   string `json:"type"`
			Action string `json:"action"`
		}
		if json.Unmarshal(evt.Data, &data) == nil {
			entry.Summary = "Syncing " + data.Item + " in " + data.Folder
		}

	case "PendingDevicesChanged":
		entry.Summary = "Pending devices changed"

	case "PendingFoldersChanged":
		entry.Summary = "Pending folders changed"

	case "ConfigSaved":
		entry.Summary = "Configuration saved"

	default:
		entry.Summary = evt.Type
	}

	return entry
}

// AddEventLog adds an entry to the log, capping at maxEvents.
// Consecutive entries with the same Type and Summary are deduplicated.
func (s *AppState) AddEventLog(entry EventEntry, maxEvents int) {
	if n := len(s.EventLog); n > 0 {
		last := s.EventLog[n-1]
		if last.Type == entry.Type && last.Summary == entry.Summary {
			return
		}
	}
	s.EventLog = append(s.EventLog, entry)
	if len(s.EventLog) > maxEvents {
		s.EventLog = s.EventLog[len(s.EventLog)-maxEvents:]
	}
}

const maxRecentChanges = 50

// AddRecentChange adds a recently synced file to the list, capping at maxRecentChanges.
func (s *AppState) AddRecentChange(rc RecentChange) {
	s.RecentChanges = append(s.RecentChanges, rc)
	if len(s.RecentChanges) > maxRecentChanges {
		s.RecentChanges = s.RecentChanges[len(s.RecentChanges)-maxRecentChanges:]
	}
}

// FolderDisplayName returns the label if set, otherwise the ID.
func (f *FolderState) DisplayName() string {
	if f.Label != "" {
		return f.Label
	}
	return f.ID
}

// SyncPercent returns the sync completion percentage for a folder.
func (f *FolderState) SyncPercent() float64 {
	if f.Summary == nil {
		return 0
	}
	if f.Summary.GlobalBytes == 0 {
		return 100
	}
	return 100 * float64(f.Summary.InSyncBytes) / float64(f.Summary.GlobalBytes)
}

// DisplayName returns the name if set, otherwise a short ID.
func (d *DeviceState) DisplayName() string {
	if d.Name != "" {
		return d.Name
	}
	return shortDeviceID(d.ID)
}
