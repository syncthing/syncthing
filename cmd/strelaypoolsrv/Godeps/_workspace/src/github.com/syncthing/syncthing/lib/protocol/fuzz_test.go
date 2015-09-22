// Copyright (C) 2015 The Protocol Authors.

// +build gofuzz

package protocol

import (
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"testing/quick"
)

// This can be used to generate a corpus of valid messages as a starting point
// for the fuzzer.
func TestGenerateCorpus(t *testing.T) {
	t.Skip("Use to generate initial corpus only")

	n := 0
	check := func(idx IndexMessage) bool {
		for i := range idx.Options {
			if len(idx.Options[i].Key) > 64 {
				idx.Options[i].Key = idx.Options[i].Key[:64]
			}
		}
		hdr := header{
			version:     0,
			msgID:       42,
			msgType:     messageTypeIndex,
			compression: false,
		}

		msgBs := idx.MustMarshalXDR()

		buf := make([]byte, 8)
		binary.BigEndian.PutUint32(buf, encodeHeader(hdr))
		binary.BigEndian.PutUint32(buf[4:], uint32(len(msgBs)))
		buf = append(buf, msgBs...)

		ioutil.WriteFile(fmt.Sprintf("testdata/corpus/test-%03d.xdr", n), buf, 0644)
		n++
		return true
	}

	if err := quick.Check(check, &quick.Config{MaxCount: 1000}); err != nil {
		t.Fatal(err)
	}
}

// Tests any crashers found by the fuzzer, for closer investigation.
func TestCrashers(t *testing.T) {
	testFiles(t, "testdata/crashers")
}

// Tests the entire corpus, which should PASS before the fuzzer starts
// fuzzing.
func TestCorpus(t *testing.T) {
	testFiles(t, "testdata/corpus")
}

func testFiles(t *testing.T, dir string) {
	fd, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	crashers, err := fd.Readdirnames(-1)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range crashers {
		if strings.HasSuffix(name, ".output") {
			continue
		}
		if strings.HasSuffix(name, ".quoted") {
			continue
		}

		t.Log(name)
		crasher, err := ioutil.ReadFile(dir + "/" + name)
		if err != nil {
			t.Fatal(err)
		}

		Fuzz(crasher)
	}
}
