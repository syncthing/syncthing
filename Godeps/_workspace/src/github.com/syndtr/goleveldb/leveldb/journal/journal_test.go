// Copyright 2011 The LevelDB-Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Taken from: https://code.google.com/p/leveldb-go/source/browse/leveldb/record/record_test.go?r=df1fa28f7f3be6c3935548169002309c12967135
// License, authors and contributors informations can be found at bellow URLs respectively:
// 	https://code.google.com/p/leveldb-go/source/browse/LICENSE
//	https://code.google.com/p/leveldb-go/source/browse/AUTHORS
//  https://code.google.com/p/leveldb-go/source/browse/CONTRIBUTORS

package journal

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"strings"
	"testing"
)

type dropper struct {
	t *testing.T
}

func (d dropper) Drop(err error) {
	d.t.Log(err)
}

func short(s string) string {
	if len(s) < 64 {
		return s
	}
	return fmt.Sprintf("%s...(skipping %d bytes)...%s", s[:20], len(s)-40, s[len(s)-20:])
}

// big returns a string of length n, composed of repetitions of partial.
func big(partial string, n int) string {
	return strings.Repeat(partial, n/len(partial)+1)[:n]
}

func TestEmpty(t *testing.T) {
	buf := new(bytes.Buffer)
	r := NewReader(buf, dropper{t}, true, true)
	if _, err := r.Next(); err != io.EOF {
		t.Fatalf("got %v, want %v", err, io.EOF)
	}
}

