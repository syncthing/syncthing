// Copyright 2011 The Snappy-Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package snappy

import (
	"bytes"
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

var (
	download = flag.Bool("download", false, "If true, download any missing files before running benchmarks")
	testdata = flag.String("testdata", "testdata", "Directory containing the test data")
)

func roundtrip(b, ebuf, dbuf []byte) error {
	d, err := Decode(dbuf, Encode(ebuf, b))
	if err != nil {
		return fmt.Errorf("decoding error: %v", err)
	}
	if !bytes.Equal(b, d) {
		return fmt.Errorf("roundtrip mismatch:\n\twant %v\n\tgot  %v", b, d)
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
	rng := rand.New(rand.NewSource(27354294))
	for n := 1; n < 20000; n += 23 {
		b := make([]byte, n)
		for i := range b {
			b[i] = uint8(rng.Uint32())
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
	data := []byte("\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\x00")
	if _, err := DecodedLen(data); err != ErrCorrupt {
		t.Errorf("DecodedLen: got %v, want ErrCorrupt", err)
	}
	if _, err := Decode(nil, data); err != ErrCorrupt {
		t.Errorf("Decode: got %v, want ErrCorrupt", err)
	}

	// The encoded varint overflows 32 bits
	data = []byte("\xff\xff\xff\xff\xff\x00")

	if _, err := DecodedLen(data); err != ErrCorrupt {
		t.Errorf("DecodedLen: got %v, want ErrCorrupt", err)
	}
	if _, err := Decode(nil, data); err != ErrCorrupt {
		t.Errorf("Decode: got %v, want ErrCorrupt", err)
	}
}

func cmp(a, b []byte) error {
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

func TestFramingFormat(t *testing.T) {
	// src is comprised of alternating 1e5-sized sequences of random
	// (incompressible) bytes and repeated (compressible) bytes. 1e5 was chosen
	// because it is larger than maxUncompressedChunkLen (64k).
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
	var gots, wants [][]byte
	const n = 20
	w, failed := NewWriter(nil), false
	for i := 0; i <= n; i++ {
		buf := new(bytes.Buffer)
		w.Reset(buf)
		want := gold[:len(gold)*i/n]
		if _, err := w.Write(want); err != nil {
			t.Errorf("#%d: Write: %v", i, err)
			failed = true
			continue
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
		return
	}
	for i := range gots {
		if err := cmp(gots[i], wants[i]); err != nil {
			t.Errorf("#%d: %v", i, err)
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

func BenchmarkWordsDecode1e3(b *testing.B) { benchWords(b, 1e3, true) }
func BenchmarkWordsDecode1e4(b *testing.B) { benchWords(b, 1e4, true) }
func BenchmarkWordsDecode1e5(b *testing.B) { benchWords(b, 1e5, true) }
func BenchmarkWordsDecode1e6(b *testing.B) { benchWords(b, 1e6, true) }
func BenchmarkWordsEncode1e3(b *testing.B) { benchWords(b, 1e3, false) }
func BenchmarkWordsEncode1e4(b *testing.B) { benchWords(b, 1e4, false) }
func BenchmarkWordsEncode1e5(b *testing.B) { benchWords(b, 1e5, false) }
func BenchmarkWordsEncode1e6(b *testing.B) { benchWords(b, 1e6, false) }

// testFiles' values are copied directly from
// https://raw.githubusercontent.com/google/snappy/master/snappy_unittest.cc
// The label field is unused in snappy-go.
var testFiles = []struct {
	label    string
	filename string
}{
	{"html", "html"},
	{"urls", "urls.10K"},
	{"jpg", "fireworks.jpeg"},
	{"jpg_200", "fireworks.jpeg"},
	{"pdf", "paper-100k.pdf"},
	{"html4", "html_x_4"},
	{"txt1", "alice29.txt"},
	{"txt2", "asyoulik.txt"},
	{"txt3", "lcet10.txt"},
	{"txt4", "plrabn12.txt"},
	{"pb", "geo.protodata"},
	{"gaviota", "kppkn.gtb"},
}

// The test data files are present at this canonical URL.
const baseURL = "https://raw.githubusercontent.com/google/snappy/master/testdata/"

func downloadTestdata(b *testing.B, basename string) (errRet error) {
	filename := filepath.Join(*testdata, basename)
	if stat, err := os.Stat(filename); err == nil && stat.Size() != 0 {
		return nil
	}

	if !*download {
		b.Skipf("test data not found; skipping benchmark without the -download flag")
	}
	// Download the official snappy C++ implementation reference test data
	// files for benchmarking.
	if err := os.Mkdir(*testdata, 0777); err != nil && !os.IsExist(err) {
		return fmt.Errorf("failed to create testdata: %s", err)
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
	url := baseURL + basename
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
	if err := downloadTestdata(b, testFiles[n].filename); err != nil {
		b.Fatalf("failed to download testdata: %s", err)
	}
	data := readFile(b, filepath.Join(*testdata, testFiles[n].filename))
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
