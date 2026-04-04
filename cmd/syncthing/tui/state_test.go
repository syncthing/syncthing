// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package tui

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/model"
	"github.com/syncthing/syncthing/lib/protocol"
)

func testConfig() config.Configuration {
	// Use distinct non-zero device IDs (not valid Luhn-encoded, but work for tests)
	var dev1 protocol.DeviceID
	dev1[0] = 1
	var dev2 protocol.DeviceID
	dev2[0] = 2

	return config.Configuration{
		Folders: []config.FolderConfiguration{
			{
				ID:    "folder1",
				Label: "My Folder",
				Path:  "/home/user/sync",
				Type:  config.FolderTypeSendReceive,
				Devices: []config.FolderDeviceConfiguration{
					{DeviceID: dev1},
					{DeviceID: dev2},
				},
			},
			{
				ID:   "folder2",
				Path: "/home/user/photos",
				Type: config.FolderTypeSendOnly,
				Devices: []config.FolderDeviceConfiguration{
					{DeviceID: dev1},
				},
			},
		},
		Devices: []config.DeviceConfiguration{
			{DeviceID: dev1, Name: "Laptop"},
			{DeviceID: dev2, Name: "Phone"},
		},
	}
}

func TestInitFromConfig(t *testing.T) {
	t.Parallel()
	var s AppState
	s.InitFromConfig(testConfig())

	if len(s.Folders) != 2 {
		t.Fatalf("expected 2 folders, got %d", len(s.Folders))
	}
	if s.Folders[0].Label != "My Folder" {
		t.Errorf("expected label 'My Folder', got %q", s.Folders[0].Label)
	}
	if len(s.Folders[0].DeviceIDs) != 2 {
		t.Errorf("expected 2 devices on folder1, got %d", len(s.Folders[0].DeviceIDs))
	}

	if len(s.Devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(s.Devices))
	}
	if s.Devices[0].Name != "Laptop" {
		t.Errorf("expected name 'Laptop', got %q", s.Devices[0].Name)
	}
	// Laptop shares both folders
	if len(s.Devices[0].FolderIDs) != 2 {
		t.Errorf("expected laptop to share 2 folders, got %d", len(s.Devices[0].FolderIDs))
	}
	// Phone shares only folder1
	if len(s.Devices[1].FolderIDs) != 1 {
		t.Errorf("expected phone to share 1 folder, got %d", len(s.Devices[1].FolderIDs))
	}
}

func TestUpdateConnections(t *testing.T) {
	t.Parallel()
	var s AppState
	s.InitFromConfig(testConfig())

	dev1ID := s.Devices[0].ID
	s.UpdateConnections(ConnectionsResponse{
		Connections: map[string]ConnectionInfo{
			dev1ID: {
				Connected:     true,
				ClientVersion: "v1.29.0",
				InBytesTotal:  1024,
				OutBytesTotal: 2048,
				Address:       "192.168.1.2:22000",
			},
		},
	})

	if !s.Devices[0].Connected {
		t.Error("expected device 0 to be connected")
	}
	if s.Devices[0].InBytesTotal != 1024 {
		t.Errorf("expected InBytesTotal 1024, got %d", s.Devices[0].InBytesTotal)
	}
	if s.Devices[1].Connected {
		t.Error("expected device 1 to remain disconnected")
	}
}

func TestUpdateFolderStatus(t *testing.T) {
	t.Parallel()
	var s AppState
	s.InitFromConfig(testConfig())

	summary := model.FolderSummary{
		GlobalFiles: 100,
		GlobalBytes: 1000,
		InSyncFiles: 90,
		InSyncBytes: 900,
		NeedFiles:   10,
		NeedBytes:   100,
		State:       "syncing",
	}

	s.UpdateFolderStatus("folder1", summary)

	if s.Folders[0].State != "syncing" {
		t.Errorf("expected state syncing, got %s", s.Folders[0].State)
	}
	if s.Folders[0].Summary.GlobalFiles != 100 {
		t.Errorf("expected 100 global files, got %d", s.Folders[0].Summary.GlobalFiles)
	}
	if s.Folders[0].SyncPercent() != 90 {
		t.Errorf("expected 90%% sync, got %f", s.Folders[0].SyncPercent())
	}
}

