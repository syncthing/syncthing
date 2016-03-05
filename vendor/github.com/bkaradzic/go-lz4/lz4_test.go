package lz4

import (
	"bytes"
	"io/ioutil"
	"testing"
)

var testfile, _ = ioutil.ReadFile("testdata/pg1661.txt")

func roundtrip(t *testing.T, input []byte) {

	dst, err := Encode(nil, input)
	if err != nil {
		t.Errorf("got error during compression: %s", err)
	}

	output, err := Decode(nil, dst)

	if err != nil {
		t.Errorf("got error during decompress: %s", err)
	}

	if !bytes.Equal(output, input) {
		t.Errorf("roundtrip failed")
	}
}

func TestEmpty(t *testing.T) {
	roundtrip(t, nil)
}

func TestLengths(t *testing.T) {

	for i := 0; i < 1024; i++ {
		roundtrip(t, testfile[:i])
	}

	for i := 1024; i < 4096; i += 23 {
		roundtrip(t, testfile[:i])
	}
}

func TestWords(t *testing.T) {
	roundtrip(t, testfile)
}

func BenchmarkLZ4Encode(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Encode(nil, testfile)
	}
}

func BenchmarkLZ4Decode(b *testing.B) {

	var compressed, _ = Encode(nil, testfile)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		Decode(nil, compressed)
	}
}
