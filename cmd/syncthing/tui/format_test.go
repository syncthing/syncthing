// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package tui

import (
	"testing"
	"time"
)

func TestFormatBytes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{1048576, "1.0 MiB"},
		{1073741824, "1.0 GiB"},
		{1099511627776, "1.0 TiB"},
		{1649267441664, "1.5 TiB"},
	}
	for _, tt := range tests {
		if got := formatBytes(tt.input); got != tt.expected {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestFormatRate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input    float64
		expected string
	}{
		{0, "0 B/s"},
		{0.5, "0 B/s"},
		{-10, "0 B/s"},
		{1, "1 B/s"},
		{512, "512 B/s"},
		{1024, "1.0 KiB/s"},
		{1536, "1.5 KiB/s"},
		{1048576, "1.0 MiB/s"},
		{1073741824, "1.0 GiB/s"},
	}
	for _, tt := range tests {
		if got := formatRate(tt.input); got != tt.expected {
			t.Errorf("formatRate(%v) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input    time.Duration
		expected string
	}{
		{0, "0m"},
		{5 * time.Second, "0m"},
		{59 * time.Second, "0m"},
		{60 * time.Second, "1m"},
		{90 * time.Second, "1m"},
		{3600 * time.Second, "1h 0m"},
		{3661 * time.Second, "1h 1m"},
		{86400 * time.Second, "1d 0h"},
		{90061 * time.Second, "1d 1h"},
		{172800 * time.Second, "2d 0h"},
	}
	for _, tt := range tests {
		if got := formatDuration(tt.input); got != tt.expected {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestFormatPercent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input    float64
		expected string
	}{
		{0, "0.0%"},
		{50.5, "50.5%"},
		{99.9, "99.9%"},
		{99.99, "100.0%"},
		{100, "100%"},
		{100.1, "100%"},
		{-5, "0.0%"},
	}
	for _, tt := range tests {
		if got := formatPercent(tt.input); got != tt.expected {
			t.Errorf("formatPercent(%v) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestShortDeviceID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input    string
		expected string
	}{
		{"ABCDEFG-HIJKLMN-OPQRSTU-VWXYZ12-3456789-0ABCDEF-GHIJKLM-NOPQRST", "ABCDEFG"},
		{"ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890ABCDEFGHIJKLMNOPQRST", "ABCDEFG"},
		{"SHORT", "SHORT"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := shortDeviceID(tt.input); got != tt.expected {
			t.Errorf("shortDeviceID(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestPluralize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		n        int
		singular string
		plural   string
		expected string
	}{
		{0, "file", "files", "0 files"},
		{1, "file", "files", "1 file"},
		{2, "file", "files", "2 files"},
	}
	for _, tt := range tests {
		if got := pluralize(tt.n, tt.singular, tt.plural); got != tt.expected {
			t.Errorf("pluralize(%d, %q, %q) = %q, want %q", tt.n, tt.singular, tt.plural, got, tt.expected)
		}
	}
}