func TestUpdateCompletion(t *testing.T) {
	t.Parallel()
	var s AppState
	s.InitFromConfig(testConfig())

	dev1ID := s.Devices[0].ID

	s.UpdateCompletion(dev1ID, "folder1", 75.0)
	s.UpdateCompletion(dev1ID, "folder2", 50.0)

	if s.Folders[0].Completions[dev1ID] != 75.0 {
		t.Errorf("expected folder1 completion 75, got %f", s.Folders[0].Completions[dev1ID])
	}

	// Device aggregate should be average of both folders: (75+50)/2 = 62.5
	if s.Devices[0].Completion != 62.5 {
		t.Errorf("expected device completion 62.5, got %f", s.Devices[0].Completion)
	}
}

func TestProcessEventDeviceConnected(t *testing.T) {
	t.Parallel()
	var s AppState
	s.InitFromConfig(testConfig())

	dev2ID := s.Devices[1].ID
	data, _ := json.Marshal(map[string]string{
		"id":   dev2ID,
		"addr": "10.0.0.1:22000",
	})

	entry := s.ProcessEvent(Event{
		Type: "DeviceConnected",
		Time: time.Now(),
		Data: data,
	})

	if !s.Devices[1].Connected {
		t.Error("expected device 1 to be connected after event")
	}
	if s.Devices[1].Address != "10.0.0.1:22000" {
		t.Errorf("expected address 10.0.0.1:22000, got %s", s.Devices[1].Address)
	}
	if entry.Type != "DeviceConnected" {
		t.Errorf("expected event type DeviceConnected, got %s", entry.Type)
	}
}

func TestProcessEventStateChanged(t *testing.T) {
	t.Parallel()
	var s AppState
	s.InitFromConfig(testConfig())

	data, _ := json.Marshal(map[string]string{
		"folder": "folder1",
		"from":   "idle",
		"to":     "syncing",
	})

	s.ProcessEvent(Event{
		Type: "StateChanged",
		Time: time.Now(),
		Data: data,
	})

	if s.Folders[0].State != "syncing" {
		t.Errorf("expected state syncing, got %s", s.Folders[0].State)
	}
}

func TestProcessEventFolderPaused(t *testing.T) {
	t.Parallel()
	var s AppState
	s.InitFromConfig(testConfig())

	data, _ := json.Marshal(map[string]string{"id": "folder1"})
	s.ProcessEvent(Event{
		Type: "FolderPaused",
		Time: time.Now(),
		Data: data,
	})

	if !s.Folders[0].Paused {
		t.Error("expected folder1 to be paused")
	}
}

func TestFolderDisplayName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		id, label, expected string
	}{
		{"folder1", "My Folder", "My Folder"},
		{"folder2", "", "folder2"},
	}
	for _, tt := range tests {
		f := FolderState{ID: tt.id, Label: tt.label}
		if got := f.DisplayName(); got != tt.expected {
			t.Errorf("FolderState{ID: %q, Label: %q}.DisplayName() = %q, want %q", tt.id, tt.label, got, tt.expected)
		}
	}
}

func TestSyncPercent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		summary  *model.FolderSummary
		expected float64
	}{
		{"nil summary", nil, 0},
		{"zero global", &model.FolderSummary{GlobalBytes: 0}, 100},
		{"half synced", &model.FolderSummary{GlobalBytes: 1000, InSyncBytes: 500}, 50},
		{"fully synced", &model.FolderSummary{GlobalBytes: 1000, InSyncBytes: 1000}, 100},
	}
	for _, tt := range tests {
		f := FolderState{Summary: tt.summary}
		if got := f.SyncPercent(); got != tt.expected {
			t.Errorf("%s: SyncPercent() = %f, want %f", tt.name, got, tt.expected)
		}
	}
}

