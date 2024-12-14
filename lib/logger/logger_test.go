// Copyright (C) 2014 Jakob Borg. All rights reserved. Use of this source code
// is governed by an MIT-style license that can be found in the LICENSE file.

package logger

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
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

type levelTest struct {
	sttrace  string
	hasIt    bool
	hasDotGo bool
	fn       func()
}

const (
	it    = "It!"
	dotGo = ".go"
	got   = true
	not   = false
)

var levelTests = []levelTest{
	// 1-8
	{"", not, not, func() { New().NewFacility("t1", "").Debugln(it) }},
	{"", not, not, func() { New().NewFacility("t1", "").Debugf("%s", it) }},
	{"", not, not, func() { New().NewFacility("t1", "").Verboseln(it) }},
	{"", not, not, func() { New().NewFacility("t1", "").Verbosef("%s", it) }},
	{"", got, not, func() { New().NewFacility("t1", "").Infoln(it) }},
	{"", got, not, func() { New().NewFacility("t1", "").Infof("%s", it) }},
	{"", got, not, func() { New().NewFacility("t1", "").Warnln(it) }},
	{"", got, not, func() { New().NewFacility("t1", "").Warnf("%s", it) }},
	// 9-16
	{"t2", got, got, func() { New().NewFacility("t2", "").Debugln(it) }},
	{"t2", got, got, func() { New().NewFacility("t2", "").Debugf("%s", it) }},
	{"t2", got, got, func() { New().NewFacility("t2", "").Verboseln(it) }},
	{"t2", got, got, func() { New().NewFacility("t2", "").Verbosef("%s", it) }},
	{"t2", got, got, func() { New().NewFacility("t2", "").Infoln(it) }},
	{"t2", got, got, func() { New().NewFacility("t2", "").Infof("%s", it) }},
	{"t2", got, got, func() { New().NewFacility("t2", "").Warnln(it) }},
	{"t2", got, got, func() { New().NewFacility("t2", "").Warnf("%s", it) }},
	// 17-24
	{"all", got, got, func() { New().NewFacility("t3", "").Debugln(it) }},
	{"all", got, got, func() { New().NewFacility("t3", "").Debugf("%s", it) }},
	{"all", got, got, func() { New().NewFacility("t3", "").Verboseln(it) }},
	{"all", got, got, func() { New().NewFacility("t3", "").Verbosef("%s", it) }},
	{"all", got, got, func() { New().NewFacility("t3", "").Infoln(it) }},
	{"all", got, got, func() { New().NewFacility("t3", "").Infof("%s", it) }},
	{"all", got, got, func() { New().NewFacility("t3", "").Warnln(it) }},
	{"all", got, got, func() { New().NewFacility("t3", "").Warnf("%s", it) }},
	// 25-32
	{"t4:debug", got, got, func() { New().NewFacility("t4", "").Debugln(it) }},
	{"t4:debug", got, got, func() { New().NewFacility("t4", "").Debugf("%s", it) }},
	{"t4:debug", got, got, func() { New().NewFacility("t4", "").Verboseln(it) }},
	{"t4:debug", got, got, func() { New().NewFacility("t4", "").Verbosef("%s", it) }},
	{"t4:debug", got, got, func() { New().NewFacility("t4", "").Infoln(it) }},
	{"t4:debug", got, got, func() { New().NewFacility("t4", "").Infof("%s", it) }},
	{"t4:debug", got, got, func() { New().NewFacility("t4", "").Warnln(it) }},
	{"t4:debug", got, got, func() { New().NewFacility("t4", "").Warnf("%s", it) }},
	// 33-40
	{"t5:verbose", not, not, func() { New().NewFacility("t5", "").Debugln(it) }},
	{"t5:verbose", not, not, func() { New().NewFacility("t5", "").Debugf("%s", it) }},
	{"t5:verbose", got, not, func() { New().NewFacility("t5", "").Verboseln(it) }},
	{"t5:verbose", got, not, func() { New().NewFacility("t5", "").Verbosef("%s", it) }},
	{"t5:verbose", got, not, func() { New().NewFacility("t5", "").Infoln(it) }},
	{"t5:verbose", got, not, func() { New().NewFacility("t5", "").Infof("%s", it) }},
	{"t5:verbose", got, not, func() { New().NewFacility("t5", "").Warnln(it) }},
	{"t5:verbose", got, not, func() { New().NewFacility("t5", "").Warnf("%s", it) }},
	// 41-48
	{"t6:info", not, not, func() { New().NewFacility("t6", "").Debugln(it) }},
	{"t6:info", not, not, func() { New().NewFacility("t6", "").Debugf("%s", it) }},
	{"t6:info", not, not, func() { New().NewFacility("t6", "").Verboseln(it) }},
	{"t6:info", not, not, func() { New().NewFacility("t6", "").Verbosef("%s", it) }},
	{"t6:info", got, not, func() { New().NewFacility("t6", "").Infoln(it) }},
	{"t6:info", got, not, func() { New().NewFacility("t6", "").Infof("%s", it) }},
	{"t6:info", got, not, func() { New().NewFacility("t6", "").Warnln(it) }},
	{"t6:info", got, not, func() { New().NewFacility("t6", "").Warnf("%s", it) }},
	// 49-56
	{"t7:warn", not, not, func() { New().NewFacility("t7", "").Debugln(it) }},
	{"t7:warn", not, not, func() { New().NewFacility("t7", "").Debugf("%s", it) }},
	{"t7:warn", not, not, func() { New().NewFacility("t7", "").Verboseln(it) }},
	{"t7:warn", not, not, func() { New().NewFacility("t7", "").Verbosef("%s", it) }},
	{"t7:warn", not, not, func() { New().NewFacility("t7", "").Infoln(it) }},
	{"t7:warn", not, not, func() { New().NewFacility("t7", "").Infof("%s", it) }},
	{"t7:warn", got, not, func() { New().NewFacility("t7", "").Warnln(it) }},
	{"t7:warn", got, not, func() { New().NewFacility("t7", "").Warnf("%s", it) }},
	// 57-64
	{"t8:error", not, not, func() { New().NewFacility("t8", "").Debugln(it) }},
	{"t8:error", not, not, func() { New().NewFacility("t8", "").Debugf("%s", it) }},
	{"t8:error", not, not, func() { New().NewFacility("t8", "").Verboseln(it) }},
	{"t8:error", not, not, func() { New().NewFacility("t8", "").Verbosef("%s", it) }},
	{"t8:error", not, not, func() { New().NewFacility("t8", "").Infoln(it) }},
	{"t8:error", not, not, func() { New().NewFacility("t8", "").Infof("%s", it) }},
	{"t8:error", not, not, func() { New().NewFacility("t8", "").Warnln(it) }},
	{"t8:error", not, not, func() { New().NewFacility("t8", "").Warnf("%s", it) }},
	// 65-72
	{"all:warn,t9:info", not, not, func() { New().NewFacility("t9", "").Debugln(it) }},
	{"all:warn,t9:info", not, not, func() { New().NewFacility("t9", "").Debugf("%s", it) }},
	{"all:warn,t9:info", not, not, func() { New().NewFacility("t9", "").Verboseln(it) }},
	{"all:warn,t9:info", not, not, func() { New().NewFacility("t9", "").Verbosef("%s", it) }},
	{"all:warn,t9:info", got, not, func() { New().NewFacility("t9", "").Infoln(it) }},
	{"all:warn,t9:info", got, not, func() { New().NewFacility("t9", "").Infof("%s", it) }},
	{"all:warn,t9:info", got, not, func() { New().NewFacility("t9", "").Warnln(it) }},
	{"all:warn,t9:info", got, not, func() { New().NewFacility("t9", "").Warnf("%s", it) }},
	// 73-80
	{"all:debug,t10:info", not, not, func() { New().NewFacility("t10", "").Debugln(it) }},
	{"all:debug,t10:info", not, not, func() { New().NewFacility("t10", "").Debugf("%s", it) }},
	{"all:debug,t10:info", not, not, func() { New().NewFacility("t10", "").Verboseln(it) }},
	{"all:debug,t10:info", not, not, func() { New().NewFacility("t10", "").Verbosef("%s", it) }},
	{"all:debug,t10:info", got, got, func() { New().NewFacility("t10", "").Infoln(it) }},
	{"all:debug,t10:info", got, got, func() { New().NewFacility("t10", "").Infof("%s", it) }},
	{"all:debug,t10:info", got, got, func() { New().NewFacility("t10", "").Warnln(it) }},
	{"all:debug,t10:info", got, got, func() { New().NewFacility("t10", "").Warnf("%s", it) }},
	// 81-88
	{"all:warn,t11:info", not, not, func() { New().NewFacility("!t11", "").Debugln(it) }},
	{"all:warn,t11:info", not, not, func() { New().NewFacility("!t11", "").Debugf("%s", it) }},
	{"all:warn,t11:info", not, not, func() { New().NewFacility("!t11", "").Verboseln(it) }},
	{"all:warn,t11:info", not, not, func() { New().NewFacility("!t11", "").Verbosef("%s", it) }},
	{"all:warn,t11:info", not, not, func() { New().NewFacility("!t11", "").Infoln(it) }},
	{"all:warn,t11:info", not, not, func() { New().NewFacility("!t11", "").Infof("%s", it) }},
	{"all:warn,t11:info", got, not, func() { New().NewFacility("!t11", "").Warnln(it) }},
	{"all:warn,t11:info", got, not, func() { New().NewFacility("!t11", "").Warnf("%s", it) }},
	// 89-96
	{"all:verbose,t12:info", not, not, func() { New().NewFacility("!t12", "").Debugln(it) }},
	{"all:verbose,t12:info", not, not, func() { New().NewFacility("!t12", "").Debugf("%s", it) }},
	{"all:verbose,t12:info", got, not, func() { New().NewFacility("!t12", "").Verboseln(it) }},
	{"all:verbose,t12:info", got, not, func() { New().NewFacility("!t12", "").Verbosef("%s", it) }},
	{"all:verbose,t12:info", got, not, func() { New().NewFacility("!t12", "").Infoln(it) }},
	{"all:verbose,t12:info", got, not, func() { New().NewFacility("!t12", "").Infof("%s", it) }},
	{"all:verbose,t12:info", got, not, func() { New().NewFacility("!t12", "").Warnln(it) }},
	{"all:verbose,t12:info", got, not, func() { New().NewFacility("!t12", "").Warnf("%s", it) }},
	// 97-104
	{"all:debug,t13:info", got, got, func() { New().NewFacility("!t13", "").Debugln(it) }},
	{"all:debug,t13:info", got, got, func() { New().NewFacility("!t13", "").Debugf("%s", it) }},
	{"all:debug,t13:info", got, got, func() { New().NewFacility("!t13", "").Verboseln(it) }},
	{"all:debug,t13:info", got, got, func() { New().NewFacility("!t13", "").Verbosef("%s", it) }},
	{"all:debug,t13:info", got, got, func() { New().NewFacility("!t13", "").Infoln(it) }},
	{"all:debug,t13:info", got, got, func() { New().NewFacility("!t13", "").Infof("%s", it) }},
	{"all:debug,t13:info", got, got, func() { New().NewFacility("!t13", "").Warnln(it) }},
	{"all:debug,t13:info", got, got, func() { New().NewFacility("!t13", "").Warnf("%s", it) }},
}

