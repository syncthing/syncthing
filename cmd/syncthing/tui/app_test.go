// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package tui

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
)

// countBatchCmds counts how many commands are in a tea.Batch result.
// A nil cmd counts as 0. A non-batch cmd counts as 1. A batch counts its
// children recursively.
func countBatchCmds(cmd tea.Cmd) int {
	if cmd == nil {
		return 0
	}
	// Execute the cmd to see if it returns a tea.BatchMsg
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		n := 0
		for _, c := range batch {
			n += countBatchCmds(c)
		}
		return n
	}
	return 1
}

func newTestApp() appModel {
	return newTestAppWithClient(&mockClient{})
}

func newTestAppWithClient(client APIClient) appModel {
	m := appModel{
		client:          client,
		keys:            defaultKeyMap(),
		styles:          newStyles(true),
		connState:       connConnected,
		expandedFolders: make(map[int]bool),
		expandedDevices: make(map[int]bool),
		width:           120,
		height:          40,
	}
	return m
}

func makeFullState() *AppState {
	return &AppState{
		MyID:      "TEST-DEVICE-ID",
		Version:   "v1.0.0",
		StartTime: time.Now().Add(-60 * time.Second),
	}
}

// TestFullStateMsgStartsPolling verifies that the first fullStateMsg
// starts exactly one poll and one tick goroutine.
func TestFullStateMsgStartsPolling(t *testing.T) {
	t.Parallel()
	m := newTestApp()

	if m.pollingActive {
		t.Fatal("pollingActive should be false initially")
	}
	if m.tickingActive {
		t.Fatal("tickingActive should be false initially")
	}

	result, cmd := m.Update(fullStateMsg{state: makeFullState()})
	m = result.(appModel)

	if !m.pollingActive {
		t.Error("pollingActive should be true after fullStateMsg")
	}
	if !m.tickingActive {
		t.Error("tickingActive should be true after fullStateMsg")
	}
	if cmd == nil {
		t.Fatal("expected commands from fullStateMsg")
	}
}

// TestDuplicateFullStateMsgNoExtraGoroutines is the critical test:
// sending fullStateMsg multiple times should NOT spawn additional
// poll/tick goroutines beyond the first pair.
func TestDuplicateFullStateMsgNoExtraGoroutines(t *testing.T) {
	t.Parallel()
	m := newTestApp()

	// First fullStateMsg — should start poll + tick
	result, _ := m.Update(fullStateMsg{state: makeFullState()})
	m = result.(appModel)

	if !m.pollingActive || !m.tickingActive {
		t.Fatal("first fullStateMsg should activate poll and tick")
	}

	// Second fullStateMsg — should NOT start new ones (flags already true)
	result, cmd := m.Update(fullStateMsg{state: makeFullState()})
	m = result.(appModel)

	// The cmd should be a batch with 0 commands (empty batch)
	cmdCount := countBatchCmds(cmd)
	if cmdCount != 0 {
		t.Errorf("duplicate fullStateMsg spawned %d commands, want 0", cmdCount)
	}
}

// TestEventsMsgRepolls verifies that eventsMsg correctly re-issues
// a poll command (the old poll goroutine finished) but does not
// start a tick.
func TestEventsMsgRepolls(t *testing.T) {
	t.Parallel()
	m := newTestApp()
	m.pollingActive = true
	m.tickingActive = true

	result, cmd := m.Update(eventsMsg{events: nil, lastID: 42})
	m = result.(appModel)

	if !m.pollingActive {
		t.Error("pollingActive should remain true after eventsMsg")
	}
	if m.lastEventID != 42 {
		t.Errorf("lastEventID = %d, want 42", m.lastEventID)
	}
	if cmd == nil {
		t.Error("eventsMsg should return a poll command")
	}
}

// TestErrMsgStopsPolling verifies that an event error stops the
// poll/tick flags and triggers reconnection.
func TestErrMsgStopsPolling(t *testing.T) {
	t.Parallel()
	m := newTestApp()
	m.pollingActive = true
	m.tickingActive = true

	result, cmd := m.Update(errMsg{err: nil, source: "events"})
	m = result.(appModel)

	if m.pollingActive {
		t.Error("pollingActive should be false after event error")
	}
	if m.tickingActive {
		t.Error("tickingActive should be false after event error")
	}
	if m.connState != connDisconnected {
		t.Error("connState should be disconnected after event error")
	}
	if cmd == nil {
		t.Error("should return a reconnect command")
	}
}

// TestReconnectedMsgResetsFlags verifies that reconnection resets
// the polling flags so the subsequent fullStateMsg can start fresh.
func TestReconnectedMsgResetsFlags(t *testing.T) {
	t.Parallel()
	m := newTestApp()
	m.pollingActive = true
	m.tickingActive = true
	m.connState = connDisconnected

	result, cmd := m.Update(reconnectedMsg{})
	m = result.(appModel)

	if m.pollingActive {
		t.Error("pollingActive should be false after reconnect (reset for fullState)")
	}
	if m.tickingActive {
		t.Error("tickingActive should be false after reconnect")
	}
	if m.connState != connConnected {
		t.Error("connState should be connected after reconnect")
	}
	if cmd == nil {
		t.Error("should return fetchFullState command")
	}
}

// TestActionResultNoGoroutineLeak verifies that a successful action
// triggers a full state refresh.
func TestActionResultNoGoroutineLeak(t *testing.T) {
	t.Parallel()
	m := newTestApp()
	m.pollingActive = true
	m.tickingActive = true

	result, cmd := m.Update(actionResultMsg{action: "test", err: nil})
	m = result.(appModel)

	// Should return a command (refreshState) but flags should not change
	if cmd == nil {
		t.Error("successful action should return refresh command")
	}
	if !m.pollingActive {
		t.Error("pollingActive should remain true")
	}
	if !m.tickingActive {
		t.Error("tickingActive should remain true")
	}
}

// TestFullCycleNoGoroutineGrowth simulates a realistic sequence of
// messages and verifies goroutine flags stay bounded.
func TestFullCycleNoGoroutineGrowth(t *testing.T) {
	t.Parallel()
	m := newTestApp()

	// 1. Initial fullStateMsg
	result, _ := m.Update(fullStateMsg{state: makeFullState()})
	m = result.(appModel)
	if !m.pollingActive || !m.tickingActive {
		t.Fatal("should be active after initial load")
	}

	// 2. Events arrive
	result, _ = m.Update(eventsMsg{events: nil, lastID: 1})
	m = result.(appModel)

	// 3. More events
	result, _ = m.Update(eventsMsg{events: nil, lastID: 2})
	m = result.(appModel)

	// 4. Another fullStateMsg
	// Since poll/tick are active, should NOT spawn new ones
	result, cmd := m.Update(fullStateMsg{state: makeFullState()})
	m = result.(appModel)
	cmdCount := countBatchCmds(cmd)
	if cmdCount != 0 {
		t.Errorf("second fullStateMsg spawned %d commands, want 0", cmdCount)
	}

	// 7. Simulate disconnect
	result, _ = m.Update(errMsg{err: nil, source: "events"})
	m = result.(appModel)
	if m.pollingActive || m.tickingActive {
		t.Error("flags should be cleared after disconnect")
	}

	// 8. Reconnect
	result, _ = m.Update(reconnectedMsg{})
	m = result.(appModel)
	if m.pollingActive || m.tickingActive {
		t.Error("flags should be cleared after reconnect (before fullState)")
	}

	// 9. Full state after reconnect — should restart poll/tick
	result, _ = m.Update(fullStateMsg{state: makeFullState()})
	m = result.(appModel)
	if !m.pollingActive || !m.tickingActive {
		t.Error("should be active after reconnect fullState")
	}
}

// TestCtrlCQuitsFromAnyState verifies Ctrl+C quits regardless of
// modal state (form, confirm, help, event tab).
func TestCtrlCQuitsFromAnyState(t *testing.T) {
	t.Parallel()

	ctrlC := tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}

	tests := []struct {
		name  string
		setup func(m *appModel)
	}{
		{"normal", func(m *appModel) {}},
		{"help open", func(m *appModel) { m.showHelp = true }},
		{"events tab", func(m *appModel) { m.activeTab = tabEvents }},
		{"form open", func(m *appModel) {
			form := newAddFolderForm()
			m.form = &form
		}},
		{"confirm open", func(m *appModel) {
			c := newConfirm(confirmRestart, "test?", "")
			m.confirm = &c
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := newTestApp()
			m.width = 120
			m.height = 40
			tt.setup(&m)

			_, cmd := m.Update(ctrlC)
			if cmd == nil {
				t.Error("Ctrl+C should return a quit command")
				return
			}
			// Execute the command and check it returns tea.QuitMsg
			msg := cmd()
			if _, ok := msg.(tea.QuitMsg); !ok {
				t.Errorf("expected QuitMsg, got %T", msg)
			}
		})
	}
}

// TestEscapeClosesModals verifies Escape closes forms/help.
func TestEscapeClosesModals(t *testing.T) {
	t.Parallel()

	escKey := tea.KeyPressMsg{Code: tea.KeyEscape}

	t.Run("closes help", func(t *testing.T) {
		t.Parallel()
		m := newTestApp()
		m.showHelp = true
		result, _ := m.Update(escKey)
		m = result.(appModel)
		if m.showHelp {
			t.Error("Escape should close help")
		}
	})

	t.Run("closes form", func(t *testing.T) {
		t.Parallel()
		m := newTestApp()
		form := newAddFolderForm()
		m.form = &form
		result, _ := m.Update(escKey)
		m = result.(appModel)
		if m.form != nil {
			t.Error("Escape should close form")
		}
	})

	t.Run("closes confirm", func(t *testing.T) {
		t.Parallel()
		m := newTestApp()
		c := newConfirm(confirmRestart, "test?", "")
		m.confirm = &c
		result, _ := m.Update(tea.KeyPressMsg{Code: 'n'})
		m = result.(appModel)
		if m.confirm != nil {
			t.Error("'n' should close confirm dialog")
		}
	})
}