func TestAddEventLog(t *testing.T) {
	t.Parallel()
	var s AppState
	for i := 0; i < 150; i++ {
		s.AddEventLog(EventEntry{Summary: fmt.Sprintf("test %d", i)}, 100)
	}
	if len(s.EventLog) != 100 {
		t.Errorf("expected 100 events, got %d", len(s.EventLog))
	}
}

func TestAddEventLogDedup(t *testing.T) {
	t.Parallel()
	var s AppState
	// Add the same event multiple times; only one should be kept.
	for i := 0; i < 10; i++ {
		s.AddEventLog(EventEntry{Type: "FolderCompletion", Summary: "Completion test-sync @ XYPCIS5: 100%"}, 100)
	}
	if len(s.EventLog) != 1 {
		t.Errorf("expected 1 deduplicated event, got %d", len(s.EventLog))
	}
	// Add a different event, then the same again.
	s.AddEventLog(EventEntry{Type: "StateChanged", Summary: "Folder test: idle -> syncing"}, 100)
	if len(s.EventLog) != 2 {
		t.Errorf("expected 2 events, got %d", len(s.EventLog))
	}
	s.AddEventLog(EventEntry{Type: "StateChanged", Summary: "Folder test: idle -> syncing"}, 100)
	if len(s.EventLog) != 2 {
		t.Errorf("expected 2 events after dedup, got %d", len(s.EventLog))
	}
}

func TestUpdatePendingDevices(t *testing.T) {
	t.Parallel()
	var s AppState
	s.UpdatePendingDevices(map[string]PendingDevice{
		"NEW-DEV-1": {Name: "New Device", Address: "192.168.1.5", Time: time.Now()},
	})
	if len(s.PendingDevs) != 1 {
		t.Fatalf("expected 1 pending device, got %d", len(s.PendingDevs))
	}
	if s.PendingDevs[0].Name != "New Device" {
		t.Errorf("expected name 'New Device', got %q", s.PendingDevs[0].Name)
	}
}

func TestBandwidthRateCalculation(t *testing.T) {
	t.Parallel()
	var s AppState
	s.InitFromConfig(testConfig())

	dev1ID := s.Devices[0].ID

	// First poll: establishes baseline, no rate yet.
	s.UpdateConnections(ConnectionsResponse{
		Connections: map[string]ConnectionInfo{
			dev1ID: {
				Connected:     true,
				InBytesTotal:  1000,
				OutBytesTotal: 500,
			},
		},
	})
	if s.Devices[0].InBytesRate != 0 {
		t.Errorf("expected InBytesRate 0 on first poll, got %f", s.Devices[0].InBytesRate)
	}

	// Simulate time passing by setting prevTime to 5 seconds ago.
	s.Devices[0].prevTime = time.Now().Add(-5 * time.Second)

	// Second poll: rate should be > 0.
	s.UpdateConnections(ConnectionsResponse{
		Connections: map[string]ConnectionInfo{
			dev1ID: {
				Connected:     true,
				InBytesTotal:  6000, // +5000 over ~5s => ~1000 B/s
				OutBytesTotal: 2500, // +2000 over ~5s => ~400 B/s
			},
		},
	})
	if s.Devices[0].InBytesRate <= 0 {
		t.Errorf("expected positive InBytesRate, got %f", s.Devices[0].InBytesRate)
	}
	if s.Devices[0].OutBytesRate <= 0 {
		t.Errorf("expected positive OutBytesRate, got %f", s.Devices[0].OutBytesRate)
	}
}

