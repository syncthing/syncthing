// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package files_test

import (
	"fmt"
	"sort"
	"testing"

	"github.com/syncthing/syncthing/files"
	"github.com/syncthing/syncthing/lamport"
	"github.com/syncthing/syncthing/protocol"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"
)

var remoteNode protocol.NodeID

func init() {
	remoteNode, _ = protocol.NodeIDFromString("AIR6LPZ-7K4PTTV-UXQSMUU-CPQ5YWH-OEDFIIQ-JUG777G-2YQXXR5-YD6AWQR")
}

func genBlocks(n int) []protocol.BlockInfo {
	b := make([]protocol.BlockInfo, n)
	for i := range b {
		h := make([]byte, 32)
		for j := range h {
			h[j] = byte(i + j)
		}
		b[i].Size = uint32(i)
		b[i].Hash = h
	}
	return b
}

func globalList(s *files.Set) []protocol.FileInfo {
	var fs []protocol.FileInfo
	s.WithGlobal(func(f protocol.FileInfo) bool {
		fs = append(fs, f)
		return true
	})
	return fs
}

func haveList(s *files.Set, n protocol.NodeID) []protocol.FileInfo {
	var fs []protocol.FileInfo
	s.WithHave(n, func(f protocol.FileInfo) bool {
		fs = append(fs, f)
		return true
	})
	return fs
}

func needList(s *files.Set, n protocol.NodeID) []protocol.FileInfo {
	var fs []protocol.FileInfo
	s.WithNeed(n, func(f protocol.FileInfo) bool {
		fs = append(fs, f)
		return true
	})
	return fs
}

type fileList []protocol.FileInfo

func (l fileList) Len() int {
	return len(l)
}

func (l fileList) Less(a, b int) bool {
	return l[a].Name < l[b].Name
}

func (l fileList) Swap(a, b int) {
	l[a], l[b] = l[b], l[a]
}

