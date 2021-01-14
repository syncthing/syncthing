// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
	"errors"
	"testing"
)

type requiresRestart struct {
	committed chan struct{}
}

func (requiresRestart) VerifyConfiguration(_, _ Configuration) error {
	return nil
}
func (c requiresRestart) CommitConfiguration(_, _ Configuration) bool {
	select {
	case c.committed <- struct{}{}:
	default:
	}
	return false
}
func (requiresRestart) String() string {
	return "requiresRestart"
}

type validationError struct{}

func (validationError) VerifyConfiguration(_, _ Configuration) error {
	return errors.New("some error")
}
func (c validationError) CommitConfiguration(_, _ Configuration) bool {
	return true
}
func (validationError) String() string {
	return "validationError"
}

func replace(t testing.TB, w Wrapper, to Configuration) {
	t.Helper()
	waiter, err := w.Modify(func(cfg *Configuration) {
		cfg = &to
	})
	if err != nil {
		t.Fatal(err)
	}
	waiter.Wait()
}

func TestReplaceCommit(t *testing.T) {
	t.Skip("broken, fails randomly, #3834")

	w := wrap("/dev/null", Configuration{Version: 0}, device1)
	defer w.stop()
	if w.RawCopy().Version != 0 {
		t.Fatal("Config incorrect")
	}

	// Replace config. We should get back a clean response and the config
	// should change.

	replace(t, w, Configuration{Version: 1})
	if w.RequiresRestart() {
		t.Fatal("Should not require restart")
	}
	if w.RawCopy().Version != CurrentVersion {
		t.Fatal("Config should have changed")
	}

	// Now with a subscriber requiring restart. We should get a clean response
	// but with the restart flag set, and the config should change.

	sub0 := requiresRestart{committed: make(chan struct{}, 1)}
	w.Subscribe(sub0)

	replace(t, w, Configuration{Version: 1})
	<-sub0.committed
	if !w.RequiresRestart() {
		t.Fatal("Should require restart")
	}
	if w.RawCopy().Version != CurrentVersion {
		t.Fatal("Config should have changed")
	}

	// Now with a subscriber that throws a validation error. The config should
	// not change.

	w.Subscribe(validationError{})

	_, err := w.Modify(func(cfg *Configuration) {
		cfg = &Configuration{Version: 3}
	})
	if err == nil {
		t.Fatal("Should have a validation error")
	}
	if !w.RequiresRestart() {
		t.Fatal("Should still require restart")
	}
	if w.RawCopy().Version != CurrentVersion {
		t.Fatal("Config should not have changed")
	}
}