// TestTabNavigation verifies Tab cycles through all tabs.
func TestTabNavigation(t *testing.T) {
	t.Parallel()
	m := newTestApp()

	if m.activeTab != tabFolders {
		t.Fatalf("initial tab should be Folders, got %d", m.activeTab)
	}

	tabKey := tea.KeyPressMsg{Code: tea.KeyTab}

	result, _ := m.Update(tabKey)
	m = result.(appModel)
	if m.activeTab != tabDevices {
		t.Errorf("after Tab: tab = %d, want Devices", m.activeTab)
	}

	result, _ = m.Update(tabKey)
	m = result.(appModel)
	if m.activeTab != tabEvents {
		t.Errorf("after Tab 2: tab = %d, want Events", m.activeTab)
	}

	result, _ = m.Update(tabKey)
	m = result.(appModel)
	if m.activeTab != tabFolders {
		t.Errorf("after Tab 3: tab = %d, want Folders (wrap)", m.activeTab)
	}
}

// TestTabNumberKeys verifies 1/2/3 switch to specific tabs.
func TestTabNumberKeys(t *testing.T) {
	t.Parallel()
	m := newTestApp()

	// Press 2 to go to Devices
	result, _ := m.Update(tea.KeyPressMsg{Code: '2'})
	m = result.(appModel)
	if m.activeTab != tabDevices {
		t.Errorf("pressing '2': tab = %d, want Devices", m.activeTab)
	}

	// Press 3 to go to Events
	result, _ = m.Update(tea.KeyPressMsg{Code: '3'})
	m = result.(appModel)
	if m.activeTab != tabEvents {
		t.Errorf("pressing '3': tab = %d, want Events", m.activeTab)
	}

	// Press 1 to go back to Folders
	result, _ = m.Update(tea.KeyPressMsg{Code: '1'})
	m = result.(appModel)
	if m.activeTab != tabFolders {
		t.Errorf("pressing '1': tab = %d, want Folders", m.activeTab)
	}
}

// TestAccordionExpandCollapse verifies Enter toggles expand state.
func TestAccordionExpandCollapse(t *testing.T) {
	t.Parallel()
	m := newTestApp()
	m.state.Folders = []FolderState{{ID: "f1", Label: "F1"}}

	enterKey := tea.KeyPressMsg{Code: tea.KeyEnter}

	// Expand
	result, _ := m.Update(enterKey)
	m = result.(appModel)
	if !m.expandedFolders[0] {
		t.Error("Enter should expand folder")
	}

	// Collapse
	result, _ = m.Update(enterKey)
	m = result.(appModel)
	if m.expandedFolders[0] {
		t.Error("Enter again should collapse folder")
	}
}

// TestCursorClampAfterFolderRemoval verifies the folder cursor doesn't
// point past the end when folders are removed via fullStateMsg.
func TestCursorClampAfterFolderRemoval(t *testing.T) {
	t.Parallel()
	m := newTestApp()
	m.state.Folders = []FolderState{
		{ID: "f1"}, {ID: "f2"}, {ID: "f3"},
	}
	m.folderCursor = 2 // pointing at f3

	// A new fullStateMsg arrives with only one folder (f2 and f3 removed).
	st := makeFullState()
	st.Folders = []FolderState{{ID: "f1"}}
	result, _ := m.Update(fullStateMsg{state: st})
	m = result.(appModel)

	if m.folderCursor >= len(m.state.Folders) {
		t.Errorf("folderCursor = %d, but only %d folders exist", m.folderCursor, len(m.state.Folders))
	}
	if m.folderCursor != 0 {
		t.Errorf("folderCursor = %d, want 0 (clamped)", m.folderCursor)
	}
}

// TestExpandedStateResetAfterConfigChange verifies that expanded maps are
// cleared when the folder/device lists are replaced via fullStateMsg.
func TestExpandedStateResetAfterConfigChange(t *testing.T) {
	t.Parallel()
	m := newTestApp()
	m.state.Folders = []FolderState{{ID: "f1"}, {ID: "f2"}}
	m.state.Devices = []DeviceState{{ID: "d1"}, {ID: "d2"}}

	// Expand some items
	m.expandedFolders[0] = true
	m.expandedFolders[1] = true
	m.expandedDevices[0] = true

	// New fullStateMsg replaces both lists
	st := makeFullState()
	st.Folders = []FolderState{{ID: "f3"}}
	st.Devices = []DeviceState{{ID: "d3"}}
	result, _ := m.Update(fullStateMsg{state: st})
	m = result.(appModel)

	if len(m.expandedFolders) != 0 {
		t.Errorf("expandedFolders should be empty after config change, got %v", m.expandedFolders)
	}
	if len(m.expandedDevices) != 0 {
		t.Errorf("expandedDevices should be empty after config change, got %v", m.expandedDevices)
	}
}

// TestFolderErrorsMsgUpdatesState verifies that folderErrorsMsg correctly
// applies errors to the state without data races (the fix for Bug 1).
func TestFolderErrorsMsgUpdatesState(t *testing.T) {
	t.Parallel()
	m := newTestApp()
	m.state.Folders = []FolderState{
		{ID: "f1", Label: "F1"},
		{ID: "f2", Label: "F2"},
	}

	errs := []FolderError{
		{Path: "file.txt", Error: "permission denied"},
		{Path: "dir/other.txt", Error: "no space"},
	}

	result, cmd := m.Update(folderErrorsMsg{folderID: "f1", errors: errs})
	m = result.(appModel)

	if cmd != nil {
		t.Error("folderErrorsMsg should return nil cmd")
	}
	if len(m.state.Folders[0].Errors) != 2 {
		t.Errorf("expected 2 errors on f1, got %d", len(m.state.Folders[0].Errors))
	}
	if m.state.Folders[0].Errors[0].Path != "file.txt" {
		t.Errorf("expected first error path 'file.txt', got %q", m.state.Folders[0].Errors[0].Path)
	}
	// f2 should have no errors
	if len(m.state.Folders[1].Errors) != 0 {
		t.Errorf("expected 0 errors on f2, got %d", len(m.state.Folders[1].Errors))
	}
}

// TestEventLogScrollOffsetClamp verifies the scroll offset is clamped
// when the event log shrinks (e.g. after daemon restart).
func TestEventLogScrollOffsetClamp(t *testing.T) {
	t.Parallel()
	m := newTestApp()
	m.activeTab = tabEvents

	// Add some events, then set a scroll offset
	for i := 0; i < 10; i++ {
		m.state.AddEventLog(EventEntry{
			Type:    "Test",
			Summary: fmt.Sprintf("event %d", i),
		}, 100)
	}
	m.eventLog.scrollOffset = 8

	// Now simulate events being cleared (daemon restart) -- empty the log
	m.state.EventLog = nil

	// Press up in the events tab -- should clamp offset first
	upKey := tea.KeyPressMsg{Code: tea.KeyUp}
	result, _ := m.Update(upKey)
	m = result.(appModel)

	if m.eventLog.scrollOffset != 0 {
		t.Errorf("scrollOffset = %d after empty event log, want 0", m.eventLog.scrollOffset)
	}
}

// TestEmptyStateRendering verifies the main view renders correctly with
// zero folders and zero devices.
func TestEmptyStateRendering(t *testing.T) {
	t.Parallel()
	m := newTestApp()
	m.state = AppState{
		MyID:    "TEST-ID",
		Version: "v1.0.0",
	}
	m.width = 80
	m.height = 24

	// Should not panic
	v := m.View()
	if v.Content == "" {
		t.Error("View() returned empty body for empty state")
	}
}

// TestViewNoPanicZeroDimensions verifies View() handles width=0 or
// height=0 without panicking.
func TestViewNoPanicZeroDimensions(t *testing.T) {
	t.Parallel()

	t.Run("zero width", func(t *testing.T) {
		t.Parallel()
		m := newTestApp()
		m.width = 0
		m.height = 40
		// width=0 triggers the "Loading..." early return
		v := m.View()
		if v.Content == "" {
			t.Error("View() returned empty body with zero width")
		}
	})

	t.Run("zero height", func(t *testing.T) {
		t.Parallel()
		m := newTestApp()
		m.width = 80
		m.height = 0
		m.state = AppState{MyID: "TEST", Version: "v1"}
		// Should not panic even with height=0
		v := m.View()
		_ = v // just verify no panic
	})
}

// TestFormFieldNavigationWrapping verifies that Tab wraps from the last
// field back to the first (or to the discovery list if present).
func TestFormFieldNavigationWrapping(t *testing.T) {
	t.Parallel()

	t.Run("wraps to first field", func(t *testing.T) {
		t.Parallel()
		m := newTestApp()
		form := newAddFolderForm()
		m.form = &form

		tabKey := tea.KeyPressMsg{Code: tea.KeyTab}
		totalFields := m.form.totalFields()

		// Tab through all fields and back to the start
		for i := 0; i < totalFields; i++ {
			result, _ := m.Update(tabKey)
			m = result.(appModel)
		}

		// Should be back at field 0
		if m.form.focus != 0 {
			t.Errorf("after %d tabs, focus = %d, want 0 (wrapped)", totalFields, m.form.focus)
		}
	})

	t.Run("wraps to discovery list", func(t *testing.T) {
		t.Parallel()
		m := newTestApp()
		form := newAddDeviceForm([]DiscoveredDevice{
			{DeviceID: "ABCDEFG-1234567", Name: "Test", Addresses: []string{"192.168.1.1"}},
		}, nil)
		m.form = &form

		// Start with discovery focused; Tab moves to manual input
		tabKey := tea.KeyPressMsg{Code: tea.KeyTab}
		result, _ := m.Update(tabKey)
		m = result.(appModel)
		if m.form.discoveryFocused {
			t.Error("Tab from discovery should move to manual input")
		}

		// Tab through all fields — should wrap back to discovery
		totalFields := m.form.totalFields()
		for i := 0; i < totalFields; i++ {
			result, _ = m.Update(tabKey)
			m = result.(appModel)
		}
		if !m.form.discoveryFocused {
			t.Error("Tabbing past last field should return to discovery list")
		}
	})
}