func TestGlobalSet(t *testing.T) {
	lamport.Default = lamport.Clock{}

	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		t.Fatal(err)
	}

	m := files.NewSet("test", db)

	local0 := []protocol.FileInfo{
		protocol.FileInfo{Name: "a", Version: 1000, Blocks: genBlocks(1)},
		protocol.FileInfo{Name: "b", Version: 1000, Blocks: genBlocks(2)},
		protocol.FileInfo{Name: "c", Version: 1000, Blocks: genBlocks(3)},
		protocol.FileInfo{Name: "d", Version: 1000, Blocks: genBlocks(4)},
		protocol.FileInfo{Name: "z", Version: 1000, Blocks: genBlocks(8)},
	}
	local1 := []protocol.FileInfo{
		protocol.FileInfo{Name: "a", Version: 1000, Blocks: genBlocks(1)},
		protocol.FileInfo{Name: "b", Version: 1000, Blocks: genBlocks(2)},
		protocol.FileInfo{Name: "c", Version: 1000, Blocks: genBlocks(3)},
		protocol.FileInfo{Name: "d", Version: 1000, Blocks: genBlocks(4)},
	}
	localTot := []protocol.FileInfo{
		local0[0],
		local0[1],
		local0[2],
		local0[3],
		protocol.FileInfo{Name: "z", Version: 1001, Flags: protocol.FlagDeleted},
	}

	remote0 := []protocol.FileInfo{
		protocol.FileInfo{Name: "a", Version: 1000, Blocks: genBlocks(1)},
		protocol.FileInfo{Name: "b", Version: 1000, Blocks: genBlocks(2)},
		protocol.FileInfo{Name: "c", Version: 1002, Blocks: genBlocks(5)},
	}
	remote1 := []protocol.FileInfo{
		protocol.FileInfo{Name: "b", Version: 1001, Blocks: genBlocks(6)},
		protocol.FileInfo{Name: "e", Version: 1000, Blocks: genBlocks(7)},
	}
	remoteTot := []protocol.FileInfo{
		remote0[0],
		remote1[0],
		remote0[2],
		remote1[1],
	}

	expectedGlobal := []protocol.FileInfo{
		remote0[0],
		remote1[0],
		remote0[2],
		localTot[3],
		remote1[1],
		localTot[4],
	}

	expectedLocalNeed := []protocol.FileInfo{
		remote1[0],
		remote0[2],
		remote1[1],
	}

	expectedRemoteNeed := []protocol.FileInfo{
		local0[3],
	}

	m.ReplaceWithDelete(protocol.LocalNodeID, local0)
	m.ReplaceWithDelete(protocol.LocalNodeID, local1)
	m.Replace(remoteNode, remote0)
	m.Update(remoteNode, remote1)

	g := globalList(m)
	sort.Sort(fileList(g))

	if fmt.Sprint(g) != fmt.Sprint(expectedGlobal) {
		t.Errorf("Global incorrect;\n A: %v !=\n E: %v", g, expectedGlobal)
	}

	h := haveList(m, protocol.LocalNodeID)
	sort.Sort(fileList(h))

	if fmt.Sprint(h) != fmt.Sprint(localTot) {
		t.Errorf("Have incorrect;\n A: %v !=\n E: %v", h, localTot)
	}

	h = haveList(m, remoteNode)
	sort.Sort(fileList(h))

	if fmt.Sprint(h) != fmt.Sprint(remoteTot) {
		t.Errorf("Have incorrect;\n A: %v !=\n E: %v", h, remoteTot)
	}

	n := needList(m, protocol.LocalNodeID)
	sort.Sort(fileList(n))

	if fmt.Sprint(n) != fmt.Sprint(expectedLocalNeed) {
		t.Errorf("Need incorrect;\n A: %v !=\n E: %v", n, expectedLocalNeed)
	}

	n = needList(m, remoteNode)
	sort.Sort(fileList(n))

	if fmt.Sprint(n) != fmt.Sprint(expectedRemoteNeed) {
		t.Errorf("Need incorrect;\n A: %v !=\n E: %v", n, expectedRemoteNeed)
	}

	f := m.Get(protocol.LocalNodeID, "b")
	if fmt.Sprint(f) != fmt.Sprint(localTot[1]) {
		t.Errorf("Get incorrect;\n A: %v !=\n E: %v", f, localTot[1])
	}

	f = m.Get(remoteNode, "b")
	if fmt.Sprint(f) != fmt.Sprint(remote1[0]) {
		t.Errorf("Get incorrect;\n A: %v !=\n E: %v", f, remote1[0])
	}

	f = m.GetGlobal("b")
	if fmt.Sprint(f) != fmt.Sprint(remote1[0]) {
		t.Errorf("GetGlobal incorrect;\n A: %v !=\n E: %v", f, remote1[0])
	}

	f = m.Get(protocol.LocalNodeID, "zz")
	if f.Name != "" {
		t.Errorf("Get incorrect;\n A: %v !=\n E: %v", f, protocol.FileInfo{})
	}

	f = m.GetGlobal("zz")
	if f.Name != "" {
		t.Errorf("GetGlobal incorrect;\n A: %v !=\n E: %v", f, protocol.FileInfo{})
	}

	av := []protocol.NodeID{protocol.LocalNodeID, remoteNode}
	a := m.Availability("a")
	if !(len(a) == 2 && (a[0] == av[0] && a[1] == av[1] || a[0] == av[1] && a[1] == av[0])) {
		t.Errorf("Availability incorrect;\n A: %v !=\n E: %v", a, av)
	}
	a = m.Availability("b")
	if len(a) != 1 || a[0] != remoteNode {
		t.Errorf("Availability incorrect;\n A: %v !=\n E: %v", a, remoteNode)
	}
	a = m.Availability("d")
	if len(a) != 1 || a[0] != protocol.LocalNodeID {
		t.Errorf("Availability incorrect;\n A: %v !=\n E: %v", a, protocol.LocalNodeID)
	}
}