var delims = []string{",", ";", " ", "\t"}

func TestLogLevel(t *testing.T) {
	// LOGGER_DISCARD needs to be unset for this test to pass
	t.Setenv("LOGGER_DISCARD", "")
	for i, test := range levelTests {
		for _, delim := range delims {
			sttrace := strings.ReplaceAll(test.sttrace, ",", delim)
			t.Setenv("STTRACE", sttrace)
			got := captureStdout(test.fn)
			if strings.Contains(got, it) != test.hasIt {
				t.Errorf("Test %d: STTRACE=%q: got %q, want %q", i+1, sttrace, got, it)
			}
		}
	}
}

func TestLogFlags(t *testing.T) {
	// LOGGER_DISCARD needs to be unset for this test to pass
	t.Setenv("LOGGER_DISCARD", "")
	for i, test := range levelTests {
		for _, delim := range delims {
			sttrace := strings.ReplaceAll(test.sttrace, ",", delim)
			t.Setenv("STTRACE", sttrace)
			got := captureStdout(test.fn)
			if strings.Contains(got, dotGo) != test.hasDotGo {
				t.Errorf("Test %d: STTRACE=%q: got %q, want %q", i+1, sttrace, got, dotGo)
			}
		}
	}
}

var levels = []LogLevel{
	LevelDebug,
	LevelVerbose,
	LevelInfo,
	LevelWarn,
	LevelError,
}

