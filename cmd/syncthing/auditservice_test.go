// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/events"
)

func TestAuditService(t *testing.T) {
	buf := new(bytes.Buffer)
	service := newAuditService(buf)

	// Event sent before start, will not be logged
	events.Default.Log(events.ConfigSaved, "the first event")

	go service.Serve()
	service.WaitForStart()

	// Event that should end up in the audit log
	events.Default.Log(events.ConfigSaved, "the second event")

	// We need to give the events time to arrive, since the channels are buffered etc.
	time.Sleep(10 * time.Millisecond)

	service.Stop()
	service.WaitForStop()

	// This event should not be logged, since we have stopped.
	events.Default.Log(events.ConfigSaved, "the third event")

	result := buf.String()
	t.Log(result)

	if strings.Contains(result, "first event") {
		t.Error("Unexpected first event")
	}

	if !strings.Contains(result, "second event") {
		t.Error("Missing second event")
	}

	if strings.Contains(result, "third event") {
		t.Error("Missing third event")
	}
}
