// Copyright 2011 The Snappy-Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package snappy

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var download = flag.Bool("download", false, "If true, download any missing files before running benchmarks")

func TestMaxEncodedLenOfMaxBlockSize(t *testing.T) {
	got := maxEncodedLenOfMaxBlockSize
	want := MaxEncodedLen(maxBlockSize)
	if got != want {
		t.Fatalf("got %d, want %d", got, want)
	}
}

func cmp(a, b []byte) error {
	if bytes.Equal(a, b) {
		return nil
	}
	if len(a) != len(b) {
		return fmt.Errorf("got %d bytes, want %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			return fmt.Errorf("byte #%d: got 0x%02x, want 0x%02x", i, a[i], b[i])
		}
	}
	return nil
}

func roundtrip(b, ebuf, dbuf []byte) error {
	d, err := Decode(dbuf, Encode(ebuf, b))
	if err != nil {
		return fmt.Errorf("decoding error: %v", err)
	}
	if err := cmp(d, b); err != nil {
		return fmt.Errorf("roundtrip mismatch: %v", err)
	}
	return nil
}

func TestEmpty(t *testing.T) {
	if err := roundtrip(nil, nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestSmallCopy(t *testing.T) {
	for _, ebuf := range [][]byte{nil, make([]byte, 20), make([]byte, 64)} {
		for _, dbuf := range [][]byte{nil, make([]byte, 20), make([]byte, 64)} {
			for i := 0; i < 32; i++ {
				s := "aaaa" + strings.Repeat("b", i) + "aaaabbbb"
				if err := roundtrip([]byte(s), ebuf, dbuf); err != nil {
					t.Errorf("len(ebuf)=%d, len(dbuf)=%d, i=%d: %v", len(ebuf), len(dbuf), i, err)
				}
			}
		}
	}
}

func TestSmallRand(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for n := 1; n < 20000; n += 23 {
		b := make([]byte, n)
		for i := range b {
			b[i] = uint8(rng.Intn(256))
		}
		if err := roundtrip(b, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSmallRegular(t *testing.T) {
	for n := 1; n < 20000; n += 23 {
		b := make([]byte, n)
		for i := range b {
			b[i] = uint8(i%10 + 'a')
		}
		if err := roundtrip(b, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
}

func TestInvalidVarint(t *testing.T) {
	testCases := []struct {
		desc  string
		input string
	}{{
		"invalid varint, final byte has continuation bit set",
		"\xff",
	}, {
		"invalid varint, value overflows uint64",
		"\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\x00",
	}, {
		// https://github.com/google/snappy/blob/master/format_description.txt
		// says that "the stream starts with the uncompressed length [as a
		// varint] (up to a maximum of 2^32 - 1)".
		"valid varint (as uint64), but value overflows uint32",
		"\x80\x80\x80\x80\x10",
	}}

	for _, tc := range testCases {
		input := []byte(tc.input)
		if _, err := DecodedLen(input); err != ErrCorrupt {
			t.Errorf("%s: DecodedLen: got %v, want ErrCorrupt", tc.desc, err)
		}
		if _, err := Decode(nil, input); err != ErrCorrupt {
			t.Errorf("%s: Decode: got %v, want ErrCorrupt", tc.desc, err)
		}
	}
}

func TestDecode(t *testing.T) {
	lit40Bytes := make([]byte, 40)
	for i := range lit40Bytes {
		lit40Bytes[i] = byte(i)
	}
	lit40 := string(lit40Bytes)

	testCases := []struct {
		desc    string
		input   string
		want    string
		wantErr error
	}{{
		`decodedLen=0; valid input`,
		"\x00",
		"",
		nil,
	}, {
		`decodedLen=3; tagLiteral, 0-byte length; length=3; valid input`,
		"\x03" + "\x08\xff\xff\xff",
		"\xff\xff\xff",
		nil,
	}, {
		`decodedLen=2; tagLiteral, 0-byte length; length=3; not enough dst bytes`,
		"\x02" + "\x08\xff\xff\xff",
		"",
		ErrCorrupt,
	}, {
		`decodedLen=3; tagLiteral, 0-byte length; length=3; not enough src bytes`,
		"\x03" + "\x08\xff\xff",
		"",
		ErrCorrupt,
	}, {
		`decodedLen=40; tagLiteral, 0-byte length; length=40; valid input`,
		"\x28" + "\x9c" + lit40,
		lit40,
		nil,
	}, {
		`decodedLen=1; tagLiteral, 1-byte length; not enough length bytes`,
		"\x01" + "\xf0",
		"",
		ErrCorrupt,
	}, {
		`decodedLen=3; tagLiteral, 1-byte length; length=3; valid input`,
		"\x03" + "\xf0\x02\xff\xff\xff",
		"\xff\xff\xff",
		nil,
	}, {
		`decodedLen=1; tagLiteral, 2-byte length; not enough length bytes`,
		"\x01" + "\xf4\x00",
		"",
		ErrCorrupt,
	}, {
		`decodedLen=3; tagLiteral, 2-byte length; length=3; valid input`,
		"\x03" + "\xf4\x02\x00\xff\xff\xff",
		"\xff\xff\xff",
		nil,
	}, {
		`decodedLen=1; tagLiteral, 3-byte length; not enough length bytes`,
		"\x01" + "\xf8\x00\x00",
		"",
		ErrCorrupt,
	}, {
		`decodedLen=3; tagLiteral, 3-byte length; length=3; valid input`,
		"\x03" + "\xf8\x02\x00\x00\xff\xff\xff",
		"\xff\xff\xff",
		nil,
	}, {
		`decodedLen=1; tagLiteral, 4-byte length; not enough length bytes`,
		"\x01" + "\xfc\x00\x00\x00",
		"",
		ErrCorrupt,
	}, {
		`decodedLen=1; tagLiteral, 4-byte length; length=3; not enough dst bytes`,
		"\x01" + "\xfc\x02\x00\x00\x00\xff\xff\xff",
		"",
		ErrCorrupt,
	}, {
		`decodedLen=4; tagLiteral, 4-byte length; length=3; not enough src bytes`,
		"\x04" + "\xfc\x02\x00\x00\x00\xff",
		"",
		ErrCorrupt,
	}, {
		`decodedLen=3; tagLiteral, 4-byte length; length=3; valid input`,
		"\x03" + "\xfc\x02\x00\x00\x00\xff\xff\xff",
		"\xff\xff\xff",
		nil,
	}, {
		`decodedLen=4; tagCopy1, 1 extra length|offset byte; not enough extra bytes`,
		"\x04" + "\x01",
		"",
		ErrCorrupt,
	}, {
		`decodedLen=4; tagCopy2, 2 extra length|offset bytes; not enough extra bytes`,
		"\x04" + "\x02\x00",
		"",
		ErrCorrupt,
	}, {
		`decodedLen=4; tagCopy4; unsupported COPY_4 tag`,
		"\x04" + "\x03\x00\x00\x00\x00",
		"",
		errUnsupportedCopy4Tag,
	}, {
		`decodedLen=4; tagLiteral (4 bytes "abcd"); valid input`,
		"\x04" + "\x0cabcd",
		"abcd",
		nil,
	}, {
		`decodedLen=13; tagLiteral (4 bytes "abcd"); tagCopy1; length=9 offset=4; valid input`,
		"\x0d" + "\x0cabcd" + "\x15\x04",
		"abcdabcdabcda",
		nil,
	}, {
		`decodedLen=8; tagLiteral (4 bytes "abcd"); tagCopy1; length=4 offset=4; valid input`,
		"\x08" + "\x0cabcd" + "\x01\x04",
		"abcdabcd",
		nil,
	}, {
		`decodedLen=8; tagLiteral (4 bytes "abcd"); tagCopy1; length=4 offset=2; valid input`,
		"\x08" + "\x0cabcd" + "\x01\x02",
		"abcdcdcd",
		nil,
	}, {
		`decodedLen=8; tagLiteral (4 bytes "abcd"); tagCopy1; length=4 offset=1; valid input`,
		"\x08" + "\x0cabcd" + "\x01\x01",
		"abcddddd",
		nil,
	}, {
		`decodedLen=8; tagLiteral (4 bytes "abcd"); tagCopy1; length=4 offset=0; zero offset`,
		"\x08" + "\x0cabcd" + "\x01\x00",
		"",
		ErrCorrupt,
	}, {
		`decodedLen=9; tagLiteral (4 bytes "abcd"); tagCopy1; length=4 offset=4; inconsistent dLen`,
		"\x09" + "\x0cabcd" + "\x01\x04",
		"",
		ErrCorrupt,
	}, {
		`decodedLen=8; tagLiteral (4 bytes "abcd"); tagCopy1; length=4 offset=5; offset too large`,
		"\x08" + "\x0cabcd" + "\x01\x05",
		"",
		ErrCorrupt,
	}, {
		`decodedLen=7; tagLiteral (4 bytes "abcd"); tagCopy1; length=4 offset=4; length too large`,
		"\x07" + "\x0cabcd" + "\x01\x04",
		"",
		ErrCorrupt,
	}, {
		`decodedLen=6; tagLiteral (4 bytes "abcd"); tagCopy2; length=2 offset=3; valid input`,
		"\x06" + "\x0cabcd" + "\x06\x03\x00",
		"abcdbc",
		nil,
	}}

	const (
		// notPresentXxx defines a range of byte values [0xa0, 0xc5) that are
		// not present in either the input or the output. It is written to dBuf
		// to check that Decode does not write bytes past the end of
		// dBuf[:dLen].
		//
		// The magic number 37 was chosen because it is prime. A more 'natural'
		// number like 32 might lead to a false negative if, for example, a
		// byte was incorrectly copied 4*8 bytes later.
		notPresentBase = 0xa0
		notPresentLen  = 37
	)

	var dBuf [100]byte
loop:
	for i, tc := range testCases {
		input := []byte(tc.input)
		for _, x := range input {
			if notPresentBase <= x && x < notPresentBase+notPresentLen {
				t.Errorf("#%d (%s): input shouldn't contain %#02x\ninput: % x", i, tc.desc, x, input)
				continue loop
			}
		}

		dLen, n := binary.Uvarint(input)
		if n <= 0 {
			t.Errorf("#%d (%s): invalid varint-encoded dLen", i, tc.desc)
			continue
		}
		if dLen > uint64(len(dBuf)) {
			t.Errorf("#%d (%s): dLen %d is too large", i, tc.desc, dLen)
			continue
		}

		for j := range dBuf {
			dBuf[j] = byte(notPresentBase + j%notPresentLen)
		}
		g, gotErr := Decode(dBuf[:], input)
		if got := string(g); got != tc.want || gotErr != tc.wantErr {
			t.Errorf("#%d (%s):\ngot  %q, %v\nwant %q, %v",
				i, tc.desc, got, gotErr, tc.want, tc.wantErr)
			continue
		}
		for j, x := range dBuf {
			if uint64(j) < dLen {
				continue
			}
			if w := byte(notPresentBase + j%notPresentLen); x != w {
				t.Errorf("#%d (%s): Decode overrun: dBuf[%d] was modified: got %#02x, want %#02x\ndBuf: % x",
					i, tc.desc, j, x, w, dBuf)
				continue loop
			}
		}
	}
}

// TestDecodeLengthOffset tests decoding an encoding of the form literal +
// copy-length-offset + literal. For example: "abcdefghijkl" + "efghij" + "AB".
func TestDecodeLengthOffset(t *testing.T) {
	const (
		prefix = "abcdefghijklmnopqr"
		suffix = "ABCDEFGHIJKLMNOPQR"

		// notPresentXxx defines a range of byte values [0xa0, 0xc5) that are
		// not present in either the input or the output. It is written to
		// gotBuf to check that Decode does not write bytes past the end of
		// gotBuf[:totalLen].
		//
		// The magic number 37 was chosen because it is prime. A more 'natural'
		// number like 32 might lead to a false negative if, for example, a
		// byte was incorrectly copied 4*8 bytes later.
		notPresentBase = 0xa0
		notPresentLen  = 37
	)
	var gotBuf, wantBuf, inputBuf [128]byte
	for length := 1; length <= 18; length++ {
		for offset := 1; offset <= 18; offset++ {
		loop:
			for suffixLen := 0; suffixLen <= 18; suffixLen++ {
				totalLen := len(prefix) + length + suffixLen

				inputLen := binary.PutUvarint(inputBuf[:], uint64(totalLen))
				inputBuf[inputLen] = tagLiteral + 4*byte(len(prefix)-1)
				inputLen++
				inputLen += copy(inputBuf[inputLen:], prefix)
				inputBuf[inputLen+0] = tagCopy2 + 4*byte(length-1)
				inputBuf[inputLen+1] = byte(offset)
				inputBuf[inputLen+2] = 0x00
				inputLen += 3
				if suffixLen > 0 {
					inputBuf[inputLen] = tagLiteral + 4*byte(suffixLen-1)
					inputLen++
					inputLen += copy(inputBuf[inputLen:], suffix[:suffixLen])
				}
				input := inputBuf[:inputLen]

				for i := range gotBuf {
					gotBuf[i] = byte(notPresentBase + i%notPresentLen)
				}
				got, err := Decode(gotBuf[:], input)
				if err != nil {
					t.Errorf("length=%d, offset=%d; suffixLen=%d: %v", length, offset, suffixLen, err)
					continue
				}

				wantLen := 0
				wantLen += copy(wantBuf[wantLen:], prefix)
				for i := 0; i < length; i++ {
					wantBuf[wantLen] = wantBuf[wantLen-offset]
					wantLen++
				}
				wantLen += copy(wantBuf[wantLen:], suffix[:suffixLen])
				want := wantBuf[:wantLen]

				for _, x := range input {
					if notPresentBase <= x && x < notPresentBase+notPresentLen {
						t.Errorf("length=%d, offset=%d; suffixLen=%d: input shouldn't contain %#02x\ninput: % x",
							length, offset, suffixLen, x, input)
						continue loop
					}
				}
				for i, x := range gotBuf {
					if i < totalLen {
						continue
					}
					if w := byte(notPresentBase + i%notPresentLen); x != w {
						t.Errorf("length=%d, offset=%d; suffixLen=%d; totalLen=%d: "+
							"Decode overrun: gotBuf[%d] was modified: got %#02x, want %#02x\ngotBuf: % x",
							length, offset, suffixLen, totalLen, i, x, w, gotBuf)
						continue loop
					}
				}
				for _, x := range want {
					if notPresentBase <= x && x < notPresentBase+notPresentLen {
						t.Errorf("length=%d, offset=%d; suffixLen=%d: want shouldn't contain %#02x\nwant: % x",
							length, offset, suffixLen, x, want)
						continue loop
					}
				}

				if !bytes.Equal(got, want) {
					t.Errorf("length=%d, offset=%d; suffixLen=%d:\ninput % x\ngot   % x\nwant  % x",
						length, offset, suffixLen, input, got, want)
					continue
				}
			}
		}
	}
}

func TestDecodeGoldenInput(t *testing.T) {
	src, err := ioutil.ReadFile("testdata/pi.txt.rawsnappy")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got, err := Decode(nil, src)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	want, err := ioutil.ReadFile("testdata/pi.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if err := cmp(got, want); err != nil {
		t.Fatal(err)
	}
}

// TestSlowForwardCopyOverrun tests the "expand the pattern" algorithm
// described in decode_amd64.s and its claim of a 10 byte overrun worst case.
func TestSlowForwardCopyOverrun(t *testing.T) {
	const base = 100

	for length := 1; length < 18; length++ {
		for offset := 1; offset < 18; offset++ {
			highWaterMark := base
			d := base
			l := length
			o := offset

			// makeOffsetAtLeast8
			for o < 8 {
				if end := d + 8; highWaterMark < end {
					highWaterMark = end
				}
				l -= o
				d += o
				o += o
			}

			// fixUpSlowForwardCopy
			a := d
			d += l

			// finishSlowForwardCopy
			for l > 0 {
				if end := a + 8; highWaterMark < end {
					highWaterMark = end
				}
				a += 8
				l -= 8
			}

			dWant := base + length
			overrun := highWaterMark - dWant
			if d != dWant || overrun < 0 || 10 < overrun {
				t.Errorf("length=%d, offset=%d: d and overrun: got (%d, %d), want (%d, something in [0, 10])",
					length, offset, d, overrun, dWant)
			}
		}
	}
}

// TestEncodeNoiseThenRepeats encodes input for which the first half is very
// incompressible and the second half is very compressible. The encoded form's
// length should be closer to 50% of the original length than 100%.
func TestEncodeNoiseThenRepeats(t *testing.T) {
	for _, origLen := range []int{32 * 1024, 256 * 1024, 2048 * 1024} {
		src := make([]byte, origLen)
		rng := rand.New(rand.NewSource(1))
		firstHalf, secondHalf := src[:origLen/2], src[origLen/2:]
		for i := range firstHalf {
			firstHalf[i] = uint8(rng.Intn(256))
		}
		for i := range secondHalf {
			secondHalf[i] = uint8(i >> 8)
		}
		dst := Encode(nil, src)
		if got, want := len(dst), origLen*3/4; got >= want {
			t.Errorf("origLen=%d: got %d encoded bytes, want less than %d", origLen, got, want)
		}
	}
}

func TestFramingFormat(t *testing.T) {
	// src is comprised of alternating 1e5-sized sequences of random
	// (incompressible) bytes and repeated (compressible) bytes. 1e5 was chosen
	// because it is larger than maxBlockSize (64k).
	src := make([]byte, 1e6)
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 10; i++ {
		if i%2 == 0 {
			for j := 0; j < 1e5; j++ {
				src[1e5*i+j] = uint8(rng.Intn(256))
			}
		} else {
			for j := 0; j < 1e5; j++ {
				src[1e5*i+j] = uint8(i)
			}
		}
	}

	buf := new(bytes.Buffer)
	if _, err := NewWriter(buf).Write(src); err != nil {
		t.Fatalf("Write: encoding: %v", err)
	}
	dst, err := ioutil.ReadAll(NewReader(buf))
	if err != nil {
		t.Fatalf("ReadAll: decoding: %v", err)
	}
	if err := cmp(dst, src); err != nil {
		t.Fatal(err)
	}
}

func TestWriterGoldenOutput(t *testing.T) {
	buf := new(bytes.Buffer)
	w := NewBufferedWriter(buf)
	defer w.Close()
	w.Write([]byte("abcd")) // Not compressible.
	w.Flush()
	w.Write(bytes.Repeat([]byte{'A'}, 100)) // Compressible.
	w.Flush()
	got := buf.String()
	want := strings.Join([]string{
		magicChunk,
		"\x01\x08\x00\x00", // Uncompressed chunk, 8 bytes long (including 4 byte checksum).
		"\x68\x10\xe6\xb6", // Checksum.
		"\x61\x62\x63\x64", // Uncompressed payload: "abcd".
		"\x00\x0d\x00\x00", // Compressed chunk, 13 bytes long (including 4 byte checksum).
		"\x37\xcb\xbc\x9d", // Checksum.
		"\x64",             // Compressed payload: Uncompressed length (varint encoded): 100.
		"\x00\x41",         // Compressed payload: tagLiteral, length=1, "A".
		"\xfe\x01\x00",     // Compressed payload: tagCopy2,   length=64, offset=1.
		"\x8a\x01\x00",     // Compressed payload: tagCopy2,   length=35, offset=1.
	}, "")
	if got != want {
		t.Fatalf("\ngot:  % x\nwant: % x", got, want)
	}
}

func TestNewBufferedWriter(t *testing.T) {
	// Test all 32 possible sub-sequences of these 5 input slices.
	//
	// Their lengths sum to 400,000, which is over 6 times the Writer ibuf
	// capacity: 6 * maxBlockSize is 393,216.
	inputs := [][]byte{
		bytes.Repeat([]byte{'a'}, 40000),
		bytes.Repeat([]byte{'b'}, 150000),
		bytes.Repeat([]byte{'c'}, 60000),
		bytes.Repeat([]byte{'d'}, 120000),
		bytes.Repeat([]byte{'e'}, 30000),
	}
loop:
	for i := 0; i < 1<<uint(len(inputs)); i++ {
		var want []byte
		buf := new(bytes.Buffer)
		w := NewBufferedWriter(buf)
		for j, input := range inputs {
			if i&(1<<uint(j)) == 0 {
				continue
			}
			if _, err := w.Write(input); err != nil {
				t.Errorf("i=%#02x: j=%d: Write: %v", i, j, err)
				continue loop
			}
			want = append(want, input...)
		}
		if err := w.Close(); err != nil {
			t.Errorf("i=%#02x: Close: %v", i, err)
			continue
		}
		got, err := ioutil.ReadAll(NewReader(buf))
		if err != nil {
			t.Errorf("i=%#02x: ReadAll: %v", i, err)
			continue
		}
		if err := cmp(got, want); err != nil {
			t.Errorf("i=%#02x: %v", i, err)
			continue
		}
	}
}

func TestFlush(t *testing.T) {
	buf := new(bytes.Buffer)
	w := NewBufferedWriter(buf)
	defer w.Close()
	if _, err := w.Write(bytes.Repeat([]byte{'x'}, 20)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n := buf.Len(); n != 0 {
		t.Fatalf("before Flush: %d bytes were written to the underlying io.Writer, want 0", n)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if n := buf.Len(); n == 0 {
		t.Fatalf("after Flush: %d bytes were written to the underlying io.Writer, want non-0", n)
	}
}

func TestReaderReset(t *testing.T) {
	gold := bytes.Repeat([]byte("All that is gold does not glitter,\n"), 10000)
	buf := new(bytes.Buffer)
	if _, err := NewWriter(buf).Write(gold); err != nil {
		t.Fatalf("Write: %v", err)
	}
	encoded, invalid, partial := buf.String(), "invalid", "partial"
	r := NewReader(nil)
	for i, s := range []string{encoded, invalid, partial, encoded, partial, invalid, encoded, encoded} {
		if s == partial {
			r.Reset(strings.NewReader(encoded))
			if _, err := r.Read(make([]byte, 101)); err != nil {
				t.Errorf("#%d: %v", i, err)
				continue
			}
			continue
		}
		r.Reset(strings.NewReader(s))
		got, err := ioutil.ReadAll(r)
		switch s {
		case encoded:
			if err != nil {
				t.Errorf("#%d: %v", i, err)
				continue
			}
			if err := cmp(got, gold); err != nil {
				t.Errorf("#%d: %v", i, err)
				continue
			}
		case invalid:
			if err == nil {
				t.Errorf("#%d: got nil error, want non-nil", i)
				continue
			}
		}
	}
}

func TestWriterReset(t *testing.T) {
	gold := bytes.Repeat([]byte("Not all those who wander are lost;\n"), 10000)
	const n = 20
	for _, buffered := range []bool{false, true} {
		var w *Writer
		if buffered {
			w = NewBufferedWriter(nil)
			defer w.Close()
		} else {
			w = NewWriter(nil)
		}

		var gots, wants [][]byte
		failed := false
		for i := 0; i <= n; i++ {
			buf := new(bytes.Buffer)
			w.Reset(buf)
			want := gold[:len(gold)*i/n]
			if _, err := w.Write(want); err != nil {
				t.Errorf("#%d: Write: %v", i, err)
				failed = true
				continue
			}
			if buffered {
				if err := w.Flush(); err != nil {
					t.Errorf("#%d: Flush: %v", i, err)
					failed = true
					continue
				}
			}
			got, err := ioutil.ReadAll(NewReader(buf))
			if err != nil {
				t.Errorf("#%d: ReadAll: %v", i, err)
				failed = true
				continue
			}
			gots = append(gots, got)
			wants = append(wants, want)
		}
		if failed {
			continue
		}
		for i := range gots {
			if err := cmp(gots[i], wants[i]); err != nil {
				t.Errorf("#%d: %v", i, err)
			}
		}
	}
}

func TestWriterResetWithoutFlush(t *testing.T) {
	buf0 := new(bytes.Buffer)
	buf1 := new(bytes.Buffer)
	w := NewBufferedWriter(buf0)
	if _, err := w.Write([]byte("xxx")); err != nil {
		t.Fatalf("Write #0: %v", err)
	}
	// Note that we don't Flush the Writer before calling Reset.
	w.Reset(buf1)
	if _, err := w.Write([]byte("yyy")); err != nil {
		t.Fatalf("Write #1: %v", err)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	got, err := ioutil.ReadAll(NewReader(buf1))
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if err := cmp(got, []byte("yyy")); err != nil {
		t.Fatal(err)
	}
}

type writeCounter int

func (c *writeCounter) Write(p []byte) (int, error) {
	*c++
	return len(p), nil
}

// TestNumUnderlyingWrites tests that each Writer flush only makes one or two
// Write calls on its underlying io.Writer, depending on whether or not the
// flushed buffer was compressible.
func TestNumUnderlyingWrites(t *testing.T) {
	testCases := []struct {
		input []byte
		want  int
	}{
		{bytes.Repeat([]byte{'x'}, 100), 1},
		{bytes.Repeat([]byte{'y'}, 100), 1},
		{[]byte("ABCDEFGHIJKLMNOPQRST"), 2},
	}

	var c writeCounter
	w := NewBufferedWriter(&c)
	defer w.Close()
	for i, tc := range testCases {
		c = 0
		if _, err := w.Write(tc.input); err != nil {
			t.Errorf("#%d: Write: %v", i, err)
			continue
		}
		if err := w.Flush(); err != nil {
			t.Errorf("#%d: Flush: %v", i, err)
			continue
		}
		if int(c) != tc.want {
			t.Errorf("#%d: got %d underlying writes, want %d", i, c, tc.want)
			continue
		}
	}
}

func benchDecode(b *testing.B, src []byte) {
	encoded := Encode(nil, src)
	// Bandwidth is in amount of uncompressed data.
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Decode(src, encoded)
	}
}

func benchEncode(b *testing.B, src []byte) {
	// Bandwidth is in amount of uncompressed data.
	b.SetBytes(int64(len(src)))
	dst := make([]byte, MaxEncodedLen(len(src)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Encode(dst, src)
	}
}

func readFile(b testing.TB, filename string) []byte {
	src, err := ioutil.ReadFile(filename)
	if err != nil {
		b.Skipf("skipping benchmark: %v", err)
	}
	if len(src) == 0 {
		b.Fatalf("%s has zero length", filename)
	}
	return src
}

// expand returns a slice of length n containing repeated copies of src.
func expand(src []byte, n int) []byte {
	dst := make([]byte, n)
	for x := dst; len(x) > 0; {
		i := copy(x, src)
		x = x[i:]
	}
	return dst
}

func benchWords(b *testing.B, n int, decode bool) {
	// Note: the file is OS-language dependent so the resulting values are not
	// directly comparable for non-US-English OS installations.
	data := expand(readFile(b, "/usr/share/dict/words"), n)
	if decode {
		benchDecode(b, data)
	} else {
		benchEncode(b, data)
	}
}

func BenchmarkWordsDecode1e1(b *testing.B) { benchWords(b, 1e1, true) }
func BenchmarkWordsDecode1e2(b *testing.B) { benchWords(b, 1e2, true) }
func BenchmarkWordsDecode1e3(b *testing.B) { benchWords(b, 1e3, true) }
func BenchmarkWordsDecode1e4(b *testing.B) { benchWords(b, 1e4, true) }
func BenchmarkWordsDecode1e5(b *testing.B) { benchWords(b, 1e5, true) }
func BenchmarkWordsDecode1e6(b *testing.B) { benchWords(b, 1e6, true) }
func BenchmarkWordsEncode1e1(b *testing.B) { benchWords(b, 1e1, false) }
func BenchmarkWordsEncode1e2(b *testing.B) { benchWords(b, 1e2, false) }
func BenchmarkWordsEncode1e3(b *testing.B) { benchWords(b, 1e3, false) }
func BenchmarkWordsEncode1e4(b *testing.B) { benchWords(b, 1e4, false) }
func BenchmarkWordsEncode1e5(b *testing.B) { benchWords(b, 1e5, false) }
func BenchmarkWordsEncode1e6(b *testing.B) { benchWords(b, 1e6, false) }

func BenchmarkRandomEncode(b *testing.B) {
	rng := rand.New(rand.NewSource(1))
	data := make([]byte, 1<<20)
	for i := range data {
		data[i] = uint8(rng.Intn(256))
	}
	benchEncode(b, data)
}

// testFiles' values are copied directly from
// https://raw.githubusercontent.com/google/snappy/master/snappy_unittest.cc
// The label field is unused in snappy-go.
var testFiles = []struct {
	label     string
	filename  string
	sizeLimit int
}{
	{"html", "html", 0},
	{"urls", "urls.10K", 0},
	{"jpg", "fireworks.jpeg", 0},
	{"jpg_200", "fireworks.jpeg", 200},
	{"pdf", "paper-100k.pdf", 0},
	{"html4", "html_x_4", 0},
	{"txt1", "alice29.txt", 0},
	{"txt2", "asyoulik.txt", 0},
	{"txt3", "lcet10.txt", 0},
	{"txt4", "plrabn12.txt", 0},
	{"pb", "geo.protodata", 0},
	{"gaviota", "kppkn.gtb", 0},
}

const (
	// The benchmark data files are at this canonical URL.
	benchURL = "https://raw.githubusercontent.com/google/snappy/master/testdata/"

	// They are copied to this local directory.
	benchDir = "testdata/bench"
)

func downloadBenchmarkFiles(b *testing.B, basename string) (errRet error) {
	filename := filepath.Join(benchDir, basename)
	if stat, err := os.Stat(filename); err == nil && stat.Size() != 0 {
		return nil
	}

	if !*download {
		b.Skipf("test data not found; skipping benchmark without the -download flag")
	}
	// Download the official snappy C++ implementation reference test data
	// files for benchmarking.
	if err := os.MkdirAll(benchDir, 0777); err != nil && !os.IsExist(err) {
		return fmt.Errorf("failed to create %s: %s", benchDir, err)
	}

	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create %s: %s", filename, err)
	}
	defer f.Close()
	defer func() {
		if errRet != nil {
			os.Remove(filename)
		}
	}()
	url := benchURL + basename
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download %s: %s", url, err)
	}
	defer resp.Body.Close()
	if s := resp.StatusCode; s != http.StatusOK {
		return fmt.Errorf("downloading %s: HTTP status code %d (%s)", url, s, http.StatusText(s))
	}
	_, err = io.Copy(f, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to download %s to %s: %s", url, filename, err)
	}
	return nil
}

func benchFile(b *testing.B, n int, decode bool) {
	if err := downloadBenchmarkFiles(b, testFiles[n].filename); err != nil {
		b.Fatalf("failed to download testdata: %s", err)
	}
	data := readFile(b, filepath.Join(benchDir, testFiles[n].filename))
	if n := testFiles[n].sizeLimit; 0 < n && n < len(data) {
		data = data[:n]
	}
	if decode {
		benchDecode(b, data)
	} else {
		benchEncode(b, data)
	}
}

// Naming convention is kept similar to what snappy's C++ implementation uses.
func Benchmark_UFlat0(b *testing.B)  { benchFile(b, 0, true) }
func Benchmark_UFlat1(b *testing.B)  { benchFile(b, 1, true) }
func Benchmark_UFlat2(b *testing.B)  { benchFile(b, 2, true) }
func Benchmark_UFlat3(b *testing.B)  { benchFile(b, 3, true) }
func Benchmark_UFlat4(b *testing.B)  { benchFile(b, 4, true) }
func Benchmark_UFlat5(b *testing.B)  { benchFile(b, 5, true) }
func Benchmark_UFlat6(b *testing.B)  { benchFile(b, 6, true) }
func Benchmark_UFlat7(b *testing.B)  { benchFile(b, 7, true) }
func Benchmark_UFlat8(b *testing.B)  { benchFile(b, 8, true) }
func Benchmark_UFlat9(b *testing.B)  { benchFile(b, 9, true) }
func Benchmark_UFlat10(b *testing.B) { benchFile(b, 10, true) }
func Benchmark_UFlat11(b *testing.B) { benchFile(b, 11, true) }
func Benchmark_ZFlat0(b *testing.B)  { benchFile(b, 0, false) }
func Benchmark_ZFlat1(b *testing.B)  { benchFile(b, 1, false) }
func Benchmark_ZFlat2(b *testing.B)  { benchFile(b, 2, false) }
func Benchmark_ZFlat3(b *testing.B)  { benchFile(b, 3, false) }
func Benchmark_ZFlat4(b *testing.B)  { benchFile(b, 4, false) }
func Benchmark_ZFlat5(b *testing.B)  { benchFile(b, 5, false) }
func Benchmark_ZFlat6(b *testing.B)  { benchFile(b, 6, false) }
func Benchmark_ZFlat7(b *testing.B)  { benchFile(b, 7, false) }
func Benchmark_ZFlat8(b *testing.B)  { benchFile(b, 8, false) }
func Benchmark_ZFlat9(b *testing.B)  { benchFile(b, 9, false) }
func Benchmark_ZFlat10(b *testing.B) { benchFile(b, 10, false) }
func Benchmark_ZFlat11(b *testing.B) { benchFile(b, 11, false) }
