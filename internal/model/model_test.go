// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package model

import (
	"bytes"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/syncthing/syncthing/internal/config"
	"github.com/syncthing/syncthing/internal/protocol"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"
)

var node1, node2 protocol.NodeID

func init() {
	node1, _ = protocol.NodeIDFromString("AIR6LPZ-7K4PTTV-UXQSMUU-CPQ5YWH-OEDFIIQ-JUG777G-2YQXXR5-YD6AWQR")
	node2, _ = protocol.NodeIDFromString("GYRZZQB-IRNPV4Z-T7TC52W-EQYJ3TT-FDQW6MW-DFLMU42-SSSU6EM-FBK2VAY")
}

var testDataExpected = map[string]protocol.FileInfo{
	"foo": protocol.FileInfo{
		Name:     "foo",
		Flags:    0,
		Modified: 0,
		Blocks:   []protocol.BlockInfo{{Offset: 0x0, Size: 0x7, Hash: []uint8{0xae, 0xc0, 0x70, 0x64, 0x5f, 0xe5, 0x3e, 0xe3, 0xb3, 0x76, 0x30, 0x59, 0x37, 0x61, 0x34, 0xf0, 0x58, 0xcc, 0x33, 0x72, 0x47, 0xc9, 0x78, 0xad, 0xd1, 0x78, 0xb6, 0xcc, 0xdf, 0xb0, 0x1, 0x9f}}},
	},
	"empty": protocol.FileInfo{
		Name:     "empty",
		Flags:    0,
		Modified: 0,
		Blocks:   []protocol.BlockInfo{{Offset: 0x0, Size: 0x0, Hash: []uint8{0xe3, 0xb0, 0xc4, 0x42, 0x98, 0xfc, 0x1c, 0x14, 0x9a, 0xfb, 0xf4, 0xc8, 0x99, 0x6f, 0xb9, 0x24, 0x27, 0xae, 0x41, 0xe4, 0x64, 0x9b, 0x93, 0x4c, 0xa4, 0x95, 0x99, 0x1b, 0x78, 0x52, 0xb8, 0x55}}},
	},
	"bar": protocol.FileInfo{
		Name:     "bar",
		Flags:    0,
		Modified: 0,
		Blocks:   []protocol.BlockInfo{{Offset: 0x0, Size: 0xa, Hash: []uint8{0x2f, 0x72, 0xcc, 0x11, 0xa6, 0xfc, 0xd0, 0x27, 0x1e, 0xce, 0xf8, 0xc6, 0x10, 0x56, 0xee, 0x1e, 0xb1, 0x24, 0x3b, 0xe3, 0x80, 0x5b, 0xf9, 0xa9, 0xdf, 0x98, 0xf9, 0x2f, 0x76, 0x36, 0xb0, 0x5c}}},
	},
}

func init() {
	// Fix expected test data to match reality
	for n, f := range testDataExpected {
		fi, _ := os.Stat("testdata/" + n)
		f.Flags = uint32(fi.Mode())
		f.Modified = fi.ModTime().Unix()
		testDataExpected[n] = f
	}
}

func TestRequest(t *testing.T) {
	db, _ := leveldb.Open(storage.NewMemStorage(), nil)
	m := NewModel("/tmp", &config.Configuration{}, "node", "syncthing", "dev", db)
	m.AddRepo(config.RepositoryConfiguration{ID: "default", Directory: "testdata"})
	m.ScanRepo("default")

	bs, err := m.Request(node1, "default", "foo", 0, 6)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Compare(bs, []byte("foobar")) != 0 {
		t.Errorf("Incorrect data from request: %q", string(bs))
	}

	bs, err = m.Request(node1, "default", "../walk.go", 0, 6)
	if err == nil {
		t.Error("Unexpected nil error on insecure file read")
	}
	if bs != nil {
		t.Errorf("Unexpected non nil data on insecure file read: %q", string(bs))
	}
}

