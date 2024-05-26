// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package stringutil

import (
	"strings"
	"time"
)

// UniqueTrimmedStrings returns a list of all unique strings in ss,
// in the order in which they first appear in ss, after trimming away
// leading and trailing spaces.
func UniqueTrimmedStrings(ss []string) []string {
	m := make(map[string]struct{}, len(ss))
	us := make([]string, 0, len(ss))
	for _, v := range ss {
		v = strings.Trim(v, " ")
		if _, ok := m[v]; ok {
			continue
		}
		m[v] = struct{}{}
		us = append(us, v)
	}

	return us
}

func NiceDurationString(d time.Duration) string {
	switch {
	case d > 24*time.Hour:
		d = d.Round(time.Hour)
	case d > time.Hour:
		d = d.Round(time.Minute)
	case d > time.Minute:
		d = d.Round(time.Second)
	case d > time.Second:
		d = d.Round(time.Millisecond)
	case d > time.Millisecond:
		d = d.Round(time.Microsecond)
	}
	return d.String()
}