func TestBandwidthRateZeroOnDisconnect(t *testing.T) {
	t.Parallel()
	var s AppState
	s.InitFromConfig(testConfig())

	dev1ID := s.Devices[0].ID

	// Establish baseline.
	s.UpdateConnections(ConnectionsResponse{
		Connections: map[string]ConnectionInfo{
			dev1ID: {Connected: true, InBytesTotal: 1000, OutBytesTotal: 500},
		},
	})
	s.Devices[0].prevTime = time.Now().Add(-5 * time.Second)

	// Second poll with data.
	s.UpdateConnections(ConnectionsResponse{
		Connections: map[string]ConnectionInfo{
			dev1ID: {Connected: true, InBytesTotal: 6000, OutBytesTotal: 2500},
		},
	})
	if s.Devices[0].InBytesRate <= 0 {
		t.Fatal("expected positive rate before disconnect")
	}

	// Device disconnects.
	s.UpdateConnections(ConnectionsResponse{
		Connections: map[string]ConnectionInfo{
			dev1ID: {Connected: false, InBytesTotal: 0, OutBytesTotal: 0},
		},
	})
	if s.Devices[0].InBytesRate != 0 {
		t.Errorf("expected InBytesRate 0 after disconnect, got %f", s.Devices[0].InBytesRate)
	}
	if s.Devices[0].OutBytesRate != 0 {
		t.Errorf("expected OutBytesRate 0 after disconnect, got %f", s.Devices[0].OutBytesRate)
	}
}

func TestBandwidthRateNegativeClamp(t *testing.T) {
	t.Parallel()
	var s AppState
	s.InitFromConfig(testConfig())

	dev1ID := s.Devices[0].ID

	// Establish baseline with high byte counts.
	s.UpdateConnections(ConnectionsResponse{
		Connections: map[string]ConnectionInfo{
			dev1ID: {Connected: true, InBytesTotal: 10000, OutBytesTotal: 5000},
		},
	})
	s.Devices[0].prevTime = time.Now().Add(-5 * time.Second)

	// Counter reset: new totals are lower than previous (e.g., daemon restart).
	s.UpdateConnections(ConnectionsResponse{
		Connections: map[string]ConnectionInfo{
			dev1ID: {Connected: true, InBytesTotal: 100, OutBytesTotal: 50},
		},
	})
	if s.Devices[0].InBytesRate < 0 {
		t.Errorf("expected non-negative InBytesRate, got %f", s.Devices[0].InBytesRate)
	}
	if s.Devices[0].OutBytesRate < 0 {
		t.Errorf("expected non-negative OutBytesRate, got %f", s.Devices[0].OutBytesRate)
	}
}

// TestProcessEventUnknownEvent verifies that ProcessEvent doesn't crash on
// unknown or malformed event types and returns a sensible log entry.
func TestProcessEventUnknownEvent(t *testing.T) {
	t.Parallel()
	var s AppState
	s.InitFromConfig(testConfig())

	// Unknown event type
	entry := s.ProcessEvent(Event{
		Type: "SomeFutureEvent",
		Time: time.Now(),
		Data: json.RawMessage(`{"key": "value"}`),
	})
	if entry == nil {
		t.Fatal("ProcessEvent returned nil for unknown event")
	}
	if entry.Summary != "SomeFutureEvent" {
		t.Errorf("expected summary 'SomeFutureEvent', got %q", entry.Summary)
	}
	if entry.Type != "SomeFutureEvent" {
		t.Errorf("expected type 'SomeFutureEvent', got %q", entry.Type)
	}
}

// TestProcessEventMalformedData verifies that ProcessEvent handles
// malformed JSON data without crashing.
func TestProcessEventMalformedData(t *testing.T) {
	t.Parallel()
	var s AppState
	s.InitFromConfig(testConfig())

	tests := []struct {
		name    string
		evtType string
		data    string
	}{
		{"empty data", "DeviceConnected", "{}"},
		{"null data", "StateChanged", "null"},
		{"invalid json", "FolderSummary", "{invalid json}"},
		{"empty string", "FolderCompletion", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			entry := s.ProcessEvent(Event{
				Type: tt.evtType,
				Time: time.Now(),
				Data: json.RawMessage(tt.data),
			})
			if entry == nil {
				t.Error("ProcessEvent returned nil")
			}
		})
	}
}

// TestProcessEventNilData verifies that ProcessEvent handles nil data.
func TestProcessEventNilData(t *testing.T) {
	t.Parallel()
	var s AppState

	entry := s.ProcessEvent(Event{
		Type: "ConfigSaved",
		Time: time.Now(),
		Data: nil,
	})
	if entry == nil {
		t.Fatal("ProcessEvent returned nil for ConfigSaved with nil data")
	}
	if entry.Summary != "Configuration saved" {
		t.Errorf("expected 'Configuration saved', got %q", entry.Summary)
	}
}