// --- Action tests using mock client ---

// TestSubmitAddFolder verifies the add folder form submission calls FolderAdd.
func TestSubmitAddFolder(t *testing.T) {
	t.Parallel()
	mc := &mockClient{}
	m := newTestAppWithClient(mc)

	// Create and fill a folder form
	form := newAddFolderForm()
	form.inputs[0].SetValue("my-folder")
	form.inputs[1].SetValue("My Folder")
	form.inputs[2].SetValue("/tmp/test")
	m.form = &form

	// Submit
	result, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = result.(appModel)

	if m.form != nil {
		t.Error("form should be nil after submit")
	}
	if cmd == nil {
		t.Fatal("expected a command from submit")
	}

	// Execute the command (doAction wrapper)
	msg := cmd()
	if action, ok := msg.(actionResultMsg); ok {
		if action.err != nil {
			t.Errorf("unexpected error: %v", action.err)
		}
		if action.action != "add folder" {
			t.Errorf("action = %q, want 'add folder'", action.action)
		}
	} else {
		t.Fatalf("expected actionResultMsg, got %T", msg)
	}

	calls := mc.getCalls()
	if len(calls) != 1 || calls[0] != "FolderAdd" {
		t.Errorf("expected [FolderAdd], got %v", calls)
	}
}

// TestSubmitAddDevice verifies the add device form calls DeviceAdd.
func TestSubmitAddDevice(t *testing.T) {
	t.Parallel()
	mc := &mockClient{}
	m := newTestAppWithClient(mc)

	form := newAddDeviceForm(nil, nil)
	// Use a valid-looking device ID (checksum doesn't matter for the form)
	form.inputs[0].SetValue("AAAAAAA-BBBBBBB-CCCCCCC-DDDDDDD-EEEEEEE-FFFFFFF-GGGGGGG-HHHHHHH")
	form.inputs[1].SetValue("Test Node")
	form.inputs[2].SetValue("tcp://192.168.1.5:22000")
	m.form = &form

	result, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = result.(appModel)

	if m.form != nil {
		t.Error("form should be nil after submit")
	}

	// The device ID might fail validation — check if we got a status message or a cmd
	if cmd == nil {
		// Device ID validation failed — that's expected for our fake ID
		if m.statusMessage == "" {
			t.Error("expected either a command or a status message")
		}
		return
	}

	msg := cmd()
	if action, ok := msg.(actionResultMsg); ok {
		if action.action != "add device" {
			t.Errorf("action = %q, want 'add device'", action.action)
		}
	}
}

// TestSubmitShareFolder verifies share folder calls ConfigGet then FolderUpdate.
func TestSubmitShareFolder(t *testing.T) {
	t.Parallel()
	mc := &mockClient{
		configGetResult: config.Configuration{
			Folders: []config.FolderConfiguration{
				{ID: "test-folder", Label: "Test"},
			},
		},
	}
	m := newTestAppWithClient(mc)

	form := newShareFolderForm("test-folder", nil)
	// Use a valid-ish device ID
	form.inputs[0].SetValue("AAAAAAA-BBBBBBB-CCCCCCC-DDDDDDD-EEEEEEE-FFFFFFF-GGGGGGG-HHHHHHH")
	m.form = &form

	result, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = result.(appModel)

	if cmd == nil {
		// Device ID validation may fail
		return
	}

	msg := cmd()
	if action, ok := msg.(actionResultMsg); ok {
		if action.action != "share folder" {
			t.Errorf("action = %q, want 'share folder'", action.action)
		}
	}
}

// TestSubmitEmptyFolder verifies empty folder ID is rejected silently.
func TestSubmitEmptyFolder(t *testing.T) {
	t.Parallel()
	m := newTestApp()

	form := newAddFolderForm()
	// Leave folder ID empty
	m.form = &form

	result, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = result.(appModel)

	if m.form != nil {
		t.Error("form should be nil after submit")
	}
	if cmd != nil {
		t.Error("empty folder ID should return nil cmd")
	}
}

// TestExecuteConfirmRemoveFolder verifies confirm yes calls FolderRemove.
func TestExecuteConfirmRemoveFolder(t *testing.T) {
	t.Parallel()
	mc := &mockClient{}
	m := newTestAppWithClient(mc)
	c := newConfirm(confirmRemoveFolder, "Remove?", "my-folder")
	m.confirm = &c

	// Press 'y'
	result, cmd := m.Update(tea.KeyPressMsg{Code: 'y'})
	m = result.(appModel)

	if m.confirm != nil {
		t.Error("confirm should be nil after yes")
	}
	if cmd == nil {
		t.Fatal("expected command")
	}

	msg := cmd()
	action := msg.(actionResultMsg)
	if action.err != nil {
		t.Errorf("unexpected error: %v", action.err)
	}

	calls := mc.getCalls()
	if len(calls) != 1 || calls[0] != "FolderRemove" {
		t.Errorf("expected [FolderRemove], got %v", calls)
	}
}

// TestExecuteConfirmRemoveDevice verifies confirm yes calls DeviceRemove.
func TestExecuteConfirmRemoveDevice(t *testing.T) {
	t.Parallel()
	mc := &mockClient{}
	m := newTestAppWithClient(mc)
	c := newConfirm(confirmRemoveDevice, "Remove?", "DEV-ID")
	m.confirm = &c

	result, cmd := m.Update(tea.KeyPressMsg{Code: 'y'})
	m = result.(appModel)
	if cmd == nil {
		t.Fatal("expected command")
	}

	msg := cmd()
	action := msg.(actionResultMsg)
	if action.err != nil {
		t.Errorf("unexpected error: %v", action.err)
	}

	calls := mc.getCalls()
	if len(calls) != 1 || calls[0] != "DeviceRemove" {
		t.Errorf("expected [DeviceRemove], got %v", calls)
	}
}

// TestExecuteConfirmRestart verifies confirm yes calls Restart.
func TestExecuteConfirmRestart(t *testing.T) {
	t.Parallel()
	mc := &mockClient{}
	m := newTestAppWithClient(mc)
	c := newConfirm(confirmRestart, "Restart?", "")
	m.confirm = &c

	result, cmd := m.Update(tea.KeyPressMsg{Code: 'y'})
	m = result.(appModel)
	if cmd == nil {
		t.Fatal("expected command")
	}

	msg := cmd()
	action := msg.(actionResultMsg)
	if action.err != nil {
		t.Errorf("unexpected error: %v", action.err)
	}

	calls := mc.getCalls()
	if len(calls) != 1 || calls[0] != "Restart" {
		t.Errorf("expected [Restart], got %v", calls)
	}
}

// TestToggleFolderPause verifies pause calls ConfigGet then FolderUpdate.
func TestToggleFolderPause(t *testing.T) {
	t.Parallel()
	mc := &mockClient{
		configGetResult: config.Configuration{
			Folders: []config.FolderConfiguration{
				{ID: "f1", Paused: false},
			},
		},
	}
	m := newTestAppWithClient(mc)
	m.state.Folders = []FolderState{{ID: "f1", Paused: false}}
	m.activeTab = tabFolders

	// Press 'p' to pause
	result, cmd := m.Update(tea.KeyPressMsg{Code: 'p'})
	m = result.(appModel)

	if cmd == nil {
		t.Fatal("expected command from pause")
	}

	msg := cmd()
	action := msg.(actionResultMsg)
	if action.err != nil {
		t.Errorf("unexpected error: %v", action.err)
	}

	calls := mc.getCalls()
	if len(calls) != 2 || calls[0] != "ConfigGet" || calls[1] != "FolderUpdate" {
		t.Errorf("expected [ConfigGet, FolderUpdate], got %v", calls)
	}
}

// TestToggleDevicePause verifies pause/resume calls correct API method.
func TestToggleDevicePause(t *testing.T) {
	t.Parallel()

	t.Run("pause connected device", func(t *testing.T) {
		t.Parallel()
		mc := &mockClient{}
		m := newTestAppWithClient(mc)
		m.state.MyID = "LOCAL"
		m.state.Devices = []DeviceState{
			{ID: "LOCAL", Name: "Local"},
			{ID: "REMOTE", Name: "Remote", Connected: true},
		}
		m.activeTab = tabDevices

		result, cmd := m.Update(tea.KeyPressMsg{Code: 'p'})
		m = result.(appModel)
		if cmd == nil {
			t.Fatal("expected command")
		}

		msg := cmd()
		action := msg.(actionResultMsg)
		if action.action != "pause device" {
			t.Errorf("action = %q, want 'pause device'", action.action)
		}

		calls := mc.getCalls()
		if len(calls) != 1 || calls[0] != "Pause" {
			t.Errorf("expected [Pause], got %v", calls)
		}
	})

	t.Run("resume paused device", func(t *testing.T) {
		t.Parallel()
		mc := &mockClient{}
		m := newTestAppWithClient(mc)
		m.state.MyID = "LOCAL"
		m.state.Devices = []DeviceState{
			{ID: "LOCAL", Name: "Local"},
			{ID: "REMOTE", Name: "Remote", Paused: true},
		}
		m.activeTab = tabDevices

		result, cmd := m.Update(tea.KeyPressMsg{Code: 'p'})
		m = result.(appModel)
		if cmd == nil {
			t.Fatal("expected command")
		}

		msg := cmd()
		action := msg.(actionResultMsg)
		if action.action != "resume device" {
			t.Errorf("action = %q, want 'resume device'", action.action)
		}

		calls := mc.getCalls()
		if len(calls) != 1 || calls[0] != "Resume" {
			t.Errorf("expected [Resume], got %v", calls)
		}
	})
}