func testGenerator(t *testing.T, reset func(), gen func() (string, bool)) {
	buf := new(bytes.Buffer)

	reset()
	w := NewWriter(buf)
	for {
		s, ok := gen()
		if !ok {
			break
		}
		ww, err := w.Next()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ww.Write([]byte(s)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	reset()
	r := NewReader(buf, dropper{t}, true, true)
	for {
		s, ok := gen()
		if !ok {
			break
		}
		rr, err := r.Next()
		if err != nil {
			t.Fatal(err)
		}
		x, err := ioutil.ReadAll(rr)
		if err != nil {
			t.Fatal(err)
		}
		if string(x) != s {
			t.Fatalf("got %q, want %q", short(string(x)), short(s))
		}
	}
	if _, err := r.Next(); err != io.EOF {
		t.Fatalf("got %v, want %v", err, io.EOF)
	}
}

func testLiterals(t *testing.T, s []string) {
	var i int
	reset := func() {
		i = 0
	}
	gen := func() (string, bool) {
		if i == len(s) {
			return "", false
		}
		i++
		return s[i-1], true
	}
	testGenerator(t, reset, gen)
}

func TestMany(t *testing.T) {
	const n = 1e5
	var i int
	reset := func() {
		i = 0
	}
	gen := func() (string, bool) {
		if i == n {
			return "", false
		}
		i++
		return fmt.Sprintf("%d.", i-1), true
	}
	testGenerator(t, reset, gen)
}

func TestRandom(t *testing.T) {
	const n = 1e2
	var (
		i int
		r *rand.Rand
	)
	reset := func() {
		i, r = 0, rand.New(rand.NewSource(0))
	}
	gen := func() (string, bool) {
		if i == n {
			return "", false
		}
		i++
		return strings.Repeat(string(uint8(i)), r.Intn(2*blockSize+16)), true
	}
	testGenerator(t, reset, gen)
}

func TestBasic(t *testing.T) {
	testLiterals(t, []string{
		strings.Repeat("a", 1000),
		strings.Repeat("b", 97270),
		strings.Repeat("c", 8000),
	})
}

func TestBoundary(t *testing.T) {
	for i := blockSize - 16; i < blockSize+16; i++ {
		s0 := big("abcd", i)
		for j := blockSize - 16; j < blockSize+16; j++ {
			s1 := big("ABCDE", j)
			testLiterals(t, []string{s0, s1})
			testLiterals(t, []string{s0, "", s1})
			testLiterals(t, []string{s0, "x", s1})
		}
	}
}

func TestFlush(t *testing.T) {
	buf := new(bytes.Buffer)
	w := NewWriter(buf)
	// Write a couple of records. Everything should still be held
	// in the record.Writer buffer, so that buf.Len should be 0.
	w0, _ := w.Next()
	w0.Write([]byte("0"))
	w1, _ := w.Next()
	w1.Write([]byte("11"))
	if got, want := buf.Len(), 0; got != want {
		t.Fatalf("buffer length #0: got %d want %d", got, want)
	}
	// Flush the record.Writer buffer, which should yield 17 bytes.
	// 17 = 2*7 + 1 + 2, which is two headers and 1 + 2 payload bytes.
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.Len(), 17; got != want {
		t.Fatalf("buffer length #1: got %d want %d", got, want)
	}
	// Do another write, one that isn't large enough to complete the block.
	// The write should not have flowed through to buf.
	w2, _ := w.Next()
	w2.Write(bytes.Repeat([]byte("2"), 10000))
	if got, want := buf.Len(), 17; got != want {
		t.Fatalf("buffer length #2: got %d want %d", got, want)
	}
	// Flushing should get us up to 10024 bytes written.
	// 10024 = 17 + 7 + 10000.
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.Len(), 10024; got != want {
		t.Fatalf("buffer length #3: got %d want %d", got, want)
	}
	// Do a bigger write, one that completes the current block.
	// We should now have 32768 bytes (a complete block), without
	// an explicit flush.
	w3, _ := w.Next()
	w3.Write(bytes.Repeat([]byte("3"), 40000))
	if got, want := buf.Len(), 32768; got != want {
		t.Fatalf("buffer length #4: got %d want %d", got, want)
	}
	// Flushing should get us up to 50038 bytes written.
	// 50038 = 10024 + 2*7 + 40000. There are two headers because
	// the one record was split into two chunks.
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.Len(), 50038; got != want {
		t.Fatalf("buffer length #5: got %d want %d", got, want)
	}
	// Check that reading those records give the right lengths.
	r := NewReader(buf, dropper{t}, true, true)
	wants := []int64{1, 2, 10000, 40000}
	for i, want := range wants {
		rr, _ := r.Next()
		n, err := io.Copy(ioutil.Discard, rr)
		if err != nil {
			t.Fatalf("read #%d: %v", i, err)
		}
		if n != want {
			t.Fatalf("read #%d: got %d bytes want %d", i, n, want)
		}
	}
}

func TestNonExhaustiveRead(t *testing.T) {
	const n = 100
	buf := new(bytes.Buffer)
	p := make([]byte, 10)
	rnd := rand.New(rand.NewSource(1))

	w := NewWriter(buf)
	for i := 0; i < n; i++ {
		length := len(p) + rnd.Intn(3*blockSize)
		s := string(uint8(i)) + "123456789abcdefgh"
		ww, _ := w.Next()
		ww.Write([]byte(big(s, length)))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	r := NewReader(buf, dropper{t}, true, true)
	for i := 0; i < n; i++ {
		rr, _ := r.Next()
		_, err := io.ReadFull(rr, p)
		if err != nil {
			t.Fatal(err)
		}
		want := string(uint8(i)) + "123456789"
		if got := string(p); got != want {
			t.Fatalf("read #%d: got %q want %q", i, got, want)
		}
	}
}

func TestStaleReader(t *testing.T) {
	buf := new(bytes.Buffer)

	w := NewWriter(buf)
	w0, err := w.Next()
	if err != nil {
		t.Fatal(err)
	}
	w0.Write([]byte("0"))
	w1, err := w.Next()
	if err != nil {
		t.Fatal(err)
	}
	w1.Write([]byte("11"))
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	r := NewReader(buf, dropper{t}, true, true)
	r0, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}
	r1, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}
	p := make([]byte, 1)
	if _, err := r0.Read(p); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale read #0: unexpected error: %v", err)
	}
	if _, err := r1.Read(p); err != nil {
		t.Fatalf("fresh read #1: got %v want nil error", err)
	}
	if p[0] != '1' {
		t.Fatalf("fresh read #1: byte contents: got '%c' want '1'", p[0])
	}
}

