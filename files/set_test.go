// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package files_test

import (
	"bytes"
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/syncthing/syncthing/files"
	"github.com/syncthing/syncthing/lamport"
	"github.com/syncthing/syncthing/protocol"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"
)

var remoteNode0, remoteNode1 protocol.NodeID

func init() {
	remoteNode0, _ = protocol.NodeIDFromString("AIR6LPZ-7K4PTTV-UXQSMUU-CPQ5YWH-OEDFIIQ-JUG777G-2YQXXR5-YD6AWQR")
	remoteNode1, _ = protocol.NodeIDFromString("I6KAH76-66SLLLB-5PFXSOA-UFJCDZC-YAOMLEK-CP2GB32-BV5RQST-3PSROAU")
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
	s.WithGlobal(func(fi protocol.FileIntf) bool {
		f := fi.(protocol.FileInfo)
		fs = append(fs, f)
		return true
	})
	return fs
}

func haveList(s *files.Set, n protocol.NodeID) []protocol.FileInfo {
	var fs []protocol.FileInfo
	s.WithHave(n, func(fi protocol.FileIntf) bool {
		f := fi.(protocol.FileInfo)
		fs = append(fs, f)
		return true
	})
	return fs
}

func needList(s *files.Set, n protocol.NodeID) []protocol.FileInfo {
	var fs []protocol.FileInfo
	s.WithNeed(n, func(fi protocol.FileIntf) bool {
		f := fi.(protocol.FileInfo)
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

func (l fileList) String() string {
	var b bytes.Buffer
	b.WriteString("[]protocol.FileList{\n")
	for _, f := range l {
		fmt.Fprintf(&b, "  %q: #%d, %d bytes, %d blocks, flags=%o\n", f.Name, f.Version, f.Size(), len(f.Blocks), f.Flags)
	}
	b.WriteString("}")
	return b.String()
}

func TestGlobalSet(t *testing.T) {
	lamport.Default = lamport.Clock{}

	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		t.Fatal(err)
	}

	m := files.NewSet("test", db)

	local0 := fileList{
		protocol.FileInfo{Name: "a", Version: 1000, Blocks: genBlocks(1)},
		protocol.FileInfo{Name: "b", Version: 1000, Blocks: genBlocks(2)},
		protocol.FileInfo{Name: "c", Version: 1000, Blocks: genBlocks(3)},
		protocol.FileInfo{Name: "d", Version: 1000, Blocks: genBlocks(4)},
		protocol.FileInfo{Name: "z", Version: 1000, Blocks: genBlocks(8)},
	}
	local1 := fileList{
		protocol.FileInfo{Name: "a", Version: 1000, Blocks: genBlocks(1)},
		protocol.FileInfo{Name: "b", Version: 1000, Blocks: genBlocks(2)},
		protocol.FileInfo{Name: "c", Version: 1000, Blocks: genBlocks(3)},
		protocol.FileInfo{Name: "d", Version: 1000, Blocks: genBlocks(4)},
	}
	localTot := fileList{
		local0[0],
		local0[1],
		local0[2],
		local0[3],
		protocol.FileInfo{Name: "z", Version: 1001, Flags: protocol.FlagDeleted},
	}

	remote0 := fileList{
		protocol.FileInfo{Name: "a", Version: 1000, Blocks: genBlocks(1)},
		protocol.FileInfo{Name: "b", Version: 1000, Blocks: genBlocks(2)},
		protocol.FileInfo{Name: "c", Version: 1002, Blocks: genBlocks(5)},
	}
	remote1 := fileList{
		protocol.FileInfo{Name: "b", Version: 1001, Blocks: genBlocks(6)},
		protocol.FileInfo{Name: "e", Version: 1000, Blocks: genBlocks(7)},
	}
	remoteTot := fileList{
		remote0[0],
		remote1[0],
		remote0[2],
		remote1[1],
	}

	expectedGlobal := fileList{
		remote0[0],  // a
		remote1[0],  // b
		remote0[2],  // c
		localTot[3], // d
		remote1[1],  // e
		localTot[4], // z
	}

	expectedLocalNeed := fileList{
		remote1[0],
		remote0[2],
		remote1[1],
	}

	expectedRemoteNeed := fileList{
		local0[3],
	}

	m.ReplaceWithDelete(protocol.LocalNodeID, local0)
	m.ReplaceWithDelete(protocol.LocalNodeID, local1)
	m.Replace(remoteNode0, remote0)
	m.Update(remoteNode0, remote1)

	g := fileList(globalList(m))
	sort.Sort(g)

	if fmt.Sprint(g) != fmt.Sprint(expectedGlobal) {
		t.Errorf("Global incorrect;\n A: %v !=\n E: %v", g, expectedGlobal)
	}

	h := fileList(haveList(m, protocol.LocalNodeID))
	sort.Sort(h)

	if fmt.Sprint(h) != fmt.Sprint(localTot) {
		t.Errorf("Have incorrect;\n A: %v !=\n E: %v", h, localTot)
	}

	h = fileList(haveList(m, remoteNode0))
	sort.Sort(h)

	if fmt.Sprint(h) != fmt.Sprint(remoteTot) {
		t.Errorf("Have incorrect;\n A: %v !=\n E: %v", h, remoteTot)
	}

	n := fileList(needList(m, protocol.LocalNodeID))
	sort.Sort(n)

	if fmt.Sprint(n) != fmt.Sprint(expectedLocalNeed) {
		t.Errorf("Need incorrect;\n A: %v !=\n E: %v", n, expectedLocalNeed)
	}

	n = fileList(needList(m, remoteNode0))
	sort.Sort(n)

	if fmt.Sprint(n) != fmt.Sprint(expectedRemoteNeed) {
		t.Errorf("Need incorrect;\n A: %v !=\n E: %v", n, expectedRemoteNeed)
	}

	f := m.Get(protocol.LocalNodeID, "b")
	if fmt.Sprint(f) != fmt.Sprint(localTot[1]) {
		t.Errorf("Get incorrect;\n A: %v !=\n E: %v", f, localTot[1])
	}

	f = m.Get(remoteNode0, "b")
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

	av := []protocol.NodeID{protocol.LocalNodeID, remoteNode0}
	a := m.Availability("a")
	if !(len(a) == 2 && (a[0] == av[0] && a[1] == av[1] || a[0] == av[1] && a[1] == av[0])) {
		t.Errorf("Availability incorrect;\n A: %v !=\n E: %v", a, av)
	}
	a = m.Availability("b")
	if len(a) != 1 || a[0] != remoteNode0 {
		t.Errorf("Availability incorrect;\n A: %v !=\n E: %v", a, remoteNode0)
	}
	a = m.Availability("d")
	if len(a) != 1 || a[0] != protocol.LocalNodeID {
		t.Errorf("Availability incorrect;\n A: %v !=\n E: %v", a, protocol.LocalNodeID)
	}
}

func TestNeedWithInvalid(t *testing.T) {
	lamport.Default = lamport.Clock{}

	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		t.Fatal(err)
	}

	s := files.NewSet("test", db)

	localHave := fileList{
		protocol.FileInfo{Name: "a", Version: 1000, Blocks: genBlocks(1)},
	}
	remote0Have := fileList{
		protocol.FileInfo{Name: "b", Version: 1001, Blocks: genBlocks(2)},
		protocol.FileInfo{Name: "c", Version: 1002, Blocks: genBlocks(5), Flags: protocol.FlagInvalid},
		protocol.FileInfo{Name: "d", Version: 1003, Blocks: genBlocks(7)},
	}
	remote1Have := fileList{
		protocol.FileInfo{Name: "c", Version: 1002, Blocks: genBlocks(7)},
		protocol.FileInfo{Name: "d", Version: 1003, Blocks: genBlocks(5), Flags: protocol.FlagInvalid},
		protocol.FileInfo{Name: "e", Version: 1004, Blocks: genBlocks(5), Flags: protocol.FlagInvalid},
	}

	expectedNeed := fileList{
		protocol.FileInfo{Name: "b", Version: 1001, Blocks: genBlocks(2)},
		protocol.FileInfo{Name: "c", Version: 1002, Blocks: genBlocks(7)},
		protocol.FileInfo{Name: "d", Version: 1003, Blocks: genBlocks(7)},
	}

	s.ReplaceWithDelete(protocol.LocalNodeID, localHave)
	s.Replace(remoteNode0, remote0Have)
	s.Replace(remoteNode1, remote1Have)

	need := fileList(needList(s, protocol.LocalNodeID))
	sort.Sort(need)

	if fmt.Sprint(need) != fmt.Sprint(expectedNeed) {
		t.Errorf("Need incorrect;\n A: %v !=\n E: %v", need, expectedNeed)
	}
}

func TestUpdateToInvalid(t *testing.T) {
	lamport.Default = lamport.Clock{}

	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		t.Fatal(err)
	}

	s := files.NewSet("test", db)

	localHave := fileList{
		protocol.FileInfo{Name: "a", Version: 1000, Blocks: genBlocks(1)},
		protocol.FileInfo{Name: "b", Version: 1001, Blocks: genBlocks(2)},
		protocol.FileInfo{Name: "c", Version: 1002, Blocks: genBlocks(5), Flags: protocol.FlagInvalid},
		protocol.FileInfo{Name: "d", Version: 1003, Blocks: genBlocks(7)},
	}

	s.ReplaceWithDelete(protocol.LocalNodeID, localHave)

	have := fileList(haveList(s, protocol.LocalNodeID))
	sort.Sort(have)

	if fmt.Sprint(have) != fmt.Sprint(localHave) {
		t.Errorf("Have incorrect before invalidation;\n A: %v !=\n E: %v", have, localHave)
	}

	localHave[1] = protocol.FileInfo{Name: "b", Version: 1001, Flags: protocol.FlagInvalid}
	s.Update(protocol.LocalNodeID, localHave[1:2])

	have = fileList(haveList(s, protocol.LocalNodeID))
	sort.Sort(have)

	if fmt.Sprint(have) != fmt.Sprint(localHave) {
		t.Errorf("Have incorrect after invalidation;\n A: %v !=\n E: %v", have, localHave)
	}
}

