// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"time"

	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
)

const (
	maxModifications = 1000
	minSaveInterval  = 5 * time.Second
)

var errTooManyModifications = errors.New("too many concurrent config modifications")

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
//
// A Committer must take care not to hold any locks while changing the
// configuration (e.g. calling Wrapper.SetFolder), that are also acquired in any
// methods of the Committer interface.
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

// ModifyFunction gets a pointer to a copy of the currently active configuration
// for modification. When it returns true, the modified config will be committed,
// otherwise the modification will be aborted.
type ModifyFunction func(*Configuration) bool

// Wrapper handles a Configuration, i.e. it provides methods to access, change
// and save the config, and notifies registered subscribers (Committer) of
// changes.
//
// Modify allows changing the currently active configuration through the given
// ModifyFunction. If it returns true, the changed config takes effect,
// otherwise it will be discarded (and modify returns without an error and a
// noop-waiter). It can be called concurrently: All calls will be queued and
// called in order.
type Wrapper interface {
	ConfigPath() string
	MyID() protocol.DeviceID

	RawCopy() Configuration
	RequiresRestart() bool
	Save() error

	Modify(ModifyFunction) (Waiter, error)
	RemoveFolder(id string) (Waiter, error)
	RemoveDevice(id protocol.DeviceID) (Waiter, error)

	GUI() GUIConfiguration
	LDAP() LDAPConfiguration
	Options() OptionsConfiguration

	Folder(id string) (FolderConfiguration, bool)
	Folders() map[string]FolderConfiguration
	FolderList() []FolderConfiguration
	FolderPasswords(device protocol.DeviceID) map[string]string

	Device(id protocol.DeviceID) (DeviceConfiguration, bool)
	Devices() map[protocol.DeviceID]DeviceConfiguration
	DeviceList() []DeviceConfiguration

	IgnoredDevices() []ObservedDevice
	IgnoredDevice(id protocol.DeviceID) bool
	IgnoredFolder(device protocol.DeviceID, folder string) bool

	Subscribe(c Committer) Configuration
	Unsubscribe(c Committer)
}

type wrapper struct {
	cfg      Configuration
	path     string
	evLogger events.Logger
	myID     protocol.DeviceID
	queue    chan modifyEntry

	waiter Waiter // Latest ongoing config change
	subs   []Committer
	mut    sync.Mutex

	requiresRestart uint32 // an atomic bool
}

// Wrap wraps an existing Configuration structure and ties it to a file on
// disk.
// The returned Wrapper is a suture.Service, thus needs to be started (added to
// a supervisor).
func Wrap(path string, cfg Configuration, myID protocol.DeviceID, evLogger events.Logger) Wrapper {
	w := &wrapper{
		cfg:      cfg,
		path:     path,
		evLogger: evLogger,
		myID:     myID,
		queue:    make(chan modifyEntry, maxModifications),
		waiter:   noopWaiter{}, // Noop until first config change
		mut:      sync.NewMutex(),
	}
	return w
}

// Load loads an existing file on disk and returns a new configuration
// wrapper.
// The returned Wrapper is a suture.Service, thus needs to be started (added to
// a supervisor).
func Load(path string, myID protocol.DeviceID, evLogger events.Logger) (Wrapper, int, error) {
	fd, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer fd.Close()

	cfg, originalVersion, err := ReadXML(fd, myID)
	if err != nil {
		return nil, 0, err
	}

	return Wrap(path, cfg, myID, evLogger), originalVersion, nil
}

func (w *wrapper) ConfigPath() string {
	return w.path
}

func (w *wrapper) MyID() protocol.DeviceID {
	return w.myID
}

// Subscribe registers the given handler to be called on any future
// configuration changes. It returns the config that is in effect while
// subscribing, that can be used for initial setup.
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

func (w *wrapper) Modify(fn ModifyFunction) (Waiter, error) {
	return w.modifyQueued(fn)
}

func (w *wrapper) modifyQueued(modifyFunc ModifyFunction) (Waiter, error) {
	e := modifyEntry{
		modifyFunc: modifyFunc,
		res:        make(chan modifyResult),
	}
	select {
	case w.queue <- e:
	default:
		return noopWaiter{}, errTooManyModifications
	}
	res := <-e.res
	return res.w, res.err
}

