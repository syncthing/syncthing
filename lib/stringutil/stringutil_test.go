// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package stringutil

import (
	"testing"
)

func TestUniqueStrings(t *testing.T) {
	tests := []struct {
		input    []string
		expected []string
	}{
		{
			[]string{"a", "b"},
			[]string{"a", "b"},
		},
		{
			[]string{"a", "a"},
			[]string{"a"},
		},
		{
			[]string{"a", "a", "a", "a"},
			[]string{"a"},
		},
		{
			nil,
			nil,
		},
		{
			[]string{"       a     ", "     a  ", "b        ", "    b"},
			[]string{"a", "b"},
		},
	}

	for _, test := range tests {
		result := UniqueTrimmedStrings(test.input)
		if len(result) != len(test.expected) {
			t.Errorf("%s != %s", result, test.expected)
		}
		for i := range result {
			if test.expected[i] != result[i] {
				t.Errorf("%s != %s", result, test.expected)
			}
		}
	}
}
