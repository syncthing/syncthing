// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package config

import (
	"os"

	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/sync"
)

// The Committer interface is implemented by objects that need to know about
// or have a say in configuration changes.
//
// When the configuration is about to be changed, VerifyConfiguration() is
// called for each subscribing object, with the old and new configuration. A
// nil error is returned if the new configuration is acceptable (i.e. does not
// contain any errors that would prevent it from being a valid config).
// Otherwise an error describing the problem is returned.
//
// If any subscriber returns an error from VerifyConfiguration(), the
// configuration change is not committed and an error is returned to whoever
// tried to commit the broken config.
//
// If all verification calls returns nil, CommitConfiguration() is called for
// each subscribing object. The callee returns true if the new configuration
// has been successfully applied, otherwise false. Any Commit() call returning
// false will result in a "restart needed" respone to the API/user. Note that
// the new configuration will still have been applied by those who were
// capable of doing so.
type Committer interface {
	VerifyConfiguration(from, to Configuration) error
	CommitConfiguration(from, to Configuration) (handled bool)
	String() string
}

type CommitResponse struct {
	ValidationError error
	RequiresRestart bool
}

var ResponseNoRestart = CommitResponse{
	ValidationError: nil,
	RequiresRestart: false,
}

// A wrapper around a Configuration that manages loads, saves and published
// notifications of changes to registered Handlers

type Wrapper struct {
	cfg  Configuration
	path string

	deviceMap map[protocol.DeviceID]DeviceConfiguration
	folderMap map[string]FolderConfiguration
	replaces  chan Configuration
	mut       sync.Mutex

	subs []Committer
	sMut sync.Mutex
}

// Wrap wraps an existing Configuration structure and ties it to a file on
// disk.
func Wrap(path string, cfg Configuration) *Wrapper {
	w := &Wrapper{
		cfg:  cfg,
		path: path,
		mut:  sync.NewMutex(),
		sMut: sync.NewMutex(),
	}
	w.replaces = make(chan Configuration)
	return w
}

// Load loads an existing file on disk and returns a new configuration
// wrapper.
func Load(path string, myID protocol.DeviceID) (*Wrapper, error) {
	fd, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	cfg, err := ReadXML(fd, myID)
	if err != nil {
		return nil, err
	}

	return Wrap(path, cfg), nil
}

func (w *Wrapper) ConfigPath() string {
	return w.path
}

// Stop stops the Serve() loop. Set and Replace operations will panic after a
// Stop.
func (w *Wrapper) Stop() {
	close(w.replaces)
}

// Subscribe registers the given handler to be called on any future
// configuration changes.
func (w *Wrapper) Subscribe(c Committer) {
	w.sMut.Lock()
	w.subs = append(w.subs, c)
	w.sMut.Unlock()
}

// Unsubscribe de-registers the given handler from any future calls to
// configuration changes
func (w *Wrapper) Unsubscribe(c Committer) {
	w.sMut.Lock()
	for i := range w.subs {
		if w.subs[i] == c {
			copy(w.subs[i:], w.subs[i+1:])
			w.subs[len(w.subs)-1] = nil
			w.subs = w.subs[:len(w.subs)-1]
			break
		}
	}
	w.sMut.Unlock()
}

// Raw returns the currently wrapped Configuration object.
func (w *Wrapper) Raw() Configuration {
	return w.cfg
}

// Replace swaps the current configuration object for the given one.
func (w *Wrapper) Replace(cfg Configuration) CommitResponse {
	w.mut.Lock()
	defer w.mut.Unlock()
	return w.replaceLocked(cfg)
}

func (w *Wrapper) replaceLocked(to Configuration) CommitResponse {
	from := w.cfg

	for _, sub := range w.subs {
		if debug {
			l.Debugln(sub, "verifying configuration")
		}
		if err := sub.VerifyConfiguration(from, to); err != nil {
			if debug {
				l.Debugln(sub, "rejected config:", err)
			}
			return CommitResponse{
				ValidationError: err,
			}
		}
	}

	allOk := true
	for _, sub := range w.subs {
		if debug {
			l.Debugln(sub, "committing configuration")
		}
		ok := sub.CommitConfiguration(from, to)
		if !ok {
			if debug {
				l.Debugln(sub, "requires restart")
			}
			allOk = false
		}
	}

	w.cfg = to
	w.deviceMap = nil
	w.folderMap = nil

	return CommitResponse{
		RequiresRestart: !allOk,
	}
}