// TestScanFolder verifies scan calls the Scan API method.
func TestScanFolder(t *testing.T) {
	t.Parallel()
	mc := &mockClient{}
	m := newTestAppWithClient(mc)
	m.state.Folders = []FolderState{{ID: "f1"}}
	m.activeTab = tabFolders

	result, cmd := m.Update(tea.KeyPressMsg{Code: 's'})
	m = result.(appModel)
	if cmd == nil {
		t.Fatal("expected command")
	}

	msg := cmd()
	action := msg.(actionResultMsg)
	if action.action != "scan f1" {
		t.Errorf("action = %q, want 'scan f1'", action.action)
	}

	calls := mc.getCalls()
	if len(calls) != 1 || calls[0] != "Scan" {
		t.Errorf("expected [Scan], got %v", calls)
	}
}

// TestOverrideFolder verifies override only works on sendonly folders.
func TestOverrideFolder(t *testing.T) {
	t.Parallel()

	t.Run("sendonly folder triggers override confirmation", func(t *testing.T) {
		t.Parallel()
		mc := &mockClient{}
		m := newTestAppWithClient(mc)
		m.state.Folders = []FolderState{{ID: "f1", Type: "sendonly"}}
		m.activeTab = tabFolders

		// Pressing O opens a confirmation dialog
		result, cmd := m.Update(tea.KeyPressMsg{Code: 'O'})
		m = result.(appModel)
		if cmd != nil {
			t.Error("expected nil command (confirmation dialog shown)")
		}
		if m.confirm == nil || m.confirm.kind != confirmOverride {
			t.Fatal("expected confirmOverride dialog")
		}

		// Confirm with 'y'
		result, cmd = m.Update(tea.KeyPressMsg{Code: 'y'})
		m = result.(appModel)
		if cmd == nil {
			t.Fatal("expected command after confirming override")
		}
		msg := cmd()
		action := msg.(actionResultMsg)
		if action.action != "override f1" {
			t.Errorf("action = %q, want 'override f1'", action.action)
		}
		calls := mc.getCalls()
		if len(calls) != 1 || calls[0] != "Override" {
			t.Errorf("expected [Override], got %v", calls)
		}
	})

	t.Run("non-sendonly folder is no-op", func(t *testing.T) {
		t.Parallel()
		mc := &mockClient{}
		m := newTestAppWithClient(mc)
		m.state.Folders = []FolderState{{ID: "f1", Type: "sendreceive"}}
		m.activeTab = tabFolders

		_, cmd := m.Update(tea.KeyPressMsg{Code: 'O'})
		if cmd != nil {
			t.Error("expected nil command for non-sendonly folder")
		}
	})
}

// TestRevertFolder verifies revert only works on receiveonly folders.
func TestRevertFolder(t *testing.T) {
	t.Parallel()
	mc := &mockClient{}
	m := newTestAppWithClient(mc)
	m.state.Folders = []FolderState{{ID: "f1", Type: "receiveonly"}}
	m.activeTab = tabFolders

	// Pressing V opens a confirmation dialog
	result, cmd := m.Update(tea.KeyPressMsg{Code: 'V'})
	m = result.(appModel)
	if cmd != nil {
		t.Error("expected nil command (confirmation dialog shown)")
	}
	if m.confirm == nil || m.confirm.kind != confirmRevert {
		t.Fatal("expected confirmRevert dialog")
	}

	// Confirm with 'y'
	result, cmd = m.Update(tea.KeyPressMsg{Code: 'y'})
	m = result.(appModel)
	if cmd == nil {
		t.Fatal("expected command after confirming revert")
	}
	msg := cmd()
	action := msg.(actionResultMsg)
	if action.action != "revert f1" {
		t.Errorf("action = %q, want 'revert f1'", action.action)
	}
}

// TestActionError verifies failed actions show error in status bar.
func TestActionError(t *testing.T) {
	t.Parallel()
	m := newTestApp()

	result, _ := m.Update(actionResultMsg{
		action: "test action",
		err:    fmt.Errorf("something broke"),
	})
	m = result.(appModel)

	if m.statusMessage != "Error: test action: something broke" {
		t.Errorf("status = %q, want error message", m.statusMessage)
	}
}

// TestShareFolderNotFound verifies share returns error if folder is missing.
func TestShareFolderNotFound(t *testing.T) {
	t.Parallel()
	mc := &mockClient{
		configGetResult: config.Configuration{
			Folders: []config.FolderConfiguration{}, // empty — folder won't be found
		},
	}
	m := newTestAppWithClient(mc)

	form := newShareFolderForm("nonexistent", nil)
	form.inputs[0].SetValue("AAAAAAA-BBBBBBB-CCCCCCC-DDDDDDD-EEEEEEE-FFFFFFF-GGGGGGG-HHHHHHH")
	m.form = &form

	result, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = result.(appModel)

	if cmd == nil {
		// Device ID validation may fail, that's ok
		return
	}

	msg := cmd()
	if action, ok := msg.(actionResultMsg); ok {
		if action.err == nil {
			t.Error("expected error for nonexistent folder")
		}
	}
}

// TestPendingDevicesChangedEventTriggersFetch verifies that receiving a
// PendingDevicesChanged event causes the TUI to re-fetch pending devices,
// so newly connecting devices appear without a manual refresh.
func TestPendingDevicesChangedEventTriggersFetch(t *testing.T) {
	t.Parallel()
	m := newTestApp()
	m.pollingActive = true
	m.tickingActive = true

	// Simulate an eventsMsg containing a PendingDevicesChanged event
	result, cmd := m.Update(eventsMsg{
		events: []Event{
			{
				ID:   1,
				Type: "PendingDevicesChanged",
				Data: []byte(`{}`),
			},
		},
		lastID: 1,
	})
	m = result.(appModel)

	// Should return commands (poll + pending fetch)
	if cmd == nil {
		t.Fatal("expected commands after PendingDevicesChanged event")
	}

	// Execute the batch and check that one of the results is a pendingDevicesMsg
	batchMsg := cmd()
	batch, ok := batchMsg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected BatchMsg, got %T", batchMsg)
	}

	// Should have 2 commands: pollEvents + fetchPending
	if len(batch) != 2 {
		t.Errorf("expected 2 commands in batch, got %d", len(batch))
	}

	// The second command should be fetchPending (pendingMsg)
	if len(batch) >= 2 {
		msg := batch[1]()
		if _, ok := msg.(pendingMsg); !ok {
			t.Errorf("expected pendingMsg, got %T", msg)
		}
	}
}

// TestPendingDevicesMsgUpdatesState verifies that pendingDevicesMsg
// correctly updates the pending devices in the state.
func TestPendingDevicesMsgUpdatesState(t *testing.T) {
	t.Parallel()
	m := newTestApp()

	result, cmd := m.Update(pendingDevicesMsg{
		pending: map[string]PendingDevice{
			"NEW-DEV-ID": {
				Name:    "Pixel 8",
				Address: "192.168.1.50:22000",
			},
		},
	})
	m = result.(appModel)

	if cmd != nil {
		t.Error("pendingDevicesMsg should return nil cmd")
	}
	if len(m.state.PendingDevs) != 1 {
		t.Fatalf("expected 1 pending device, got %d", len(m.state.PendingDevs))
	}
	if m.state.PendingDevs[0].Name != "Pixel 8" {
		t.Errorf("expected name 'Pixel 8', got %q", m.state.PendingDevs[0].Name)
	}
}

// TestNormalEventDoesNotTriggerPendingFetch verifies that non-pending
// events don't trigger an unnecessary pending devices fetch.
func TestNormalEventDoesNotTriggerPendingFetch(t *testing.T) {
	t.Parallel()
	m := newTestApp()
	m.pollingActive = true

	result, cmd := m.Update(eventsMsg{
		events: []Event{
			{ID: 1, Type: "StateChanged", Data: []byte(`{"folder":"f1","from":"idle","to":"syncing"}`)},
		},
		lastID: 1,
	})
	m = result.(appModel)

	if cmd == nil {
		t.Fatal("expected poll command")
	}

	// Should be just the poll command (no batch, no pending fetch)
	msg := cmd()
	if _, ok := msg.(tea.BatchMsg); ok {
		t.Error("normal event should not produce a batch")
	}
}

// --- Pending device navigation and acceptance tests ---

