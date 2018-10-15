// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"fmt"

	"github.com/syncthing/syncthing/lib/events"
)

// The verbose logging service subscribes to events and prints these in
// verbose format to the console using INFO level.
type verboseService struct {
	stop    chan struct{} // signals time to stop
	started chan struct{} // signals startup complete
}

func newVerboseService() *verboseService {
	return &verboseService{
		stop:    make(chan struct{}),
		started: make(chan struct{}),
	}
}

// Serve runs the verbose logging service.
func (s *verboseService) Serve() {
	sub := events.Default.Subscribe(events.AllEvents)
	defer events.Default.Unsubscribe(sub)

	select {
	case <-s.started:
		// The started channel has already been closed; do nothing.
	default:
		// This is the first time around. Indicate that we're ready to start
		// processing events.
		close(s.started)
	}

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
func (s *verboseService) Stop() {
	close(s.stop)
}

// WaitForStart returns once the verbose logging service is ready to receive
// events, or immediately if it's already running.
func (s *verboseService) WaitForStart() {
	<-s.started
}

func (s *verboseService) formatEvent(ev events.Event) string {
	switch ev.Type {
	case events.DownloadProgress, events.LocalIndexUpdated:
		// Skip
		return ""

	case events.Starting:
		return fmt.Sprintf("Starting up (%s)", ev.Data["home"])

	case events.StartupComplete:
		return "Startup complete"

	case events.DeviceDiscovered:
		return fmt.Sprintf("Discovered device %v at %v", ev.Data["device"], ev.Data["addrs"])

	case events.DeviceConnected:
		return fmt.Sprintf("Connected to device %v at %v (type %s)", ev.Data["id"], ev.Data["addr"], ev.Data["type"])

	case events.DeviceDisconnected:
		return fmt.Sprintf("Disconnected from device %v", ev.Data["id"])

	case events.StateChanged:
		return fmt.Sprintf("Folder %q is now %v", ev.Data["folder"], ev.Data["to"])

	case events.LocalChangeDetected:
		return fmt.Sprintf("Local change detected in folder %q: %s %s %s", ev.Data["folder"], ev.Data["action"], ev.Data["type"], ev.Data["path"])

	case events.RemoteChangeDetected:
		return fmt.Sprintf("Remote change detected in folder %q: %s %s %s", ev.Data["folder"], ev.Data["action"], ev.Data["type"], ev.Data["path"])

	case events.RemoteIndexUpdated:
		return fmt.Sprintf("Device %v sent an index update for %q with %d items", ev.Data["device"], ev.Data["folder"], ev.Data["items"])

	case events.DeviceRejected:
		return fmt.Sprintf("Rejected connection from device %v at %v", ev.Data["device"], ev.Data["address"])

	case events.FolderRejected:
		return fmt.Sprintf("Rejected unshared folder %q from device %v", ev.Data["folder"], ev.Data["device"])

	case events.ItemStarted:
		return fmt.Sprintf("Started syncing %q / %q (%v %v)", ev.Data["folder"], ev.Data["item"], ev.Data["action"], ev.Data["type"])

	case events.ItemFinished:
		if err, ok := ev.Data["error"].(*string); ok && err != nil {
			// If the err interface{} is not nil, it is a string pointer.
			// Dereference it to get the actual error or Sprintf will print
			// the pointer value....
			return fmt.Sprintf("Finished syncing %q / %q (%v %v): %v", ev.Data["folder"], ev.Data["item"], ev.Data["action"], ev.Data["type"], *err)
		}
		return fmt.Sprintf("Finished syncing %q / %q (%v %v): Success", ev.Data["folder"], ev.Data["item"], ev.Data["action"], ev.Data["type"])

	case events.ConfigSaved:
		return "Configuration was saved"

	case events.FolderCompletion:
		return fmt.Sprintf("Completion for folder %q on device %v is %v%%", ev.Data["folder"], ev.Data["device"], ev.Data["completion"])

	case events.FolderSummary:
		sum := make(map[string]interface{})
		for k, v := range ev.Data["summary"].(map[string]interface{}) {
			if k == "invalid" || k == "ignorePatterns" || k == "stateChanged" {
				continue
			}
			sum[k] = v
		}
		return fmt.Sprintf("Summary for folder %q is %v", ev.Data["folder"], sum)

	case events.FolderScanProgress:
		folder := ev.Data["folder"].(string)
		current := ev.Data["current"].(int64)
		total := ev.Data["total"].(int64)
		rate := ev.Data["rate"].(float64) / 1024 / 1024
		var pct int64
		if total > 0 {
			pct = 100 * current / total
		}
		return fmt.Sprintf("Scanning folder %q, %d%% done (%.01f MiB/s)", folder, pct, rate)

	case events.DevicePaused:
		device := ev.Data["device"]
		return fmt.Sprintf("Device %v was paused", device)

	case events.DeviceResumed:
		device := ev.Data["device"]
		return fmt.Sprintf("Device %v was resumed", device)

	case events.FolderPaused:
		id := ev.Data["id"]
		label := ev.Data["label"]
		return fmt.Sprintf("Folder %v (%v) was paused", id, label)

	case events.FolderResumed:
		id := ev.Data["id"]
		label := ev.Data["label"]
		return fmt.Sprintf("Folder %v (%v) was resumed", id, label)

	case events.ListenAddressesChanged:
		address := ev.Data["address"]
		lan := ev.Data["lan"]
		wan := ev.Data["wan"]
		return fmt.Sprintf("Listen address %s resolution has changed: lan addresses: %s wan addresses: %s", address, lan, wan)

	case events.LoginAttempt:
		username := ev.Data["username"].(string)
		var success string
		if ev.Data["success"].(bool) {
			success = "successful"
		} else {
			success = "failed"
		}
		return fmt.Sprintf("Login %s for username %s.", success, username)
	}

	return fmt.Sprintf("%s %#v", ev.Type, ev)
}
