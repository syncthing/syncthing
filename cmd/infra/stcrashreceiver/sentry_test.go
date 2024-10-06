// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"fmt"
	"os"
	"testing"
)

func TestParseReport(t *testing.T) {
	bs, err := os.ReadFile("_testdata/panic.log")
	if err != nil {
		t.Fatal(err)
	}

	pkt, err := parseCrashReport("1/2/345", bs)
	if err != nil {
		t.Fatal(err)
	}

	bs, err = pkt.JSON()
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("%s\n", bs)
}

func TestCrashReportFingerprint(t *testing.T) {
	cases := []struct {
		message, exp string
		ldb          bool
	}{
		{
			message: "panic: leveldb/table: corruption on data-block (pos=51308946): checksum mismatch, want=0xa89f9aa0 got=0xd27cc4c7 [file=004003.ldb]",
			exp:     "panic: leveldb/table: corruption on data-block (pos=x): checksum mismatch, want=0xX got=0xX [file=x.ldb]",
			ldb:     true,
		},
		{
			message: "panic: leveldb/table: corruption on table-footer (pos=248): bad magic number [file=001370.ldb]",
			exp:     "panic: leveldb/table: corruption on table-footer (pos=x): bad magic number [file=x.ldb]",
			ldb:     true,
		},
		{
			message: "panic: runtime error: slice bounds out of range [4294967283:4194304]",
			exp:     "panic: runtime error: slice bounds out of range [x]",
		},
		{
			message: "panic: runtime error: slice bounds out of range [-2:]",
			exp:     "panic: runtime error: slice bounds out of range [x]",
		},
		{
			message: "panic: runtime error: slice bounds out of range [:4294967283] with capacity 32768",
			exp:     "panic: runtime error: slice bounds out of range [x] with capacity x",
		},
		{
			message: "panic: runtime error: index out of range [0] with length 0",
			exp:     "panic: runtime error: index out of range [x] with length x",
		},
		{
			message: `panic: leveldb: internal key "\x01", len=1: invalid length`,
			exp:     `panic: leveldb: internal key "x", len=x: invalid length`,
			ldb:     true,
		},
		{
			message: `panic: write /var/syncthing/config/index-v0.14.0.db/2732813.log: cannot allocate memory`,
			exp:     `panic: write x: cannot allocate memory`,
			ldb:     true,
		},
		{
			message: `panic: filling Blocks: read C:\Users\Serv-Resp-Tizayuca\AppData\Local\Syncthing\index-v0.14.0.db\006561.ldb: Error de datos (comprobación de redundancia cíclica).`,
			exp:     `panic: filling Blocks: read x: Error de datos (comprobación de redundancia cíclica).`,
			ldb:     true,
		},
	}

	for i, tc := range cases {
		fingerprint := crashReportFingerprint(tc.message)

		expLen := 2
		if tc.ldb {
			expLen = 1
		}
		if l := len(fingerprint); l != expLen {
			t.Errorf("tc %v: Unexpected fingerprint length: %v != %v", i, l, expLen)
		} else if msg := fingerprint[expLen-1]; msg != tc.exp {
			t.Errorf("tc %v:\n\"%v\" !=\n\"%v\"", i, msg, tc.exp)
		}
	}
}
