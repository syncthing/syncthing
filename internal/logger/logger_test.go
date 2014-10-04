// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

package logger

import (
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
