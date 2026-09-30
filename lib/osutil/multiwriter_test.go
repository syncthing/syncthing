// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package osutil

import (
	"bytes"
	"errors"
	"testing"
)

// failingWriter simulates a writer that always fails. This is enough to verify
// that the tolerant multi-writer still writes to the remaining targets and
// reports the failure in a combined error.
type failingWriter struct {
	err error
	buf *bytes.Buffer
}

func (w *failingWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	return w.buf.Write(p)
}

func TestTolerantMultiWriter(t *testing.T) {
	// A successful writer should still receive the payload even when another
	// writer in the set fails. The failing writer is expected to be reported in
	// the returned error, but it must not prevent the other write from happening.
	var ok bytes.Buffer
	fail := &failingWriter{err: errors.New("boom")}
	mw := TolerantMultiWriter{&ok, fail}

	const payload = "hello"
	n, err := mw.Write([]byte(payload))
	if n != len(payload) {
		t.Fatalf("Write returned %d bytes, want %d", n, len(payload))
	}
	if err == nil || !errors.Is(err, fail.err) {
		t.Fatalf("Write returned %v, want wrapped failing write error", err)
	}
	if got := ok.String(); got != payload {
		t.Fatalf("successful writer got %q, want %q", got, payload)
	}
}
