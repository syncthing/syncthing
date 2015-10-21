// Copyright (C) 2014 The Protocol Authors.

package protocol

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"flag"
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
	c0ID     = NewDeviceID([]byte{1})
	c1ID     = NewDeviceID([]byte{2})
	quickCfg = &quick.Config{}
)

func TestMain(m *testing.M) {
	flag.Parse()
	if flag.Lookup("test.short").Value.String() != "false" {
		quickCfg.MaxCount = 10
	}
	os.Exit(m.Run())
}

func TestHeaderFunctions(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	ar, aw := io.Pipe()
	br, bw := io.Pipe()

	c0 := NewConnection(c0ID, ar, bw, newTestModel(), "name", CompressAlways).(wireFormatConnection).next.(*rawConnection)
	c0.Start()
	c1 := NewConnection(c1ID, br, aw, newTestModel(), "name", CompressAlways).(wireFormatConnection).next.(*rawConnection)
	c1.Start()
	c0.ClusterConfig(ClusterConfigMessage{})
	c1.ClusterConfig(ClusterConfigMessage{})

	if ok := c0.ping(); !ok {
		t.Error("c0 ping failed")
	}
	if ok := c1.ping(); !ok {
		t.Error("c1 ping failed")
	}
}

func TestVersionErr(t *testing.T) {
	t.Parallel()
	m0 := newTestModel()
	m1 := newTestModel()

	ar, aw := io.Pipe()
	br, bw := io.Pipe()

	c0 := NewConnection(c0ID, ar, bw, m0, "name", CompressAlways).(wireFormatConnection).next.(*rawConnection)
	c0.Start()
	c1 := NewConnection(c1ID, br, aw, m1, "name", CompressAlways)
	c1.Start()
	c0.ClusterConfig(ClusterConfigMessage{})
	c1.ClusterConfig(ClusterConfigMessage{})

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
	t.Parallel()
	m0 := newTestModel()
	m1 := newTestModel()

	ar, aw := io.Pipe()
	br, bw := io.Pipe()

	c0 := NewConnection(c0ID, ar, bw, m0, "name", CompressAlways).(wireFormatConnection).next.(*rawConnection)
	c0.Start()
	c1 := NewConnection(c1ID, br, aw, m1, "name", CompressAlways)
	c1.Start()
	c0.ClusterConfig(ClusterConfigMessage{})
	c1.ClusterConfig(ClusterConfigMessage{})

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
	t.Parallel()
	m0 := newTestModel()
	m1 := newTestModel()

	ar, aw := io.Pipe()
	br, bw := io.Pipe()

	c0 := NewConnection(c0ID, ar, bw, m0, "name", CompressAlways).(wireFormatConnection).next.(*rawConnection)
	c0.Start()
	c1 := NewConnection(c1ID, br, aw, m1, "name", CompressAlways)
	c1.Start()
	c0.ClusterConfig(ClusterConfigMessage{})
	c1.ClusterConfig(ClusterConfigMessage{})

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
	t.Parallel()
	m := ClusterConfigMessage{
		ClientName: "longstringlongstringlongstringinglongstringlongstringlonlongstringlongstringlon",
	}
	_, err := m.EncodeXDR(ioutil.Discard)
	if err == nil {
		t.Errorf("ID length %d > max 64, but no error", len(m.Folders[0].ID))
	}
}

func TestMarshalIndexMessage(t *testing.T) {
	t.Parallel()
	f := func(m1 IndexMessage) bool {
		for i, f := range m1.Files {
			m1.Files[i].CachedSize = 0
			for j := range f.Blocks {
				f.Blocks[j].Offset = 0
				if len(f.Blocks[j].Hash) == 0 {
					f.Blocks[j].Hash = nil
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
	t.Parallel()
	f := func(m1 RequestMessage) bool {
		return testMarshal(t, "request", &m1, &RequestMessage{})
	}

	if err := quick.Check(f, quickCfg); err != nil {
		t.Error(err)
	}
}

func TestMarshalResponseMessage(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	f := func(m1 ClusterConfigMessage) bool {
		return testMarshal(t, "clusterconfig", &m1, &ClusterConfigMessage{})
	}

	if err := quick.Check(f, quickCfg); err != nil {
		t.Error(err)
	}
}

func TestMarshalCloseMessage(t *testing.T) {
	t.Parallel()
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