func TestAddRecentChange(t *testing.T) {
	t.Parallel()
	var s AppState

	// Add changes up to the cap
	for i := 0; i < 60; i++ {
		s.AddRecentChange(RecentChange{
			Time:   time.Now(),
			Folder: "f1",
			Path:   fmt.Sprintf("file%d.txt", i),
			Action: "update",
			Type:   "file",
		})
	}
	if len(s.RecentChanges) != 50 {
		t.Errorf("expected 50 recent changes (capped), got %d", len(s.RecentChanges))
	}
	// Oldest entries should have been trimmed; newest should be file59
	if s.RecentChanges[49].Path != "file59.txt" {
		t.Errorf("expected last entry to be file59.txt, got %s", s.RecentChanges[49].Path)
	}
	// First entry should be file10 (60-50=10)
	if s.RecentChanges[0].Path != "file10.txt" {
		t.Errorf("expected first entry to be file10.txt, got %s", s.RecentChanges[0].Path)
	}
}

func TestRecentChangeFromItemFinished(t *testing.T) {
	t.Parallel()
	var s AppState
	s.InitFromConfig(testConfig())

	data, _ := json.Marshal(map[string]string{
		"folder": "folder1",
		"item":   "docs/readme.txt",
		"error":  "",
		"type":   "file",
		"action": "update",
	})

	s.ProcessEvent(Event{
		Type: "ItemFinished",
		Time: time.Now(),
		Data: data,
	})

	if len(s.RecentChanges) != 1 {
		t.Fatalf("expected 1 recent change, got %d", len(s.RecentChanges))
	}
	rc := s.RecentChanges[0]
	if rc.Folder != "folder1" {
		t.Errorf("Folder = %q, want 'folder1'", rc.Folder)
	}
	if rc.Path != "docs/readme.txt" {
		t.Errorf("Path = %q, want 'docs/readme.txt'", rc.Path)
	}
	if rc.Action != "update" {
		t.Errorf("Action = %q, want 'update'", rc.Action)
	}
	if rc.Type != "file" {
		t.Errorf("Type = %q, want 'file'", rc.Type)
	}
}

func TestRecentChangeNotAddedOnError(t *testing.T) {
	t.Parallel()
	var s AppState
	s.InitFromConfig(testConfig())

	data, _ := json.Marshal(map[string]string{
		"folder": "folder1",
		"item":   "broken.txt",
		"error":  "permission denied",
		"type":   "file",
		"action": "update",
	})

	s.ProcessEvent(Event{
		Type: "ItemFinished",
		Time: time.Now(),
		Data: data,
	})

	// Errors should not be recorded as recent changes
	if len(s.RecentChanges) != 0 {
		t.Errorf("expected 0 recent changes for errored item, got %d", len(s.RecentChanges))
	}
}

