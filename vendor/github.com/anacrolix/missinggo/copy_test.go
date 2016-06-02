package missinggo

import (
	"bytes"
	"strings"
	"testing"
)

func TestCopyToArray(t *testing.T) {
	var arr [3]byte
	bb := []byte{1, 2, 3}
	CopyExact(&arr, bb)
	if !bytes.Equal(arr[:], bb) {
		t.FailNow()
	}
}

func TestCopyToSlicedArray(t *testing.T) {
	var arr [5]byte
	CopyExact(arr[:], "hello")
	if !bytes.Equal(arr[:], []byte("hello")) {
		t.FailNow()
	}
}

func TestCopyDestNotAddr(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.FailNow()
		}
		t.Log(r)
	}()
	var arr [3]byte
	CopyExact(arr, "nope")
}

func TestCopyLenMismatch(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.FailNow()
		}
		t.Log(r)
	}()
	CopyExact(make([]byte, 2), "abc")
}

func TestCopySrcString(t *testing.T) {
	dest := make([]byte, 3)
	CopyExact(dest, "lol")
	if string(dest) != "lol" {
		t.FailNow()
	}
	func() {
		defer func() {
			r := recover()
			if r == nil {
				t.FailNow()
			}
		}()
		CopyExact(dest, "rofl")
	}()
	var arr [5]byte
	CopyExact(&arr, interface{}("hello"))
	if string(arr[:]) != "hello" {
		t.FailNow()
	}
}

func TestCopySrcNilInterface(t *testing.T) {
	var arr [3]byte
	defer func() {
		r := recover().(string)
		if !strings.Contains(r, "invalid source") {
			t.FailNow()
		}
	}()
	CopyExact(&arr, nil)
}

func TestCopySrcPtr(t *testing.T) {
	var bigDst [1024]byte
	var bigSrc [1024]byte = [1024]byte{'h', 'i'}
	CopyExact(&bigDst, &bigSrc)
	if !bytes.Equal(bigDst[:], bigSrc[:]) {
		t.FailNow()
	}
}
