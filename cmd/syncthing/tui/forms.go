// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/syncthing/syncthing/lib/config"
)

// formKind identifies which form is active.
type formKind int

const (
	formAddFolder formKind = iota + 1
	formAddDevice
	formShareFolder
	formAcceptFolder
	formEditFolder
	formEditDevice
)

// formSelector represents a single selector field with left/right cycling.
type formSelector struct {
	label    string
	options  []string // internal values
	labels   []string // display labels
	selected int
}

// formModel manages input forms for write operations.
type formModel struct {
	kind   formKind
	inputs []textinput.Model
	focus  int
	title  string
	err    string

	// Context for the form
	folderID string // for share folder / accept folder / edit folder form
	deviceID string // for accept folder / edit device form

	// Folder type selection (for add/accept folder)
	folderTypeIdx int
	folderTypes   []string

	// Additional selector fields (for edit forms)
	selectors []formSelector

	// Discovered/available devices (for add device and share folder forms)
	discoveredDevices []DiscoveredDevice
	discoveryCursor   int
	discoveryFocused  bool // true when discovery list is focused

	// Folder sharing checkboxes (for add/accept device forms)
	folderCheckboxes []folderCheckbox
	checkboxCursor   int
	checkboxFocused  bool // true when checkbox list is focused
}

// folderCheckbox represents a toggleable folder in the sharing section.
type folderCheckbox struct {
	FolderID string
	Label    string
	Selected bool
}

var folderTypeOptions = []string{"sendreceive", "sendonly", "receiveonly", "receiveencrypted"}
var folderTypeLabels = []string{"Send & Receive", "Send Only", "Receive Only", "Receive Encrypted"}

func newAddFolderForm() formModel {
	idInput := textinput.New()
	idInput.Placeholder = "my-folder"
	idInput.Focus()

	labelInput := textinput.New()
	labelInput.Placeholder = "My Folder"

	pathInput := textinput.New()
	pathInput.Placeholder = "/home/user/sync"

	return formModel{
		kind:        formAddFolder,
		title:       "Add Folder",
		inputs:      []textinput.Model{idInput, labelInput, pathInput},
		folderTypes: folderTypeOptions,
	}
}

func newAcceptFolderForm(folderID, label, deviceID string) formModel {
	idInput := textinput.New()
	idInput.Placeholder = "my-folder"
	idInput.SetValue(folderID)

	labelInput := textinput.New()
	labelInput.Placeholder = "My Folder"
	labelInput.SetValue(label)

	pathInput := textinput.New()
	pathInput.Placeholder = "~/Sync"
	if label != "" {
		pathInput.SetValue("~/" + label)
	} else {
		pathInput.SetValue("~/" + folderID)
	}
	pathInput.Focus()

	return formModel{
		kind:        formAcceptFolder,
		title:       "Accept Folder Share",
		inputs:      []textinput.Model{idInput, labelInput, pathInput},
		folderTypes: folderTypeOptions,
		folderID:    folderID,
		deviceID:    deviceID,
	}
}

func newAddDeviceForm(discovered []DiscoveredDevice, folders []FolderState) formModel {
	idInput := textinput.New()
	idInput.Placeholder = "AAAAAAA-BBBBBBB-CCCCCCC-DDDDDDD-EEEEEEE-FFFFFFF-GGGGGGG-HHHHHHH"

	nameInput := textinput.New()
	nameInput.Placeholder = "My Device"

	addrInput := textinput.New()
	addrInput.Placeholder = "dynamic"

	// Build folder checkboxes from available folders
	var checkboxes []folderCheckbox
	for _, f := range folders {
		checkboxes = append(checkboxes, folderCheckbox{
			FolderID: f.ID,
			Label:    f.DisplayName(),
			Selected: false,
		})
	}

	fm := formModel{
		kind:              formAddDevice,
		title:             "Add Device",
		inputs:            []textinput.Model{idInput, nameInput, addrInput},
		discoveredDevices: discovered,
		folderCheckboxes:  checkboxes,
	}

	// If there are discovered devices, start with discovery list focused;
	// otherwise focus the Device ID input.
	if len(discovered) > 0 {
		fm.discoveryFocused = true
	} else {
		idInput.Focus()
		fm.inputs[0] = idInput
	}

	return fm
}