func TestUpdateSystemStatusListenerDetails(t *testing.T) {
	t.Parallel()
	var s AppState

	status := SystemStatus{
		MyID:      "TEST-ID",
		StartTime: time.Now().Format(time.RFC3339),
		ConnectionServiceStatus: map[string]ConnectionSvcStatus{
			"tcp://0.0.0.0:22000": {
				Error:        nil,
				LANAddresses: []string{"192.168.1.5:22000"},
				WANAddresses: []string{"1.2.3.4:22000"},
			},
			"quic://0.0.0.0:22000": {
				Error:        "listen failed",
				LANAddresses: nil,
				WANAddresses: nil,
			},
		},
		DiscoveryStatus: map[string]ConnectionSvcStatus{
			"IPv4 local": {Error: nil},
			"IPv6 local": {Error: "address family not supported"},
			"global":     {Error: nil},
		},
	}

	s.UpdateSystemStatus(status)

	// Listener counts
	if s.ListenersTotal != 2 {
		t.Errorf("ListenersTotal = %d, want 2", s.ListenersTotal)
	}
	if s.ListenersRunning != 1 {
		t.Errorf("ListenersRunning = %d, want 1", s.ListenersRunning)
	}

	// Listener details
	if len(s.ListenerDetails) != 2 {
		t.Fatalf("expected 2 listener details, got %d", len(s.ListenerDetails))
	}
	// Find TCP listener
	var tcpFound, quicFound bool
	for _, ld := range s.ListenerDetails {
		switch ld.Name {
		case "tcp://0.0.0.0:22000":
			tcpFound = true
			if ld.Error != "" {
				t.Errorf("TCP listener error = %q, want empty", ld.Error)
			}
			if len(ld.LANAddresses) != 1 || ld.LANAddresses[0] != "192.168.1.5:22000" {
				t.Errorf("TCP LAN addresses = %v, want [192.168.1.5:22000]", ld.LANAddresses)
			}
			if len(ld.WANAddresses) != 1 || ld.WANAddresses[0] != "1.2.3.4:22000" {
				t.Errorf("TCP WAN addresses = %v, want [1.2.3.4:22000]", ld.WANAddresses)
			}
		case "quic://0.0.0.0:22000":
			quicFound = true
			if ld.Error != "listen failed" {
				t.Errorf("QUIC listener error = %q, want 'listen failed'", ld.Error)
			}
		}
	}
	if !tcpFound {
		t.Error("TCP listener detail not found")
	}
	if !quicFound {
		t.Error("QUIC listener detail not found")
	}

	// Discovery counts
	if s.DiscoveryTotal != 3 {
		t.Errorf("DiscoveryTotal = %d, want 3", s.DiscoveryTotal)
	}
	if s.DiscoveryRunning != 2 {
		t.Errorf("DiscoveryRunning = %d, want 2", s.DiscoveryRunning)
	}

	// Discovery details
	if len(s.DiscoveryDetails) != 3 {
		t.Fatalf("expected 3 discovery details, got %d", len(s.DiscoveryDetails))
	}
	var ipv6Found bool
	for _, dd := range s.DiscoveryDetails {
		if dd.Name == "IPv6 local" {
			ipv6Found = true
			if dd.Error != "address family not supported" {
				t.Errorf("IPv6 discovery error = %q, want 'address family not supported'", dd.Error)
			}
		}
	}
	if !ipv6Found {
		t.Error("IPv6 local discovery detail not found")
	}
}

func TestConnectionSvcErrorString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		err      interface{}
		expected string
	}{
		{"nil", nil, ""},
		{"empty string", "", ""},
		{"nil string", "<nil>", ""},
		{"error string", "listen failed", "listen failed"},
		{"number", 42, "42"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := connectionSvcErrorString(tt.err)
			if result != tt.expected {
				t.Errorf("connectionSvcErrorString(%v) = %q, want %q", tt.err, result, tt.expected)
			}
		})
	}
}

// --- Stable ordering tests ---

func TestPendingDevicesStableOrder(t *testing.T) {
	t.Parallel()
	var s AppState

	// Call multiple times with the same map — order should be consistent
	pending := map[string]PendingDevice{
		"ZZZ-DEV": {Name: "Z Device"},
		"AAA-DEV": {Name: "A Device"},
		"MMM-DEV": {Name: "M Device"},
	}

	s.UpdatePendingDevices(pending)
	first := make([]string, len(s.PendingDevs))
	for i, p := range s.PendingDevs {
		first[i] = p.DeviceID
	}

	// Call again — should produce same order
	s.UpdatePendingDevices(pending)
	for i, p := range s.PendingDevs {
		if p.DeviceID != first[i] {
			t.Errorf("position %d: got %q, want %q (order changed)", i, p.DeviceID, first[i])
		}
	}

	// Verify sorted order
	if s.PendingDevs[0].DeviceID != "AAA-DEV" {
		t.Errorf("first = %q, want AAA-DEV (sorted)", s.PendingDevs[0].DeviceID)
	}
	if s.PendingDevs[2].DeviceID != "ZZZ-DEV" {
		t.Errorf("last = %q, want ZZZ-DEV (sorted)", s.PendingDevs[2].DeviceID)
	}
}

