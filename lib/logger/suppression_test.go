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

	// Now cause suppression

	for i := 0; i < 10000; i++ {
		_ = l.Output(1, fmt.Sprintf("Message nr %d", i))
	}

	l.rotateBuckets()
	lines := strings.Split(buf.String(), "\n")
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