func newShareFolderForm(folderID string, availableDevices []DeviceState) formModel {
	devIDInput := textinput.New()
	devIDInput.Placeholder = "AAAAAAA-BBBBBBB-CCCCCCC-DDDDDDD-EEEEEEE-FFFFFFF-GGGGGGG-HHHHHHH"

	// Convert available devices to DiscoveredDevice entries for the picker
	var discovered []DiscoveredDevice
	for _, d := range availableDevices {
		discovered = append(discovered, DiscoveredDevice{
			DeviceID: d.ID,
			Name:     d.Name,
		})
	}

	fm := formModel{
		kind:              formShareFolder,
		title:             "Share Folder to Device",
		inputs:            []textinput.Model{devIDInput},
		folderID:          folderID,
		discoveredDevices: discovered,
	}

	// If there are available devices, start with the device list focused;
	// otherwise focus the manual Device ID input.
	if len(discovered) > 0 {
		fm.discoveryFocused = true
	} else {
		devIDInput.Focus()
		fm.inputs[0] = devIDInput
	}

	return fm
}

// Pull order options for the selector.
var pullOrderOptions = []string{"random", "alphabetic", "smallestFirst", "largestFirst", "oldestFirst", "newestFirst"}
var pullOrderLabels = []string{"Random", "Alphabetic", "Smallest First", "Largest First", "Oldest First", "Newest First"}

// Compression options for the selector.
var compressionOptions = []string{"metadata", "always", "never"}
var compressionLabels = []string{"Metadata Only", "All Data", "Off"}

// Boolean toggle options.
var boolOptions = []string{"false", "true"}
var boolLabels = []string{"No", "Yes"}

func indexOfString(options []string, val string) int {
	for i, o := range options {
		if o == val {
			return i
		}
	}
	return 0
}

func newEditFolderForm(f *FolderState, cfg config.FolderConfiguration) formModel {
	labelInput := textinput.New()
	labelInput.Placeholder = "My Folder"
	labelInput.SetValue(cfg.Label)
	labelInput.Focus()

	rescanInput := textinput.New()
	rescanInput.Placeholder = "3600"
	rescanInput.SetValue(fmt.Sprintf("%d", cfg.RescanIntervalS))

	return formModel{
		kind:     formEditFolder,
		title:    "Edit Folder: " + f.DisplayName(),
		inputs:   []textinput.Model{labelInput, rescanInput},
		folderID: cfg.ID,
		selectors: []formSelector{
			{
				label:    "Folder Type",
				options:  folderTypeOptions,
				labels:   folderTypeLabels,
				selected: folderTypeIndex(cfg.Type.String()),
			},
			{
				label:    "File Pull Order",
				options:  pullOrderOptions,
				labels:   pullOrderLabels,
				selected: indexOfString(pullOrderOptions, cfg.Order.String()),
			},
			{
				label:    "FS Watcher",
				options:  boolOptions,
				labels:   boolLabels,
				selected: boolToInt(cfg.FSWatcherEnabled),
			},
			{
				label:    "Paused",
				options:  boolOptions,
				labels:   boolLabels,
				selected: boolToInt(cfg.Paused),
			},
		},
	}
}