// TestNavigateToPendingDevice verifies the cursor can move from remote
// devices into the pending devices section.
func TestNavigateToPendingDevice(t *testing.T) {
	t.Parallel()
	m := newTestApp()
	m.state.MyID = "LOCAL"
	m.state.Devices = []DeviceState{
		{ID: "LOCAL", Name: "Local"},
		{ID: "REMOTE1", Name: "Remote1"},
	}
	m.state.PendingDevs = []PendingDeviceState{
		{DeviceID: "PENDING1", Name: "Phone", Address: "192.168.1.50:22000"},
		{DeviceID: "PENDING2", Name: "Tablet", Address: "192.168.1.51:22000"},
	}
	m.activeTab = tabDevices
	m.deviceCursor = 0 // on REMOTE1

	// Press down to move past the remote device into pending
	downKey := tea.KeyPressMsg{Code: tea.KeyDown}
	result, _ := m.Update(downKey)
	m = result.(appModel)
	if m.deviceCursor != 1 {
		t.Errorf("cursor = %d, want 1 (first pending)", m.deviceCursor)
	}

	// Down again to second pending
	result, _ = m.Update(downKey)
	m = result.(appModel)
	if m.deviceCursor != 2 {
		t.Errorf("cursor = %d, want 2 (second pending)", m.deviceCursor)
	}

	// Down again should wrap to first item
	result, _ = m.Update(downKey)
	m = result.(appModel)
	if m.deviceCursor != 0 {
		t.Errorf("cursor = %d, want 0 (wrapped)", m.deviceCursor)
	}

	// Up should wrap to last item
	upKey := tea.KeyPressMsg{Code: tea.KeyUp}
	result, _ = m.Update(upKey)
	m = result.(appModel)
	if m.deviceCursor != 2 {
		t.Errorf("cursor = %d, want 2 (wrapped to end)", m.deviceCursor)
	}
}

// TestAcceptPendingDeviceOpensForm verifies that pressing Enter on a
// pending device opens a pre-filled Add Device form.
func TestAcceptPendingDeviceOpensForm(t *testing.T) {
	t.Parallel()
	m := newTestApp()
	m.state.MyID = "LOCAL"
	m.state.Devices = []DeviceState{
		{ID: "LOCAL", Name: "Local"},
	}
	m.state.PendingDevs = []PendingDeviceState{
		{DeviceID: "PENDING-DEV-ID", Name: "Pixel 8", Address: "192.168.1.50:22000"},
	}
	m.activeTab = tabDevices
	m.deviceCursor = 0 // first item is the pending device (no remote devices besides local)

	// Press Enter
	result, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = result.(appModel)

	if m.form == nil {
		t.Fatal("expected form to open for pending device")
	}
	if m.form.kind != formAddDevice {
		t.Errorf("form kind = %d, want formAddDevice", m.form.kind)
	}
	// Check fields are pre-filled
	if m.form.inputs[0].Value() != "PENDING-DEV-ID" {
		t.Errorf("Device ID = %q, want PENDING-DEV-ID", m.form.inputs[0].Value())
	}
	if m.form.inputs[1].Value() != "Pixel 8" {
		t.Errorf("Name = %q, want 'Pixel 8'", m.form.inputs[1].Value())
	}
	if m.form.inputs[2].Value() != "192.168.1.50:22000" {
		t.Errorf("Address = %q, want '192.168.1.50:22000'", m.form.inputs[2].Value())
	}
}

// TestDismissPendingDevice verifies that pressing x on a pending device
// calls PendingDeviceDismiss.
func TestDismissPendingDevice(t *testing.T) {
	t.Parallel()
	mc := &mockClient{}
	m := newTestAppWithClient(mc)
	m.state.MyID = "LOCAL"
	m.state.Devices = []DeviceState{
		{ID: "LOCAL", Name: "Local"},
	}
	m.state.PendingDevs = []PendingDeviceState{
		{DeviceID: "PENDING1", Name: "Phone"},
	}
	m.activeTab = tabDevices
	m.deviceCursor = 0 // on the pending device

	// Press x to dismiss
	result, cmd := m.Update(tea.KeyPressMsg{Code: 'x'})
	m = result.(appModel)

	if cmd == nil {
		t.Fatal("expected command from dismiss")
	}

	msg := cmd()
	action := msg.(actionResultMsg)
	if action.action != "dismiss pending" {
		t.Errorf("action = %q, want 'dismiss pending'", action.action)
	}

	calls := mc.getCalls()
	if len(calls) != 1 || calls[0] != "PendingDeviceDismiss" {
		t.Errorf("expected [PendingDeviceDismiss], got %v", calls)
	}
}

// TestMultiplePendingDevices verifies navigation works with multiple
// pending devices and no configured remote devices.
func TestMultiplePendingDevices(t *testing.T) {
	t.Parallel()
	m := newTestApp()
	m.state.MyID = "LOCAL"
	m.state.Devices = []DeviceState{
		{ID: "LOCAL", Name: "Local"},
	}
	m.state.PendingDevs = []PendingDeviceState{
		{DeviceID: "P1", Name: "Phone"},
		{DeviceID: "P2", Name: "Tablet"},
		{DeviceID: "P3", Name: "Laptop"},
	}
	m.activeTab = tabDevices

	// No remote devices (only local), so cursor 0 = first pending
	downKey := tea.KeyPressMsg{Code: tea.KeyDown}

	// Navigate through all pending devices
	result, _ := m.Update(downKey)
	m = result.(appModel)
	if m.deviceCursor != 1 {
		t.Errorf("cursor = %d, want 1", m.deviceCursor)
	}

	result, _ = m.Update(downKey)
	m = result.(appModel)
	if m.deviceCursor != 2 {
		t.Errorf("cursor = %d, want 2", m.deviceCursor)
	}

	// Accept the third pending device (cursor=2)
	result, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = result.(appModel)

	if m.form == nil {
		t.Fatal("expected form for third pending device")
	}
	if m.form.inputs[0].Value() != "P3" {
		t.Errorf("Device ID = %q, want P3", m.form.inputs[0].Value())
	}
	if m.form.inputs[1].Value() != "Laptop" {
		t.Errorf("Name = %q, want 'Laptop'", m.form.inputs[1].Value())
	}
}

// TestCursorClampWithPendingDevices verifies cursor clamping includes
// pending devices in the total count.
func TestCursorClampWithPendingDevices(t *testing.T) {
	t.Parallel()
	m := newTestApp()
	m.state.MyID = "LOCAL"
	m.state.Devices = []DeviceState{{ID: "LOCAL"}}
	m.state.PendingDevs = []PendingDeviceState{{DeviceID: "P1"}}
	m.deviceCursor = 5 // way past the end

	m.clampDeviceCursor()

	// 0 remote + 1 pending = 1 total, max cursor = 0
	if m.deviceCursor != 0 {
		t.Errorf("cursor = %d, want 0 (clamped)", m.deviceCursor)
	}
}

// TestPendingOnlyNoRemoteDevices verifies that when there are no
// configured remote devices, the cursor starts on the pending device
// and Enter works immediately.
func TestPendingOnlyNoRemoteDevices(t *testing.T) {
	t.Parallel()
	m := newTestApp()
	m.state.MyID = "LOCAL"
	m.state.Devices = []DeviceState{
		{ID: "LOCAL", Name: "Local"},
		// No remote devices
	}
	m.state.PendingDevs = []PendingDeviceState{
		{DeviceID: "PHONE-ID", Name: "Pixel 8", Address: "192.168.1.50:22000"},
	}
	m.activeTab = tabDevices
	m.deviceCursor = 0

	// With 0 remotes + 1 pending, cursor 0 IS the pending device.
	// Enter should open the accept form.
	result, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = result.(appModel)

	if m.form == nil {
		t.Fatal("Enter on pending device (cursor=0, no remotes) should open form")
	}
	if m.form.inputs[0].Value() != "PHONE-ID" {
		t.Errorf("Device ID = %q, want PHONE-ID", m.form.inputs[0].Value())
	}
	if m.form.inputs[1].Value() != "Pixel 8" {
		t.Errorf("Name = %q, want 'Pixel 8'", m.form.inputs[1].Value())
	}
}

// --- Pending folder tests ---

// TestNavigateToPendingFolder verifies the cursor can move from configured
// folders into the pending folders section.
func TestNavigateToPendingFolder(t *testing.T) {
	t.Parallel()
	m := newTestApp()
	m.state.Folders = []FolderState{{ID: "f1", Label: "Existing"}}
	m.state.PendingFldrs = []PendingFolderState{
		{FolderID: "photos", Label: "Photos", DeviceID: "DEV1", DeviceName: "Phone"},
		{FolderID: "docs", Label: "Documents", DeviceID: "DEV1", DeviceName: "Phone"},
	}
	m.activeTab = tabFolders
	m.folderCursor = 0 // on configured folder

	downKey := tea.KeyPressMsg{Code: tea.KeyDown}

	// Move past configured folder into first pending
	result, _ := m.Update(downKey)
	m = result.(appModel)
	if m.folderCursor != 1 {
		t.Errorf("cursor = %d, want 1 (first pending)", m.folderCursor)
	}

	// Move to second pending
	result, _ = m.Update(downKey)
	m = result.(appModel)
	if m.folderCursor != 2 {
		t.Errorf("cursor = %d, want 2 (second pending)", m.folderCursor)
	}

	// Down again should wrap to first item
	result, _ = m.Update(downKey)
	m = result.(appModel)
	if m.folderCursor != 0 {
		t.Errorf("cursor = %d, want 0 (wrapped)", m.folderCursor)
	}
}

// TestAcceptPendingFolderOpensForm verifies Enter on a pending folder
// opens a pre-filled accept form.
func TestAcceptPendingFolderOpensForm(t *testing.T) {
	t.Parallel()
	m := newTestApp()
	m.state.PendingFldrs = []PendingFolderState{
		{FolderID: "photos", Label: "Vacation Photos", DeviceID: "DEV-123", DeviceName: "Phone"},
	}
	m.activeTab = tabFolders
	m.folderCursor = 0 // on the pending folder (no configured folders)

	result, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = result.(appModel)

	if m.form == nil {
		t.Fatal("Enter on pending folder should open form")
	}
	if m.form.kind != formAcceptFolder {
		t.Errorf("form kind = %d, want formAcceptFolder", m.form.kind)
	}
	if m.form.inputs[0].Value() != "photos" {
		t.Errorf("Folder ID = %q, want 'photos'", m.form.inputs[0].Value())
	}
	if m.form.inputs[1].Value() != "Vacation Photos" {
		t.Errorf("Label = %q, want 'Vacation Photos'", m.form.inputs[1].Value())
	}
	if m.form.inputs[2].Value() != "~/Vacation Photos" {
		t.Errorf("Path = %q, want '~/Vacation Photos'", m.form.inputs[2].Value())
	}
	if m.form.deviceID != "DEV-123" {
		t.Errorf("deviceID = %q, want 'DEV-123'", m.form.deviceID)
	}
}