// Devices returns a map of devices. Device structures should not be changed,
// other than for the purpose of updating via SetDevice().
func (w *Wrapper) Devices() map[protocol.DeviceID]DeviceConfiguration {
	w.mut.Lock()
	defer w.mut.Unlock()
	if w.deviceMap == nil {
		w.deviceMap = make(map[protocol.DeviceID]DeviceConfiguration, len(w.cfg.Devices))
		for _, dev := range w.cfg.Devices {
			w.deviceMap[dev.DeviceID] = dev
		}
	}
	return w.deviceMap
}

// SetDevice adds a new device to the configuration, or overwrites an existing
// device with the same ID.
func (w *Wrapper) SetDevice(dev DeviceConfiguration) CommitResponse {
	w.mut.Lock()
	defer w.mut.Unlock()

	newCfg := w.cfg.Copy()
	replaced := false
	for i := range newCfg.Devices {
		if newCfg.Devices[i].DeviceID == dev.DeviceID {
			newCfg.Devices[i] = dev
			replaced = true
			break
		}
	}
	if !replaced {
		newCfg.Devices = append(w.cfg.Devices, dev)
	}

	return w.replaceLocked(newCfg)
}

// Folders returns a map of folders. Folder structures should not be changed,
// other than for the purpose of updating via SetFolder().
func (w *Wrapper) Folders() map[string]FolderConfiguration {
	w.mut.Lock()
	defer w.mut.Unlock()
	if w.folderMap == nil {
		w.folderMap = make(map[string]FolderConfiguration, len(w.cfg.Folders))
		for _, fld := range w.cfg.Folders {
			w.folderMap[fld.ID] = fld
		}
	}
	return w.folderMap
}

// SetFolder adds a new folder to the configuration, or overwrites an existing
// folder with the same ID.
func (w *Wrapper) SetFolder(fld FolderConfiguration) CommitResponse {
	w.mut.Lock()
	defer w.mut.Unlock()

	newCfg := w.cfg.Copy()
	replaced := false
	for i := range newCfg.Folders {
		if newCfg.Folders[i].ID == fld.ID {
			newCfg.Folders[i] = fld
			replaced = true
			break
		}
	}
	if !replaced {
		newCfg.Folders = append(w.cfg.Folders, fld)
	}

	return w.replaceLocked(newCfg)
}

// Options returns the current options configuration object.
func (w *Wrapper) Options() OptionsConfiguration {
	w.mut.Lock()
	defer w.mut.Unlock()
	return w.cfg.Options
}

// SetOptions replaces the current options configuration object.
func (w *Wrapper) SetOptions(opts OptionsConfiguration) CommitResponse {
	w.mut.Lock()
	defer w.mut.Unlock()
	newCfg := w.cfg.Copy()
	newCfg.Options = opts
	return w.replaceLocked(newCfg)
}

// GUI returns the current GUI configuration object.
func (w *Wrapper) GUI() GUIConfiguration {
	w.mut.Lock()
	defer w.mut.Unlock()
	return w.cfg.GUI
}

// SetGUI replaces the current GUI configuration object.
func (w *Wrapper) SetGUI(gui GUIConfiguration) CommitResponse {
	w.mut.Lock()
	defer w.mut.Unlock()
	newCfg := w.cfg.Copy()
	newCfg.GUI = gui
	return w.replaceLocked(newCfg)
}

// IgnoredDevice returns whether or not connection attempts from the given
// device should be silently ignored.
func (w *Wrapper) IgnoredDevice(id protocol.DeviceID) bool {
	w.mut.Lock()
	defer w.mut.Unlock()
	for _, device := range w.cfg.IgnoredDevices {
		if device == id {
			return true
		}
	}
	return false
}

// Save writes the configuration to disk, and generates a ConfigSaved event.
func (w *Wrapper) Save() error {
	fd, err := osutil.CreateAtomic(w.path, 0600)
	if err != nil {
		return err
	}

	if err := w.cfg.WriteXML(fd); err != nil {
		fd.Close()
		return err
	}

	if err := fd.Close(); err != nil {
		return err
	}

	events.Default.Log(events.ConfigSaved, w.cfg)
	return nil
}