type isEnabledTest struct {
	facility   string
	sttrace    string
	minEnabled LogLevel
	fn         func() Logger
}

var isEnabledTests = []isEnabledTest{
	{"t1", "", LevelError + 1, func() Logger { return New().NewFacility("t1", "") }},
	{"t2", "t2", LevelDebug, func() Logger { return New().NewFacility("t2", "") }},
	{"t3", "all", LevelDebug, func() Logger { return New().NewFacility("t3", "") }},
	{"t4", "t4:debug", LevelDebug, func() Logger { return New().NewFacility("t4", "") }},
	{"t5", "t5:verbose", LevelVerbose, func() Logger { return New().NewFacility("t5", "") }},
	{"t6", "t6:info", LevelInfo, func() Logger { return New().NewFacility("t6", "") }},
	{"t7", "t7:warn", LevelWarn, func() Logger { return New().NewFacility("t7", "") }},
	{"t8", "t8:error", LevelError, func() Logger { return New().NewFacility("t8", "") }},
}

func TestIsEnabledFor(t *testing.T) {
	for i, test := range isEnabledTests {
		for _, delim := range delims {
			sttrace := strings.ReplaceAll(test.sttrace, ",", delim)
			t.Setenv("STTRACE", sttrace)
			l := test.fn().(*facilityLogger)
			for _, level := range levels {
				got := l.IsEnabledFor(test.facility, level)
				want := test.minEnabled <= level
				if got != want {
					t.Errorf("Test %d: STTRACE=%q IsEnabledFor(%q, %v): got %v, want %v", i+1, sttrace, test.facility, level, got, want)
				}
			}
		}
	}
}