func genFiles(n int) []protocol.FileInfo {
	files := make([]protocol.FileInfo, n)
	t := time.Now().Unix()
	for i := 0; i < n; i++ {
		files[i] = protocol.FileInfo{
			Name:     fmt.Sprintf("file%d", i),
			Modified: t,
			Blocks:   []protocol.BlockInfo{{0, 100, []byte("some hash bytes")}},
		}
	}

	return files
}

func BenchmarkIndex10000(b *testing.B) {
	db, _ := leveldb.Open(storage.NewMemStorage(), nil)
	m := NewModel("/tmp", nil, "node", "syncthing", "dev", db)
	m.AddRepo(config.RepositoryConfiguration{ID: "default", Directory: "testdata"})
	m.ScanRepo("default")
	files := genFiles(10000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Index(node1, "default", files)
	}
}

func BenchmarkIndex00100(b *testing.B) {
	db, _ := leveldb.Open(storage.NewMemStorage(), nil)
	m := NewModel("/tmp", nil, "node", "syncthing", "dev", db)
	m.AddRepo(config.RepositoryConfiguration{ID: "default", Directory: "testdata"})
	m.ScanRepo("default")
	files := genFiles(100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Index(node1, "default", files)
	}
}

func BenchmarkIndexUpdate10000f10000(b *testing.B) {
	db, _ := leveldb.Open(storage.NewMemStorage(), nil)
	m := NewModel("/tmp", nil, "node", "syncthing", "dev", db)
	m.AddRepo(config.RepositoryConfiguration{ID: "default", Directory: "testdata"})
	m.ScanRepo("default")
	files := genFiles(10000)
	m.Index(node1, "default", files)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.IndexUpdate(node1, "default", files)
	}
}

func BenchmarkIndexUpdate10000f00100(b *testing.B) {
	db, _ := leveldb.Open(storage.NewMemStorage(), nil)
	m := NewModel("/tmp", nil, "node", "syncthing", "dev", db)
	m.AddRepo(config.RepositoryConfiguration{ID: "default", Directory: "testdata"})
	m.ScanRepo("default")
	files := genFiles(10000)
	m.Index(node1, "default", files)

	ufiles := genFiles(100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.IndexUpdate(node1, "default", ufiles)
	}
}

func BenchmarkIndexUpdate10000f00001(b *testing.B) {
	db, _ := leveldb.Open(storage.NewMemStorage(), nil)
	m := NewModel("/tmp", nil, "node", "syncthing", "dev", db)
	m.AddRepo(config.RepositoryConfiguration{ID: "default", Directory: "testdata"})
	m.ScanRepo("default")
	files := genFiles(10000)
	m.Index(node1, "default", files)

	ufiles := genFiles(1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.IndexUpdate(node1, "default", ufiles)
	}
}

type FakeConnection struct {
	id          protocol.NodeID
	requestData []byte
}

func (FakeConnection) Close() error {
	return nil
}

func (f FakeConnection) ID() protocol.NodeID {
	return f.id
}

func (f FakeConnection) Name() string {
	return ""
}

func (f FakeConnection) Option(string) string {
	return ""
}

func (FakeConnection) Index(string, []protocol.FileInfo) error {
	return nil
}

func (FakeConnection) IndexUpdate(string, []protocol.FileInfo) error {
	return nil
}

func (f FakeConnection) Request(repo, name string, offset int64, size int) ([]byte, error) {
	return f.requestData, nil
}

func (FakeConnection) ClusterConfig(protocol.ClusterConfigMessage) {}

func (FakeConnection) Ping() bool {
	return true
}

func (FakeConnection) Statistics() protocol.Statistics {
	return protocol.Statistics{}
}