func TestInvalidAvailability(t *testing.T) {
	lamport.Default = lamport.Clock{}

	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		t.Fatal(err)
	}

	s := files.NewSet("test", db)

	remote0Have := fileList{
		protocol.FileInfo{Name: "both", Version: 1001, Blocks: genBlocks(2)},
		protocol.FileInfo{Name: "r1only", Version: 1002, Blocks: genBlocks(5), Flags: protocol.FlagInvalid},
		protocol.FileInfo{Name: "r0only", Version: 1003, Blocks: genBlocks(7)},
		protocol.FileInfo{Name: "none", Version: 1004, Blocks: genBlocks(5), Flags: protocol.FlagInvalid},
	}
	remote1Have := fileList{
		protocol.FileInfo{Name: "both", Version: 1001, Blocks: genBlocks(2)},
		protocol.FileInfo{Name: "r1only", Version: 1002, Blocks: genBlocks(7)},
		protocol.FileInfo{Name: "r0only", Version: 1003, Blocks: genBlocks(5), Flags: protocol.FlagInvalid},
		protocol.FileInfo{Name: "none", Version: 1004, Blocks: genBlocks(5), Flags: protocol.FlagInvalid},
	}

	s.Replace(remoteNode0, remote0Have)
	s.Replace(remoteNode1, remote1Have)

	if av := s.Availability("both"); len(av) != 2 {
		t.Error("Incorrect availability for 'both':", av)
	}

	if av := s.Availability("r0only"); len(av) != 1 || av[0] != remoteNode0 {
		t.Error("Incorrect availability for 'r0only':", av)
	}

	if av := s.Availability("r1only"); len(av) != 1 || av[0] != remoteNode1 {
		t.Error("Incorrect availability for 'r1only':", av)
	}

	if av := s.Availability("none"); len(av) != 0 {
		t.Error("Incorrect availability for 'none':", av)
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
	m.Replace(remoteNode0, remote)

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
	m.Replace(remoteNode0, remote)

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
	m.Replace(remoteNode0, remote)

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
	m.Replace(remoteNode0, remote)

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
	m.Replace(remoteNode0, remote)

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

	m.Replace(remoteNode0, remote)
	m.Replace(remoteNode0, nil)

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
	m.Replace(remoteNode0, remote)

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

func TestListDropRepo(t *testing.T) {
	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		t.Fatal(err)
	}

	s0 := files.NewSet("test0", db)
	local1 := []protocol.FileInfo{
		protocol.FileInfo{Name: "a", Version: 1000},
		protocol.FileInfo{Name: "b", Version: 1000},
		protocol.FileInfo{Name: "c", Version: 1000},
	}
	s0.Replace(protocol.LocalNodeID, local1)

	s1 := files.NewSet("test1", db)
	local2 := []protocol.FileInfo{
		protocol.FileInfo{Name: "d", Version: 1002},
		protocol.FileInfo{Name: "e", Version: 1002},
		protocol.FileInfo{Name: "f", Version: 1002},
	}
	s1.Replace(remoteNode0, local2)

	// Check that we have both repos and their data is in the global list

	expectedRepoList := []string{"test0", "test1"}
	if actualRepoList := files.ListRepos(db); !reflect.DeepEqual(actualRepoList, expectedRepoList) {
		t.Fatalf("RepoList mismatch\nE: %v\nA: %v", expectedRepoList, actualRepoList)
	}
	if l := len(globalList(s0)); l != 3 {
		t.Errorf("Incorrect global length %d != 3 for s0", l)
	}
	if l := len(globalList(s1)); l != 3 {
		t.Errorf("Incorrect global length %d != 3 for s1", l)
	}

	// Drop one of them and check that it's gone.

	files.DropRepo(db, "test1")

	expectedRepoList = []string{"test0"}
	if actualRepoList := files.ListRepos(db); !reflect.DeepEqual(actualRepoList, expectedRepoList) {
		t.Fatalf("RepoList mismatch\nE: %v\nA: %v", expectedRepoList, actualRepoList)
	}
	if l := len(globalList(s0)); l != 3 {
		t.Errorf("Incorrect global length %d != 3 for s0", l)
	}
	if l := len(globalList(s1)); l != 0 {
		t.Errorf("Incorrect global length %d != 0 for s1", l)
	}
}

