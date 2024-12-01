// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package protocol

import "testing"

var (
	formatted   = "P56IOI7-MZJNU2Y-IQGDREY-DM2MGTI-MGL3BXN-PQ6W5BM-TBBZ4TJ-XZWICQ2"
	formatCases = []string{
		"P56IOI-7MZJNU-2IQGDR-EYDM2M-GTMGL3-BXNPQ6-W5BTBB-Z4TJXZ-WICQ",
		"P56IOI-7MZJNU2Y-IQGDR-EYDM2M-GTI-MGL3-BXNPQ6-W5BM-TBB-Z4TJXZ-WICQ2",
		"P56IOI7 MZJNU2I QGDREYD M2MGTMGL 3BXNPQ6W 5BTB BZ4T JXZWICQ",
		"P56IOI7 MZJNU2Y IQGDREY DM2MGTI MGL3BXN PQ6W5BM TBBZ4TJ XZWICQ2",
		"P56IOI7MZJNU2IQGDREYDM2MGTMGL3BXNPQ6W5BTBBZ4TJXZWICQ",
		"p56ioi7mzjnu2iqgdreydm2mgtmgl3bxnpq6w5btbbz4tjxzwicq",
		"P56IOI7MZJNU2YIQGDREYDM2MGTIMGL3BXNPQ6W5BMTBBZ4TJXZWICQ2",
		"P561017MZJNU2YIQGDREYDM2MGTIMGL3BXNPQ6W5BMT88Z4TJXZWICQ2",
		"p56ioi7mzjnu2yiqgdreydm2mgtimgl3bxnpq6w5bmtbbz4tjxzwicq2",
		"p561017mzjnu2yiqgdreydm2mgtimgl3bxnpq6w5bmt88z4tjxzwicq2",
	}
)

func TestFormatDeviceID(t *testing.T) {
	for i, tc := range formatCases {
		var id DeviceID
		err := id.UnmarshalText([]byte(tc))
		if err != nil {
			t.Errorf("#%d UnmarshalText(%q); %v", i, tc, err)
		} else if f := id.String(); f != formatted {
			t.Errorf("#%d FormatDeviceID(%q)\n\t%q !=\n\t%q", i, tc, f, formatted)
		}
	}
}

var validateCases = []struct {
	s  string
	ok bool
}{
	{"", true}, // Empty device ID, all zeroes
	{"a", false},
	{"P56IOI7-MZJNU2Y-IQGDREY-DM2MGTI-MGL3BXN-PQ6W5BM-TBBZ4TJ-XZWICQ2", true},
	{"P56IOI7-MZJNU2-IQGDREY-DM2MGT-MGL3BXN-PQ6W5B-TBBZ4TJ-XZWICQ", true},
	{"P56IOI7 MZJNU2I QGDREYD M2MGTMGL 3BXNPQ6W 5BTB BZ4T JXZWICQ", true},
	{"P56IOI7MZJNU2IQGDREYDM2MGTMGL3BXNPQ6W5BTBBZ4TJXZWICQ", true},
	{"P56IOI7MZJNU2IQGDREYDM2MGTMGL3BXNPQ6W5BTBBZ4TJXZWICQCCCC", false},
	{"p56ioi7mzjnu2iqgdreydm2mgtmgl3bxnpq6w5btbbz4tjxzwicq", true},
	{"p56ioi7mzjnu2iqgdreydm2mgtmgl3bxnpq6w5btbbz4tjxzwicqCCCC", false},
}

func TestValidateDeviceID(t *testing.T) {
	for _, tc := range validateCases {
		var id DeviceID
		err := id.UnmarshalText([]byte(tc.s))
		if (err == nil && !tc.ok) || (err != nil && tc.ok) {
			t.Errorf("ValidateDeviceID(%q); %v != %v", tc.s, err, tc.ok)
		}
	}
}

func TestMarshallingDeviceID(t *testing.T) {
	n0 := DeviceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 10, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	n1 := DeviceID{}
	n2 := DeviceID{}

	bs, _ := n0.MarshalText()
	if err := n1.UnmarshalText(bs); err != nil {
		t.Fatal(err)
	}
	bs, _ = n1.MarshalText()
	if err := n2.UnmarshalText(bs); err != nil {
		t.Fatal(err)
	}

	if n2.String() != n0.String() {
		t.Errorf("String marshalling error; %q != %q", n2.String(), n0.String())
	}
	if !n2.Equals(n0) {
		t.Error("Equals error")
	}
	if n2.Compare(n0) != 0 {
		t.Error("Compare error")
	}
}

func TestShortIDString(t *testing.T) {
	id, _ := DeviceIDFromString(formatted)

	sid := id.Short().String()
	// keep consistent with ShortIDStringLength in lib/protocol/deviceid.go
	if len(sid) != 7 {
		t.Errorf("Wrong length for short ID: got %d, want 7", len(sid))
	}

	want := formatted[:len(sid)]
	if sid != want {
		t.Errorf("Wrong short ID: got %q, want %q", sid, want)
	}
}

func TestDeviceIDFromBytes(t *testing.T) {
	id0, _ := DeviceIDFromString(formatted)
	id1, err := DeviceIDFromBytes(id0[:])
	if err != nil {
		t.Fatal(err)
	} else if id1.String() != formatted {
		t.Errorf("Wrong device ID, got %q, want %q", id1, formatted)
	}
}

var resStr string

func BenchmarkLuhnify(b *testing.B) {
	str := "ABCDEFGHIJABCDEFGHIJABCDEFGHIJABCDEFGHIJABCDEFGHIJAB"
	var err error
	for i := 0; i < b.N; i++ {
		resStr, err = luhnify(str)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnluhnify(b *testing.B) {
	str, _ := luhnify("ABCDEFGHIJABCDEFGHIJABCDEFGHIJABCDEFGHIJABCDEFGHIJAB")
	var err error
	for i := 0; i < b.N; i++ {
		resStr, err = unluhnify(str)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkChunkify(b *testing.B) {
	str := "ABCDEFGHIJABCDEFGHIJABCDEFGHIJABCDEFGHIJABCDEFGHIJAB"
	for i := 0; i < b.N; i++ {
		resStr = chunkify(str)
	}
}

func BenchmarkUnchunkify(b *testing.B) {
	str := chunkify("ABCDEFGHIJABCDEFGHIJABCDEFGHIJABCDEFGHIJABCDEFGHIJAB")
	for i := 0; i < b.N; i++ {
		resStr = unchunkify(str)
	}
}