func BenchmarkRequest(b *testing.B) {
	db, _ := leveldb.Open(storage.NewMemStorage(), nil)
	m := NewModel("/tmp", nil, "node", "syncthing", "dev", db)
	m.AddRepo(config.RepositoryConfiguration{ID: "default", Directory: "testdata"})
	m.ScanRepo("default")

	const n = 1000
	files := make([]protocol.FileInfo, n)
	t := time.Now().Unix()
	for i := 0; i < n; i++ {
		files[i] = protocol.FileInfo{
			Name:     fmt.Sprintf("file%d", i),
			Modified: t,
			Blocks:   []protocol.BlockInfo{{0, 100, []byte("some hash bytes")}},
		}
	}

	fc := FakeConnection{
		id:          node1,
		requestData: []byte("some data to return"),
	}
	m.AddConnection(fc, fc)
	m.Index(node1, "default", files)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data, err := m.requestGlobal(node1, "default", files[i%n].Name, 0, 32, nil)
		if err != nil {
			b.Error(err)
		}
		if data == nil {
			b.Error("nil data")
		}
	}
}

func TestActivityMap(t *testing.T) {
	isValid := func(protocol.NodeID) bool {
		return true
	}
	m := make(activityMap)
	if node := m.leastBusyNode([]protocol.NodeID{node1}, isValid); node != node1 {
		t.Errorf("Incorrect least busy node %q", node)
	}
	if node := m.leastBusyNode([]protocol.NodeID{node2}, isValid); node != node2 {
		t.Errorf("Incorrect least busy node %q", node)
	}
	if node := m.leastBusyNode([]protocol.NodeID{node1, node2}, isValid); node != node1 {
		t.Errorf("Incorrect least busy node %q", node)
	}
	if node := m.leastBusyNode([]protocol.NodeID{node1, node2}, isValid); node != node2 {
		t.Errorf("Incorrect least busy node %q", node)
	}
}

func TestNodeRename(t *testing.T) {
	ccm := protocol.ClusterConfigMessage{
		ClientName:    "syncthing",
		ClientVersion: "v0.9.4",
	}

	cfg := config.New("/tmp/test", node1)
	cfg.Nodes = []config.NodeConfiguration{
		{
			NodeID: node1,
		},
	}

	db, _ := leveldb.Open(storage.NewMemStorage(), nil)
	m := NewModel("/tmp", &cfg, "node", "syncthing", "dev", db)
	if cfg.Nodes[0].Name != "" {
		t.Errorf("Node already has a name")
	}

	m.ClusterConfig(node1, ccm)
	if cfg.Nodes[0].Name != "" {
		t.Errorf("Node already has a name")
	}

	ccm.Options = []protocol.Option{
		{
			Key:   "name",
			Value: "tester",
		},
	}
	m.ClusterConfig(node1, ccm)
	if cfg.Nodes[0].Name != "tester" {
		t.Errorf("Node did not get a name")
	}

	ccm.Options[0].Value = "tester2"
	m.ClusterConfig(node1, ccm)
	if cfg.Nodes[0].Name != "tester" {
		t.Errorf("Node name got overwritten")
	}
}