func TestLongPath(t *testing.T) {
	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		t.Fatal(err)
	}

	s := files.NewSet("test", db)

	var b bytes.Buffer
	for i := 0; i < 100; i++ {
		b.WriteString("012345678901234567890123456789012345678901234567890")
	}
	name := b.String() // 5000 characters

	local := []protocol.FileInfo{
		protocol.FileInfo{Name: string(name), Version: 1000},
	}

	s.ReplaceWithDelete(protocol.LocalNodeID, local)

	gf := globalList(s)
	if l := len(gf); l != 1 {
		t.Fatalf("Incorrect len %d != 1 for global list", l)
	}
	if gf[0].Name != local[0].Name {
		t.Error("Incorrect long filename;\n%q !=\n%q", gf[0].Name, local[0].Name)
	}
}

/*
var gf protocol.FileInfo

func TestStressGlobalVersion(t *testing.T) {
	dur := 15 * time.Second
	if testing.Short() {
		dur = 1 * time.Second
	}

	set1 := []protocol.FileInfo{
		protocol.FileInfo{Name: "a", Version: 1000},
		protocol.FileInfo{Name: "b", Version: 1000},
	}
	set2 := []protocol.FileInfo{
		protocol.FileInfo{Name: "b", Version: 1001},
		protocol.FileInfo{Name: "c", Version: 1000},
	}

	db, err := leveldb.OpenFile("testdata/global.db", nil)
	if err != nil {
		t.Fatal(err)
	}

	m := files.NewSet("test", db)

	done := make(chan struct{})
	go stressWriter(m, remoteNode0, set1, nil, done)
	go stressWriter(m, protocol.LocalNodeID, set2, nil, done)

	t0 := time.Now()
	for time.Since(t0) < dur {
		m.WithGlobal(func(f protocol.FileInfo) bool {
			gf = f
			return true
		})
	}

	close(done)
}

func stressWriter(s *files.Set, id protocol.NodeID, set1, set2 []protocol.FileInfo, done chan struct{}) {
	one := true
	i := 0
	for {
		select {
		case <-done:
			return

		default:
			if one {
				s.Replace(id, set1)
			} else {
				s.Replace(id, set2)
			}
			one = !one
		}
		i++
	}
}
*/
