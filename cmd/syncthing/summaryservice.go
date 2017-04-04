// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"time"

	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/thejerf/suture"
)

// The folderSummaryService adds summary information events (FolderSummary and
// FolderCompletion) into the event stream at certain intervals.
type folderSummaryService struct {
	*suture.Supervisor

	cfg       configIntf
	model     modelIntf
	stop      chan struct{}
	immediate chan string

	// For keeping track of folders to recalculate for
	foldersMut sync.Mutex
	folders    map[string]struct{}

	// For keeping track of when the last event request on the API was
	lastEventReq    time.Time
	lastEventReqMut sync.Mutex
}

func newFolderSummaryService(cfg configIntf, m modelIntf) *folderSummaryService {
	service := &folderSummaryService{
		Supervisor:      suture.NewSimple("folderSummaryService"),
		cfg:             cfg,
		model:           m,
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

// listenForUpdates subscribes to the event bus and makes note of folders that
// need their data recalculated.
func (c *folderSummaryService) listenForUpdates() {
	sub := events.Default.Subscribe(events.LocalIndexUpdated | events.RemoteIndexUpdated | events.StateChanged | events.RemoteDownloadProgress | events.DeviceConnected)
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

					select {
					case c.immediate <- folder:
						c.foldersMut.Lock()
						delete(c.folders, folder)
						c.foldersMut.Unlock()

					default:
					}
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
	if time.Since(last) > defaultEventTimeout {
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
	data := folderSummary(c.cfg, c.model, folder)
	events.Default.Log(events.FolderSummary, map[string]interface{}{
		"folder":  folder,
		"summary": data,
	})

	for _, devCfg := range c.cfg.Folders()[folder].Devices {
		if devCfg.DeviceID.Equals(myID) {
			// We already know about ourselves.
			continue
		}
		if !c.model.ConnectedTo(devCfg.DeviceID) {
			// We're not interested in disconnected devices.
			continue
		}

		// Get completion percentage of this folder for the
		// remote device.
		comp := c.model.Completion(devCfg.DeviceID, folder)
		events.Default.Log(events.FolderCompletion, map[string]interface{}{
			"folder":      folder,
			"device":      devCfg.DeviceID.String(),
			"completion":  comp.CompletionPct,
			"needBytes":   comp.NeedBytes,
			"globalBytes": comp.GlobalBytes,
		})
	}
}

func (c *folderSummaryService) gotEventRequest() {
	c.lastEventReqMut.Lock()
	c.lastEventReq = time.Now()
	c.lastEventReqMut.Unlock()
}

// serviceFunc wraps a function to create a suture.Service without stop
// functionality.
type serviceFunc func()

func (f serviceFunc) Serve() { f() }
func (f serviceFunc) Stop()  {}