func TestClusterConfig(t *testing.T) {
	cfg := config.New("/tmp/test", node1)
	cfg.Nodes = []config.NodeConfiguration{
		{
			NodeID:     node1,
			Introducer: true,
		},
		{
			NodeID: node2,
		},
	}
	cfg.Repositories = []config.RepositoryConfiguration{
		{
			ID: "repo1",
			Nodes: []config.RepositoryNodeConfiguration{
				{NodeID: node1},
				{NodeID: node2},
			},
		},
		{
			ID: "repo2",
			Nodes: []config.RepositoryNodeConfiguration{
				{NodeID: node1},
				{NodeID: node2},
			},
		},
	}

	db, _ := leveldb.Open(storage.NewMemStorage(), nil)

	m := NewModel("/tmp", &cfg, "node", "syncthing", "dev", db)
	m.AddRepo(cfg.Repositories[0])
	m.AddRepo(cfg.Repositories[1])

	cm := m.clusterConfig(node2)

	if l := len(cm.Repositories); l != 2 {
		t.Fatalf("Incorrect number of repos %d != 2", l)
	}

	r := cm.Repositories[0]
	if r.ID != "repo1" {
		t.Errorf("Incorrect repo %q != repo1", r.ID)
	}
	if l := len(r.Nodes); l != 2 {
		t.Errorf("Incorrect number of nodes %d != 2", l)
	}
	if id := r.Nodes[0].ID; bytes.Compare(id, node1[:]) != 0 {
		t.Errorf("Incorrect node ID %x != %x", id, node1)
	}
	if r.Nodes[0].Flags&protocol.FlagIntroducer == 0 {
		t.Error("Node1 should be flagged as Introducer")
	}
	if id := r.Nodes[1].ID; bytes.Compare(id, node2[:]) != 0 {
		t.Errorf("Incorrect node ID %x != %x", id, node2)
	}
	if r.Nodes[1].Flags&protocol.FlagIntroducer != 0 {
		t.Error("Node2 should not be flagged as Introducer")
	}

	r = cm.Repositories[1]
	if r.ID != "repo2" {
		t.Errorf("Incorrect repo %q != repo2", r.ID)
	}
	if l := len(r.Nodes); l != 2 {
		t.Errorf("Incorrect number of nodes %d != 2", l)
	}
	if id := r.Nodes[0].ID; bytes.Compare(id, node1[:]) != 0 {
		t.Errorf("Incorrect node ID %x != %x", id, node1)
	}
	if r.Nodes[0].Flags&protocol.FlagIntroducer == 0 {
		t.Error("Node1 should be flagged as Introducer")
	}
	if id := r.Nodes[1].ID; bytes.Compare(id, node2[:]) != 0 {
		t.Errorf("Incorrect node ID %x != %x", id, node2)
	}
	if r.Nodes[1].Flags&protocol.FlagIntroducer != 0 {
		t.Error("Node2 should not be flagged as Introducer")
	}
}

func TestIgnores(t *testing.T) {
	arrEqual := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}

		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}

	db, _ := leveldb.Open(storage.NewMemStorage(), nil)
	m := NewModel("/tmp", nil, "node", "syncthing", "dev", db)
	m.AddRepo(config.RepositoryConfiguration{ID: "default", Directory: "testdata"})

	expected := []string{
		".*",
		"quux",
	}

	ignores, err := m.GetIgnores("default")
	if err != nil {
		t.Error(err)
	}

	if !arrEqual(ignores, expected) {
		t.Errorf("Incorrect ignores: %v != %v", ignores, expected)
	}

	ignores = append(ignores, "pox")

	err = m.SetIgnores("default", ignores)
	if err != nil {
		t.Error(err)
	}

	ignores2, err := m.GetIgnores("default")
	if err != nil {
		t.Error(err)
	}

	if arrEqual(expected, ignores2) {
		t.Errorf("Incorrect ignores: %v == %v", ignores2, expected)
	}

	if !arrEqual(ignores, ignores2) {
		t.Errorf("Incorrect ignores: %v != %v", ignores2, ignores)
	}

	err = m.SetIgnores("default", expected)
	if err != nil {
		t.Error(err)
	}

	ignores, err = m.GetIgnores("default")
	if err != nil {
		t.Error(err)
	}

	if !arrEqual(ignores, expected) {
		t.Errorf("Incorrect ignores: %v != %v", ignores, expected)
	}

	ignores, err = m.GetIgnores("doesnotexist")
	if err == nil {
		t.Error("No error")
	}

	err = m.SetIgnores("doesnotexist", expected)
	if err == nil {
		t.Error("No error")
	}

	m.AddRepo(config.RepositoryConfiguration{ID: "fresh", Directory: "XXX"})
	ignores, err = m.GetIgnores("fresh")
	if err != nil {
		t.Error(err)
	}
	if len(ignores) > 0 {
		t.Errorf("Expected no ignores, got: %v", ignores)
	}
}
