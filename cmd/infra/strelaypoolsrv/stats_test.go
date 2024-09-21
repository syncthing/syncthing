// Copyright (C) 2015 Audrius Butkevicius and Contributors (see the CONTRIBUTORS file).

package main

import (
	"testing"
)

func TestMerge(t *testing.T) {
	if mergeValue(1001, 1000) != 1001 {
		t.Error("the computer says no")
	}

	if mergeValue(999, 1000) != 1000 {
		t.Error("the computer says no")
	}

	if mergeValue(1, 1000) != 1 {
		t.Error("the computer says no")
	}
}
