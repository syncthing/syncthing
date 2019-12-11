// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
	"os"
	"sync/atomic"
	"time"

	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
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
// false will result in a "restart needed" response to the API/user. Note that
// the new configuration will still have been applied by those who were
// capable of doing so.
type Committer interface {
	VerifyConfiguration(from, to Configuration) error
	CommitConfiguration(from, to Configuration) (handled bool)
	String() string
}

// Waiter allows to wait for the given config operation to complete.
type Waiter interface {
	Wait()
}

type noopWaiter struct{}

func (noopWaiter) Wait() {}

// A Wrapper around a Configuration that manages loads, saves and published
// notifications of changes to registered Handlers
type Wrapper interface {
	ConfigPath() string
	RequiresRestart() bool
	Save() error

	Replace(cfg Configuration) (Waiter, error)
	ReplaceFoldersAndDevices(folders []FolderConfiguration, devices []DeviceConfiguration) (Waiter, error)

	SetGUI(gui GUIConfiguration) (Waiter, error)

	SetOptions(opts OptionsConfiguration) (Waiter, error)

	SetFolder(fld FolderConfiguration) (Waiter, error)
	SetFolders(folders []FolderConfiguration) (Waiter, error)

	RemoveDevice(id protocol.DeviceID) (Waiter, error)
	SetDevice(DeviceConfiguration) (Waiter, error)
	SetDevices([]DeviceConfiguration) (Waiter, error)

	AddOrUpdatePendingDevice(device protocol.DeviceID, name, address string)
	AddOrUpdatePendingFolder(id, label string, device protocol.DeviceID)

	Subscribe(c Committer) Configuration
	Unsubscribe(c Committer)
}

type wrapper struct {
	cfg      Configuration
	path     string
	evLogger events.Logger

	waiter Waiter // Latest ongoing config change
	subs   []Committer
	mut    sync.Mutex

	requiresRestart uint32 // an atomic bool
}

// Wrap wraps an existing Configuration structure and ties it to a file on
// disk.
func Wrap(path string, cfg Configuration, evLogger events.Logger) Wrapper {
	w := &wrapper{
		cfg:      cfg,
		path:     path,
		evLogger: evLogger,
		waiter:   noopWaiter{}, // Noop until first config change
		mut:      sync.NewMutex(),
	}
	return w
}

// Load loads an existing file on disk and returns a new configuration
// wrapper.
func Load(path string, myID protocol.DeviceID, evLogger events.Logger) (Wrapper, Configuration, error) {
	fd, err := os.Open(path)
	if err != nil {
		return nil, Configuration{}, err
	}
	defer fd.Close()

	cfg, err := ReadXML(fd, myID)
	if err != nil {
		return nil, Configuration{}, err
	}

	return Wrap(path, cfg, evLogger), cfg.Copy(), nil
}

func (w *wrapper) ConfigPath() string {
	return w.path
}

// Subscribe registers the given handler to be called on any future
// configuration changes.
func (w *wrapper) Subscribe(c Committer) Configuration {
	w.mut.Lock()
	defer w.mut.Unlock()
	w.subs = append(w.subs, c)
	return w.cfg.Copy()
}

// Unsubscribe de-registers the given handler from any future calls to
// configuration changes and only returns after a potential ongoing config
// change is done.
func (w *wrapper) Unsubscribe(c Committer) {
	w.mut.Lock()
	for i := range w.subs {
		if w.subs[i] == c {
			copy(w.subs[i:], w.subs[i+1:])
			w.subs[len(w.subs)-1] = nil
			w.subs = w.subs[:len(w.subs)-1]
			break
		}
	}
	waiter := w.waiter
	w.mut.Unlock()
	// Waiting mustn't be done under lock, as the goroutines in notifyListener
	// may dead-lock when trying to access lock on config read operations.
	waiter.Wait()
}

