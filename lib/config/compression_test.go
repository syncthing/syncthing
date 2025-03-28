// Copyright (C) 2015 The Protocol Authors.

package config

import "testing"

func TestCompressionMarshal(t *testing.T) {
	uTestcases := []struct {
		s string
		c Compression
	}{
		{"true", CompressionMetadata},
		{"false", CompressionNever},
		{"never", CompressionNever},
		{"metadata", CompressionMetadata},
		{"always", CompressionAlways},
		{"whatever", CompressionMetadata},
	}

	mTestcases := []struct {
		s string
		c Compression
	}{
		{"never", CompressionNever},
		{"metadata", CompressionMetadata},
		{"always", CompressionAlways},
	}

	var c Compression
	for _, tc := range uTestcases {
		err := c.UnmarshalText([]byte(tc.s))
		if err != nil {
			t.Error(err)
		}
		if c != tc.c {
			t.Errorf("%s unmarshalled to %d, not %d", tc.s, c, tc.c)
		}
	}

	for _, tc := range mTestcases {
		bs, err := tc.c.MarshalText()
		if err != nil {
			t.Error(err)
		}
		if s := string(bs); s != tc.s {
			t.Errorf("%d marshalled to %q, not %q", tc.c, s, tc.s)
		}
	}
}