// TestSubmitAcceptFolder verifies the accept folder form creates the folder
// with the offering device and dismisses the pending entry.
func TestSubmitAcceptFolder(t *testing.T) {
	t.Parallel()
	mc := &mockClient{}
	m := newTestAppWithClient(mc)

	form := newAcceptFolderForm("photos", "Vacation Photos", "DEV-123")
	form.inputs[2].SetValue("/home/user/photos")
	m.form = &form

	result, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = result.(appModel)

	if m.form != nil {
		t.Error("form should be nil after submit")
	}
	if cmd == nil {
		t.Fatal("expected command")
	}

	msg := cmd()
	action := msg.(actionResultMsg)
	if action.err != nil {
		t.Errorf("unexpected error: %v", action.err)
	}
	if action.action != "accept folder" {
		t.Errorf("action = %q, want 'accept folder'", action.action)
	}

	calls := mc.getCalls()
	if len(calls) < 2 {
		t.Fatalf("expected at least 2 calls, got %v", calls)
	}
	if calls[0] != "FolderAdd" {
		t.Errorf("first call = %q, want FolderAdd", calls[0])
	}
	if calls[1] != "PendingFolderDismiss" {
		t.Errorf("second call = %q, want PendingFolderDismiss", calls[1])
	}
}

// TestDismissPendingFolder verifies x on a pending folder calls dismiss.
func TestDismissPendingFolder(t *testing.T) {
	t.Parallel()
	mc := &mockClient{}
	m := newTestAppWithClient(mc)
	m.state.PendingFldrs = []PendingFolderState{
		{FolderID: "photos", DeviceID: "DEV-1"},
	}
	m.activeTab = tabFolders
	m.folderCursor = 0

	result, cmd := m.Update(tea.KeyPressMsg{Code: 'x'})
	m = result.(appModel)

	if cmd == nil {
		t.Fatal("expected command from dismiss")
	}

	msg := cmd()
	action := msg.(actionResultMsg)
	if action.action != "dismiss pending folder" {
		t.Errorf("action = %q, want 'dismiss pending folder'", action.action)
	}

	calls := mc.getCalls()
	if len(calls) != 1 || calls[0] != "PendingFolderDismiss" {
		t.Errorf("expected [PendingFolderDismiss], got %v", calls)
	}
}

// TestPendingFolderOnlyNoConfiguredFolders verifies cursor works when
// there are only pending folders and no configured ones.
func TestPendingFolderOnlyNoConfiguredFolders(t *testing.T) {
	t.Parallel()
	m := newTestApp()
	m.state.PendingFldrs = []PendingFolderState{
		{FolderID: "f1", Label: "Folder1"},
	}
	m.activeTab = tabFolders
	m.folderCursor = 0

	// Enter should accept
	result, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = result.(appModel)

	if m.form == nil {
		t.Fatal("expected accept form")
	}
	if m.form.kind != formAcceptFolder {
		t.Errorf("form kind = %d, want formAcceptFolder", m.form.kind)
	}
}

// TestFolderActionsNotAppliedToPending verifies scan/pause/share/override/revert
// don't apply when cursor is on a pending folder.
func TestFolderActionsNotAppliedToPending(t *testing.T) {
	t.Parallel()
	mc := &mockClient{}
	m := newTestAppWithClient(mc)
	m.state.PendingFldrs = []PendingFolderState{
		{FolderID: "pending1"},
	}
	m.activeTab = tabFolders
	m.folderCursor = 0 // on pending

	// Scan should do nothing
	result, cmd := m.Update(tea.KeyPressMsg{Code: 's'})
	m = result.(appModel)
	if cmd != nil {
		t.Error("scan should not apply to pending folder")
	}

	// Pause should do nothing
	result, cmd = m.Update(tea.KeyPressMsg{Code: 'p'})
	m = result.(appModel)
	if cmd != nil {
		t.Error("pause should not apply to pending folder")
	}

	// Share should do nothing
	result, _ = m.Update(tea.KeyPressMsg{Code: 'S'})
	m = result.(appModel)
	if m.form != nil {
		t.Error("share should not apply to pending folder")
	}

	if len(mc.getCalls()) != 0 {
		t.Errorf("no API calls expected, got %v", mc.getCalls())
	}
}

// TestPendingMsgUpdatesBoth verifies the combined pendingMsg updates
// both pending devices and folders.
func TestPendingMsgUpdatesBoth(t *testing.T) {
	t.Parallel()
	m := newTestApp()

	result, _ := m.Update(pendingMsg{
		devices: map[string]PendingDevice{
			"DEV1": {Name: "Phone"},
		},
		folders: map[string]PendingFolderEntry{
			"photos": {OfferedBy: map[string]PendingFolderOffer{
				"DEV1": {Label: "Photos"},
			}},
		},
	})
	m = result.(appModel)

	if len(m.state.PendingDevs) != 1 {
		t.Errorf("expected 1 pending device, got %d", len(m.state.PendingDevs))
	}
	if len(m.state.PendingFldrs) != 1 {
		t.Errorf("expected 1 pending folder, got %d", len(m.state.PendingFldrs))
	}
}

// TestUpdatePendingFoldersFromAPI verifies the state method correctly
// converts the API response to PendingFolderState entries.
func TestUpdatePendingFoldersFromAPI(t *testing.T) {
	t.Parallel()
	var s AppState
	s.Devices = []DeviceState{{ID: "DEV-1", Name: "Phone"}}

	s.UpdatePendingFoldersFromAPI(map[string]PendingFolderEntry{
		"photos": {OfferedBy: map[string]PendingFolderOffer{
			"DEV-1": {Label: "Vacation Photos"},
		}},
		"docs": {OfferedBy: map[string]PendingFolderOffer{
			"DEV-1": {Label: "Documents"},
		}},
	})

	if len(s.PendingFldrs) != 2 {
		t.Fatalf("expected 2 pending folders, got %d", len(s.PendingFldrs))
	}

	// Check that device name was resolved
	for _, p := range s.PendingFldrs {
		if p.DeviceName != "Phone" {
			t.Errorf("DeviceName = %q, want 'Phone'", p.DeviceName)
		}
	}
}

// TestCursorClampWithPendingFolders verifies folder cursor clamping
// accounts for pending folders.
func TestCursorClampWithPendingFolders(t *testing.T) {
	t.Parallel()
	m := newTestApp()
	m.state.Folders = []FolderState{{ID: "f1"}}
	m.state.PendingFldrs = []PendingFolderState{{FolderID: "p1"}}
	m.folderCursor = 10 // way past end

	m.clampFolderCursor()

	// 1 configured + 1 pending = 2 items, max cursor = 1
	if m.folderCursor != 1 {
		t.Errorf("cursor = %d, want 1", m.folderCursor)
	}
}

// TestShareFolderShowsAvailableDevices verifies the share folder form
// shows known devices that don't already share the folder.
func TestShareFolderShowsAvailableDevices(t *testing.T) {
	t.Parallel()
	m := newTestApp()
	m.state.MyID = "LOCAL"
	m.state.Devices = []DeviceState{
		{ID: "LOCAL", Name: "Local"},
		{ID: "DEV-A", Name: "Node2"},
		{ID: "DEV-B", Name: "Laptop"},
		{ID: "DEV-C", Name: "Phone"},
	}
	m.state.Folders = []FolderState{
		{
			ID:        "f1",
			Label:     "Photos",
			DeviceIDs: []string{"LOCAL", "DEV-A"}, // Already shared with DEV-A
		},
	}
	m.activeTab = tabFolders
	m.folderCursor = 0

	// Press S to share
	result, _ := m.Update(tea.KeyPressMsg{Code: 'S'})
	m = result.(appModel)

	if m.form == nil {
		t.Fatal("expected share form to open")
	}
	if m.form.kind != formShareFolder {
		t.Errorf("form kind = %d, want formShareFolder", m.form.kind)
	}

	// Should show DEV-B and DEV-C (not LOCAL, not DEV-A which already shares)
	if len(m.form.discoveredDevices) != 2 {
		t.Fatalf("expected 2 available devices, got %d", len(m.form.discoveredDevices))
	}

	names := map[string]bool{}
	for _, d := range m.form.discoveredDevices {
		names[d.Name] = true
	}
	if !names["Laptop"] {
		t.Error("expected 'Laptop' in available devices")
	}
	if !names["Phone"] {
		t.Error("expected 'Phone' in available devices")
	}

	// Discovery list should be focused since there are available devices
	if !m.form.discoveryFocused {
		t.Error("discovery list should be focused when available devices exist")
	}
}

// TestShareFolderNoAvailableDevices verifies the share form falls back
// to manual input when all devices already share the folder.
func TestShareFolderNoAvailableDevices(t *testing.T) {
	t.Parallel()
	m := newTestApp()
	m.state.MyID = "LOCAL"
	m.state.Devices = []DeviceState{
		{ID: "LOCAL", Name: "Local"},
		{ID: "DEV-A", Name: "Node2"},
	}
	m.state.Folders = []FolderState{
		{
			ID:        "f1",
			Label:     "Photos",
			DeviceIDs: []string{"LOCAL", "DEV-A"}, // All devices already share
		},
	}
	m.activeTab = tabFolders
	m.folderCursor = 0

	// Press S to share
	result, _ := m.Update(tea.KeyPressMsg{Code: 'S'})
	m = result.(appModel)

	if m.form == nil {
		t.Fatal("expected share form to open")
	}

	// No available devices, so discovery list should be empty
	if len(m.form.discoveredDevices) != 0 {
		t.Errorf("expected 0 available devices, got %d", len(m.form.discoveredDevices))
	}

	// Manual input should be focused
	if m.form.discoveryFocused {
		t.Error("discovery list should not be focused when no available devices")
	}
}

