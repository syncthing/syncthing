// Copyright (C) 2014 The Protocol Authors.

package protocol

import (
	"encoding/binary"
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
	"time"

	"github.com/calmh/xdr"
)

var (
	c0ID     = NewDeviceID([]byte{1})
	c1ID     = NewDeviceID([]byte{2})
	quickCfg = &quick.Config{}
)

func TestHeaderEncodeDecode(t *testing.T) {
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

func TestHeaderMarshalUnmarshal(t *testing.T) {
	f := func(ver, id, typ int) bool {
		ver = int(uint(ver) % 16)
		id = int(uint(id) % 4096)
		typ = int(uint(typ) % 256)
		buf := make([]byte, 4)

		h0 := header{version: ver, msgID: id, msgType: typ}
		h0.MarshalXDRInto(&xdr.Marshaller{Data: buf})

		var h1 header
		h1.UnmarshalXDRFrom(&xdr.Unmarshaller{Data: buf})
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

	c0 := NewConnection(c0ID, ar, bw, newTestModel(), "name", CompressAlways).(wireFormatConnection).Connection.(*rawConnection)
	c0.Start()
	c1 := NewConnection(c1ID, br, aw, newTestModel(), "name", CompressAlways).(wireFormatConnection).Connection.(*rawConnection)
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
	m0 := newTestModel()
	m1 := newTestModel()

	ar, aw := io.Pipe()
	br, bw := io.Pipe()

	c0 := NewConnection(c0ID, ar, bw, m0, "name", CompressAlways).(wireFormatConnection).Connection.(*rawConnection)
	c0.Start()
	c1 := NewConnection(c1ID, br, aw, m1, "name", CompressAlways)
	c1.Start()
	c0.ClusterConfig(ClusterConfigMessage{})
	c1.ClusterConfig(ClusterConfigMessage{})

	timeoutWriteHeader(c0.cw, header{
		version: 2, // higher than supported
		msgID:   0,
		msgType: messageTypeIndex,
	})

	if err := m1.closedError(); err == nil || !strings.Contains(err.Error(), "unknown protocol version") {
		t.Error("Connection should close due to unknown version, not", err)
	}
}

func TestTypeErr(t *testing.T) {
	m0 := newTestModel()
	m1 := newTestModel()

	ar, aw := io.Pipe()
	br, bw := io.Pipe()

	c0 := NewConnection(c0ID, ar, bw, m0, "name", CompressAlways).(wireFormatConnection).Connection.(*rawConnection)
	c0.Start()
	c1 := NewConnection(c1ID, br, aw, m1, "name", CompressAlways)
	c1.Start()
	c0.ClusterConfig(ClusterConfigMessage{})
	c1.ClusterConfig(ClusterConfigMessage{})

	timeoutWriteHeader(c0.cw, header{
		version: 0,
		msgID:   0,
		msgType: 42, // unknown type
	})

	if err := m1.closedError(); err == nil || !strings.Contains(err.Error(), "unknown message type") {
		t.Error("Connection should close due to unknown message type, not", err)
	}
}

func TestClose(t *testing.T) {
	m0 := newTestModel()
	m1 := newTestModel()

	ar, aw := io.Pipe()
	br, bw := io.Pipe()

	c0 := NewConnection(c0ID, ar, bw, m0, "name", CompressAlways).(wireFormatConnection).Connection.(*rawConnection)
	c0.Start()
	c1 := NewConnection(c1ID, br, aw, m1, "name", CompressAlways)
	c1.Start()
	c0.ClusterConfig(ClusterConfigMessage{})
	c1.ClusterConfig(ClusterConfigMessage{})

	c0.close(errors.New("manual close"))

	<-c0.closed
	if err := m0.closedError(); err == nil || !strings.Contains(err.Error(), "manual close") {
		t.Fatal("Connection should be closed")
	}

	// None of these should panic, some should return an error

	if c0.ping() {
		t.Error("Ping should not return true")
	}

	c0.Index("default", nil, 0, nil)
	c0.Index("default", nil, 0, nil)

	if _, err := c0.Request("default", "foo", 0, 0, nil, false); err == nil {
		t.Error("Request should return an error")
	}
}

func TestMarshalIndexMessage(t *testing.T) {
	if testing.Short() {
		quickCfg.MaxCount = 10
	}

	f := func(m1 IndexMessage) bool {
		if len(m1.Options) == 0 {
			m1.Options = nil
		}
		if len(m1.Files) == 0 {
			m1.Files = nil
		}
		for i, f := range m1.Files {
			m1.Files[i].CachedSize = 0
			if len(f.Blocks) == 0 {
				m1.Files[i].Blocks = nil
			} else {
				for j := range f.Blocks {
					f.Blocks[j].Offset = 0
					if len(f.Blocks[j].Hash) == 0 {
						f.Blocks[j].Hash = nil
					}
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
	if testing.Short() {
		quickCfg.MaxCount = 10
	}

	f := func(m1 RequestMessage) bool {
		if len(m1.Options) == 0 {
			m1.Options = nil
		}
		if len(m1.Hash) == 0 {
			m1.Hash = nil
		}
		return testMarshal(t, "request", &m1, &RequestMessage{})
	}

	if err := quick.Check(f, quickCfg); err != nil {
		t.Error(err)
	}
}

func TestMarshalResponseMessage(t *testing.T) {
	if testing.Short() {
		quickCfg.MaxCount = 10
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
	if testing.Short() {
		quickCfg.MaxCount = 10
	}

	f := func(m1 ClusterConfigMessage) bool {
		if len(m1.Options) == 0 {
			m1.Options = nil
		}
		if len(m1.Folders) == 0 {
			m1.Folders = nil
		}
		for i := range m1.Folders {
			if len(m1.Folders[i].Devices) == 0 {
				m1.Folders[i].Devices = nil
			}
			if len(m1.Folders[i].Options) == 0 {
				m1.Folders[i].Options = nil
			}
		}
		return testMarshal(t, "clusterconfig", &m1, &ClusterConfigMessage{})
	}

	if err := quick.Check(f, quickCfg); err != nil {
		t.Error(err)
	}
}

func TestMarshalCloseMessage(t *testing.T) {
	if testing.Short() {
		quickCfg.MaxCount = 10
	}

	f := func(m1 CloseMessage) bool {
		return testMarshal(t, "close", &m1, &CloseMessage{})
	}

	if err := quick.Check(f, quickCfg); err != nil {
		t.Error(err)
	}
}

type message interface {
	MarshalXDR() ([]byte, error)
	UnmarshalXDR([]byte) error
}

func testMarshal(t *testing.T, prefix string, m1, m2 message) bool {
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

	buf, err := m1.MarshalXDR()
	if err != nil && strings.Contains(err.Error(), "exceeds size") {
		return true
	}
	if err != nil {
		failed(nil)
		t.Fatal(err)
	}

	err = m2.UnmarshalXDR(buf)
	if err != nil {
		failed(buf)
		t.Fatal(err)
	}

	ok := reflect.DeepEqual(m1, m2)
	if !ok {
		failed(buf)
	}
	return ok
}

func timeoutWriteHeader(w io.Writer, hdr header) {
	// This tries to write a message header to w, but times out after a while.
	// This is useful because in testing, with a PipeWriter, it will block
	// forever if the other side isn't reading any more. On the other hand we
	// can't just "go" it into the background, because if the other side is
	// still there we should wait for the write to complete. Yay.

	var buf [8]byte // header and message length
	binary.BigEndian.PutUint32(buf[:], encodeHeader(hdr))
	binary.BigEndian.PutUint32(buf[4:], 0) // zero message length, explicitly

	done := make(chan struct{})
	go func() {
		w.Write(buf[:])
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
	}
}

func TestFileInfoSize(t *testing.T) {
	fi := FileInfo{
		Blocks: []BlockInfo{
			{Size: 42},
			{Offset: 42, Size: 23},
			{Offset: 42 + 23, Size: 34},
		},
	}

	size := fi.Size()
	want := int64(42 + 23 + 34)
	if size != want {
		t.Errorf("Incorrect size reported, got %d, want %d", size, want)
	}

	size = fi.Size() // Cached, this time
	if size != want {
		t.Errorf("Incorrect cached size reported, got %d, want %d", size, want)
	}

	fi.CachedSize = 8
	want = 8
	size = fi.Size() // Ensure it came from the cache
	if size != want {
		t.Errorf("Incorrect cached size reported, got %d, want %d", size, want)
	}

	fi.CachedSize = 0
	fi.Flags = FlagDirectory
	want = 128
	size = fi.Size() // Directories are 128 bytes large
	if size != want {
		t.Errorf("Incorrect cached size reported, got %d, want %d", size, want)
	}

	fi.CachedSize = 0
	fi.Flags = FlagDeleted
	want = 128
	size = fi.Size() // Also deleted files
	if size != want {
		t.Errorf("Incorrect cached size reported, got %d, want %d", size, want)
	}
}
