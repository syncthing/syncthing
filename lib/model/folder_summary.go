// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"fmt"
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/thejerf/suture"
)

const minSummaryInterval = time.Minute

type FolderSummaryService interface {
	suture.Service
	Summary(folder string) (map[string]interface{}, error)
	OnEventRequest()
}

// The folderSummaryService adds summary information events (FolderSummary and
// FolderCompletion) into the event stream at certain intervals.
type folderSummaryService struct {
	*suture.Supervisor

	cfg       config.Wrapper
	model     Model
	id        protocol.DeviceID
	stop      chan struct{}
	immediate chan string

	// For keeping track of folders to recalculate for
	foldersMut sync.Mutex
	folders    map[string]struct{}

	// For keeping track of when the last event request on the API was
	lastEventReq    time.Time
	lastEventReqMut sync.Mutex
}

func NewFolderSummaryService(cfg config.Wrapper, m Model, id protocol.DeviceID) FolderSummaryService {
	service := &folderSummaryService{
		Supervisor: suture.New("folderSummaryService", suture.Spec{
			PassThroughPanics: true,
		}),
		cfg:             cfg,
		model:           m,
		id:              id,
		stop:            make(chan struct{}),
		immediate:       make(chan string),
		folders:         make(map[string]struct{}),
		foldersMut:      sync.NewMutex(),
		lastEventReqMut: sync.NewMutex(),
	}

	service.Add(serviceFunc(service.listenForUpdates))
	service.Add(serviceFunc(service.calculateSummaries))

	return service
}

func (c *folderSummaryService) Stop() {
	c.Supervisor.Stop()
	close(c.stop)
}

func (c *folderSummaryService) String() string {
	return fmt.Sprintf("FolderSummaryService@%p", c)
}

func (c *folderSummaryService) Summary(folder string) (map[string]interface{}, error) {
	var res = make(map[string]interface{})

	errors, err := c.model.FolderErrors(folder)
	if err != nil && err != ErrFolderPaused {
		// Stats from the db can still be obtained if the folder is just paused
		return nil, err
	}
	res["errors"] = len(errors)
	res["pullErrors"] = len(errors) // deprecated

	res["invalid"] = "" // Deprecated, retains external API for now

	global := c.model.GlobalSize(folder)
	res["globalFiles"], res["globalDirectories"], res["globalSymlinks"], res["globalDeleted"], res["globalBytes"], res["globalTotalItems"] = global.Files, global.Directories, global.Symlinks, global.Deleted, global.Bytes, global.TotalItems()

	local := c.model.LocalSize(folder)
	res["localFiles"], res["localDirectories"], res["localSymlinks"], res["localDeleted"], res["localBytes"], res["localTotalItems"] = local.Files, local.Directories, local.Symlinks, local.Deleted, local.Bytes, local.TotalItems()

	need := c.model.NeedSize(folder)
	res["needFiles"], res["needDirectories"], res["needSymlinks"], res["needDeletes"], res["needBytes"], res["needTotalItems"] = need.Files, need.Directories, need.Symlinks, need.Deleted, need.Bytes, need.TotalItems()

	if c.cfg.Folders()[folder].Type == config.FolderTypeReceiveOnly {
		// Add statistics for things that have changed locally in a receive
		// only folder.
		ro := c.model.ReceiveOnlyChangedSize(folder)
		res["receiveOnlyChangedFiles"] = ro.Files
		res["receiveOnlyChangedDirectories"] = ro.Directories
		res["receiveOnlyChangedSymlinks"] = ro.Symlinks
		res["receiveOnlyChangedDeletes"] = ro.Deleted
		res["receiveOnlyChangedBytes"] = ro.Bytes
		res["receiveOnlyTotalItems"] = ro.TotalItems()
	}

	res["inSyncFiles"], res["inSyncBytes"] = global.Files-need.Files, global.Bytes-need.Bytes

	res["state"], res["stateChanged"], err = c.model.State(folder)
	if err != nil {
		res["error"] = err.Error()
	}

	ourSeq, _ := c.model.CurrentSequence(folder)
	remoteSeq, _ := c.model.RemoteSequence(folder)

	res["version"] = ourSeq + remoteSeq  // legacy
	res["sequence"] = ourSeq + remoteSeq // new name

	ignorePatterns, _, _ := c.model.GetIgnores(folder)
	res["ignorePatterns"] = false
	for _, line := range ignorePatterns {
		if len(line) > 0 && !strings.HasPrefix(line, "//") {
			res["ignorePatterns"] = true
			break
		}
	}

	err = c.model.WatchError(folder)
	if err != nil {
		res["watchError"] = err.Error()
	}

	return res, nil
}

func (c *folderSummaryService) OnEventRequest() {
	c.lastEventReqMut.Lock()
	c.lastEventReq = time.Now()
	c.lastEventReqMut.Unlock()
}

