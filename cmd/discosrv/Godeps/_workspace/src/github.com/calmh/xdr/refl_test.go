// Copyright (C) 2014 Jakob Borg. All rights reserved. Use of this source code
// is governed by an MIT-style license that can be found in the LICENSE file.

// +build refl

package xdr_test

import (
	"bytes"
	"testing"

	refl "github.com/davecgh/go-xdr/xdr"
)

func TestCompareMarshals(t *testing.T) {
	e0 := s.MarshalXDR()
	e1, err := refl.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Compare(e0, e1) != 0 {
		t.Fatalf("Encoding mismatch;\n\t%x (this)\n\t%x (refl)", e0, e1)
	}
}

func BenchmarkReflMarshal(b *testing.B) {
	var err error
	for i := 0; i < b.N; i++ {
		res, err = refl.Marshal(s)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReflUnmarshal(b *testing.B) {
	var t XDRBenchStruct
	for i := 0; i < b.N; i++ {
		_, err := refl.Unmarshal(e, &t)
		if err != nil {
			b.Fatal(err)
		}
	}
}
