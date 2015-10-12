// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"encoding/json"
	"io"

	"github.com/syncthing/syncthing/lib/events"
)

// The auditSvc subscribes to events and writes these in JSON format, one
// event per line, to the specified writer.
type auditSvc struct {
	w       io.Writer     // audit destination
	stop    chan struct{} // signals time to stop
	started chan struct{} // signals startup complete
	stopped chan struct{} // signals stop complete
}

func newAuditSvc(w io.Writer) *auditSvc {
	return &auditSvc{
		w:       w,
		stop:    make(chan struct{}),
		started: make(chan struct{}),
		stopped: make(chan struct{}),
	}
}

// Serve runs the audit service.
func (s *auditSvc) Serve() {
	defer close(s.stopped)
	sub := events.Default.Subscribe(events.AllEvents)
	defer events.Default.Unsubscribe(sub)
	enc := json.NewEncoder(s.w)

	// We're ready to start processing events.
	close(s.started)

	for {
		select {
		case ev := <-sub.C():
			enc.Encode(ev)
		case <-s.stop:
			return
		}
	}
}

// Stop stops the audit service.
func (s *auditSvc) Stop() {
	close(s.stop)
}

// WaitForStart returns once the audit service is ready to receive events, or
// immediately if it's already running.
func (s *auditSvc) WaitForStart() {
	<-s.started
}

// WaitForStop returns once the audit service has stopped.
// (Needed by the tests.)
func (s *auditSvc) WaitForStop() {
	<-s.stopped
}
