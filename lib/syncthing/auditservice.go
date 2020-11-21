// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package syncthing

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/syncthing/syncthing/lib/events"
)

// The auditService subscribes to events and writes these in JSON format, one
// event per line, to the specified writer.
type auditService struct {
	w        io.Writer // audit destination
	evLogger events.Logger
}

func newAuditService(w io.Writer, evLogger events.Logger) *auditService {
	return &auditService{
		w:        w,
		evLogger: evLogger,
	}
}

// serve runs the audit service.
func (s *auditService) Serve(ctx context.Context) error {
	sub := s.evLogger.Subscribe(events.AllEvents)
	defer sub.Unsubscribe()

	enc := json.NewEncoder(s.w)

	for {
		select {
		case ev := <-sub.C():
			enc.Encode(ev)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (s *auditService) String() string {
	return fmt.Sprintf("auditService@%p", s)
}
