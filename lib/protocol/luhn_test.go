// Copyright (C) 2014 The Protocol Authors.

package protocol

import (
	"strings"
	"testing"
)

func TestLuhn32(t *testing.T) {
	c, err := luhn32("AB725E4GHIQPL3ZFGT")
	if err != nil {
		t.Fatal(err)
	}
	if c != 'G' {
		t.Errorf("Incorrect check digit %c != G", c)
	}

	_, err = luhn32("3734EJEKMRHWPZQTWYQ1")
	if err == nil {
		t.Error("Unexpected nil error")
	}
	if !strings.Contains(err.Error(), "'1'") {
		t.Errorf("luhn32 should have errored on digit '1', got %v", err)
	}
}
