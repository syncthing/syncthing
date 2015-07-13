// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package config

import (
	"errors"
	"testing"
)

type requiresRestart struct{}

func (requiresRestart) VerifyConfiguration(_, _ Configuration) error {
	return nil
}
func (requiresRestart) CommitConfiguration(_, _ Configuration) bool {
	return false
}
func (requiresRestart) String() string {
	return "requiresRestart"
}

type validationError struct{}

func (validationError) VerifyConfiguration(_, _ Configuration) error {
	return errors.New("some error")
}
func (validationError) CommitConfiguration(_, _ Configuration) bool {
	return true
}
func (validationError) String() string {
	return "validationError"
}

func TestReplaceCommit(t *testing.T) {
	w := Wrap("/dev/null", Configuration{Version: 0})
	if w.Raw().Version != 0 {
		t.Fatal("Config incorrect")
	}

	// Replace config. We should get back a clean response and the config
	// should change.

	resp := w.Replace(Configuration{Version: 1})
	if resp.ValidationError != nil {
		t.Fatal("Should not have a validation error")
	}
	if resp.RequiresRestart {
		t.Fatal("Should not require restart")
	}
	if w.Raw().Version != 1 {
		t.Fatal("Config should have changed")
	}

	// Now with a subscriber requiring restart. We should get a clean response
	// but with the restart flag set, and the config should change.

	w.Subscribe(requiresRestart{})

	resp = w.Replace(Configuration{Version: 2})
	if resp.ValidationError != nil {
		t.Fatal("Should not have a validation error")
	}
	if !resp.RequiresRestart {
		t.Fatal("Should require restart")
	}
	if w.Raw().Version != 2 {
		t.Fatal("Config should have changed")
	}

	// Now with a subscriber that throws a validation error. The config should
	// not change.

	w.Subscribe(validationError{})

	resp = w.Replace(Configuration{Version: 3})
	if resp.ValidationError == nil {
		t.Fatal("Should have a validation error")
	}
	if resp.RequiresRestart {
		t.Fatal("Should not require restart")
	}
	if w.Raw().Version != 2 {
		t.Fatal("Config should not have changed")
	}
}
