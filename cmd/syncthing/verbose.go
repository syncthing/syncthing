// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"fmt"

	"github.com/syncthing/syncthing/lib/events"
)

// The verbose logging service subscribes to events and prints these in
// verbose format to the console using INFO level.
type verboseSvc struct {
	stop    chan struct{} // signals time to stop
	started chan struct{} // signals startup complete
}

func newVerboseSvc() *verboseSvc {
	return &verboseSvc{
		stop:    make(chan struct{}),
		started: make(chan struct{}),
	}
}

// Serve runs the verbose logging service.
func (s *verboseSvc) Serve() {
	sub := events.Default.Subscribe(events.AllEvents)
	defer events.Default.Unsubscribe(sub)

	// We're ready to start processing events.
	close(s.started)

	for {
		select {
		case ev := <-sub.C():
			formatted := s.formatEvent(ev)
			if formatted != "" {
				l.Verboseln(formatted)
			}
		case <-s.stop:
			return
		}
	}
}

// Stop stops the verbose logging service.
func (s *verboseSvc) Stop() {
	close(s.stop)
}

// WaitForStart returns once the verbose logging service is ready to receive
// events, or immediately if it's already running.
func (s *verboseSvc) WaitForStart() {
	<-s.started
}

func (s *verboseSvc) formatEvent(ev events.Event) string {
	switch ev.Type {
	case events.Ping, events.DownloadProgress, events.LocalIndexUpdated:
		// Skip
		return ""

	case events.Starting:
		return fmt.Sprintf("Starting up (%s)", ev.Data.(map[string]string)["home"])
	case events.StartupComplete:
		return "Startup complete"

	case events.DeviceDiscovered:
		data := ev.Data.(map[string]interface{})
		return fmt.Sprintf("Discovered device %v at %v", data["device"], data["addrs"])
	case events.DeviceConnected:
		data := ev.Data.(map[string]string)
		return fmt.Sprintf("Connected to device %v at %v (type %s)", data["id"], data["addr"], data["type"])
	case events.DeviceDisconnected:
		data := ev.Data.(map[string]string)
		return fmt.Sprintf("Disconnected from device %v", data["id"])

	case events.StateChanged:
		data := ev.Data.(map[string]interface{})
		return fmt.Sprintf("Folder %q is now %v", data["folder"], data["to"])

	case events.RemoteIndexUpdated:
		data := ev.Data.(map[string]interface{})
		return fmt.Sprintf("Device %v sent an index update for %q with %d items", data["device"], data["folder"], data["items"])

	case events.DeviceRejected:
		data := ev.Data.(map[string]interface{})
		return fmt.Sprintf("Rejected connection from device %v at %v", data["device"], data["address"])
	case events.FolderRejected:
		data := ev.Data.(map[string]string)
		return fmt.Sprintf("Rejected unshared folder %q from device %v", data["folder"], data["device"])

	case events.ItemStarted:
		data := ev.Data.(map[string]interface{})
		return fmt.Sprintf("Started syncing %q / %q (%v %v)", data["folder"], data["item"], data["action"], data["type"])
	case events.ItemFinished:
		data := ev.Data.(map[string]interface{})
		if err, ok := data["error"].(*string); ok && err != nil {
			// If the err interface{} is not nil, it is a string pointer.
			// Dereference it to get the actual error or Sprintf will print
			// the pointer value....
			return fmt.Sprintf("Finished syncing %q / %q (%v %v): %v", data["folder"], data["item"], data["action"], data["type"], *err)
		}
		return fmt.Sprintf("Finished syncing %q / %q (%v %v): Success", data["folder"], data["item"], data["action"], data["type"])

	case events.ConfigSaved:
		return "Configuration was saved"

	case events.FolderCompletion:
		data := ev.Data.(map[string]interface{})
		return fmt.Sprintf("Completion for folder %q on device %v is %v%%", data["folder"], data["device"], data["completion"])
	case events.FolderSummary:
		data := ev.Data.(map[string]interface{})
		sum := data["summary"].(map[string]interface{})
		delete(sum, "invalid")
		delete(sum, "ignorePatterns")
		delete(sum, "stateChanged")
		return fmt.Sprintf("Summary for folder %q is %v", data["folder"], data["summary"])
	case events.FolderScanProgress:
		data := ev.Data.(map[string]interface{})
		folder := data["folder"].(string)
		current := data["current"].(int64)
		total := data["total"].(int64)
		var pct int64
		if total > 0 {
			pct = 100 * current / total
		}
		return fmt.Sprintf("Scanning folder %q, %d%% done", folder, pct)

	case events.DevicePaused:
		data := ev.Data.(map[string]string)
		device := data["device"]
		return fmt.Sprintf("Device %v was paused", device)
	case events.DeviceResumed:
		data := ev.Data.(map[string]string)
		device := data["device"]
		return fmt.Sprintf("Device %v was resumed", device)
	}

	return fmt.Sprintf("%s %#v", ev.Type, ev)
}