func newEditDeviceForm(d *DeviceState, cfg config.DeviceConfiguration) formModel {
	nameInput := textinput.New()
	nameInput.Placeholder = "My Device"
	nameInput.SetValue(cfg.Name)
	nameInput.Focus()

	addrInput := textinput.New()
	addrInput.Placeholder = "dynamic"
	addrInput.SetValue(strings.Join(cfg.Addresses, ", "))

	compText, _ := cfg.Compression.MarshalText()

	return formModel{
		kind:     formEditDevice,
		title:    "Edit Device: " + d.DisplayName(),
		inputs:   []textinput.Model{nameInput, addrInput},
		deviceID: cfg.DeviceID.String(),
		selectors: []formSelector{
			{
				label:    "Compression",
				options:  compressionOptions,
				labels:   compressionLabels,
				selected: indexOfString(compressionOptions, string(compText)),
			},
			{
				label:    "Introducer",
				options:  boolOptions,
				labels:   boolLabels,
				selected: boolToInt(cfg.Introducer),
			},
			{
				label:    "Auto Accept Folders",
				options:  boolOptions,
				labels:   boolLabels,
				selected: boolToInt(cfg.AutoAcceptFolders),
			},
			{
				label:    "Paused",
				options:  boolOptions,
				labels:   boolLabels,
				selected: boolToInt(cfg.Paused),
			},
		},
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (f *formModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		// When discovery list is focused, handle its own navigation
		if f.discoveryFocused && len(f.discoveredDevices) > 0 {
			n := len(f.discoveredDevices)
			switch msg.String() {
			case "up", "k":
				f.discoveryCursor = (f.discoveryCursor - 1 + n) % n
				return nil
			case "down", "j":
				f.discoveryCursor = (f.discoveryCursor + 1) % n
				return nil
			case "tab":
				// Switch from discovery list to manual input fields
				f.discoveryFocused = false
				f.focus = 0
				f.updateFocus()
				return nil
			case "shift+tab":
				// Wrap around: from discovery list go to last field
				f.discoveryFocused = false
				f.focus = f.totalFields() - 1
				f.updateFocus()
				return nil
			case "enter":
				// Select device: fill in Device ID (and Name/Addresses for add device form)
				d := f.discoveredDevices[f.discoveryCursor]
				f.inputs[0].SetValue(d.DeviceID)
				if f.kind == formAddDevice {
					if d.Name != "" {
						f.inputs[1].SetValue(d.Name)
					}
					if len(d.Addresses) > 0 {
						f.inputs[2].SetValue(strings.Join(d.Addresses, ", "))
					}
					// Move focus to the Name field so user can review/edit
					f.discoveryFocused = false
					f.focus = 1
					f.updateFocus()
				} else {
					// For share folder form, selecting a device is enough
					// Move focus to the Device ID field (already filled)
					f.discoveryFocused = false
					f.focus = 0
					f.updateFocus()
				}
				return nil
			}
			return nil
		}

		// When checkbox list is focused, handle its own navigation
		if f.checkboxFocused && len(f.folderCheckboxes) > 0 {
			switch msg.String() {
			case "up", "k":
				n := len(f.folderCheckboxes)
				f.checkboxCursor = (f.checkboxCursor - 1 + n) % n
				return nil
			case "down", "j":
				n := len(f.folderCheckboxes)
				f.checkboxCursor = (f.checkboxCursor + 1) % n
				return nil
			case "space":
				f.folderCheckboxes[f.checkboxCursor].Selected = !f.folderCheckboxes[f.checkboxCursor].Selected
				return nil
			case "tab":
				// Go back to first input field
				f.checkboxFocused = false
				if len(f.discoveredDevices) > 0 {
					f.discoveryFocused = true
				} else {
					f.focus = 0
					f.updateFocus()
				}
				return nil
			case "shift+tab":
				f.checkboxFocused = false
				f.focus = f.totalFields() - 1
				f.updateFocus()
				return nil
			}
			return nil
		}

		switch msg.String() {
		case "tab", "down":
			f.nextField()
			return nil
		case "shift+tab", "up":
			f.prevField()
			return nil
		case "left":
			if f.folderTypes != nil && f.focus == f.folderTypeFieldIndex() {
				if f.folderTypeIdx > 0 {
					f.folderTypeIdx--
				}
				return nil
			}
			if si := f.focusedSelector(); si >= 0 {
				if f.selectors[si].selected > 0 {
					f.selectors[si].selected--
				}
				return nil
			}
		case "right":
			if f.folderTypes != nil && f.focus == f.folderTypeFieldIndex() {
				if f.folderTypeIdx < len(f.folderTypes)-1 {
					f.folderTypeIdx++
				}
				return nil
			}
			if si := f.focusedSelector(); si >= 0 {
				if f.selectors[si].selected < len(f.selectors[si].options)-1 {
					f.selectors[si].selected++
				}
				return nil
			}
		}
	}

	// Update the focused input
	if !f.discoveryFocused && f.focus < len(f.inputs) {
		var cmd tea.Cmd
		f.inputs[f.focus], cmd = f.inputs[f.focus].Update(msg)
		return cmd
	}
	return nil
}

func (f *formModel) folderTypeFieldIndex() int {
	return len(f.inputs) // the type selector is after all text inputs
}

func (f *formModel) totalFields() int {
	n := len(f.inputs)
	if f.folderTypes != nil {
		n++ // type selector
	}
	n += len(f.selectors)
	return n
}

// selectorFieldIndex returns the focus index for the i-th selector.
func (f *formModel) selectorFieldIndex(i int) int {
	base := len(f.inputs)
	if f.folderTypes != nil {
		base++
	}
	return base + i
}

// focusedSelector returns the selector index if a selector is focused, or -1.
func (f *formModel) focusedSelector() int {
	base := len(f.inputs)
	if f.folderTypes != nil {
		base++
	}
	idx := f.focus - base
	if idx >= 0 && idx < len(f.selectors) {
		return idx
	}
	return -1
}

func (f *formModel) nextField() {
	next := f.focus + 1
	if next >= f.totalFields() {
		// Past the last field: go to checkboxes if available, then discovery, then wrap
		if len(f.folderCheckboxes) > 0 {
			f.checkboxFocused = true
			f.blurAll()
			return
		}
		if len(f.discoveredDevices) > 0 {
			f.discoveryFocused = true
			f.blurAll()
			return
		}
		next = 0
	}
	f.focus = next
	f.updateFocus()
}

func (f *formModel) prevField() {
	if f.focus == 0 && len(f.discoveredDevices) > 0 {
		// Wrap to discovery list
		f.discoveryFocused = true
		f.blurAll()
		return
	}
	f.focus--
	if f.focus < 0 {
		f.focus = f.totalFields() - 1
	}
	f.updateFocus()
}

func (f *formModel) blurAll() {
	for i := range f.inputs {
		f.inputs[i].Blur()
	}
}

func (f *formModel) updateFocus() {
	for i := range f.inputs {
		if i == f.focus {
			f.inputs[i].Focus()
		} else {
			f.inputs[i].Blur()
		}
	}
}

func (f *formModel) View(styles Styles) string {
	var b strings.Builder

	b.WriteString(styles.Title.Render(f.title))
	b.WriteString("\n\n")

	// Read-only context for edit forms
	if f.kind == formEditFolder && f.folderID != "" {
		b.WriteString(fmt.Sprintf("  %s  %s\n\n",
			styles.FormLabel.Render("Folder ID"),
			styles.Muted.Render(f.folderID+" (read-only)"),
		))
	}
	if f.kind == formEditDevice && f.deviceID != "" {
		b.WriteString(fmt.Sprintf("  %s  %s\n\n",
			styles.FormLabel.Render("Device ID"),
			styles.Muted.Render(shortDeviceID(f.deviceID)+" (read-only)"),
		))
	}

	// Show device list for Add Device and Share Folder forms
	if (f.kind == formAddDevice || f.kind == formShareFolder) && len(f.discoveredDevices) > 0 {
		listLabel := "Nearby Devices"
		if f.kind == formShareFolder {
			listLabel = "Available Devices"
		}
		b.WriteString(fmt.Sprintf("  %s\n", styles.FormLabel.Render(listLabel)))
		for i, d := range f.discoveredDevices {
			label := shortDeviceID(d.DeviceID)
			if d.Name != "" {
				label = d.Name + " (" + shortDeviceID(d.DeviceID) + ")"
			}
			var line string
			if f.kind == formAddDevice {
				addrStr := ""
				if len(d.Addresses) > 0 {
					addrStr = " (" + strings.Join(d.Addresses, ", ") + ")"
				}
				line = fmt.Sprintf("    %s%s", label, addrStr)
			} else {
				line = fmt.Sprintf("    %s", label)
			}
			if f.discoveryFocused && i == f.discoveryCursor {
				b.WriteString(styles.Selected.Render("> " + line))
			} else {
				b.WriteString("  " + line)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  %s\n", styles.Muted.Render("Or enter Device ID manually:")))
		b.WriteString("\n")
	}

	labels := f.fieldLabels()
	for i, label := range labels {
		b.WriteString(fmt.Sprintf("  %s\n", styles.FormLabel.Render(label)))
		if i < len(f.inputs) {
			b.WriteString(fmt.Sprintf("  %s\n\n", f.inputs[i].View()))
		}
	}

	// Folder type selector (for add/accept folder forms)
	if f.folderTypes != nil {
		isFocused := f.focus == f.folderTypeFieldIndex()
		b.WriteString(fmt.Sprintf("  %s\n", styles.FormLabel.Render("Type")))
		if isFocused {
			label := folderTypeLabels[f.folderTypeIdx]
			b.WriteString(fmt.Sprintf("  < %s >\n\n", styles.Selected.Render(label)))
		} else {
			label := folderTypeLabels[f.folderTypeIdx]
			b.WriteString(fmt.Sprintf("  < %s >\n\n", label))
		}
	}

	// Additional selectors (for edit forms)
	for i, sel := range f.selectors {
		isFocused := f.focus == f.selectorFieldIndex(i)
		b.WriteString(fmt.Sprintf("  %s\n", styles.FormLabel.Render(sel.label)))
		label := sel.labels[sel.selected]
		if isFocused {
			b.WriteString(fmt.Sprintf("  < %s >\n\n", styles.Selected.Render(label)))
		} else {
			b.WriteString(fmt.Sprintf("  < %s >\n\n", label))
		}
	}

	// Folder sharing checkboxes
	if len(f.folderCheckboxes) > 0 {
		b.WriteString(fmt.Sprintf("  %s\n", styles.FormLabel.Render("Share Folders")))
		b.WriteString(fmt.Sprintf("  %s\n", styles.Muted.Render("space: toggle  j/k: navigate")))
		for i, cb := range f.folderCheckboxes {
			check := "[ ]"
			if cb.Selected {
				check = "[x]"
			}
			line := fmt.Sprintf("  %s %s", check, cb.Label)
			if f.checkboxFocused && i == f.checkboxCursor {
				b.WriteString(styles.Selected.Render(line))
			} else {
				b.WriteString(line)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if f.err != "" {
		b.WriteString("  " + styles.StateError.Render(f.err) + "\n\n")
	}

	if f.kind == formEditFolder || f.kind == formEditDevice {
		b.WriteString(styles.Muted.Render("  enter: save  tab: next field  esc: cancel"))
	} else {
		b.WriteString(styles.Muted.Render("  enter: submit  tab: next field  esc: cancel"))
	}

	return b.String()
}

func (f *formModel) fieldLabels() []string {
	switch f.kind {
	case formAddFolder:
		return []string{"Folder ID", "Label", "Path"}
	case formAcceptFolder:
		return []string{"Folder ID", "Label", "Path"}
	case formAddDevice:
		return []string{"Device ID", "Name", "Addresses"}
	case formShareFolder:
		return []string{"Device ID"}
	case formEditFolder:
		return []string{"Label", "Rescan Interval (seconds)"}
	case formEditDevice:
		return []string{"Name", "Addresses"}
	default:
		return nil
	}
}

func (f *formModel) values() map[string]string {
	labels := f.fieldLabels()
	vals := make(map[string]string, len(labels)+len(f.selectors)+1)
	for i, label := range labels {
		if i < len(f.inputs) {
			vals[label] = f.inputs[i].Value()
		}
	}
	if f.folderTypes != nil {
		vals["Type"] = f.folderTypes[f.folderTypeIdx]
	}
	for _, sel := range f.selectors {
		vals[sel.label] = sel.options[sel.selected]
	}
	return vals
}

// selectedFolderIDs returns the IDs of folders checked in the sharing section.
func (f *formModel) selectedFolderIDs() []string {
	var ids []string
	for _, cb := range f.folderCheckboxes {
		if cb.Selected {
			ids = append(ids, cb.FolderID)
		}
	}
	return ids
}
