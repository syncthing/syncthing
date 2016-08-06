// Copyright (C) 2014 The Protocol Authors.

package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"github.com/syncthing/syncthing/lib/rand"
)

var (
	c0ID     = NewDeviceID([]byte{1})
	c1ID     = NewDeviceID([]byte{2})
	quickCfg = &quick.Config{}
)

func TestPing(t *testing.T) {
	ar, aw := io.Pipe()
	br, bw := io.Pipe()

	c0 := NewConnection(c0ID, ar, bw, newTestModel(), "name", CompressAlways).(wireFormatConnection).Connection.(*rawConnection)
	c0.Start()
	c1 := NewConnection(c1ID, br, aw, newTestModel(), "name", CompressAlways).(wireFormatConnection).Connection.(*rawConnection)
	c1.Start()
	c0.ClusterConfig(ClusterConfig{})
	c1.ClusterConfig(ClusterConfig{})

	if ok := c0.ping(); !ok {
		t.Error("c0 ping failed")
	}
	if ok := c1.ping(); !ok {
		t.Error("c1 ping failed")
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
	c0.ClusterConfig(ClusterConfig{})
	c1.ClusterConfig(ClusterConfig{})

	c0.close(errors.New("manual close"))

	<-c0.closed
	if err := m0.closedError(); err == nil || !strings.Contains(err.Error(), "manual close") {
		t.Fatal("Connection should be closed")
	}

	// None of these should panic, some should return an error

	if c0.ping() {
		t.Error("Ping should not return true")
	}

	c0.Index("default", nil)
	c0.Index("default", nil)

	if _, err := c0.Request("default", "foo", 0, 0, nil, false); err == nil {
		t.Error("Request should return an error")
	}
}

func TestMarshalIndexMessage(t *testing.T) {
	if testing.Short() {
		quickCfg.MaxCount = 10
	}

	f := func(m1 Index) bool {
		if len(m1.Files) == 0 {
			m1.Files = nil
		}
		for i, f := range m1.Files {
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
			if len(f.Version.Counters) == 0 {
				m1.Files[i].Version.Counters = nil
			}
		}

		return testMarshal(t, "index", &m1, &Index{})
	}

	if err := quick.Check(f, quickCfg); err != nil {
		t.Error(err)
	}
}

func TestMarshalRequestMessage(t *testing.T) {
	if testing.Short() {
		quickCfg.MaxCount = 10
	}

	f := func(m1 Request) bool {
		if len(m1.Hash) == 0 {
			m1.Hash = nil
		}
		return testMarshal(t, "request", &m1, &Request{})
	}

	if err := quick.Check(f, quickCfg); err != nil {
		t.Error(err)
	}
}

func TestMarshalResponseMessage(t *testing.T) {
	if testing.Short() {
		quickCfg.MaxCount = 10
	}

	f := func(m1 Response) bool {
		if len(m1.Data) == 0 {
			m1.Data = nil
		}
		return testMarshal(t, "response", &m1, &Response{})
	}

	if err := quick.Check(f, quickCfg); err != nil {
		t.Error(err)
	}
}

func TestMarshalClusterConfigMessage(t *testing.T) {
	if testing.Short() {
		quickCfg.MaxCount = 10
	}

	f := func(m1 ClusterConfig) bool {
		if len(m1.Folders) == 0 {
			m1.Folders = nil
		}
		for i := range m1.Folders {
			if len(m1.Folders[i].Devices) == 0 {
				m1.Folders[i].Devices = nil
			}
		}
		return testMarshal(t, "clusterconfig", &m1, &ClusterConfig{})
	}

	if err := quick.Check(f, quickCfg); err != nil {
		t.Error(err)
	}
}

func TestMarshalCloseMessage(t *testing.T) {
	if testing.Short() {
		quickCfg.MaxCount = 10
	}

	f := func(m1 Close) bool {
		return testMarshal(t, "close", &m1, &Close{})
	}

	if err := quick.Check(f, quickCfg); err != nil {
		t.Error(err)
	}
}

func testMarshal(t *testing.T, prefix string, m1, m2 message) bool {
	buf, err := m1.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	err = m2.Unmarshal(buf)
	if err != nil {
		t.Fatal(err)
	}

	bs1, _ := json.MarshalIndent(m1, "", "  ")
	bs2, _ := json.MarshalIndent(m2, "", "  ")
	if !bytes.Equal(bs1, bs2) {
		ioutil.WriteFile(prefix+"-1.txt", bs1, 0644)
		ioutil.WriteFile(prefix+"-2.txt", bs1, 0644)
		return false
	}

	return true
}

func TestMarshalledIndexMessageSize(t *testing.T) {
	// We should be able to handle a 1 TiB file without
	// blowing the default max message size.

	if testing.Short() {
		t.Skip("this test requires a lot of memory")
		return
	}

	const (
		maxMessageSize = MaxMessageLen
		fileSize       = 1 << 40
		blockSize      = BlockSize
	)

	f := FileInfo{
		Name:        "a normal length file name withoout any weird stuff.txt",
		Type:        FileInfoTypeFile,
		Size:        fileSize,
		Permissions: 0666,
		ModifiedS:   time.Now().Unix(),
		Version:     Vector{Counters: []Counter{{ID: 1 << 60, Value: 1}, {ID: 2 << 60, Value: 1}}},
		Blocks:      make([]BlockInfo, fileSize/blockSize),
	}

	for i := 0; i < fileSize/blockSize; i++ {
		f.Blocks[i].Offset = int64(i) * blockSize
		f.Blocks[i].Size = blockSize
		f.Blocks[i].Hash = []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 1, 2, 3, 4, 5, 6, 7, 8, 9, 20, 1, 2, 3, 4, 5, 6, 7, 8, 9, 30, 1, 2}
	}

	idx := Index{
		Folder: "some folder ID",
		Files:  []FileInfo{f},
	}

	msgSize := idx.ProtoSize()
	if msgSize > maxMessageSize {
		t.Errorf("Message size %d bytes is larger than max %d", msgSize, maxMessageSize)
	}
}

func TestLZ4Compression(t *testing.T) {
	c := new(rawConnection)

	for i := 0; i < 10; i++ {
		dataLen := 150 + rand.Intn(150)
		data := make([]byte, dataLen)
		_, err := io.ReadFull(rand.Reader, data[100:])
		if err != nil {
			t.Fatal(err)
		}
		comp, err := c.lz4Compress(data)
		if err != nil {
			t.Errorf("compressing %d bytes: %v", dataLen, err)
			continue
		}

		res, err := c.lz4Decompress(comp)
		if err != nil {
			t.Errorf("decompressing %d bytes to %d: %v", len(comp), dataLen, err)
			continue
		}
		if len(res) != len(data) {
			t.Errorf("Incorrect len %d != expected %d", len(res), len(data))
		}
		if !bytes.Equal(data, res) {
			t.Error("Incorrect decompressed data")
		}
		t.Logf("OK #%d, %d -> %d -> %d", i, dataLen, len(comp), dataLen)
	}
}