// RawCopy returns a copy of the currently wrapped Configuration object.
func (w *wrapper) RawCopy() Configuration {
	w.mut.Lock()
	defer w.mut.Unlock()
	return w.cfg.Copy()
}

// Replace swaps the current configuration object for the given one.
func (w *wrapper) Replace(cfg Configuration) (Waiter, error) {
	w.mut.Lock()
	defer w.mut.Unlock()
	return w.replaceLocked(cfg.Copy())
}

func (w *wrapper) replaceLocked(to Configuration) (Waiter, error) {
	from := w.cfg

	if err := to.clean(); err != nil {
		return noopWaiter{}, err
	}

	to.populateMaps()

	for _, sub := range w.subs {
		l.Debugln(sub, "verifying configuration")
		if err := sub.VerifyConfiguration(from.Copy(), to.Copy()); err != nil {
			l.Debugln(sub, "rejected config:", err)
			return noopWaiter{}, err
		}
	}

	w.cfg = to

	w.waiter = w.notifyListeners(from, to)

	return w.waiter, nil
}

func (w *wrapper) notifyListeners(from, to Configuration) Waiter {
	wg := sync.NewWaitGroup()
	wg.Add(len(w.subs))
	for _, sub := range w.subs {
		go func(commiter Committer) {
			w.notifyListener(commiter, from.Copy(), to.Copy())
			wg.Done()
		}(sub)
	}
	return wg
}

func (w *wrapper) notifyListener(sub Committer, from, to Configuration) {
	l.Debugln(sub, "committing configuration")
	if !sub.CommitConfiguration(from, to) {
		l.Debugln(sub, "requires restart")
		w.setRequiresRestart()
	}
}

// SetDevices adds new devices to the configuration, or overwrites existing
// devices with the same ID.
func (w *wrapper) SetDevices(devs []DeviceConfiguration) (Waiter, error) {
	w.mut.Lock()
	defer w.mut.Unlock()

	newCfg := w.cfg.Copy()
	var replaced bool
	for oldIndex := range devs {
		replaced = false
		for newIndex := range newCfg.Devices {
			if newCfg.Devices[newIndex].DeviceID == devs[oldIndex].DeviceID {
				newCfg.Devices[newIndex] = devs[oldIndex].Copy()
				replaced = true
				break
			}
		}
		if !replaced {
			newCfg.Devices = append(newCfg.Devices, devs[oldIndex].Copy())
		}
	}

	return w.replaceLocked(newCfg)
}

// SetDevice adds a new device to the configuration, or overwrites an existing
// device with the same ID.
func (w *wrapper) SetDevice(dev DeviceConfiguration) (Waiter, error) {
	return w.SetDevices([]DeviceConfiguration{dev})
}

// RemoveDevice removes the device from the configuration
func (w *wrapper) RemoveDevice(id protocol.DeviceID) (Waiter, error) {
	w.mut.Lock()
	defer w.mut.Unlock()

	newCfg := w.cfg.Copy()
	for i := range newCfg.Devices {
		if newCfg.Devices[i].DeviceID == id {
			newCfg.Devices = append(newCfg.Devices[:i], newCfg.Devices[i+1:]...)
			return w.replaceLocked(newCfg)
		}
	}

	return noopWaiter{}, nil
}

// SetFolder adds a new folder to the configuration, or overwrites an existing
// folder with the same ID.
func (w *wrapper) SetFolder(fld FolderConfiguration) (Waiter, error) {
	return w.SetFolders([]FolderConfiguration{fld})
}

// SetFolders adds new folders to the configuration, or overwrites existing
// folders with the same ID.
func (w *wrapper) SetFolders(folders []FolderConfiguration) (Waiter, error) {
	w.mut.Lock()
	defer w.mut.Unlock()

	newCfg := w.cfg.Copy()
	var replaced bool
	for oldIndex := range folders {
		replaced = false
		for newIndex := range newCfg.Folders {
			if newCfg.Folders[newIndex].ID == folders[oldIndex].ID {
				newCfg.Folders[newIndex] = folders[oldIndex].Copy()
				replaced = true
				break
			}
		}
		if !replaced {
			newCfg.Folders = append(newCfg.Folders, folders[oldIndex].Copy())
		}
	}

	return w.replaceLocked(newCfg)
}

