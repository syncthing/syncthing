// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package syncthing

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/events"
)

func TestAuditService(t *testing.T) {
	buf := new(bytes.Buffer)
	evLogger := events.NewLogger()
	ctx, cancel := context.WithCancel(context.Background())
	go evLogger.Serve(ctx)
	defer cancel()
	sub := evLogger.Subscribe(events.AllEvents)
	defer sub.Unsubscribe()

	// Event sent before start, will not be logged
	evLogger.Log(events.ConfigSaved, "the first event")
	// Make sure the event goes through before creating the service
	<-sub.C()

	auditCtx, auditCancel := context.WithCancel(context.Background())
	service := newAuditService(buf, evLogger)
	done := make(chan struct{})
	go func() {
		service.Serve(auditCtx)
		close(done)
	}()

	// Subscription needs to happen in service.Serve
	time.Sleep(10 * time.Millisecond)

	// Event that should end up in the audit log
	evLogger.Log(events.ConfigSaved, "the second event")

	// We need to give the events time to arrive, since the channels are buffered etc.
	time.Sleep(10 * time.Millisecond)

	auditCancel()
	<-done

	// This event should not be logged, since we have stopped.
	evLogger.Log(events.ConfigSaved, "the third event")

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