type effectiveLevelTest struct {
	facility       string
	sttrace        string
	effectiveLevel LogLevel
	fn             func() Logger
}

var effectiveLevelTests = []effectiveLevelTest{
	{"t1", "", LevelError, func() Logger { return New().NewFacility("t1", "") }},
	{"t2", "t2", LevelDebug, func() Logger { return New().NewFacility("t2", "") }},
	{"t3", "all", LevelDebug, func() Logger { return New().NewFacility("t3", "") }},
	{"t4", "t4:debug", LevelDebug, func() Logger { return New().NewFacility("t4", "") }},
	{"t5", "t5:verbose", LevelVerbose, func() Logger { return New().NewFacility("t5", "") }},
	{"t6", "t6:info", LevelInfo, func() Logger { return New().NewFacility("t6", "") }},
	{"t7", "t7:warn", LevelWarn, func() Logger { return New().NewFacility("t7", "") }},
	{"t8", "t8:error", LevelError, func() Logger { return New().NewFacility("t8", "") }},
	{"t9", "all:info,t9:debug", LevelDebug, func() Logger { return New().NewFacility("t9", "") }},
	{"t10", "all:info,t10:verbose", LevelVerbose, func() Logger { return New().NewFacility("t10", "") }},
	{"t11", "all:info,t11:info", LevelInfo, func() Logger { return New().NewFacility("t11", "") }},
	{"t12", "all:info,t12:warn", LevelWarn, func() Logger { return New().NewFacility("t12", "") }},
	{"t13", "all:info,t13:error", LevelError, func() Logger { return New().NewFacility("t13", "") }},
	{"t14", "all:info,!t14:debug", LevelInfo, func() Logger { return New().NewFacility("t4", "") }},
	{"t15", "all:info,!t15:verbose", LevelInfo, func() Logger { return New().NewFacility("t15", "") }},
	{"t16", "all:info,!t16:info", LevelInfo, func() Logger { return New().NewFacility("t16", "") }},
	{"t17", "all:info,!t17:warn", LevelInfo, func() Logger { return New().NewFacility("t17", "") }},
	{"t18", "all:info,!t18:error", LevelInfo, func() Logger { return New().NewFacility("t18", "") }},
}

func TestEffectiveLevel(t *testing.T) {
	for i, test := range effectiveLevelTests {
		for _, delim := range delims {
			sttrace := strings.ReplaceAll(test.sttrace, ",", delim)
			t.Setenv("STTRACE", sttrace)
			l := test.fn().(*facilityLogger)
			got := l.EffectiveLevel(test.facility)
			want := test.effectiveLevel
			if got != want {
				t.Errorf("Test %d: STTRACE=%q EffectiveLevel(%q): got %v, want %v", i+1, sttrace, test.facility, got, want)
			}
		}
	}
}

func captureStdout(f func()) string {
	stdout := os.Stdout

	r, w, err := os.Pipe()
	if err != nil {
		panic("pipe failed: " + err.Error())
	}

	os.Stdout = w

	output := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		output <- buf.String()
	}()

	f()

	_ = w.Close()
	os.Stdout = stdout
	return <-output
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
