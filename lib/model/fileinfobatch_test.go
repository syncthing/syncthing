// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"errors"
	"testing"

	"github.com/syncthing/syncthing/lib/protocol"
)

func TestFileInfoBatchError(t *testing.T) {
	// Verify behaviour of the flush function returning an error.

	var errReturn error
	var called int
	b := NewFileInfoBatch(func([]protocol.FileInfo) error {
		called += 1
		return errReturn
	})

	// Flush should work when the flush function error is nil
	b.Append(protocol.FileInfo{Name: "test"})
	if err := b.Flush(); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if called != 1 {
		t.Fatalf("expected 1, got %d", called)
	}

	// Flush should fail with an error retur
	errReturn = errors.New("problem")
	b.Append(protocol.FileInfo{Name: "test"})
	if err := b.Flush(); err != errReturn {
		t.Fatalf("expected %v, got %v", errReturn, err)
	}
	if called != 2 {
		t.Fatalf("expected 2, got %d", called)
	}

	// Flush function should not be called again when it's already errored,
	// same error should be returned by Flush()
	if err := b.Flush(); err != errReturn {
		t.Fatalf("expected %v, got %v", errReturn, err)
	}
	if called != 2 {
		t.Fatalf("expected 2, got %d", called)
	}

	// Reset should clear the error (and the file list)
	errReturn = nil
	b.Reset()
	b.Append(protocol.FileInfo{Name: "test"})
	if err := b.Flush(); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if called != 3 {
		t.Fatalf("expected 3, got %d", called)
	}
}