func TestStaleWriter(t *testing.T) {
	buf := new(bytes.Buffer)

	w := NewWriter(buf)
	w0, err := w.Next()
	if err != nil {
		t.Fatal(err)
	}
	w1, err := w.Next()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w0.Write([]byte("0")); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale write #0: unexpected error: %v", err)
	}
	if _, err := w1.Write([]byte("11")); err != nil {
		t.Fatalf("fresh write #1: got %v want nil error", err)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if _, err := w1.Write([]byte("0")); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale write #1: unexpected error: %v", err)
	}
}

func TestCorrupt_MissingLastBlock(t *testing.T) {
	buf := new(bytes.Buffer)

	w := NewWriter(buf)

	// First record.
	ww, err := w.Next()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ww.Write(bytes.Repeat([]byte("0"), blockSize-1024)); err != nil {
		t.Fatalf("write #0: unexpected error: %v", err)
	}

	// Second record.
	ww, err = w.Next()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ww.Write(bytes.Repeat([]byte("0"), blockSize-headerSize)); err != nil {
		t.Fatalf("write #1: unexpected error: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	// Cut the last block.
	b := buf.Bytes()[:blockSize]
	r := NewReader(bytes.NewReader(b), dropper{t}, false, true)

	// First read.
	rr, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}
	n, err := io.Copy(ioutil.Discard, rr)
	if err != nil {
		t.Fatalf("read #0: %v", err)
	}
	if n != blockSize-1024 {
		t.Fatalf("read #0: got %d bytes want %d", n, blockSize-1024)
	}

	// Second read.
	rr, err = r.Next()
	if err != nil {
		t.Fatal(err)
	}
	n, err = io.Copy(ioutil.Discard, rr)
	if err != io.ErrUnexpectedEOF {
		t.Fatalf("read #1: unexpected error: %v", err)
	}

	if _, err := r.Next(); err != io.EOF {
		t.Fatalf("last next: unexpected error: %v", err)
	}
}

func TestCorrupt_CorruptedFirstBlock(t *testing.T) {
	buf := new(bytes.Buffer)

	w := NewWriter(buf)

	// First record.
	ww, err := w.Next()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ww.Write(bytes.Repeat([]byte("0"), blockSize/2)); err != nil {
		t.Fatalf("write #0: unexpected error: %v", err)
	}

	// Second record.
	ww, err = w.Next()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ww.Write(bytes.Repeat([]byte("0"), blockSize-headerSize)); err != nil {
		t.Fatalf("write #1: unexpected error: %v", err)
	}

	// Third record.
	ww, err = w.Next()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ww.Write(bytes.Repeat([]byte("0"), (blockSize-headerSize)+1)); err != nil {
		t.Fatalf("write #2: unexpected error: %v", err)
	}

	// Fourth record.
	ww, err = w.Next()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ww.Write(bytes.Repeat([]byte("0"), (blockSize-headerSize)+2)); err != nil {
		t.Fatalf("write #3: unexpected error: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	b := buf.Bytes()
	// Corrupting block #0.
	for i := 0; i < 1024; i++ {
		b[i] = '1'
	}

	r := NewReader(bytes.NewReader(b), dropper{t}, false, true)

	// First read (third record).
	rr, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}
	n, err := io.Copy(ioutil.Discard, rr)
	if err != nil {
		t.Fatalf("read #0: %v", err)
	}
	if want := int64(blockSize-headerSize) + 1; n != want {
		t.Fatalf("read #0: got %d bytes want %d", n, want)
	}

	// Second read (fourth record).
	rr, err = r.Next()
	if err != nil {
		t.Fatal(err)
	}
	n, err = io.Copy(ioutil.Discard, rr)
	if err != nil {
		t.Fatalf("read #1: %v", err)
	}
	if want := int64(blockSize-headerSize) + 2; n != want {
		t.Fatalf("read #1: got %d bytes want %d", n, want)
	}

	if _, err := r.Next(); err != io.EOF {
		t.Fatalf("last next: unexpected error: %v", err)
	}
}

func TestCorrupt_CorruptedMiddleBlock(t *testing.T) {
	buf := new(bytes.Buffer)

	w := NewWriter(buf)

	// First record.
	ww, err := w.Next()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ww.Write(bytes.Repeat([]byte("0"), blockSize/2)); err != nil {
		t.Fatalf("write #0: unexpected error: %v", err)
	}

	// Second record.
	ww, err = w.Next()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ww.Write(bytes.Repeat([]byte("0"), blockSize-headerSize)); err != nil {
		t.Fatalf("write #1: unexpected error: %v", err)
	}

	// Third record.
	ww, err = w.Next()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ww.Write(bytes.Repeat([]byte("0"), (blockSize-headerSize)+1)); err != nil {
		t.Fatalf("write #2: unexpected error: %v", err)
	}

	// Fourth record.
	ww, err = w.Next()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ww.Write(bytes.Repeat([]byte("0"), (blockSize-headerSize)+2)); err != nil {
		t.Fatalf("write #3: unexpected error: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	b := buf.Bytes()
	// Corrupting block #1.
	for i := 0; i < 1024; i++ {
		b[blockSize+i] = '1'
	}

	r := NewReader(bytes.NewReader(b), dropper{t}, false, true)

	// First read (first record).
	rr, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}
	n, err := io.Copy(ioutil.Discard, rr)
	if err != nil {
		t.Fatalf("read #0: %v", err)
	}
	if want := int64(blockSize / 2); n != want {
		t.Fatalf("read #0: got %d bytes want %d", n, want)
	}

	// Second read (second record).
	rr, err = r.Next()
	if err != nil {
		t.Fatal(err)
	}
	n, err = io.Copy(ioutil.Discard, rr)
	if err != io.ErrUnexpectedEOF {
		t.Fatalf("read #1: unexpected error: %v", err)
	}

	// Third read (fourth record).
	rr, err = r.Next()
	if err != nil {
		t.Fatal(err)
	}
	n, err = io.Copy(ioutil.Discard, rr)
	if err != nil {
		t.Fatalf("read #2: %v", err)
	}
	if want := int64(blockSize-headerSize) + 2; n != want {
		t.Fatalf("read #2: got %d bytes want %d", n, want)
	}

	if _, err := r.Next(); err != io.EOF {
		t.Fatalf("last next: unexpected error: %v", err)
	}
}

func TestCorrupt_CorruptedLastBlock(t *testing.T) {
	buf := new(bytes.Buffer)

	w := NewWriter(buf)

	// First record.
	ww, err := w.Next()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ww.Write(bytes.Repeat([]byte("0"), blockSize/2)); err != nil {
		t.Fatalf("write #0: unexpected error: %v", err)
	}

	// Second record.
	ww, err = w.Next()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ww.Write(bytes.Repeat([]byte("0"), blockSize-headerSize)); err != nil {
		t.Fatalf("write #1: unexpected error: %v", err)
	}

	// Third record.
	ww, err = w.Next()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ww.Write(bytes.Repeat([]byte("0"), (blockSize-headerSize)+1)); err != nil {
		t.Fatalf("write #2: unexpected error: %v", err)
	}

	// Fourth record.
	ww, err = w.Next()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ww.Write(bytes.Repeat([]byte("0"), (blockSize-headerSize)+2)); err != nil {
		t.Fatalf("write #3: unexpected error: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	b := buf.Bytes()
	// Corrupting block #3.
	for i := len(b) - 1; i > len(b)-1024; i-- {
		b[i] = '1'
	}

	r := NewReader(bytes.NewReader(b), dropper{t}, false, true)

	// First read (first record).
	rr, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}
	n, err := io.Copy(ioutil.Discard, rr)
	if err != nil {
		t.Fatalf("read #0: %v", err)
	}
	if want := int64(blockSize / 2); n != want {
		t.Fatalf("read #0: got %d bytes want %d", n, want)
	}

	// Second read (second record).
	rr, err = r.Next()
	if err != nil {
		t.Fatal(err)
	}
	n, err = io.Copy(ioutil.Discard, rr)
	if err != nil {
		t.Fatalf("read #1: %v", err)
	}
	if want := int64(blockSize - headerSize); n != want {
		t.Fatalf("read #1: got %d bytes want %d", n, want)
	}

	// Third read (third record).
	rr, err = r.Next()
	if err != nil {
		t.Fatal(err)
	}
	n, err = io.Copy(ioutil.Discard, rr)
	if err != nil {
		t.Fatalf("read #2: %v", err)
	}
	if want := int64(blockSize-headerSize) + 1; n != want {
		t.Fatalf("read #2: got %d bytes want %d", n, want)
	}

	// Fourth read (fourth record).
	rr, err = r.Next()
	if err != nil {
		t.Fatal(err)
	}
	n, err = io.Copy(ioutil.Discard, rr)
	if err != io.ErrUnexpectedEOF {
		t.Fatalf("read #3: unexpected error: %v", err)
	}

	if _, err := r.Next(); err != io.EOF {
		t.Fatalf("last next: unexpected error: %v", err)
	}
}

func TestCorrupt_FirstChuckLengthOverflow(t *testing.T) {
	buf := new(bytes.Buffer)

	w := NewWriter(buf)

	// First record.
	ww, err := w.Next()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ww.Write(bytes.Repeat([]byte("0"), blockSize/2)); err != nil {
		t.Fatalf("write #0: unexpected error: %v", err)
	}

	// Second record.
	ww, err = w.Next()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ww.Write(bytes.Repeat([]byte("0"), blockSize-headerSize)); err != nil {
		t.Fatalf("write #1: unexpected error: %v", err)
	}

	// Third record.
	ww, err = w.Next()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ww.Write(bytes.Repeat([]byte("0"), (blockSize-headerSize)+1)); err != nil {
		t.Fatalf("write #2: unexpected error: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	b := buf.Bytes()
	// Corrupting record #1.
	x := blockSize
	binary.LittleEndian.PutUint16(b[x+4:], 0xffff)

	r := NewReader(bytes.NewReader(b), dropper{t}, false, true)

	// First read (first record).
	rr, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}
	n, err := io.Copy(ioutil.Discard, rr)
	if err != nil {
		t.Fatalf("read #0: %v", err)
	}
	if want := int64(blockSize / 2); n != want {
		t.Fatalf("read #0: got %d bytes want %d", n, want)
	}

	// Second read (second record).
	rr, err = r.Next()
	if err != nil {
		t.Fatal(err)
	}
	n, err = io.Copy(ioutil.Discard, rr)
	if err != io.ErrUnexpectedEOF {
		t.Fatalf("read #1: unexpected error: %v", err)
	}

	if _, err := r.Next(); err != io.EOF {
		t.Fatalf("last next: unexpected error: %v", err)
	}
}

func TestCorrupt_MiddleChuckLengthOverflow(t *testing.T) {
	buf := new(bytes.Buffer)

	w := NewWriter(buf)

	// First record.
	ww, err := w.Next()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ww.Write(bytes.Repeat([]byte("0"), blockSize/2)); err != nil {
		t.Fatalf("write #0: unexpected error: %v", err)
	}

	// Second record.
	ww, err = w.Next()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ww.Write(bytes.Repeat([]byte("0"), blockSize-headerSize)); err != nil {
		t.Fatalf("write #1: unexpected error: %v", err)
	}

	// Third record.
	ww, err = w.Next()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ww.Write(bytes.Repeat([]byte("0"), (blockSize-headerSize)+1)); err != nil {
		t.Fatalf("write #2: unexpected error: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	b := buf.Bytes()
	// Corrupting record #1.
	x := blockSize/2 + headerSize
	binary.LittleEndian.PutUint16(b[x+4:], 0xffff)

	r := NewReader(bytes.NewReader(b), dropper{t}, false, true)

	// First read (first record).
	rr, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}
	n, err := io.Copy(ioutil.Discard, rr)
	if err != nil {
		t.Fatalf("read #0: %v", err)
	}
	if want := int64(blockSize / 2); n != want {
		t.Fatalf("read #0: got %d bytes want %d", n, want)
	}

	// Second read (third record).
	rr, err = r.Next()
	if err != nil {
		t.Fatal(err)
	}
	n, err = io.Copy(ioutil.Discard, rr)
	if err != nil {
		t.Fatalf("read #1: %v", err)
	}
	if want := int64(blockSize-headerSize) + 1; n != want {
		t.Fatalf("read #1: got %d bytes want %d", n, want)
	}

	if _, err := r.Next(); err != io.EOF {
		t.Fatalf("last next: unexpected error: %v", err)
	}
}