func TestPendingFoldersStableOrder(t *testing.T) {
	t.Parallel()
	var s AppState

	pending := map[string]PendingFolderEntry{
		"z-folder": {OfferedBy: map[string]PendingFolderOffer{
			"DEV-1": {Label: "Z Folder"},
		}},
		"a-folder": {OfferedBy: map[string]PendingFolderOffer{
			"DEV-1": {Label: "A Folder"},
		}},
		"m-folder": {OfferedBy: map[string]PendingFolderOffer{
			"DEV-1": {Label: "M Folder"},
		}},
	}

	s.UpdatePendingFoldersFromAPI(pending)
	first := make([]string, len(s.PendingFldrs))
	for i, p := range s.PendingFldrs {
		first[i] = p.FolderID
	}

	// Call again — same order
	s.UpdatePendingFoldersFromAPI(pending)
	for i, p := range s.PendingFldrs {
		if p.FolderID != first[i] {
			t.Errorf("position %d: got %q, want %q", i, p.FolderID, first[i])
		}
	}

	// Verify sorted
	if s.PendingFldrs[0].FolderID != "a-folder" {
		t.Errorf("first = %q, want a-folder", s.PendingFldrs[0].FolderID)
	}
}

func TestDiscoveredDevicesStableOrder(t *testing.T) {
	t.Parallel()
	var s AppState

	discovered := map[string]DiscoveryEntry{
		"ZZZ-DEV": {Addresses: []string{"addr1"}},
		"AAA-DEV": {Addresses: []string{"addr2"}},
		"MMM-DEV": {Addresses: []string{"addr3"}},
	}

	s.UpdateDiscovery(discovered)
	first := make([]string, len(s.DiscoveredDevices))
	for i, d := range s.DiscoveredDevices {
		first[i] = d.DeviceID
	}

	s.UpdateDiscovery(discovered)
	for i, d := range s.DiscoveredDevices {
		if d.DeviceID != first[i] {
			t.Errorf("position %d: got %q, want %q", i, d.DeviceID, first[i])
		}
	}

	if s.DiscoveredDevices[0].DeviceID != "AAA-DEV" {
		t.Errorf("first = %q, want AAA-DEV", s.DiscoveredDevices[0].DeviceID)
	}
}

func TestPendingFoldersStableAfterAccept(t *testing.T) {
	t.Parallel()
	var s AppState

	// Three pending folders
	pending := map[string]PendingFolderEntry{
		"photos": {OfferedBy: map[string]PendingFolderOffer{
			"DEV-1": {Label: "Photos"},
		}},
		"docs": {OfferedBy: map[string]PendingFolderOffer{
			"DEV-1": {Label: "Documents"},
		}},
		"music": {OfferedBy: map[string]PendingFolderOffer{
			"DEV-1": {Label: "Music"},
		}},
	}

	s.UpdatePendingFoldersFromAPI(pending)
	if len(s.PendingFldrs) != 3 {
		t.Fatalf("expected 3 pending, got %d", len(s.PendingFldrs))
	}

	// Record order
	ids := make([]string, len(s.PendingFldrs))
	for i, p := range s.PendingFldrs {
		ids[i] = p.FolderID
	}

	// Simulate accepting "docs" — remove it from the map and re-update
	delete(pending, "docs")
	s.UpdatePendingFoldersFromAPI(pending)

	if len(s.PendingFldrs) != 2 {
		t.Fatalf("expected 2 pending after accept, got %d", len(s.PendingFldrs))
	}

	// Remaining items should still be sorted (music, photos)
	if s.PendingFldrs[0].FolderID != "music" {
		t.Errorf("first = %q, want music", s.PendingFldrs[0].FolderID)
	}
	if s.PendingFldrs[1].FolderID != "photos" {
		t.Errorf("second = %q, want photos", s.PendingFldrs[1].FolderID)
	}
}
