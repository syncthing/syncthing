// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package syncthing

import (
	"encoding/json"
	"io"

	"github.com/thejerf/suture"

	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/util"
)

// The auditService subscribes to events and writes these in JSON format, one
// event per line, to the specified writer.
type auditService struct {
	suture.Service
	w   io.Writer // audit destination
	sub events.Subscription
}

func newAuditService(w io.Writer, evLogger events.Logger) *auditService {
	s := &auditService{
		w:   w,
		sub: evLogger.Subscribe(events.AllEvents),
	}
	s.Service = util.AsService(s.serve)
	return s
}

// serve runs the audit service.
func (s *auditService) serve(stop chan struct{}) {
	enc := json.NewEncoder(s.w)

	for {
		select {
		case ev := <-s.sub.C():
			enc.Encode(ev)
		case <-stop:
			return
		}
	}
}

// Stop stops the audit service.
func (s *auditService) Stop() {
	s.Service.Stop()
	s.sub.Unsubscribe()
}
