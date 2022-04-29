// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package syncthing

import (
	"context"
	"fmt"
	"regexp"

	"github.com/syncthing/syncthing/lib/api"
	"github.com/syncthing/syncthing/lib/connections"
	"github.com/syncthing/syncthing/lib/discover"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/model"
	"github.com/syncthing/syncthing/lib/scanner"
)

// The verbose logging service subscribes to events and prints these in
// verbose format to the console using INFO level.
type verboseService struct {
	evLogger events.Logger
}

func newVerboseService(evLogger events.Logger) *verboseService {
	return &verboseService{
		evLogger: evLogger,
	}
}

// serve runs the verbose logging service.
func (s *verboseService) Serve(ctx context.Context) error {
	sub := s.evLogger.Subscribe(events.AllEvents)
	defer sub.Unsubscribe()
	for {
		select {
		case ev, ok := <-sub.C():
			if !ok {
				<-ctx.Done()
				return ctx.Err()
			}
			formatted := s.formatEvent(ev)
			if formatted != "" {
				l.Verboseln(formatted)
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

var folderSummaryRemoveDeprecatedRe = regexp.MustCompile(`(Invalid|IgnorePatterns|StateChanged):\S+\s?`)

func (s *verboseService) formatEvent(ev events.Event) string {
	switch ev.Type {
	case events.DownloadProgress, events.LocalIndexUpdated:
		// Skip
		return ""

	case events.Starting:
		data := ev.Data.(StartingEventData)
		return fmt.Sprintf("Starting up (%s)", data.Home)

	case events.StartupComplete:
		return "Startup complete"

	case events.DeviceDiscovered:
		data := ev.Data.(discover.DeviceDiscoveredEventData)
		return fmt.Sprintf("Discovered device %v at %v", data.Device, data.Addresses)

	case events.DeviceConnected:
		data := ev.Data.(model.DeviceConnectedEventData)
		return fmt.Sprintf("Connected to device %v at %v (type %s)", data.ID, data.Address, data.Type)

	case events.DeviceDisconnected:
		data := ev.Data.(model.DeviceDisconnectedEventData)
		return fmt.Sprintf("Disconnected from device %v", data.ID)

	case events.StateChanged:
		data := ev.Data.(model.StateChangedEventData)
		return fmt.Sprintf("Folder %q is now %v", data.Folder, data.To)

	case events.LocalChangeDetected:
		data := ev.Data.(model.DiskChangeDetectedEventData)
		return fmt.Sprintf("Local change detected in folder %q: %s %s %s", data.Folder, data.Action, data.Type, data.Path)

	case events.RemoteChangeDetected:
		data := ev.Data.(model.DiskChangeDetectedEventData)
		return fmt.Sprintf("Remote change detected in folder %q: %s %s %s", data.Folder, data.Action, data.Type, data.Path)

	case events.RemoteIndexUpdated:
		data := ev.Data.(model.RemoteIndexUpdatedEventData)
		return fmt.Sprintf("Device %v sent an index update for %q with %d items", data.Device, data.Folder, data.Items)

	case events.DeviceRejected:
		data := ev.Data.(model.DeviceRejectedEventData)
		return fmt.Sprintf("Rejected connection from device %v at %v", data.Device, data.Address)

	case events.FolderRejected:
		data := ev.Data.(model.FolderRejectedEventData)
		return fmt.Sprintf("Rejected unshared folder %q from device %v", data.Folder, data.Device)

	case events.ItemStarted:
		data := ev.Data.(model.ItemStartedEventData)
		return fmt.Sprintf("Started syncing %q / %q (%v %v)", data.Folder, data.Item, data.Action, data.Type)

	case events.ItemFinished:
		data := ev.Data.(model.ItemFinishedEventData)
		err := "Success"
		if data.Error != nil {
			err = *data.Error
		}
		return fmt.Sprintf("Finished syncing %q / %q (%v %v): %v", data.Folder, data.Item, data.Action, data.Type, err)

	case events.ConfigSaved:
		return "Configuration was saved"

	case events.FolderCompletion:
		data := ev.Data.(model.FolderCompletionEventData)
		return fmt.Sprintf("Completion for folder %q on device %v is %v%% (state: %s)", data.Folder, data.Device, data.CompletionPct, data.RemoteState)

	case events.FolderSummary:
		data := ev.Data.(model.FolderSummaryEventData)
		return folderSummaryRemoveDeprecatedRe.ReplaceAllString(fmt.Sprintf("Summary for folder %q is %+v", data.Folder, data.Summary), "")

	case events.FolderScanProgress:
		data := ev.Data.(scanner.FolderScanProgressEventData)
		rate := data.Rate / 1024 / 1024
		var pct int64
		if data.Total > 0 {
			pct = 100 * data.Current / data.Total
		}
		return fmt.Sprintf("Scanning folder %q, %d%% done (%.01f MiB/s)", data.Folder, pct, rate)

	case events.DevicePaused:
		data := ev.Data.(model.DevicePausedEventData)
		return fmt.Sprintf("Device %v was paused", data.Device)

	case events.DeviceResumed:
		data := ev.Data.(model.DeviceResumedEventData)
		return fmt.Sprintf("Device %v was resumed", data.Device)

	case events.ClusterConfigReceived:
		data := ev.Data.(model.ClusterConfigReceivedEventData)
		return fmt.Sprintf("Received ClusterConfig from device %v", data.Device)

	case events.FolderPaused:
		data := ev.Data.(model.FolderPausedEventData)
		return fmt.Sprintf("Folder %v (%v) was paused", data.ID, data.Label)

	case events.FolderResumed:
		data := ev.Data.(model.FolderResumedEventData)
		return fmt.Sprintf("Folder %v (%v) was resumed", data.ID, data.Label)

	case events.ListenAddressesChanged:
		data := ev.Data.(connections.ListenAddressesChangedEventData)
		return fmt.Sprintf("Listen address %s resolution has changed: lan addresses: %s wan addresses: %s", data.Address, data.LAN, data.WAN)

	case events.LoginAttempt:
		data := ev.Data.(api.LoginAttemptEventData)
		success := "failed"
		if data.Success {
			success = "successful"
		}
		return fmt.Sprintf("Login %s for username %s.", success, data.Username)
	}

	return fmt.Sprintf("%s %#v", ev.Type, ev)
}

func (s *verboseService) String() string {
	return fmt.Sprintf("verboseService@%p", s)
}
