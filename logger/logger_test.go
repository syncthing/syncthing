// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package logger

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAPI(t *testing.T) {
	l := New()
	l.SetFlags(0)
	l.SetPrefix("testing")

	debug := 0
	l.AddHandler(LevelDebug, checkFunc(t, LevelDebug, "test 0", &debug))
	info := 0
	l.AddHandler(LevelInfo, checkFunc(t, LevelInfo, "test 1", &info))
	warn := 0
	l.AddHandler(LevelWarn, checkFunc(t, LevelWarn, "test 2", &warn))
	ok := 0
	l.AddHandler(LevelOK, checkFunc(t, LevelOK, "test 3", &ok))

	l.Debugf("test %d", 0)
	l.Debugln("test", 0)
	l.Infof("test %d", 1)
	l.Infoln("test", 1)
	l.Warnf("test %d", 2)
	l.Warnln("test", 2)
	l.Okf("test %d", 3)
	l.Okln("test", 3)

	if debug != 2 {
		t.Errorf("Debug handler called %d != 2 times", debug)
	}
	if info != 2 {
		t.Errorf("Info handler called %d != 2 times", info)
	}
	if warn != 2 {
		t.Errorf("Warn handler called %d != 2 times", warn)
	}
	if ok != 2 {
		t.Errorf("Ok handler called %d != 2 times", ok)
	}
}

func checkFunc(t *testing.T, expectl LogLevel, expectmsg string, counter *int) func(LogLevel, string) {
	return func(l LogLevel, msg string) {
		*counter++
		if l != expectl {
			t.Errorf("Incorrect message level %d != %d", l, expectl)
		}
		if !strings.HasSuffix(msg, expectmsg) {
			t.Errorf("%q does not end with %q", msg, expectmsg)
		}
	}
}

func TestPanic(t *testing.T) {
	bin, err := exec.LookPath(os.Args[0])
	if err != nil {
		t.Error(err)
	}
	log := filepath.Join(filepath.Dir(bin), "panic.log")
	os.Remove(log)

	tests := map[string]func(){
		"Test panic": func() { panic("Test panic") },
		"runtime error: assignment to entry in nil map": func() {
			var x map[int]int
			x[1] = 1
		},
		"runtime error: index out of range": func() {
			x := []int{
				1: 1,
			}
			x[2] = 1
		},
	}

	for msg, testfunc := range tests {
		_, err = os.Stat(log)
		if !os.IsNotExist(err) {
			t.Error(err)
		}

		done := make(chan bool)
		go func() {
			defer func() {
				r := recover()
				if r == nil {
					t.Error("Didn't repanic")
				}
				if fmt.Sprintf("%s", r) != msg {
					t.Errorf("Incorrect repanic message: %s != %s", r, msg)
				}
				done <- true
			}()
			defer New().CaptureAndRepanic()
			testfunc()
		}()

		<-done

		bytes, err := ioutil.ReadFile(log)
		if err != nil {
			t.Error(err)
		}
		content := string(bytes)

		if !strings.Contains(string(content), msg) {
			t.Errorf("Does not contain '%s':\n%v", msg, content)
		} else if !strings.Contains(string(content), "Stack trace:") {
			t.Errorf("Does not contain 'Stack trace:':\n%v", content)
		}
		os.Remove(log)
	}
}