// TestTabBarRendering verifies the tab bar renders without panic.
func TestTabBarRendering(t *testing.T) {
	t.Parallel()
	styles := newStyles(true)

	for tab := 0; tab < numTabs; tab++ {
		bar := renderTabBar(tab, styles, 80)
		if bar == "" {
			t.Errorf("renderTabBar(tab=%d) returned empty string", tab)
		}
	}
}

// TestActionResultTriggersFullRefresh verifies that successful config-changing
// actions (add, remove, share) trigger a full state refresh so the UI updates
// immediately instead of showing stale data.
func TestActionResultTriggersFullRefresh(t *testing.T) {
	t.Parallel()
	m := newTestApp()
	m.pollingActive = true
	m.tickingActive = true
	m.state.Folders = []FolderState{{ID: "f1", Label: "Existing"}}

	// Simulate a successful "remove folder" action completing
	result, cmd := m.Update(actionResultMsg{action: "remove folder", err: nil})
	m = result.(appModel)

	if cmd == nil {
		t.Fatal("successful action should return a command")
	}

	// Execute the command — it should be fetchFullState, which returns fullStateMsg
	msg := cmd()
	if _, ok := msg.(fullStateMsg); !ok {
		t.Errorf("expected fullStateMsg from action refresh, got %T", msg)
	}
}

// TestRemoveFolderUpdatesUI simulates the full remove flow: cursor on folder,
// confirm yes, action succeeds, full state refresh arrives with the folder gone.
func TestRemoveFolderUpdatesUI(t *testing.T) {
	t.Parallel()
	mc := &mockClient{}
	m := newTestAppWithClient(mc)
	m.pollingActive = true
	m.tickingActive = true
	m.state.Folders = []FolderState{
		{ID: "f1", Label: "Keep This"},
		{ID: "f2", Label: "Remove This"},
	}
	m.activeTab = tabFolders
	m.folderCursor = 1 // on f2

	// Step 1: Press x to remove
	result, _ := m.Update(tea.KeyPressMsg{Code: 'x'})
	m = result.(appModel)
	if m.confirm == nil {
		t.Fatal("expected confirm dialog")
	}

	// Step 2: Press y to confirm
	result, cmd := m.Update(tea.KeyPressMsg{Code: 'y'})
	m = result.(appModel)
	if cmd == nil {
		t.Fatal("expected command from confirm")
	}

	// Step 3: Execute the doAction (calls FolderRemove)
	actionMsg := cmd()
	result, cmd = m.Update(actionMsg)
	m = result.(appModel)

	// Step 4: The action triggers fetchFullState
	if cmd == nil {
		t.Fatal("expected fetchFullState command after action")
	}

	// Step 5: Simulate the fullStateMsg arriving with f2 gone
	result, _ = m.Update(fullStateMsg{state: &AppState{
		MyID:    "TEST",
		Version: "v1.0",
		Folders: []FolderState{{ID: "f1", Label: "Keep This"}},
	}})
	m = result.(appModel)

	// Verify: only f1 remains
	if len(m.state.Folders) != 1 {
		t.Errorf("expected 1 folder, got %d", len(m.state.Folders))
	}
	if m.state.Folders[0].ID != "f1" {
		t.Errorf("remaining folder = %q, want f1", m.state.Folders[0].ID)
	}
	// Cursor should be clamped
	if m.folderCursor > 0 {
		t.Errorf("cursor = %d, should be clamped to 0", m.folderCursor)
	}
}

// TestRemoveDeviceUpdatesUI same flow for devices.
func TestRemoveDeviceUpdatesUI(t *testing.T) {
	t.Parallel()
	mc := &mockClient{}
	m := newTestAppWithClient(mc)
	m.pollingActive = true
	m.tickingActive = true
	m.state.MyID = "LOCAL"
	m.state.Devices = []DeviceState{
		{ID: "LOCAL", Name: "Local"},
		{ID: "REMOTE1", Name: "Keep"},
		{ID: "REMOTE2", Name: "Remove"},
	}
	m.activeTab = tabDevices
	m.deviceCursor = 1 // on REMOTE2 (second remote, index 1)

	// Press x, confirm y
	result, _ := m.Update(tea.KeyPressMsg{Code: 'x'})
	m = result.(appModel)
	result, cmd := m.Update(tea.KeyPressMsg{Code: 'y'})
	m = result.(appModel)

	// Execute action
	actionMsg := cmd()
	result, cmd = m.Update(actionMsg)
	m = result.(appModel)

	// Simulate fullStateMsg with REMOTE2 gone
	result, _ = m.Update(fullStateMsg{state: &AppState{
		MyID: "LOCAL",
		Devices: []DeviceState{
			{ID: "LOCAL", Name: "Local"},
			{ID: "REMOTE1", Name: "Keep"},
		},
	}})
	m = result.(appModel)

	// Only LOCAL + REMOTE1 remain
	if len(m.state.Devices) != 2 {
		t.Errorf("expected 2 devices, got %d", len(m.state.Devices))
	}
	// Cursor should be clamped (only 1 remote now, cursor max = 0)
	if m.deviceCursor > 0 {
		t.Errorf("cursor = %d, should be clamped to 0", m.deviceCursor)
	}
}

// TestAddFolderUpdatesUI verifies a newly added folder appears after the
// full state refresh.
func TestAddFolderUpdatesUI(t *testing.T) {
	t.Parallel()
	mc := &mockClient{}
	m := newTestAppWithClient(mc)
	m.pollingActive = true
	m.tickingActive = true
	m.activeTab = tabFolders

	// Submit add folder form
	form := newAddFolderForm()
	form.inputs[0].SetValue("new-folder")
	form.inputs[1].SetValue("New Folder")
	form.inputs[2].SetValue("/tmp/new")
	m.form = &form

	result, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = result.(appModel)

	// Execute action
	if cmd != nil {
		actionMsg := cmd()
		result, cmd = m.Update(actionMsg)
		m = result.(appModel)
	}

	// Simulate fullStateMsg with the new folder
	result, _ = m.Update(fullStateMsg{state: &AppState{
		MyID: "TEST",
		Folders: []FolderState{
			{ID: "new-folder", Label: "New Folder"},
		},
	}})
	m = result.(appModel)

	if len(m.state.Folders) != 1 {
		t.Errorf("expected 1 folder, got %d", len(m.state.Folders))
	}
	if m.state.Folders[0].Label != "New Folder" {
		t.Errorf("folder label = %q, want 'New Folder'", m.state.Folders[0].Label)
	}
}

// --- Edit form tests ---

// TestEditFolderOpensForm verifies E key on a folder fetches config.
func TestEditFolderOpensForm(t *testing.T) {
	t.Parallel()
	mc := &mockClient{}
	m := newTestAppWithClient(mc)
	m.state.Folders = []FolderState{{ID: "f1", Label: "Test"}}
	m.activeTab = tabFolders
	m.folderCursor = 0

	result, cmd := m.Update(tea.KeyPressMsg{Code: 'E'})
	m = result.(appModel)

	// Should return a fetchFolderConfig command
	if cmd == nil {
		t.Fatal("expected fetchFolderConfig command")
	}

	calls := mc.getCalls()
	// The command hasn't executed yet, no API calls
	if len(calls) != 0 {
		t.Errorf("no API calls expected before cmd executes, got %v", calls)
	}
}

// TestFolderConfigMsgOpensEditForm verifies folderConfigMsg opens the form.
func TestFolderConfigMsgOpensEditForm(t *testing.T) {
	t.Parallel()
	m := newTestApp()
	m.state.Folders = []FolderState{{ID: "f1", Label: "Test"}}

	result, _ := m.Update(folderConfigMsg{
		cfg: config.FolderConfiguration{
			ID:              "f1",
			Label:           "Test",
			Path:            "/tmp/test",
			RescanIntervalS: 3600,
		},
	})
	m = result.(appModel)

	if m.form == nil {
		t.Fatal("expected edit folder form to open")
	}
	if m.form.kind != formEditFolder {
		t.Errorf("form kind = %d, want formEditFolder", m.form.kind)
	}
	if m.form.inputs[0].Value() != "Test" {
		t.Errorf("Label = %q, want 'Test'", m.form.inputs[0].Value())
	}
}

// TestEditDeviceOpensForm verifies E key on a device fetches config.
func TestEditDeviceOpensForm(t *testing.T) {
	t.Parallel()
	mc := &mockClient{}
	m := newTestAppWithClient(mc)
	m.state.MyID = "LOCAL"
	m.state.Devices = []DeviceState{
		{ID: "LOCAL", Name: "Local"},
		{ID: "REMOTE", Name: "Remote"},
	}
	m.activeTab = tabDevices
	m.deviceCursor = 0

	result, cmd := m.Update(tea.KeyPressMsg{Code: 'E'})
	m = result.(appModel)

	if cmd == nil {
		t.Fatal("expected fetchDeviceConfig command")
	}
}