func TestLocalDeleted(t *testing.T) {
	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		t.Fatal(err)
	}
	m := files.NewSet("test", db)
	lamport.Default = lamport.Clock{}

	local1 := []protocol.FileInfo{
		protocol.FileInfo{Name: "a", Version: 1000},
		protocol.FileInfo{Name: "b", Version: 1000},
		protocol.FileInfo{Name: "c", Version: 1000},
		protocol.FileInfo{Name: "d", Version: 1000},
		protocol.FileInfo{Name: "z", Version: 1000, Flags: protocol.FlagDirectory},
	}

	m.ReplaceWithDelete(protocol.LocalNodeID, local1)

	m.ReplaceWithDelete(protocol.LocalNodeID, []protocol.FileInfo{
		local1[0],
		// [1] removed
		local1[2],
		local1[3],
		local1[4],
	})
	m.ReplaceWithDelete(protocol.LocalNodeID, []protocol.FileInfo{
		local1[0],
		local1[2],
		// [3] removed
		local1[4],
	})
	m.ReplaceWithDelete(protocol.LocalNodeID, []protocol.FileInfo{
		local1[0],
		local1[2],
		// [4] removed
	})

	expectedGlobal1 := []protocol.FileInfo{
		local1[0],
		protocol.FileInfo{Name: "b", Version: 1001, Flags: protocol.FlagDeleted},
		local1[2],
		protocol.FileInfo{Name: "d", Version: 1002, Flags: protocol.FlagDeleted},
		protocol.FileInfo{Name: "z", Version: 1003, Flags: protocol.FlagDeleted | protocol.FlagDirectory},
	}

	g := globalList(m)
	sort.Sort(fileList(g))
	sort.Sort(fileList(expectedGlobal1))

	if fmt.Sprint(g) != fmt.Sprint(expectedGlobal1) {
		t.Errorf("Global incorrect;\n A: %v !=\n E: %v", g, expectedGlobal1)
	}

	m.ReplaceWithDelete(protocol.LocalNodeID, []protocol.FileInfo{
		local1[0],
		// [2] removed
	})

	expectedGlobal2 := []protocol.FileInfo{
		local1[0],
		protocol.FileInfo{Name: "b", Version: 1001, Flags: protocol.FlagDeleted},
		protocol.FileInfo{Name: "c", Version: 1004, Flags: protocol.FlagDeleted},
		protocol.FileInfo{Name: "d", Version: 1002, Flags: protocol.FlagDeleted},
		protocol.FileInfo{Name: "z", Version: 1003, Flags: protocol.FlagDeleted | protocol.FlagDirectory},
	}

	g = globalList(m)
	sort.Sort(fileList(g))
	sort.Sort(fileList(expectedGlobal2))

	if fmt.Sprint(g) != fmt.Sprint(expectedGlobal2) {
		t.Errorf("Global incorrect;\n A: %v !=\n E: %v", g, expectedGlobal2)
	}
}

func Benchmark10kReplace(b *testing.B) {
	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		b.Fatal(err)
	}

	var local []protocol.FileInfo
	for i := 0; i < 10000; i++ {
		local = append(local, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: 1000})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := files.NewSet("test", db)
		m.ReplaceWithDelete(protocol.LocalNodeID, local)
	}
}

func Benchmark10kUpdateChg(b *testing.B) {
	var remote []protocol.FileInfo
	for i := 0; i < 10000; i++ {
		remote = append(remote, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: 1000})
	}

	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		b.Fatal(err)
	}

	m := files.NewSet("test", db)
	m.Replace(remoteNode, remote)

	var local []protocol.FileInfo
	for i := 0; i < 10000; i++ {
		local = append(local, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: 1000})
	}

	m.ReplaceWithDelete(protocol.LocalNodeID, local)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		for j := range local {
			local[j].Version++
		}
		b.StartTimer()
		m.Update(protocol.LocalNodeID, local)
	}
}

func Benchmark10kUpdateSme(b *testing.B) {
	var remote []protocol.FileInfo
	for i := 0; i < 10000; i++ {
		remote = append(remote, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: 1000})
	}

	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		b.Fatal(err)
	}
	m := files.NewSet("test", db)
	m.Replace(remoteNode, remote)

	var local []protocol.FileInfo
	for i := 0; i < 10000; i++ {
		local = append(local, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: 1000})
	}

	m.ReplaceWithDelete(protocol.LocalNodeID, local)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Update(protocol.LocalNodeID, local)
	}
}

func Benchmark10kNeed2k(b *testing.B) {
	var remote []protocol.FileInfo
	for i := 0; i < 10000; i++ {
		remote = append(remote, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: 1000})
	}

	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		b.Fatal(err)
	}

	m := files.NewSet("test", db)
	m.Replace(remoteNode, remote)

	var local []protocol.FileInfo
	for i := 0; i < 8000; i++ {
		local = append(local, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: 1000})
	}
	for i := 8000; i < 10000; i++ {
		local = append(local, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: 980})
	}

	m.ReplaceWithDelete(protocol.LocalNodeID, local)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fs := needList(m, protocol.LocalNodeID)
		if l := len(fs); l != 2000 {
			b.Errorf("wrong length %d != 2k", l)
		}
	}
}

func Benchmark10kHaveFullList(b *testing.B) {
	var remote []protocol.FileInfo
	for i := 0; i < 10000; i++ {
		remote = append(remote, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: 1000})
	}

	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		b.Fatal(err)
	}

	m := files.NewSet("test", db)
	m.Replace(remoteNode, remote)

	var local []protocol.FileInfo
	for i := 0; i < 2000; i++ {
		local = append(local, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: 1000})
	}
	for i := 2000; i < 10000; i++ {
		local = append(local, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: 980})
	}

	m.ReplaceWithDelete(protocol.LocalNodeID, local)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fs := haveList(m, protocol.LocalNodeID)
		if l := len(fs); l != 10000 {
			b.Errorf("wrong length %d != 10k", l)
		}
	}
}