func (w *wrapper) Serve(ctx context.Context) error {
	defer w.serveSave()

	var e modifyEntry
	saveTimer := time.NewTimer(0)
	<-saveTimer.C
	saveTimerRunning := false
	for {
		select {
		case e = <-w.queue:
		case <-saveTimer.C:
			w.serveSave()
			saveTimerRunning = false
			continue
		case <-ctx.Done():
			return ctx.Err()
		}

		var waiter Waiter = noopWaiter{}
		var err error

		w.mut.Lock()
		to := w.cfg.Copy()
		if e.modifyFunc(&to) {
			waiter, err = w.replaceLocked(to)
		}
		w.mut.Unlock()

		e.res <- modifyResult{
			w:   waiter,
			err: err,
		}

		if !saveTimerRunning {
			saveTimer.Reset(minSaveInterval)
			saveTimerRunning = true
		}

		done := make(chan struct{})
		go func() {
			waiter.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (w *wrapper) serveSave() {
	if err := w.Save(); err != nil {
		l.Warnln("Failed to save config:", err)
	}
}

func (w *wrapper) replaceLocked(to Configuration) (Waiter, error) {
	from := w.cfg

	if err := to.prepare(w.myID); err != nil {
		return noopWaiter{}, err
	}

	for _, sub := range w.subs {
		l.Debugln(sub, "verifying configuration")
		if err := sub.VerifyConfiguration(from.Copy(), to.Copy()); err != nil {
			l.Debugln(sub, "rejected config:", err)
			return noopWaiter{}, err
		}
	}

	w.cfg = to

	w.waiter = w.notifyListeners(from.Copy(), to.Copy())

	return w.waiter, nil
}

func (w *wrapper) notifyListeners(from, to Configuration) Waiter {
	wg := sync.NewWaitGroup()
	wg.Add(len(w.subs))
	for _, sub := range w.subs {
		go func(commiter Committer) {
			w.notifyListener(commiter, from, to)
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

// Devices returns a map of devices.
func (w *wrapper) Devices() map[protocol.DeviceID]DeviceConfiguration {
	w.mut.Lock()
	defer w.mut.Unlock()
	deviceMap := make(map[protocol.DeviceID]DeviceConfiguration, len(w.cfg.Devices))
	for _, dev := range w.cfg.Devices {
		deviceMap[dev.DeviceID] = dev.Copy()
	}
	return deviceMap
}

// DeviceList returns a slice of devices.
func (w *wrapper) DeviceList() []DeviceConfiguration {
	w.mut.Lock()
	defer w.mut.Unlock()
	return w.cfg.Copy().Devices
}

// RemoveDevice removes the device from the configuration
func (w *wrapper) RemoveDevice(id protocol.DeviceID) (Waiter, error) {
	return w.modifyQueued(func(cfg *Configuration) bool {
		_, i, ok := cfg.Device(id)
		if !ok {
			return false
		}
		cfg.Devices = append(cfg.Devices[:i], cfg.Devices[i+1:]...)
		return true
	})
}

// Folders returns a map of folders.
func (w *wrapper) Folders() map[string]FolderConfiguration {
	w.mut.Lock()
	defer w.mut.Unlock()
	folderMap := make(map[string]FolderConfiguration, len(w.cfg.Folders))
	for _, fld := range w.cfg.Folders {
		folderMap[fld.ID] = fld.Copy()
	}
	return folderMap
}

// FolderList returns a slice of folders.
func (w *wrapper) FolderList() []FolderConfiguration {
	w.mut.Lock()
	defer w.mut.Unlock()
	return w.cfg.Copy().Folders
}

// RemoveFolder removes the folder from the configuration
func (w *wrapper) RemoveFolder(id string) (Waiter, error) {
	return w.modifyQueued(func(cfg *Configuration) bool {
		_, i, ok := cfg.Folder(id)
		if !ok {
			return false
		}
		cfg.Folders = append(cfg.Folders[:i], cfg.Folders[i+1:]...)
		return true
	})
}

// FolderPasswords returns the folder passwords set for this device, for
// folders that have an encryption password set.
func (w *wrapper) FolderPasswords(device protocol.DeviceID) map[string]string {
	w.mut.Lock()
	defer w.mut.Unlock()
	return w.cfg.FolderPasswords(device)
}

// Options returns the current options configuration object.
func (w *wrapper) Options() OptionsConfiguration {
	w.mut.Lock()
	defer w.mut.Unlock()
	return w.cfg.Options.Copy()
}

func (w *wrapper) LDAP() LDAPConfiguration {
	w.mut.Lock()
	defer w.mut.Unlock()
	return w.cfg.LDAP.Copy()
}

// GUI returns the current GUI configuration object.
func (w *wrapper) GUI() GUIConfiguration {
	w.mut.Lock()
	defer w.mut.Unlock()
	return w.cfg.GUI.Copy()
}

// IgnoredDevice returns whether or not connection attempts from the given
// device should be silently ignored.
func (w *wrapper) IgnoredDevice(id protocol.DeviceID) bool {
	w.mut.Lock()
	defer w.mut.Unlock()
	for _, device := range w.cfg.IgnoredDevices {
		if device.ID == id {
			return true
		}
	}
	return false
}

// IgnoredDevices returns a slice of ignored devices.
func (w *wrapper) IgnoredDevices() []ObservedDevice {
	w.mut.Lock()
	defer w.mut.Unlock()
	res := make([]ObservedDevice, len(w.cfg.IgnoredDevices))
	copy(res, w.cfg.IgnoredDevices)
	return res
}

// IgnoredFolder returns whether or not share attempts for the given
// folder should be silently ignored.
func (w *wrapper) IgnoredFolder(device protocol.DeviceID, folder string) bool {
	dev, ok := w.Device(device)
	if !ok {
		return false
	}
	return dev.IgnoredFolder(folder)
}

// Device returns the configuration for the given device and an "ok" bool.
func (w *wrapper) Device(id protocol.DeviceID) (DeviceConfiguration, bool) {
	w.mut.Lock()
	defer w.mut.Unlock()
	device, _, ok := w.cfg.Device(id)
	if !ok {
		return DeviceConfiguration{}, false
	}
	return device.Copy(), ok
}

// Folder returns the configuration for the given folder and an "ok" bool.
func (w *wrapper) Folder(id string) (FolderConfiguration, bool) {
	w.mut.Lock()
	defer w.mut.Unlock()
	fcfg, _, ok := w.cfg.Folder(id)
	if !ok {
		return FolderConfiguration{}, false
	}
	return fcfg.Copy(), ok
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

type modifyEntry struct {
	modifyFunc ModifyFunction
	res        chan modifyResult
}

type modifyResult struct {
	w   Waiter
	err error
}