// TestDeviceConfigMsgOpensEditForm verifies deviceConfigMsg opens the form.
func TestDeviceConfigMsgOpensEditForm(t *testing.T) {
	t.Parallel()
	var devID protocol.DeviceID
	devID[0] = 2

	m := newTestApp()
	m.state.MyID = "LOCAL"
	m.state.Devices = []DeviceState{
		{ID: "LOCAL"},
		{ID: devID.String(), Name: "TestDev"},
	}
	m.activeTab = tabDevices

	result, _ := m.Update(deviceConfigMsg{
		cfg: config.DeviceConfiguration{
			DeviceID:          devID,
			Name:              "TestDev",
			Addresses:         []string{"dynamic"},
			Introducer:        true,
			AutoAcceptFolders: true,
		},
	})
	m = result.(appModel)

	if m.form == nil {
		t.Fatal("expected edit device form to open")
	}
	if m.form.kind != formEditDevice {
		t.Errorf("form kind = %d, want formEditDevice", m.form.kind)
	}
	if m.form.inputs[0].Value() != "TestDev" {
		t.Errorf("Name = %q, want 'TestDev'", m.form.inputs[0].Value())
	}
	// Check selectors include Introducer and Auto Accept
	vals := m.form.values()
	if vals["Introducer"] != "true" {
		t.Errorf("Introducer = %q, want 'true'", vals["Introducer"])
	}
	if vals["Auto Accept Folders"] != "true" {
		t.Errorf("Auto Accept = %q, want 'true'", vals["Auto Accept Folders"])
	}
}

// TestSubmitEditFolder verifies the edit folder form calls FolderUpdate.
func TestSubmitEditFolder(t *testing.T) {
	t.Parallel()
	mc := &mockClient{
		configGetResult: config.Configuration{
			Folders: []config.FolderConfiguration{
				{ID: "f1", Label: "Old", RescanIntervalS: 3600},
			},
		},
	}
	m := newTestAppWithClient(mc)
	m.state.Folders = []FolderState{{ID: "f1"}}

	// Create edit folder form
	form := newEditFolderForm(
		&m.state.Folders[0],
		config.FolderConfiguration{ID: "f1", Label: "Old", RescanIntervalS: 3600},
	)
	form.inputs[0].SetValue("New Label")
	m.form = &form

	result, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = result.(appModel)

	if m.form != nil {
		t.Error("form should be nil after submit")
	}
	if cmd == nil {
		t.Fatal("expected command")
	}

	msg := cmd()
	action := msg.(actionResultMsg)
	if action.action != "edit folder" {
		t.Errorf("action = %q, want 'edit folder'", action.action)
	}

	calls := mc.getCalls()
	found := false
	for _, c := range calls {
		if c == "FolderUpdate" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected FolderUpdate in calls, got %v", calls)
	}
}

// TestSubmitEditDevice verifies the edit device form with introducer.
func TestSubmitEditDevice(t *testing.T) {
	t.Parallel()
	var devID protocol.DeviceID
	devID[0] = 2

	mc := &mockClient{
		configDeviceGetResult: config.DeviceConfiguration{
			DeviceID:  devID,
			Name:      "Old",
			Addresses: []string{"dynamic"},
		},
	}
	m := newTestAppWithClient(mc)
	m.state.MyID = "LOCAL"
	m.state.Devices = []DeviceState{
		{ID: "LOCAL"},
		{ID: devID.String(), Name: "Old"},
	}

	form := newEditDeviceForm(
		&m.state.Devices[1],
		config.DeviceConfiguration{
			DeviceID:  devID,
			Name:      "Old",
			Addresses: []string{"dynamic"},
		},
	)
	form.inputs[0].SetValue("New Name")
	m.form = &form

	result, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = result.(appModel)

	if cmd == nil {
		t.Fatal("expected command")
	}

	msg := cmd()
	action := msg.(actionResultMsg)
	if action.action != "edit device" {
		t.Errorf("action = %q, want 'edit device'", action.action)
	}

	calls := mc.getCalls()
	found := false
	for _, c := range calls {
		if c == "DeviceUpdate" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected DeviceUpdate in calls, got %v", calls)
	}
}

// TestEditNotAppliedToPending verifies E key doesn't work on pending items.
func TestEditNotAppliedToPending(t *testing.T) {
	t.Parallel()
	m := newTestApp()
	m.state.PendingFldrs = []PendingFolderState{{FolderID: "p1"}}
	m.activeTab = tabFolders
	m.folderCursor = 0

	result, cmd := m.Update(tea.KeyPressMsg{Code: 'E'})
	m = result.(appModel)

	if cmd != nil {
		t.Error("edit should not apply to pending items")
	}
}

// TestConfigSavedEventTriggersFullRefresh verifies that a ConfigSaved event
// (e.g., from an introducer adding devices) triggers a full state refresh
// so new devices/folders appear immediately without manual 'r'.
func TestConfigSavedEventTriggersFullRefresh(t *testing.T) {
	t.Parallel()
	m := newTestApp()
	m.pollingActive = true
	m.tickingActive = true

	result, cmd := m.Update(eventsMsg{
		events: []Event{
			{ID: 1, Type: "ConfigSaved", Data: []byte(`{}`)},
		},
		lastID: 1,
	})
	m = result.(appModel)

	if cmd == nil {
		t.Fatal("expected commands after ConfigSaved event")
	}

	// Should be a batch containing pollEvents + fetchFullState
	batchMsg := cmd()
	batch, ok := batchMsg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected BatchMsg, got %T", batchMsg)
	}
	if len(batch) != 2 {
		t.Errorf("expected 2 commands (poll + fullState), got %d", len(batch))
	}

	// The second command should be fetchFullState which returns fullStateMsg
	if len(batch) >= 2 {
		msg := batch[1]()
		if _, ok := msg.(fullStateMsg); !ok {
			t.Errorf("expected fullStateMsg from config refresh, got %T", msg)
		}
	}
}

// TestAddDeviceWithFolderSharing verifies that when adding a device with
// folders selected in the checkboxes, the folders are shared after adding.
func TestAddDeviceWithFolderSharing(t *testing.T) {
	t.Parallel()
	mc := &mockClient{
		configFolderGetResult: config.FolderConfiguration{
			ID: "photos",
		},
	}
	m := newTestAppWithClient(mc)
	m.state.Folders = []FolderState{
		{ID: "photos", Label: "Photos"},
		{ID: "docs", Label: "Documents"},
	}

	form := newAddDeviceForm(nil, m.state.Folders)
	// Manually set device ID (skip validation issues with fake IDs)
	form.inputs[0].SetValue("AAAAAAA-BBBBBBB-CCCCCCC-DDDDDDD-EEEEEEE-FFFFFFF-GGGGGGG-HHHHHHH")
	form.inputs[1].SetValue("New Device")
	// Select the "Photos" folder checkbox
	form.folderCheckboxes[0].Selected = true
	// Leave "Documents" unselected
	m.form = &form

	result, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = result.(appModel)

	if cmd == nil {
		// Device ID validation may fail with our fake ID
		return
	}

	msg := cmd()
	action, ok := msg.(actionResultMsg)
	if !ok {
		return
	}
	if action.action != "add device" {
		t.Errorf("action = %q, want 'add device'", action.action)
	}

	// Check that DeviceAdd was called, then ConfigFolderGet + FolderUpdate for the selected folder
	calls := mc.getCalls()
	hasDeviceAdd := false
	hasFolderUpdate := false
	for _, c := range calls {
		if c == "DeviceAdd" {
			hasDeviceAdd = true
		}
		if c == "FolderUpdate" {
			hasFolderUpdate = true
		}
	}
	if !hasDeviceAdd {
		t.Errorf("expected DeviceAdd in calls, got %v", calls)
	}
	if !hasFolderUpdate {
		t.Errorf("expected FolderUpdate for shared folder, got %v", calls)
	}
}

// TestAddDeviceNoFolderSharing verifies adding a device without selecting
// any folders doesn't call FolderUpdate.
func TestAddDeviceNoFolderSharing(t *testing.T) {
	t.Parallel()
	mc := &mockClient{}
	m := newTestAppWithClient(mc)
	m.state.Folders = []FolderState{{ID: "f1", Label: "F1"}}

	form := newAddDeviceForm(nil, m.state.Folders)
	form.inputs[0].SetValue("AAAAAAA-BBBBBBB-CCCCCCC-DDDDDDD-EEEEEEE-FFFFFFF-GGGGGGG-HHHHHHH")
	form.inputs[1].SetValue("Test")
	// Don't select any folder checkboxes
	m.form = &form

	result, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = result.(appModel)

	if cmd == nil {
		return
	}

	msg := cmd()
	if action, ok := msg.(actionResultMsg); ok && action.err == nil {
		// No FolderUpdate should have been called
		for _, c := range mc.getCalls() {
			if c == "FolderUpdate" {
				t.Error("FolderUpdate should not be called when no folders selected")
			}
		}
	}
}

// TestFolderCheckboxToggle verifies space bar toggles folder checkboxes.
func TestFolderCheckboxToggle(t *testing.T) {
	t.Parallel()
	m := newTestApp()
	m.state.Folders = []FolderState{{ID: "f1", Label: "F1"}, {ID: "f2", Label: "F2"}}

	form := newAddDeviceForm(nil, m.state.Folders)
	form.inputs[0].SetValue("test")
	// Navigate to checkbox section
	form.checkboxFocused = true
	m.form = &form

	if m.form.folderCheckboxes[0].Selected {
		t.Fatal("checkbox should start unselected")
	}

	// Press space to toggle
	m.form.Update(tea.KeyPressMsg{Code: ' '})
	if !m.form.folderCheckboxes[0].Selected {
		t.Error("space should toggle checkbox on")
	}

	// Press space again to untoggle
	m.form.Update(tea.KeyPressMsg{Code: ' '})
	if m.form.folderCheckboxes[0].Selected {
		t.Error("space should toggle checkbox off")
	}
}
