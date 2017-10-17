// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
	"os"
	"sync/atomic"

	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/util"
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

// A wrapper around a Configuration that manages loads, saves and published
// notifications of changes to registered Handlers

type Wrapper struct {
	cfg  Configuration
	path string

	deviceMap map[protocol.DeviceID]DeviceConfiguration
	folderMap map[string]FolderConfiguration
	replaces  chan Configuration
	subs      []Committer
	mut       sync.Mutex

	requiresRestart uint32 // an atomic bool
}

// Wrap wraps an existing Configuration structure and ties it to a file on
// disk.
func Wrap(path string, cfg Configuration) *Wrapper {
	w := &Wrapper{
		cfg:  cfg,
		path: path,
		mut:  sync.NewMutex(),
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
	w.mut.Lock()
	w.subs = append(w.subs, c)
	w.mut.Unlock()
}

// Unsubscribe de-registers the given handler from any future calls to
// configuration changes
func (w *Wrapper) Unsubscribe(c Committer) {
	w.mut.Lock()
	for i := range w.subs {
		if w.subs[i] == c {
			copy(w.subs[i:], w.subs[i+1:])
			w.subs[len(w.subs)-1] = nil
			w.subs = w.subs[:len(w.subs)-1]
			break
		}
	}
	w.mut.Unlock()
}

// RawCopy returns a copy of the currently wrapped Configuration object.
func (w *Wrapper) RawCopy() Configuration {
	w.mut.Lock()
	defer w.mut.Unlock()
	return w.cfg.Copy()
}

// ReplaceBlocking swaps the current configuration object for the given one,
// and waits for subscribers to be notified.
func (w *Wrapper) ReplaceBlocking(cfg Configuration) error {
	w.mut.Lock()
	wg := sync.NewWaitGroup()
	err := w.replaceLocked(cfg, wg)
	w.mut.Unlock()
	wg.Wait()
	return err
}

// Replace swaps the current configuration object for the given one.
func (w *Wrapper) Replace(cfg Configuration) error {
	w.mut.Lock()
	defer w.mut.Unlock()

	return w.replaceLocked(cfg, nil)
}

func (w *Wrapper) replaceLocked(to Configuration, wg sync.WaitGroup) error {
	from := w.cfg

	if err := to.clean(); err != nil {
		return err
	}

	for _, sub := range w.subs {
		l.Debugln(sub, "verifying configuration")
		if err := sub.VerifyConfiguration(from, to); err != nil {
			l.Debugln(sub, "rejected config:", err)
			return err
		}
	}

	w.cfg = to
	w.deviceMap = nil
	w.folderMap = nil

	w.notifyListeners(from, to, wg)

	return nil
}

func (w *Wrapper) notifyListeners(from, to Configuration, wg sync.WaitGroup) {
	if wg != nil {
		wg.Add(len(w.subs))
	}
	for _, sub := range w.subs {
		go func(commiter Committer) {
			w.notifyListener(commiter, from.Copy(), to.Copy())
			if wg != nil {
				wg.Done()
			}
		}(sub)
	}
}

func (w *Wrapper) notifyListener(sub Committer, from, to Configuration) {
	l.Debugln(sub, "committing configuration")
	if !sub.CommitConfiguration(from, to) {
		l.Debugln(sub, "requires restart")
		w.setRequiresRestart()
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

// SetDevices adds new devices to the configuration, or overwrites existing
// devices with the same ID.
func (w *Wrapper) SetDevices(devs []DeviceConfiguration) error {
	w.mut.Lock()
	defer w.mut.Unlock()

	newCfg := w.cfg.Copy()
	var replaced bool
	for oldIndex := range devs {
		replaced = false
		for newIndex := range newCfg.Devices {
			if newCfg.Devices[newIndex].DeviceID == devs[oldIndex].DeviceID {
				newCfg.Devices[newIndex] = devs[oldIndex]
				replaced = true
				break
			}
		}
		if !replaced {
			newCfg.Devices = append(newCfg.Devices, devs[oldIndex])
		}
	}

	return w.replaceLocked(newCfg, nil)
}

// SetDevice adds a new device to the configuration, or overwrites an existing
// device with the same ID.
func (w *Wrapper) SetDevice(dev DeviceConfiguration) error {
	return w.SetDevices([]DeviceConfiguration{dev})
}

// RemoveDevice removes the device from the configuration
func (w *Wrapper) RemoveDevice(id protocol.DeviceID) error {
	w.mut.Lock()
	defer w.mut.Unlock()

	newCfg := w.cfg.Copy()
	removed := false
	for i := range newCfg.Devices {
		if newCfg.Devices[i].DeviceID == id {
			newCfg.Devices = append(newCfg.Devices[:i], newCfg.Devices[i+1:]...)
			removed = true
			break
		}
	}
	if !removed {
		return nil
	}

	return w.replaceLocked(newCfg, nil)
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
func (w *Wrapper) SetFolder(fld FolderConfiguration) error {
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

	return w.replaceLocked(newCfg, nil)
}

// Options returns the current options configuration object.
func (w *Wrapper) Options() OptionsConfiguration {
	w.mut.Lock()
	defer w.mut.Unlock()
	return w.cfg.Options
}

// SetOptions replaces the current options configuration object.
func (w *Wrapper) SetOptions(opts OptionsConfiguration) error {
	w.mut.Lock()
	defer w.mut.Unlock()
	newCfg := w.cfg.Copy()
	newCfg.Options = opts
	return w.replaceLocked(newCfg, nil)
}

// GUI returns the current GUI configuration object.
func (w *Wrapper) GUI() GUIConfiguration {
	w.mut.Lock()
	defer w.mut.Unlock()
	return w.cfg.GUI
}

// SetGUI replaces the current GUI configuration object.
func (w *Wrapper) SetGUI(gui GUIConfiguration) error {
	w.mut.Lock()
	defer w.mut.Unlock()
	newCfg := w.cfg.Copy()
	newCfg.GUI = gui
	return w.replaceLocked(newCfg, nil)
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

// IgnoredFolder returns whether or not share attempts for the given
// folder should be silently ignored.
func (w *Wrapper) IgnoredFolder(folder string) bool {
	w.mut.Lock()
	defer w.mut.Unlock()
	for _, nfolder := range w.cfg.IgnoredFolders {
		if folder == nfolder {
			return true
		}
	}
	return false
}

// Device returns the configuration for the given device and an "ok" bool.
func (w *Wrapper) Device(id protocol.DeviceID) (DeviceConfiguration, bool) {
	w.mut.Lock()
	defer w.mut.Unlock()
	for _, device := range w.cfg.Devices {
		if device.DeviceID == id {
			return device, true
		}
	}
	return DeviceConfiguration{}, false
}

// Folder returns the configuration for the given folder and an "ok" bool.
func (w *Wrapper) Folder(id string) (FolderConfiguration, bool) {
	w.mut.Lock()
	defer w.mut.Unlock()
	for _, folder := range w.cfg.Folders {
		if folder.ID == id {
			return folder, true
		}
	}
	return FolderConfiguration{}, false
}

// Save writes the configuration to disk, and generates a ConfigSaved event.
func (w *Wrapper) Save() error {
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

	events.Default.Log(events.ConfigSaved, w.cfg)
	return nil
}

func (w *Wrapper) GlobalDiscoveryServers() []string {
	var servers []string
	for _, srv := range w.cfg.Options.GlobalAnnServers {
		switch srv {
		case "default":
			servers = append(servers, DefaultDiscoveryServers...)
		case "default-v4":
			servers = append(servers, DefaultDiscoveryServersV4...)
		case "default-v6":
			servers = append(servers, DefaultDiscoveryServersV6...)
		default:
			servers = append(servers, srv)
		}
	}
	return util.UniqueStrings(servers)
}

func (w *Wrapper) ListenAddresses() []string {
	var addresses []string
	for _, addr := range w.cfg.Options.ListenAddresses {
		switch addr {
		case "default":
			addresses = append(addresses, DefaultListenAddresses...)
		default:
			addresses = append(addresses, addr)
		}
	}
	return util.UniqueStrings(addresses)
}

func (w *Wrapper) RequiresRestart() bool {
	return atomic.LoadUint32(&w.requiresRestart) != 0
}

func (w *Wrapper) setRequiresRestart() {
	atomic.StoreUint32(&w.requiresRestart, 1)
}

func (w *Wrapper) StunServers() []string {
	var addresses []string
	for _, addr := range w.cfg.Options.StunServers {
		switch addr {
		case "default":
			addresses = append(addresses, DefaultStunServers...)
		default:
			addresses = append(addresses, addr)
		}
	}

	addresses = util.UniqueStrings(addresses)

	// Shuffle
	l := len(addresses)
	for i := range addresses {
		r := rand.Intn(l)
		addresses[i], addresses[r] = addresses[r], addresses[i]
	}

	return addresses
}

func (w *Wrapper) MyName() string {
	w.mut.Lock()
	myID := w.cfg.MyID
	w.mut.Unlock()
	cfg, _ := w.Device(myID)
	return cfg.Name
}
