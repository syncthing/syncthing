// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build ios

package syncthing

func (a *App) IsIdle() bool {
	if M == nil {
		return true
	}

  defer func() {
    if r := recover(); r != nil {
        l.Warnln("Panic in IsIdle")
    }
  }()

	overallIdle := true
	for _, folder := range a.cfg.Folders() {
		state, _, err := M.State(folder.ID)
		if err == nil {
			overallIdle = overallIdle && (state == "idle")
			// l.Infoln("IsIdle folder", folder.Label, "state", state)
		} else {
			// l.Infoln("IsIdle folder", folder.Label, "error", err.Error())
		}
	}

	connectionStats := M.ConnectionStats()
	for _, device := range a.cfg.Devices() {
		completion := M.Completion(device.DeviceID, "")
		connections, _ := connectionStats["connections"].(map[string]interface{})
		connection, _ := connections[device.DeviceID.String()].(map[string]interface{})
		connected, _ := connection["connected"].(bool)
		paused, _ := connection["paused"].(bool)
		idle := !connected || paused || completion.CompletionPct >= 100.0
		overallIdle = overallIdle && idle
		// l.Infoln("IsIdle device", device.Name, idle, "connected", connected, "paused", paused,"completion", completion.CompletionPct)
	}

	return overallIdle
}
