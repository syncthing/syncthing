// Copyright (C) 2014 The Protocol Authors.

package protocol

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"testing"
	"testing/quick"

	"github.com/calmh/xdr"
)

var (
	c0ID = NewDeviceID([]byte{1})
	c1ID = NewDeviceID([]byte{2})
)

func TestHeaderFunctions(t *testing.T) {
	f := func(ver, id, typ int) bool {
		ver = int(uint(ver) % 16)
		id = int(uint(id) % 4096)
		typ = int(uint(typ) % 256)
		h0 := header{version: ver, msgID: id, msgType: typ}
		h1 := decodeHeader(encodeHeader(h0))
		return h0 == h1
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

func TestHeaderLayout(t *testing.T) {
	var e, a uint32

	// Version are the first four bits
	e = 0xf0000000
	a = encodeHeader(header{version: 0xf})
	if a != e {
		t.Errorf("Header layout incorrect; %08x != %08x", a, e)
	}

	// Message ID are the following 12 bits
	e = 0x0fff0000
	a = encodeHeader(header{msgID: 0xfff})
	if a != e {
		t.Errorf("Header layout incorrect; %08x != %08x", a, e)
	}

	// Type are the last 8 bits before reserved
	e = 0x0000ff00
	a = encodeHeader(header{msgType: 0xff})
	if a != e {
		t.Errorf("Header layout incorrect; %08x != %08x", a, e)
	}
}

func TestPing(t *testing.T) {
	ar, aw := io.Pipe()
	br, bw := io.Pipe()

	c0 := NewConnection(c0ID, ar, bw, nil, "name", CompressAlways).(wireFormatConnection).next.(*rawConnection)
	c1 := NewConnection(c1ID, br, aw, nil, "name", CompressAlways).(wireFormatConnection).next.(*rawConnection)

	if ok := c0.ping(); !ok {
		t.Error("c0 ping failed")
	}
	if ok := c1.ping(); !ok {
		t.Error("c1 ping failed")
	}
}

func TestPingErr(t *testing.T) {
	e := errors.New("something broke")

	for i := 0; i < 16; i++ {
		for j := 0; j < 16; j++ {
			m0 := newTestModel()
			m1 := newTestModel()

			ar, aw := io.Pipe()
			br, bw := io.Pipe()
			eaw := &ErrPipe{PipeWriter: *aw, max: i, err: e}
			ebw := &ErrPipe{PipeWriter: *bw, max: j, err: e}

			c0 := NewConnection(c0ID, ar, ebw, m0, "name", CompressAlways).(wireFormatConnection).next.(*rawConnection)
			NewConnection(c1ID, br, eaw, m1, "name", CompressAlways)

			res := c0.ping()
			if (i < 8 || j < 8) && res {
				t.Errorf("Unexpected ping success; i=%d, j=%d", i, j)
			} else if (i >= 12 && j >= 12) && !res {
				t.Errorf("Unexpected ping fail; i=%d, j=%d", i, j)
			}
		}
	}
}

// func TestRequestResponseErr(t *testing.T) {
// 	e := errors.New("something broke")

// 	var pass bool
// 	for i := 0; i < 48; i++ {
// 		for j := 0; j < 38; j++ {
// 			m0 := newTestModel()
// 			m0.data = []byte("response data")
// 			m1 := newTestModel()

// 			ar, aw := io.Pipe()
// 			br, bw := io.Pipe()
// 			eaw := &ErrPipe{PipeWriter: *aw, max: i, err: e}
// 			ebw := &ErrPipe{PipeWriter: *bw, max: j, err: e}

// 			NewConnection(c0ID, ar, ebw, m0, nil)
// 			c1 := NewConnection(c1ID, br, eaw, m1, nil).(wireFormatConnection).next.(*rawConnection)

// 			d, err := c1.Request("default", "tn", 1234, 5678)
// 			if err == e || err == ErrClosed {
// 				t.Logf("Error at %d+%d bytes", i, j)
// 				if !m1.isClosed() {
// 					t.Fatal("c1 not closed")
// 				}
// 				if !m0.isClosed() {
// 					t.Fatal("c0 not closed")
// 				}
// 				continue
// 			}
// 			if err != nil {
// 				t.Fatal(err)
// 			}
// 			if string(d) != "response data" {
// 				t.Fatalf("Incorrect response data %q", string(d))
// 			}
// 			if m0.folder != "default" {
// 				t.Fatalf("Incorrect folder %q", m0.folder)
// 			}
// 			if m0.name != "tn" {
// 				t.Fatalf("Incorrect name %q", m0.name)
// 			}
// 			if m0.offset != 1234 {
// 				t.Fatalf("Incorrect offset %d", m0.offset)
// 			}
// 			if m0.size != 5678 {
// 				t.Fatalf("Incorrect size %d", m0.size)
// 			}
// 			t.Logf("Pass at %d+%d bytes", i, j)
// 			pass = true
// 		}
// 	}
// 	if !pass {
// 		t.Fatal("Never passed")
// 	}
// }

func TestVersionErr(t *testing.T) {
	m0 := newTestModel()
	m1 := newTestModel()

	ar, aw := io.Pipe()
	br, bw := io.Pipe()

	c0 := NewConnection(c0ID, ar, bw, m0, "name", CompressAlways).(wireFormatConnection).next.(*rawConnection)
	NewConnection(c1ID, br, aw, m1, "name", CompressAlways)

	w := xdr.NewWriter(c0.cw)
	w.WriteUint32(encodeHeader(header{
		version: 2,
		msgID:   0,
		msgType: 0,
	}))
	w.WriteUint32(0) // Avoids reader closing due to EOF

	if !m1.isClosed() {
		t.Error("Connection should close due to unknown version")
	}
}

func TestTypeErr(t *testing.T) {
	m0 := newTestModel()
	m1 := newTestModel()

	ar, aw := io.Pipe()
	br, bw := io.Pipe()

	c0 := NewConnection(c0ID, ar, bw, m0, "name", CompressAlways).(wireFormatConnection).next.(*rawConnection)
	NewConnection(c1ID, br, aw, m1, "name", CompressAlways)

	w := xdr.NewWriter(c0.cw)
	w.WriteUint32(encodeHeader(header{
		version: 0,
		msgID:   0,
		msgType: 42,
	}))
	w.WriteUint32(0) // Avoids reader closing due to EOF

	if !m1.isClosed() {
		t.Error("Connection should close due to unknown message type")
	}
}

func TestClose(t *testing.T) {
	m0 := newTestModel()
	m1 := newTestModel()

	ar, aw := io.Pipe()
	br, bw := io.Pipe()

	c0 := NewConnection(c0ID, ar, bw, m0, "name", CompressAlways).(wireFormatConnection).next.(*rawConnection)
	NewConnection(c1ID, br, aw, m1, "name", CompressAlways)

	c0.close(nil)

	<-c0.closed
	if !m0.isClosed() {
		t.Fatal("Connection should be closed")
	}

	// None of these should panic, some should return an error

	if c0.ping() {
		t.Error("Ping should not return true")
	}

	c0.Index("default", nil, 0, nil)
	c0.Index("default", nil, 0, nil)

	if _, err := c0.Request("default", "foo", 0, 0, nil, 0, nil); err == nil {
		t.Error("Request should return an error")
	}
}

func TestElementSizeExceededNested(t *testing.T) {
	m := ClusterConfigMessage{
		Folders: []Folder{
			{ID: "longstringlongstringlongstringinglongstringlongstringlonlongstringlongstringlon"},
		},
	}
	_, err := m.EncodeXDR(ioutil.Discard)
	if err == nil {
		t.Errorf("ID length %d > max 64, but no error", len(m.Folders[0].ID))
	}
}

func TestMarshalIndexMessage(t *testing.T) {
	var quickCfg = &quick.Config{MaxCountScale: 10}
	if testing.Short() {
		quickCfg = nil
	}

	f := func(m1 IndexMessage) bool {
		for _, f := range m1.Files {
			for i := range f.Blocks {
				f.Blocks[i].Offset = 0
				if len(f.Blocks[i].Hash) == 0 {
					f.Blocks[i].Hash = nil
				}
			}
		}

		return testMarshal(t, "index", &m1, &IndexMessage{})
	}

	if err := quick.Check(f, quickCfg); err != nil {
		t.Error(err)
	}
}

func TestMarshalRequestMessage(t *testing.T) {
	var quickCfg = &quick.Config{MaxCountScale: 10}
	if testing.Short() {
		quickCfg = nil
	}

	f := func(m1 RequestMessage) bool {
		return testMarshal(t, "request", &m1, &RequestMessage{})
	}

	if err := quick.Check(f, quickCfg); err != nil {
		t.Error(err)
	}
}

func TestMarshalResponseMessage(t *testing.T) {
	var quickCfg = &quick.Config{MaxCountScale: 10}
	if testing.Short() {
		quickCfg = nil
	}

	f := func(m1 ResponseMessage) bool {
		if len(m1.Data) == 0 {
			m1.Data = nil
		}
		return testMarshal(t, "response", &m1, &ResponseMessage{})
	}

	if err := quick.Check(f, quickCfg); err != nil {
		t.Error(err)
	}
}

func TestMarshalClusterConfigMessage(t *testing.T) {
	var quickCfg = &quick.Config{MaxCountScale: 10}
	if testing.Short() {
		quickCfg = nil
	}

	f := func(m1 ClusterConfigMessage) bool {
		return testMarshal(t, "clusterconfig", &m1, &ClusterConfigMessage{})
	}

	if err := quick.Check(f, quickCfg); err != nil {
		t.Error(err)
	}
}

func TestMarshalCloseMessage(t *testing.T) {
	var quickCfg = &quick.Config{MaxCountScale: 10}
	if testing.Short() {
		quickCfg = nil
	}

	f := func(m1 CloseMessage) bool {
		return testMarshal(t, "close", &m1, &CloseMessage{})
	}

	if err := quick.Check(f, quickCfg); err != nil {
		t.Error(err)
	}
}

type message interface {
	EncodeXDR(io.Writer) (int, error)
	DecodeXDR(io.Reader) error
}

func testMarshal(t *testing.T, prefix string, m1, m2 message) bool {
	var buf bytes.Buffer

	failed := func(bc []byte) {
		bs, _ := json.MarshalIndent(m1, "", "  ")
		ioutil.WriteFile(prefix+"-1.txt", bs, 0644)
		bs, _ = json.MarshalIndent(m2, "", "  ")
		ioutil.WriteFile(prefix+"-2.txt", bs, 0644)
		if len(bc) > 0 {
			f, _ := os.Create(prefix + "-data.txt")
			fmt.Fprint(f, hex.Dump(bc))
			f.Close()
		}
	}

	_, err := m1.EncodeXDR(&buf)
	if err != nil && strings.Contains(err.Error(), "exceeds size") {
		return true
	}
	if err != nil {
		failed(nil)
		t.Fatal(err)
	}

	bc := make([]byte, len(buf.Bytes()))
	copy(bc, buf.Bytes())

	err = m2.DecodeXDR(&buf)
	if err != nil {
		failed(bc)
		t.Fatal(err)
	}

	ok := reflect.DeepEqual(m1, m2)
	if !ok {
		failed(bc)
	}
	return ok
}
