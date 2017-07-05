package io_test

import (
	"encoding/binary"
	"io/ioutil"
	"math/rand"
	"testing"
	"time"

	"github.com/gogo/protobuf/test"
	example "github.com/gogo/protobuf/test/example"

	"github.com/gogo/protobuf/io"
)

func BenchmarkUint32DelimWriterMarshaller(b *testing.B) {
	w := io.NewUint32DelimitedWriter(ioutil.Discard, binary.BigEndian)
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	msg := example.NewPopulatedA(r, true)

	for i := 0; i < b.N; i++ {
		if err := w.WriteMsg(msg); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUint32DelimWriterFallback(b *testing.B) {
	w := io.NewUint32DelimitedWriter(ioutil.Discard, binary.BigEndian)
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	msg := test.NewPopulatedNinOptNative(r, true)

	for i := 0; i < b.N; i++ {
		if err := w.WriteMsg(msg); err != nil {
			b.Fatal(err)
		}
	}
}
