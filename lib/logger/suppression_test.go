// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package logger

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"testing"
	"time"
)

func TestSuppression(t *testing.T) {
	// Suppression kicks in at 100 messages average over 10 buckets. Time is
	// irrelevant, we rotate manually.

	buf := new(bytes.Buffer)
	l := newSuppressingLogger(log.New(buf, "", 0), 10, 100, time.Hour)

	if l.active {
		t.Error("suppression should be inactive at start")
	}

	// Identical messages don't count towards suppression (but are
	// suppressed on their own)

	for i := 0; i < 10000; i++ {
		_ = l.Output(1, "A log message")
	}

	l.rotateBuckets()
	lines := strings.Split(buf.String(), "\n")
	buf.Reset()

	if l.active {
		t.Error("suppression should not be active after repeated messages")
	}
	if len(lines) > 10 {
		t.Error("repeated line should have been skipped")
	}

	// Now cause suppression

	for i := 0; i < 10000; i++ {
		_ = l.Output(1, fmt.Sprintf("Message nr %d", i))
	}

	l.rotateBuckets()
	lines = strings.Split(buf.String(), "\n")
	buf.Reset()

	if !l.active {
		t.Error("suppression should be active after lots of unique messages")
	}
	if len(lines) < 10000 {
		t.Error("all lines should be shown the first time we see them")
	}

	// Now log the same lines with suppression

	for i := 0; i < 10000; i++ {
		_ = l.Output(1, fmt.Sprintf("Message nr %d", i))
	}

	l.rotateBuckets()
	lines = strings.Split(buf.String(), "\n")
	buf.Reset()

	if !l.active {
		t.Error("suppression should be active after lots of unique messages")
	}
	if len(lines) > 10 {
		t.Error("all lines should have been suppressed")
	}

	// Let some time pass

	for i := 0; i < 10; i++ {
		l.rotateBuckets()
	}
	if l.active {
		t.Error("suppression should disengage when time passes")
	}
}

func TestLastMessageRepetition(t *testing.T) {
	buf := new(bytes.Buffer)
	l := newSuppressingLogger(log.New(buf, "", 0), 10, 100, time.Hour)

	// Output a thousand identical messages, occasionally interspersed with
	// something else, ending with something specific
	for i := 0; i < 1000; i++ {
		_ = l.Output(1, "A log message")
		if i > 0 && i%200 == 0 {
			_ = l.Output(1, "Another log message")
		}
	}
	_ = l.Output(1, "Final message")

	lines := strings.Split(buf.String(), "\n")
	// Log lines should be roughly 5 * ("A log message" + "repeated 199
	// times" + "another log message") + "final message"
	if len(lines) > 20 {
		t.Error("expected fewer log lines than", len(lines))
	}

	// Check that we see the expected messages *somewhere* in there, and
	// that there are no repeats.

	var last string
	seen := make(map[string]int)
	for _, line := range lines {
		if line == last {
			t.Error("repeated line:", line)
		}
		last = line
		seen[line]++
	}

	if seen["A log message"] == 0 {
		t.Error(`should contain "A log message"`)
	}
	if seen["Another log message"] == 0 {
		t.Error(`should contain "Another log message"`)
	}
	if seen["Final message"] != 1 {
		t.Error(`should contain "Final message" once`)
	}
}
