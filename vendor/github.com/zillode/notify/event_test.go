// Copyright (c) 2014-2015 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

package notify

import (
	"sort"
	"strings"
	"testing"
)

// S is a workaround for random event strings concatenation order.
func s(s string) string {
	z := strings.Split(s, "|")
	sort.StringSlice(z).Sort()
	return strings.Join(z, "|")
}

// This test is not safe to run in parallel with others.
func TestEventString(t *testing.T) {
	cases := map[Event]string{
		Create:                  "notify.Create",
		Create | Remove:         "notify.Create|notify.Remove",
		Create | Remove | Write: "notify.Create|notify.Remove|notify.Write",
		Create | Write | Rename: "notify.Create|notify.Rename|notify.Write",
	}
	for e, str := range cases {
		if s := s(e.String()); s != str {
			t.Errorf("want s=%s; got %s (e=%#x)", str, s, e)
		}
	}
}
