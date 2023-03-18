// Copyright (C) 2014 Jakob Borg. All rights reserved. Use of this source code
// is governed by an MIT-style license that can be found in the LICENSE file.

package logger

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"strings"
	"testing"
	"time"
)

func TestAPI(t *testing.T) {
	l := New()
	l.SetFlags(0)
	l.SetPrefix("testing")

	debug := 0
	l.AddHandler(LevelDebug, checkFunc(t, LevelDebug, &debug))
	info := 0
	l.AddHandler(LevelInfo, checkFunc(t, LevelInfo, &info))
	warn := 0
	l.AddHandler(LevelWarn, checkFunc(t, LevelWarn, &warn))

	l.Debugf("test %d", 0)
	l.Debugln("test", 0)
	l.Infof("test %d", 1)
	l.Infoln("test", 1)
	l.Warnf("test %d", 3)
	l.Warnln("test", 3)

	if debug != 6 {
		t.Errorf("Debug handler called %d != 8 times", debug)
	}
	if info != 4 {
		t.Errorf("Info handler called %d != 6 times", info)
	}
	if warn != 2 {
		t.Errorf("Warn handler called %d != 2 times", warn)
	}
}

func checkFunc(t *testing.T, expectl LogLevel, counter *int) func(LogLevel, string) {
	return func(l LogLevel, msg string) {
		*counter++
		if l < expectl {
			t.Errorf("Incorrect message level %d < %d", l, expectl)
		}
	}
}

func TestFacilityDebugging(t *testing.T) {
	l := New()
	l.SetFlags(0)

	msgs := 0
	l.AddHandler(LevelDebug, func(l LogLevel, msg string) {
		msgs++
		if strings.Contains(msg, "f1") {
			t.Fatal("Should not get message for facility f1")
		}
	})

	f0 := l.NewFacility("f0", "foo#0")
	f1 := l.NewFacility("f1", "foo#1")

	l.SetDebug("f0", true)
	l.SetDebug("f1", false)

	f0.Debugln("Debug line from f0")
	f1.Debugln("Debug line from f1")

	if msgs != 1 {
		t.Fatalf("Incorrect number of messages, %d != 1", msgs)
	}
}

func TestRecorder(t *testing.T) {
	l := New()
	l.SetFlags(0)

	// Keep the last five warnings or higher, no special initial handling.
	r0 := NewRecorder(l, LevelWarn, 5, 0)
	// Keep the last ten infos or higher, with the first three being permanent.
	r1 := NewRecorder(l, LevelInfo, 10, 3)

	// Log a bunch of messages.
	for i := 0; i < 15; i++ {
		l.Debugf("Debug#%d", i)
		l.Infof("Info#%d", i)
		l.Warnf("Warn#%d", i)
	}

	// r0 should contain the last five warnings
	lines := r0.Since(time.Time{})
	if len(lines) != 5 {
		t.Fatalf("Incorrect length %d != 5", len(lines))
	}
	for i := 0; i < 5; i++ {
		expected := fmt.Sprintf("Warn#%d", i+10)
		if lines[i].Message != expected {
			t.Error("Incorrect warning in r0:", lines[i].Message, "!=", expected)
		}
	}

	// r0 should contain:
	// - The first three messages
	// - A "..." marker
	// - The last six messages
	// (totalling ten)
	lines = r1.Since(time.Time{})
	if len(lines) != 10 {
		t.Fatalf("Incorrect length %d != 10", len(lines))
	}
	expected := []string{
		"Info#0",
		"Warn#0",
		"Info#1",
		"...",
		"Info#12",
		"Warn#12",
		"Info#13",
		"Warn#13",
		"Info#14",
		"Warn#14",
	}
	for i := 0; i < 10; i++ {
		if lines[i].Message != expected[i] {
			t.Error("Incorrect warning in r0:", lines[i].Message, "!=", expected[i])
		}
	}

	// Check that since works
	now := time.Now()

	time.Sleep(time.Millisecond)

	lines = r1.Since(now)
	if len(lines) != 0 {
		t.Error("unexpected lines")
	}

	l.Infoln("hah")

	lines = r1.Since(now)
	if len(lines) != 1 {
		t.Fatalf("unexpected line count: %d", len(lines))
	}
	if lines[0].Message != "hah" {
		t.Errorf("incorrect line: %s", lines[0].Message)
	}
}

func TestStackLevel(t *testing.T) {
	b := new(bytes.Buffer)
	l := newLogger(b)

	l.SetFlags(log.Lshortfile)
	l.Infoln("testing")
	res := b.String()

	if !strings.Contains(res, "logger_test.go:") {
		t.Logf("%q", res)
		t.Error("Should identify this file as the source (bad level?)")
	}
}

func TestControlStripper(t *testing.T) {
	b := new(bytes.Buffer)
	l := newLogger(controlStripper{b})

	l.Infoln("testing\x07testing\ntesting")
	res := b.String()

	if !strings.Contains(res, "testing testing\ntesting") {
		t.Logf("%q", res)
		t.Error("Control character should become space")
	}
	if strings.Contains(res, "\x07") {
		t.Logf("%q", res)
		t.Error("Control character should be removed")
	}
}

func BenchmarkLog(b *testing.B) {
	l := newLogger(controlStripper{io.Discard})
	benchmarkLogger(b, l)
}

func BenchmarkLogNoStripper(b *testing.B) {
	l := newLogger(io.Discard)
	benchmarkLogger(b, l)
}

func benchmarkLogger(b *testing.B, l Logger) {
	l.SetFlags(log.Lshortfile | log.Lmicroseconds)
	l.SetPrefix("ABCDEFG")

	for i := 0; i < b.N; i++ {
		l.Infoln("This is a somewhat representative log line")
		l.Infof("This is a log line with a couple of formatted things: %d %q", 42, "a file name maybe, who knows?")
	}

	b.ReportAllocs()
	b.SetBytes(2) // log entries per iteration
}