// listenForUpdates subscribes to the event bus and makes note of folders that
// need their data recalculated.
func (c *folderSummaryService) listenForUpdates() {
	sub := events.Default.Subscribe(events.LocalIndexUpdated | events.RemoteIndexUpdated | events.StateChanged | events.RemoteDownloadProgress | events.DeviceConnected | events.FolderWatchStateChanged)
	defer events.Default.Unsubscribe(sub)

	for {
		// This loop needs to be fast so we don't miss too many events.

		select {
		case ev := <-sub.C():
			if ev.Type == events.DeviceConnected {
				// When a device connects we schedule a refresh of all
				// folders shared with that device.

				data := ev.Data.(map[string]string)
				deviceID, _ := protocol.DeviceIDFromString(data["id"])

				c.foldersMut.Lock()
			nextFolder:
				for _, folder := range c.cfg.Folders() {
					for _, dev := range folder.Devices {
						if dev.DeviceID == deviceID {
							c.folders[folder.ID] = struct{}{}
							continue nextFolder
						}
					}
				}
				c.foldersMut.Unlock()

				continue
			}

			// The other events all have a "folder" attribute that they
			// affect. Whenever the local or remote index is updated for a
			// given folder we make a note of it.

			data := ev.Data.(map[string]interface{})
			folder := data["folder"].(string)

			switch ev.Type {
			case events.StateChanged:
				if data["to"].(string) == "idle" && data["from"].(string) == "syncing" {
					// The folder changed to idle from syncing. We should do an
					// immediate refresh to update the GUI. The send to
					// c.immediate must be nonblocking so that we can continue
					// handling events.

					c.foldersMut.Lock()
					select {
					case c.immediate <- folder:
						delete(c.folders, folder)
					default:
						c.folders[folder] = struct{}{}
					}
					c.foldersMut.Unlock()
				}

			default:
				// This folder needs to be refreshed whenever we do the next
				// refresh.

				c.foldersMut.Lock()
				c.folders[folder] = struct{}{}
				c.foldersMut.Unlock()
			}

		case <-c.stop:
			return
		}
	}
}

// calculateSummaries periodically recalculates folder summaries and
// completion percentage, and sends the results on the event bus.
func (c *folderSummaryService) calculateSummaries() {
	const pumpInterval = 2 * time.Second
	pump := time.NewTimer(pumpInterval)

	for {
		select {
		case <-pump.C:
			t0 := time.Now()
			for _, folder := range c.foldersToHandle() {
				c.sendSummary(folder)
			}

			// We don't want to spend all our time calculating summaries. Lets
			// set an arbitrary limit at not spending more than about 30% of
			// our time here...
			wait := 2*time.Since(t0) + pumpInterval
			pump.Reset(wait)

		case folder := <-c.immediate:
			c.sendSummary(folder)

		case <-c.stop:
			return
		}
	}
}

// foldersToHandle returns the list of folders needing a summary update, and
// clears the list.
func (c *folderSummaryService) foldersToHandle() []string {
	// We only recalculate summaries if someone is listening to events
	// (a request to /rest/events has been made within the last
	// pingEventInterval).

	c.lastEventReqMut.Lock()
	last := c.lastEventReq
	c.lastEventReqMut.Unlock()
	if time.Since(last) > minSummaryInterval {
		return nil
	}

	c.foldersMut.Lock()
	res := make([]string, 0, len(c.folders))
	for folder := range c.folders {
		res = append(res, folder)
		delete(c.folders, folder)
	}
	c.foldersMut.Unlock()
	return res
}

// sendSummary send the summary events for a single folder
func (c *folderSummaryService) sendSummary(folder string) {
	// The folder summary contains how many bytes, files etc
	// are in the folder and how in sync we are.
	data, err := c.Summary(folder)
	if err != nil {
		return
	}
	events.Default.Log(events.FolderSummary, map[string]interface{}{
		"folder":  folder,
		"summary": data,
	})

	for _, devCfg := range c.cfg.Folders()[folder].Devices {
		if devCfg.DeviceID.Equals(c.id) {
			// We already know about ourselves.
			continue
		}
		if _, ok := c.model.Connection(devCfg.DeviceID); !ok {
			// We're not interested in disconnected devices.
			continue
		}

		// Get completion percentage of this folder for the
		// remote device.
		comp := c.model.Completion(devCfg.DeviceID, folder).Map()
		comp["folder"] = folder
		comp["device"] = devCfg.DeviceID.String()
		events.Default.Log(events.FolderCompletion, comp)
	}
}

// serviceFunc wraps a function to create a suture.Service without stop
// functionality.
type serviceFunc func()

func (f serviceFunc) Serve() { f() }
func (f serviceFunc) Stop()  {}
