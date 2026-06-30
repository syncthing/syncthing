// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"errors"
	"testing"

	"github.com/syncthing/syncthing/lib/events"
)

// TestStateTrackerErrorClearedOnSetState guards the invariant that the error
// is non-nil exactly when the state is FolderError. A transition out of the
// error state via setState (e.g. the run loop flipping to FolderIdle before a
// pull) must not leave a stale error behind, as that previously caused the
// folder to report "idle" (shown as "Up to Date" in the GUI) while still
// carrying an error. See https://github.com/syncthing/syncthing/issues/10546.
func TestStateTrackerErrorClearedOnSetState(t *testing.T) {
	evLogger := events.NewLogger()
	go evLogger.Serve(t.Context())

	s := newStateTracker("default", evLogger)

	wantErr := errors.New("folder path missing")
	s.setError(wantErr)

	if state, _, err := s.getState(); state != FolderError || err == nil {
		t.Fatalf("after setError: got state %v, err %v; want FolderError with non-nil error", state, err)
	}

	// Transitioning out of the error state via setState must clear the error.
	s.setState(FolderIdle)

	if state, _, err := s.getState(); state != FolderIdle || err != nil {
		t.Fatalf("after setState(FolderIdle): got state %v, err %v; want FolderIdle with nil error", state, err)
	}
}
