// Copyright (C) 2014 The Syncthing Authors.
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

package config

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/internal/events"
	"github.com/syncthing/syncthing/internal/osutil"
)

// An interface to handle configuration changes, and a wrapper type á la
// http.Handler

type Handler interface {
	Changed(Configuration) error
}

type HandlerFunc func(Configuration) error

func (fn HandlerFunc) Changed(cfg Configuration) error {
	return fn(cfg)
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

	subs []Handler
	sMut sync.Mutex
}

// Wrap wraps an existing Configuration structure and ties it to a file on
// disk.
func Wrap(path string, cfg Configuration) *Wrapper {
	w := &Wrapper{cfg: cfg, path: path}
	w.replaces = make(chan Configuration)
	go w.Serve()
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

// Serve handles configuration replace events and calls any interested
// handlers. It is started automatically by Wrap() and Load() and should not
// be run manually.
func (w *Wrapper) Serve() {
	for cfg := range w.replaces {
		w.sMut.Lock()
		subs := w.subs
		w.sMut.Unlock()
		for _, h := range subs {
			h.Changed(cfg)
		}
	}
}

// Stop stops the Serve() loop. Set and Replace operations will panic after a
// Stop.
func (w *Wrapper) Stop() {
	close(w.replaces)
}

// Subscribe registers the given handler to be called on any future
// configuration changes.
func (w *Wrapper) Subscribe(h Handler) {
	w.sMut.Lock()
	w.subs = append(w.subs, h)
	w.sMut.Unlock()
}

// Raw returns the currently wrapped Configuration object.
func (w *Wrapper) Raw() Configuration {
	return w.cfg
}

// Replace swaps the current configuration object for the given one.
func (w *Wrapper) Replace(cfg Configuration) {
	w.mut.Lock()
	defer w.mut.Unlock()

	w.cfg = cfg
	w.deviceMap = nil
	w.folderMap = nil
	w.replaces <- cfg
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
func (w *Wrapper) SetDevice(dev DeviceConfiguration) {
	w.mut.Lock()
	defer w.mut.Unlock()

	w.deviceMap = nil

	for i := range w.cfg.Devices {
		if w.cfg.Devices[i].DeviceID == dev.DeviceID {
			w.cfg.Devices[i] = dev
			w.replaces <- w.cfg
			return
		}
	}

	w.cfg.Devices = append(w.cfg.Devices, dev)
	w.replaces <- w.cfg
}

// Devices returns a map of folders. Folder structures should not be changed,
// other than for the purpose of updating via SetFolder().
func (w *Wrapper) Folders() map[string]FolderConfiguration {
	w.mut.Lock()
	defer w.mut.Unlock()
	if w.folderMap == nil {
		w.folderMap = make(map[string]FolderConfiguration, len(w.cfg.Folders))
		for _, fld := range w.cfg.Folders {
			path, err := osutil.ExpandTilde(fld.Path)
			if err != nil {
				l.Warnln("home:", err)
				continue
			}
			fld.Path = path
			w.folderMap[fld.ID] = fld
		}
	}
	return w.folderMap
}

// SetFolder adds a new folder to the configuration, or overwrites an existing
// folder with the same ID.
func (w *Wrapper) SetFolder(fld FolderConfiguration) {
	w.mut.Lock()
	defer w.mut.Unlock()

	w.folderMap = nil

	for i := range w.cfg.Folders {
		if w.cfg.Folders[i].ID == fld.ID {
			w.cfg.Folders[i] = fld
			w.replaces <- w.cfg
			return
		}
	}

	w.cfg.Folders = append(w.cfg.Folders, fld)
	w.replaces <- w.cfg
}

// Options returns the current options configuration object.
func (w *Wrapper) Options() OptionsConfiguration {
	w.mut.Lock()
	defer w.mut.Unlock()
	return w.cfg.Options
}

// SetOptions replaces the current options configuration object.
func (w *Wrapper) SetOptions(opts OptionsConfiguration) {
	w.mut.Lock()
	defer w.mut.Unlock()
	w.cfg.Options = opts
	w.replaces <- w.cfg
}

// GUI returns the current GUI configuration object.
func (w *Wrapper) GUI() GUIConfiguration {
	w.mut.Lock()
	defer w.mut.Unlock()
	return w.cfg.GUI
}

// SetGUI replaces the current GUI configuration object.
func (w *Wrapper) SetGUI(gui GUIConfiguration) {
	w.mut.Lock()
	defer w.mut.Unlock()
	w.cfg.GUI = gui
	w.replaces <- w.cfg
}

// InvalidateFolder sets the invalid marker on the given folder.
func (w *Wrapper) InvalidateFolder(id string, err string) {
	w.mut.Lock()
	defer w.mut.Unlock()

	w.folderMap = nil

	for i := range w.cfg.Folders {
		if w.cfg.Folders[i].ID == id {
			w.cfg.Folders[i].Invalid = err
			w.replaces <- w.cfg
			return
		}
	}
}

// Returns whether or not connection attempts from the given device should be
// silently ignored.
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
	fd, err := ioutil.TempFile(filepath.Dir(w.path), "cfg")
	if err != nil {
		return err
	}
	defer os.Remove(fd.Name())

	err = w.cfg.WriteXML(fd)
	if err != nil {
		fd.Close()
		return err
	}

	err = fd.Close()
	if err != nil {
		return err
	}

	events.Default.Log(events.ConfigSaved, w.cfg)

	return osutil.Rename(fd.Name(), w.path)
}
