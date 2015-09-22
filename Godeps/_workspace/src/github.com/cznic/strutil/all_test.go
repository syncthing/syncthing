// Copyright (c) 2014 The sortutil Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package strutil

import (
	"bytes"
	"fmt"
	"github.com/cznic/mathutil"
	"math"
	"os"
	"path"
	"runtime"
	"strings"
	"testing"
	"unsafe"
)

func caller(s string, va ...interface{}) {
	_, fn, fl, _ := runtime.Caller(2)
	fmt.Fprintf(os.Stderr, "caller: %s:%d: ", path.Base(fn), fl)
	fmt.Fprintf(os.Stderr, s, va...)
	fmt.Fprintln(os.Stderr)
	_, fn, fl, _ = runtime.Caller(1)
	fmt.Fprintf(os.Stderr, "\tcallee: %s:%d: ", path.Base(fn), fl)
	fmt.Fprintln(os.Stderr)
}

func dbg(s string, va ...interface{}) {
	if s == "" {
		s = strings.Repeat("%v ", len(va))
	}
	_, fn, fl, _ := runtime.Caller(1)
	fmt.Fprintf(os.Stderr, "dbg %s:%d: ", path.Base(fn), fl)
	fmt.Fprintf(os.Stderr, s, va...)
	fmt.Fprintln(os.Stderr)
}

func TODO(...interface{}) string {
	_, fn, fl, _ := runtime.Caller(1)
	return fmt.Sprintf("TODO: %s:%d:\n", path.Base(fn), fl)
}

func use(...interface{}) {}
func TestBase64(t *testing.T) {
	const max = 768
	r, err := mathutil.NewFC32(math.MinInt32, math.MaxInt32, true)
	if err != nil {
		t.Fatal(err)
	}

	bin := []byte{}
	for i := 0; i < max; i++ {
		bin = append(bin, byte(r.Next()))
		cmp, err := Base64Decode(Base64Encode(bin))
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(bin, cmp) {
			t.Fatalf("a: % x\nb: % x", bin, cmp)
		}
	}
}

func TestBase32Ext(t *testing.T) {
	const max = 640
	r, err := mathutil.NewFC32(math.MinInt32, math.MaxInt32, true)
	if err != nil {
		t.Fatal(err)
	}

	bin := []byte{}
	for i := 0; i < max; i++ {
		bin = append(bin, byte(r.Next()))
		cmp, err := Base32ExtDecode(Base32ExtEncode(bin))
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(bin, cmp) {
			t.Fatalf("a: % x\nb: % x", bin, cmp)
		}
	}
}

func TestFields(t *testing.T) {
	p := []string{"", "\\", "|", "0", "1", "2"}
	one := func(n int) string {
		s := ""
		for i := 0; i < 3; i++ {
			s += p[n%len(p)]
			n /= len(p)
		}
		return s
	}
	max := len(p) * len(p) * len(p)
	var a [3]string
	for x := 0; x < max; x++ {
		a[0] = one(x)
		for x := 0; x < max; x++ {
			a[1] = one(x)
			for x := 0; x < len(p)*len(p); x++ {
				a[2] = one(x)
				enc := JoinFields(a[:], "|")
				dec := SplitFields(enc, "|")
				if g, e := strings.Join(dec, ","), strings.Join(a[:], ","); g != e {
					t.Fatal(g, e)
				}
			}
		}
	}
}

func ExamplePrettyString() {
	type prettyStringType struct {
		Array         [3]int
		Bool          bool
		Chan          <-chan int
		Complex128    complex128
		Complex64     complex64
		Float32       float32
		Float64       float64
		Func          func()
		Func2         interface{}
		Func3         interface{}
		Int           int
		Int16         int16
		Int32         int32
		Int64         int64
		Int8          int8
		Interface     interface{}
		Map           map[int]string
		Ptr           *int
		Slice         []int
		String        string
		Struct        *prettyStringType
		Struct2       *prettyStringType
		Uint          uint
		Uint16        uint16
		Uint32        uint32
		Uint64        uint64
		Uint8         byte
		Uintptr       uintptr
		UnsafePointer unsafe.Pointer
	}

	i := 314
	v := &prettyStringType{
		Array:         [...]int{10, 20, 30},
		Bool:          true,
		Chan:          make(<-chan int, 100),
		Complex128:    3 - 4i,
		Complex64:     1 + 2i,
		Float32:       1.5,
		Float64:       3.5,
		Func2:         func(a, b, c int, z ...string) (d, e, f string) { return },
		Func3:         func(a, b, c int, z ...string) {},
		Func:          func() {},
		Int16:         -44,
		Int32:         -45,
		Int64:         -46,
		Int8:          -43,
		Int:           -42,
		Map:           map[int]string{100: "100", 200: "200", 300: "300"},
		Ptr:           &i,
		Slice:         []int{10, 20, 30},
		String:        "foo",
		Struct:        &prettyStringType{Int: 8888},
		Struct2:       &prettyStringType{},
		Uint16:        44,
		Uint32:        45,
		Uint64:        46,
		Uint8:         43,
		Uint:          42,
		Uintptr:       uintptr(99),
		UnsafePointer: unsafe.Pointer(uintptr(0x12345678)),
	}
	v.Interface = v
	fmt.Println(PrettyString(v, "", "", nil))
	// Output:
	// &strutil.prettyStringType{
	// · Array: [3]int{
	// · · 0: 10,
	// · · 1: 20,
	// · · 2: 30,
	// · },
	// · Bool: true,
	// · Chan: <-chan int// capacity: 100,
	// · Complex128: (3-4i),
	// · Complex64: (1+2i),
	// · Float32: 1.5,
	// · Float64: 3.5,
	// · Func: func() { ... },
	// · Func2: func(int, int, int, ...string) (string, string, string) { ... },
	// · Func3: func(int, int, int, ...string) { ... },
	// · Int: -42,
	// · Int16: -44,
	// · Int32: -45,
	// · Int64: -46,
	// · Int8: -43,
	// · Interface: &strutil.prettyStringType{ /* recursive/repetitive pointee not shown */ },
	// · Map: map[int]string{
	// · · 100: "100",
	// · · 200: "200",
	// · · 300: "300",
	// · },
	// · Ptr: &314,
	// · Slice: []int{ // len 3
	// · · 0: 10,
	// · · 1: 20,
	// · · 2: 30,
	// · },
	// · String: "foo",
	// · Struct: &strutil.prettyStringType{
	// · · Int: 8888,
	// · },
	// · Uint: 42,
	// · Uint16: 44,
	// · Uint32: 45,
	// · Uint64: 46,
	// · Uint8: 43,
	// · Uintptr: 99,
	// · UnsafePointer: 0x12345678,
	// }
}
