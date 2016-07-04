// Copyright (C) 2016 The Protocol Authors.

// +build windows

package protocol

import "testing"

func TestFixupFiles(t *testing.T) {
	fs := []FileInfo{
		{Name: "ok"},  // This file is OK
		{Name: "b<d"}, // The rest should be marked as invalid
		{Name: "b?d"},
		{Name: "bad "},
	}

	fixupFiles("default", fs)

	if fs[0].IsInvalid() {
		t.Error("fs[0] should not be invalid")
	}
	for i := 1; i < len(fs); i++ {
		if !fs[i].IsInvalid() {
			t.Errorf("fs[%d] should be invalid", i)
		}
	}
}