// ReplaceFoldersAndDevices removes all preexisitng folders and devices and
// replaces them with the ones given as parameters.
func (w *wrapper) ReplaceFoldersAndDevices(folders []FolderConfiguration, devices []DeviceConfiguration) (Waiter, error) {
	w.mut.Lock()
	defer w.mut.Unlock()

	newCfg := w.cfg.Copy()

	newCfg.Folders = folders
	newCfg.Devices = devices

	return w.replaceLocked(newCfg)
}

// SetOptions replaces the current options configuration object.
func (w *wrapper) SetOptions(opts OptionsConfiguration) (Waiter, error) {
	w.mut.Lock()
	defer w.mut.Unlock()
	newCfg := w.cfg.Copy()
	newCfg.Options = opts.Copy()
	return w.replaceLocked(newCfg)
}

// SetGUI replaces the current GUI configuration object.
func (w *wrapper) SetGUI(gui GUIConfiguration) (Waiter, error) {
	w.mut.Lock()
	defer w.mut.Unlock()
	newCfg := w.cfg.Copy()
	newCfg.GUI = gui.Copy()
	return w.replaceLocked(newCfg)
}

// Save writes the configuration to disk, and generates a ConfigSaved event.
func (w *wrapper) Save() error {
	w.mut.Lock()
	defer w.mut.Unlock()

	fd, err := osutil.CreateAtomic(w.path)
	if err != nil {
		l.Debugln("CreateAtomic:", err)
		return err
	}

	if err := w.cfg.WriteXML(fd); err != nil {
		l.Debugln("WriteXML:", err)
		fd.Close()
		return err
	}

	if err := fd.Close(); err != nil {
		l.Debugln("Close:", err)
		return err
	}

	w.evLogger.Log(events.ConfigSaved, w.cfg)
	return nil
}

func (w *wrapper) RequiresRestart() bool {
	return atomic.LoadUint32(&w.requiresRestart) != 0
}

func (w *wrapper) setRequiresRestart() {
	atomic.StoreUint32(&w.requiresRestart, 1)
}

func (w *wrapper) AddOrUpdatePendingDevice(device protocol.DeviceID, name, address string) {
	w.mut.Lock()
	defer w.mut.Unlock()

	for i := range w.cfg.PendingDevices {
		if w.cfg.PendingDevices[i].ID == device {
			w.cfg.PendingDevices[i].Time = time.Now().Round(time.Second)
			w.cfg.PendingDevices[i].Name = name
			w.cfg.PendingDevices[i].Address = address
			return
		}
	}

	w.cfg.PendingDevices = append(w.cfg.PendingDevices, ObservedDevice{
		Time:    time.Now().Round(time.Second),
		ID:      device,
		Name:    name,
		Address: address,
	})
}

func (w *wrapper) AddOrUpdatePendingFolder(id, label string, device protocol.DeviceID) {
	w.mut.Lock()
	defer w.mut.Unlock()

	for i := range w.cfg.Devices {
		if w.cfg.Devices[i].DeviceID == device {
			for j := range w.cfg.Devices[i].PendingFolders {
				if w.cfg.Devices[i].PendingFolders[j].ID == id {
					w.cfg.Devices[i].PendingFolders[j].Label = label
					w.cfg.Devices[i].PendingFolders[j].Time = time.Now().Round(time.Second)
					return
				}
			}
			w.cfg.Devices[i].PendingFolders = append(w.cfg.Devices[i].PendingFolders, ObservedFolder{
				Time:  time.Now().Round(time.Second),
				ID:    id,
				Label: label,
			})
			return
		}
	}

	panic("bug: adding pending folder for non-existing device")
}