func Benchmark10kGlobal(b *testing.B) {
	var remote []protocol.FileInfo
	for i := 0; i < 10000; i++ {
		remote = append(remote, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: 1000})
	}

	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		b.Fatal(err)
	}

	m := files.NewSet("test", db)
	m.Replace(remoteNode, remote)

	var local []protocol.FileInfo
	for i := 0; i < 2000; i++ {
		local = append(local, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: 1000})
	}
	for i := 2000; i < 10000; i++ {
		local = append(local, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: 980})
	}

	m.ReplaceWithDelete(protocol.LocalNodeID, local)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fs := globalList(m)
		if l := len(fs); l != 10000 {
			b.Errorf("wrong length %d != 10k", l)
		}
	}
}

func TestGlobalReset(t *testing.T) {
	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		t.Fatal(err)
	}

	m := files.NewSet("test", db)

	local := []protocol.FileInfo{
		protocol.FileInfo{Name: "a", Version: 1000},
		protocol.FileInfo{Name: "b", Version: 1000},
		protocol.FileInfo{Name: "c", Version: 1000},
		protocol.FileInfo{Name: "d", Version: 1000},
	}

	remote := []protocol.FileInfo{
		protocol.FileInfo{Name: "a", Version: 1000},
		protocol.FileInfo{Name: "b", Version: 1001},
		protocol.FileInfo{Name: "c", Version: 1002},
		protocol.FileInfo{Name: "e", Version: 1000},
	}

	m.ReplaceWithDelete(protocol.LocalNodeID, local)
	g := globalList(m)
	sort.Sort(fileList(g))

	if fmt.Sprint(g) != fmt.Sprint(local) {
		t.Errorf("Global incorrect;\n%v !=\n%v", g, local)
	}

	m.Replace(remoteNode, remote)
	m.Replace(remoteNode, nil)

	g = globalList(m)
	sort.Sort(fileList(g))

	if fmt.Sprint(g) != fmt.Sprint(local) {
		t.Errorf("Global incorrect;\n%v !=\n%v", g, local)
	}
}

func TestNeed(t *testing.T) {
	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		t.Fatal(err)
	}

	m := files.NewSet("test", db)

	local := []protocol.FileInfo{
		protocol.FileInfo{Name: "a", Version: 1000},
		protocol.FileInfo{Name: "b", Version: 1000},
		protocol.FileInfo{Name: "c", Version: 1000},
		protocol.FileInfo{Name: "d", Version: 1000},
	}

	remote := []protocol.FileInfo{
		protocol.FileInfo{Name: "a", Version: 1000},
		protocol.FileInfo{Name: "b", Version: 1001},
		protocol.FileInfo{Name: "c", Version: 1002},
		protocol.FileInfo{Name: "e", Version: 1000},
	}

	shouldNeed := []protocol.FileInfo{
		protocol.FileInfo{Name: "b", Version: 1001},
		protocol.FileInfo{Name: "c", Version: 1002},
		protocol.FileInfo{Name: "e", Version: 1000},
	}

	m.ReplaceWithDelete(protocol.LocalNodeID, local)
	m.Replace(remoteNode, remote)

	need := needList(m, protocol.LocalNodeID)

	sort.Sort(fileList(need))
	sort.Sort(fileList(shouldNeed))

	if fmt.Sprint(need) != fmt.Sprint(shouldNeed) {
		t.Errorf("Need incorrect;\n%v !=\n%v", need, shouldNeed)
	}
}

func TestLocalVersion(t *testing.T) {
	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		t.Fatal(err)
	}

	m := files.NewSet("test", db)

	local1 := []protocol.FileInfo{
		protocol.FileInfo{Name: "a", Version: 1000},
		protocol.FileInfo{Name: "b", Version: 1000},
		protocol.FileInfo{Name: "c", Version: 1000},
		protocol.FileInfo{Name: "d", Version: 1000},
	}

	local2 := []protocol.FileInfo{
		local1[0],
		// [1] deleted
		local1[2],
		protocol.FileInfo{Name: "d", Version: 1002},
		protocol.FileInfo{Name: "e", Version: 1000},
	}

	m.ReplaceWithDelete(protocol.LocalNodeID, local1)
	c0 := m.LocalVersion(protocol.LocalNodeID)

	m.ReplaceWithDelete(protocol.LocalNodeID, local2)
	c1 := m.LocalVersion(protocol.LocalNodeID)
	if !(c1 > c0) {
		t.Fatal("Local version number should have incremented")
	}

	m.ReplaceWithDelete(protocol.LocalNodeID, local2)
	c2 := m.LocalVersion(protocol.LocalNodeID)
	if c2 != c1 {
		t.Fatal("Local version number should be unchanged")
	}
}
