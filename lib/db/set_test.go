// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db_test

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/d4l3k/messagediff"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
)

var remoteDevice0, remoteDevice1 protocol.DeviceID

func init() {
	remoteDevice0, _ = protocol.DeviceIDFromString("AIR6LPZ-7K4PTTV-UXQSMUU-CPQ5YWH-OEDFIIQ-JUG777G-2YQXXR5-YD6AWQR")
	remoteDevice1, _ = protocol.DeviceIDFromString("I6KAH76-66SLLLB-5PFXSOA-UFJCDZC-YAOMLEK-CP2GB32-BV5RQST-3PSROAU")
}

const myID = 1

func genBlocks(n int) []protocol.BlockInfo {
	b := make([]protocol.BlockInfo, n)
	for i := range b {
		h := make([]byte, 32)
		for j := range h {
			h[j] = byte(i + j)
		}
		b[i].Size = int32(i)
		b[i].Hash = h
	}
	return b
}

func globalList(s *db.FileSet) []protocol.FileInfo {
	var fs []protocol.FileInfo
	s.WithGlobal(func(fi db.FileIntf) bool {
		f := fi.(protocol.FileInfo)
		fs = append(fs, f)
		return true
	})
	return fs
}
func globalListPrefixed(s *db.FileSet, prefix string) []db.FileInfoTruncated {
	var fs []db.FileInfoTruncated
	s.WithPrefixedGlobalTruncated(prefix, func(fi db.FileIntf) bool {
		f := fi.(db.FileInfoTruncated)
		fs = append(fs, f)
		return true
	})
	return fs
}

func haveList(s *db.FileSet, n protocol.DeviceID) []protocol.FileInfo {
	var fs []protocol.FileInfo
	s.WithHave(n, func(fi db.FileIntf) bool {
		f := fi.(protocol.FileInfo)
		fs = append(fs, f)
		return true
	})
	return fs
}

func haveListPrefixed(s *db.FileSet, n protocol.DeviceID, prefix string) []db.FileInfoTruncated {
	var fs []db.FileInfoTruncated
	s.WithPrefixedHaveTruncated(n, prefix, func(fi db.FileIntf) bool {
		f := fi.(db.FileInfoTruncated)
		fs = append(fs, f)
		return true
	})
	return fs
}

func needList(s *db.FileSet, n protocol.DeviceID) []protocol.FileInfo {
	var fs []protocol.FileInfo
	s.WithNeed(n, func(fi db.FileIntf) bool {
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
		fmt.Fprintf(&b, "  %q: #%v, %d bytes, %d blocks, perms=%o\n", f.Name, f.Version, f.Size, len(f.Blocks), f.Permissions)
	}
	b.WriteString("}")
	return b.String()
}

func TestGlobalSet(t *testing.T) {
	ldb := db.OpenMemory()

	m := db.NewFileSet("test", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

	local0 := fileList{
		protocol.FileInfo{Name: "a", Sequence: 1, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(1)},
		protocol.FileInfo{Name: "b", Sequence: 2, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(2)},
		protocol.FileInfo{Name: "c", Sequence: 3, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(3)},
		protocol.FileInfo{Name: "d", Sequence: 4, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(4)},
		protocol.FileInfo{Name: "z", Sequence: 5, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(8)},
	}
	local1 := fileList{
		protocol.FileInfo{Name: "a", Sequence: 6, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(1)},
		protocol.FileInfo{Name: "b", Sequence: 7, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(2)},
		protocol.FileInfo{Name: "c", Sequence: 8, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(3)},
		protocol.FileInfo{Name: "d", Sequence: 9, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(4)},
		protocol.FileInfo{Name: "z", Sequence: 10, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1001}}}, Deleted: true},
	}
	localTot := fileList{
		local1[0],
		local1[1],
		local1[2],
		local1[3],
		protocol.FileInfo{Name: "z", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1001}}}, Deleted: true},
	}

	remote0 := fileList{
		protocol.FileInfo{Name: "a", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(1)},
		protocol.FileInfo{Name: "b", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(2)},
		protocol.FileInfo{Name: "c", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1001}}}, Blocks: genBlocks(5)},
	}
	remote1 := fileList{
		protocol.FileInfo{Name: "b", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1001}}}, Blocks: genBlocks(6)},
		protocol.FileInfo{Name: "e", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(7)},
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

	replace(m, protocol.LocalDeviceID, local0)
	replace(m, protocol.LocalDeviceID, local1)
	replace(m, remoteDevice0, remote0)
	m.Update(remoteDevice0, remote1)

	g := fileList(globalList(m))
	sort.Sort(g)

	if fmt.Sprint(g) != fmt.Sprint(expectedGlobal) {
		t.Errorf("Global incorrect;\n A: %v !=\n E: %v", g, expectedGlobal)
	}

	globalFiles, globalDirectories, globalDeleted, globalBytes := int32(0), int32(0), int32(0), int64(0)
	for _, f := range g {
		if f.IsInvalid() {
			continue
		}
		switch {
		case f.IsDeleted():
			globalDeleted++
		case f.IsDirectory():
			globalDirectories++
		default:
			globalFiles++
		}
		globalBytes += f.FileSize()
	}
	gs := m.GlobalSize()
	if gs.Files != globalFiles {
		t.Errorf("Incorrect GlobalSize files; %d != %d", gs.Files, globalFiles)
	}
	if gs.Directories != globalDirectories {
		t.Errorf("Incorrect GlobalSize directories; %d != %d", gs.Directories, globalDirectories)
	}
	if gs.Deleted != globalDeleted {
		t.Errorf("Incorrect GlobalSize deleted; %d != %d", gs.Deleted, globalDeleted)
	}
	if gs.Bytes != globalBytes {
		t.Errorf("Incorrect GlobalSize bytes; %d != %d", gs.Bytes, globalBytes)
	}

	h := fileList(haveList(m, protocol.LocalDeviceID))
	sort.Sort(h)

	if fmt.Sprint(h) != fmt.Sprint(localTot) {
		t.Errorf("Have incorrect;\n A: %v !=\n E: %v", h, localTot)
	}

	haveFiles, haveDirectories, haveDeleted, haveBytes := int32(0), int32(0), int32(0), int64(0)
	for _, f := range h {
		if f.IsInvalid() {
			continue
		}
		switch {
		case f.IsDeleted():
			haveDeleted++
		case f.IsDirectory():
			haveDirectories++
		default:
			haveFiles++
		}
		haveBytes += f.FileSize()
	}
	ls := m.LocalSize()
	if ls.Files != haveFiles {
		t.Errorf("Incorrect LocalSize files; %d != %d", ls.Files, haveFiles)
	}
	if ls.Directories != haveDirectories {
		t.Errorf("Incorrect LocalSize directories; %d != %d", ls.Directories, haveDirectories)
	}
	if ls.Deleted != haveDeleted {
		t.Errorf("Incorrect LocalSize deleted; %d != %d", ls.Deleted, haveDeleted)
	}
	if ls.Bytes != haveBytes {
		t.Errorf("Incorrect LocalSize bytes; %d != %d", ls.Bytes, haveBytes)
	}

	h = fileList(haveList(m, remoteDevice0))
	sort.Sort(h)

	if fmt.Sprint(h) != fmt.Sprint(remoteTot) {
		t.Errorf("Have incorrect;\n A: %v !=\n E: %v", h, remoteTot)
	}

	n := fileList(needList(m, protocol.LocalDeviceID))
	sort.Sort(n)

	if fmt.Sprint(n) != fmt.Sprint(expectedLocalNeed) {
		t.Errorf("Need incorrect;\n A: %v !=\n E: %v", n, expectedLocalNeed)
	}

	n = fileList(needList(m, remoteDevice0))
	sort.Sort(n)

	if fmt.Sprint(n) != fmt.Sprint(expectedRemoteNeed) {
		t.Errorf("Need incorrect;\n A: %v !=\n E: %v", n, expectedRemoteNeed)
	}

	f, ok := m.Get(protocol.LocalDeviceID, "b")
	if !ok {
		t.Error("Unexpectedly not OK")
	}
	if fmt.Sprint(f) != fmt.Sprint(localTot[1]) {
		t.Errorf("Get incorrect;\n A: %v !=\n E: %v", f, localTot[1])
	}

	f, ok = m.Get(remoteDevice0, "b")
	if !ok {
		t.Error("Unexpectedly not OK")
	}
	if fmt.Sprint(f) != fmt.Sprint(remote1[0]) {
		t.Errorf("Get incorrect;\n A: %v !=\n E: %v", f, remote1[0])
	}

	f, ok = m.GetGlobal("b")
	if !ok {
		t.Error("Unexpectedly not OK")
	}
	if fmt.Sprint(f) != fmt.Sprint(remote1[0]) {
		t.Errorf("GetGlobal incorrect;\n A: %v !=\n E: %v", f, remote1[0])
	}

	f, ok = m.Get(protocol.LocalDeviceID, "zz")
	if ok {
		t.Error("Unexpectedly OK")
	}
	if f.Name != "" {
		t.Errorf("Get incorrect;\n A: %v !=\n E: %v", f, protocol.FileInfo{})
	}

	f, ok = m.GetGlobal("zz")
	if ok {
		t.Error("Unexpectedly OK")
	}
	if f.Name != "" {
		t.Errorf("GetGlobal incorrect;\n A: %v !=\n E: %v", f, protocol.FileInfo{})
	}

	av := []protocol.DeviceID{protocol.LocalDeviceID, remoteDevice0}
	a := m.Availability("a")
	if !(len(a) == 2 && (a[0] == av[0] && a[1] == av[1] || a[0] == av[1] && a[1] == av[0])) {
		t.Errorf("Availability incorrect;\n A: %v !=\n E: %v", a, av)
	}
	a = m.Availability("b")
	if len(a) != 1 || a[0] != remoteDevice0 {
		t.Errorf("Availability incorrect;\n A: %v !=\n E: %v", a, remoteDevice0)
	}
	a = m.Availability("d")
	if len(a) != 1 || a[0] != protocol.LocalDeviceID {
		t.Errorf("Availability incorrect;\n A: %v !=\n E: %v", a, protocol.LocalDeviceID)
	}
}

func TestNeedWithInvalid(t *testing.T) {
	ldb := db.OpenMemory()

	s := db.NewFileSet("test", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

	localHave := fileList{
		protocol.FileInfo{Name: "a", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(1)},
	}
	remote0Have := fileList{
		protocol.FileInfo{Name: "b", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1001}}}, Blocks: genBlocks(2)},
		protocol.FileInfo{Name: "c", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1002}}}, Blocks: genBlocks(5), RawInvalid: true},
		protocol.FileInfo{Name: "d", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1003}}}, Blocks: genBlocks(7)},
	}
	remote1Have := fileList{
		protocol.FileInfo{Name: "c", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1002}}}, Blocks: genBlocks(7)},
		protocol.FileInfo{Name: "d", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1003}}}, Blocks: genBlocks(5), RawInvalid: true},
		protocol.FileInfo{Name: "e", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1004}}}, Blocks: genBlocks(5), RawInvalid: true},
	}

	expectedNeed := fileList{
		protocol.FileInfo{Name: "b", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1001}}}, Blocks: genBlocks(2)},
		protocol.FileInfo{Name: "c", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1002}}}, Blocks: genBlocks(7)},
		protocol.FileInfo{Name: "d", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1003}}}, Blocks: genBlocks(7)},
	}

	replace(s, protocol.LocalDeviceID, localHave)
	replace(s, remoteDevice0, remote0Have)
	replace(s, remoteDevice1, remote1Have)

	need := fileList(needList(s, protocol.LocalDeviceID))
	sort.Sort(need)

	if fmt.Sprint(need) != fmt.Sprint(expectedNeed) {
		t.Errorf("Need incorrect;\n A: %v !=\n E: %v", need, expectedNeed)
	}
}

func TestUpdateToInvalid(t *testing.T) {
	ldb := db.OpenMemory()

	folder := "test"
	s := db.NewFileSet(folder, fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)
	f := db.NewBlockFinder(ldb)

	localHave := fileList{
		protocol.FileInfo{Name: "a", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(1)},
		protocol.FileInfo{Name: "b", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1001}}}, Blocks: genBlocks(2)},
		protocol.FileInfo{Name: "c", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1002}}}, Blocks: genBlocks(5), LocalFlags: protocol.FlagLocalIgnored},
		protocol.FileInfo{Name: "d", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1003}}}, Blocks: genBlocks(7)},
		protocol.FileInfo{Name: "e", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1003}}}, LocalFlags: protocol.FlagLocalIgnored},
	}

	replace(s, protocol.LocalDeviceID, localHave)

	have := fileList(haveList(s, protocol.LocalDeviceID))
	sort.Sort(have)

	if fmt.Sprint(have) != fmt.Sprint(localHave) {
		t.Errorf("Have incorrect before invalidation;\n A: %v !=\n E: %v", have, localHave)
	}

	oldBlockHash := localHave[1].Blocks[0].Hash

	localHave[1].LocalFlags = protocol.FlagLocalIgnored
	localHave[1].Blocks = nil

	localHave[4].LocalFlags = 0
	localHave[4].Blocks = genBlocks(3)

	s.Update(protocol.LocalDeviceID, append(fileList{}, localHave[1], localHave[4]))

	have = fileList(haveList(s, protocol.LocalDeviceID))
	sort.Sort(have)

	if fmt.Sprint(have) != fmt.Sprint(localHave) {
		t.Errorf("Have incorrect after invalidation;\n A: %v !=\n E: %v", have, localHave)
	}

	f.Iterate([]string{folder}, oldBlockHash, func(folder, file string, index int32) bool {
		if file == localHave[1].Name {
			t.Errorf("Found unexpected block in blockmap for invalidated file")
			return true
		}
		return false
	})

	if !f.Iterate([]string{folder}, localHave[4].Blocks[0].Hash, func(folder, file string, index int32) bool {
		if file == localHave[4].Name {
			return true
		}
		return false
	}) {
		t.Errorf("First block of un-invalidated file is missing from blockmap")
	}
}

func TestInvalidAvailability(t *testing.T) {
	ldb := db.OpenMemory()

	s := db.NewFileSet("test", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

	remote0Have := fileList{
		protocol.FileInfo{Name: "both", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1001}}}, Blocks: genBlocks(2)},
		protocol.FileInfo{Name: "r1only", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1002}}}, Blocks: genBlocks(5), RawInvalid: true},
		protocol.FileInfo{Name: "r0only", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1003}}}, Blocks: genBlocks(7)},
		protocol.FileInfo{Name: "none", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1004}}}, Blocks: genBlocks(5), RawInvalid: true},
	}
	remote1Have := fileList{
		protocol.FileInfo{Name: "both", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1001}}}, Blocks: genBlocks(2)},
		protocol.FileInfo{Name: "r1only", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1002}}}, Blocks: genBlocks(7)},
		protocol.FileInfo{Name: "r0only", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1003}}}, Blocks: genBlocks(5), RawInvalid: true},
		protocol.FileInfo{Name: "none", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1004}}}, Blocks: genBlocks(5), RawInvalid: true},
	}

	replace(s, remoteDevice0, remote0Have)
	replace(s, remoteDevice1, remote1Have)

	if av := s.Availability("both"); len(av) != 2 {
		t.Error("Incorrect availability for 'both':", av)
	}

	if av := s.Availability("r0only"); len(av) != 1 || av[0] != remoteDevice0 {
		t.Error("Incorrect availability for 'r0only':", av)
	}

	if av := s.Availability("r1only"); len(av) != 1 || av[0] != remoteDevice1 {
		t.Error("Incorrect availability for 'r1only':", av)
	}

	if av := s.Availability("none"); len(av) != 0 {
		t.Error("Incorrect availability for 'none':", av)
	}
}

func TestGlobalReset(t *testing.T) {
	ldb := db.OpenMemory()

	m := db.NewFileSet("test", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

	local := []protocol.FileInfo{
		{Name: "a", Sequence: 1, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
		{Name: "b", Sequence: 2, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
		{Name: "c", Sequence: 3, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
		{Name: "d", Sequence: 4, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
	}

	remote := []protocol.FileInfo{
		{Name: "a", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
		{Name: "b", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1001}}}},
		{Name: "c", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1002}}}},
		{Name: "e", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
	}

	replace(m, protocol.LocalDeviceID, local)
	g := globalList(m)
	sort.Sort(fileList(g))

	if diff, equal := messagediff.PrettyDiff(local, g); !equal {
		t.Errorf("Global incorrect;\nglobal: %v\n!=\nlocal: %v\ndiff:\n%s", g, local, diff)
	}

	replace(m, remoteDevice0, remote)
	replace(m, remoteDevice0, nil)

	g = globalList(m)
	sort.Sort(fileList(g))

	if diff, equal := messagediff.PrettyDiff(local, g); !equal {
		t.Errorf("Global incorrect;\nglobal: %v\n!=\nlocal: %v\ndiff:\n%s", g, local, diff)
	}
}

func TestNeed(t *testing.T) {
	ldb := db.OpenMemory()

	m := db.NewFileSet("test", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

	local := []protocol.FileInfo{
		{Name: "b", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
		{Name: "a", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
		{Name: "c", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
		{Name: "d", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
	}

	remote := []protocol.FileInfo{
		{Name: "a", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
		{Name: "b", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1001}}}},
		{Name: "c", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1002}}}},
		{Name: "e", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
	}

	shouldNeed := []protocol.FileInfo{
		{Name: "b", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1001}}}},
		{Name: "c", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1002}}}},
		{Name: "e", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
	}

	replace(m, protocol.LocalDeviceID, local)
	replace(m, remoteDevice0, remote)

	need := needList(m, protocol.LocalDeviceID)

	sort.Sort(fileList(need))
	sort.Sort(fileList(shouldNeed))

	if fmt.Sprint(need) != fmt.Sprint(shouldNeed) {
		t.Errorf("Need incorrect;\n%v !=\n%v", need, shouldNeed)
	}
}

func TestSequence(t *testing.T) {
	ldb := db.OpenMemory()

	m := db.NewFileSet("test", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

	local1 := []protocol.FileInfo{
		{Name: "a", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
		{Name: "b", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
		{Name: "c", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
		{Name: "d", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
	}

	local2 := []protocol.FileInfo{
		local1[0],
		// [1] deleted
		local1[2],
		{Name: "d", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1002}}}},
		{Name: "e", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
	}

	replace(m, protocol.LocalDeviceID, local1)
	c0 := m.Sequence(protocol.LocalDeviceID)

	replace(m, protocol.LocalDeviceID, local2)
	c1 := m.Sequence(protocol.LocalDeviceID)
	if !(c1 > c0) {
		t.Fatal("Local version number should have incremented")
	}
}

func TestListDropFolder(t *testing.T) {
	ldb := db.OpenMemory()

	s0 := db.NewFileSet("test0", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)
	local1 := []protocol.FileInfo{
		{Name: "a", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
		{Name: "b", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
		{Name: "c", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
	}
	replace(s0, protocol.LocalDeviceID, local1)

	s1 := db.NewFileSet("test1", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)
	local2 := []protocol.FileInfo{
		{Name: "d", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1002}}}},
		{Name: "e", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1002}}}},
		{Name: "f", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1002}}}},
	}
	replace(s1, remoteDevice0, local2)

	// Check that we have both folders and their data is in the global list

	expectedFolderList := []string{"test0", "test1"}
	actualFolderList := ldb.ListFolders()
	if diff, equal := messagediff.PrettyDiff(expectedFolderList, actualFolderList); !equal {
		t.Fatalf("FolderList mismatch. Diff:\n%s", diff)
	}
	if l := len(globalList(s0)); l != 3 {
		t.Errorf("Incorrect global length %d != 3 for s0", l)
	}
	if l := len(globalList(s1)); l != 3 {
		t.Errorf("Incorrect global length %d != 3 for s1", l)
	}

	// Drop one of them and check that it's gone.

	db.DropFolder(ldb, "test1")

	expectedFolderList = []string{"test0"}
	actualFolderList = ldb.ListFolders()
	if diff, equal := messagediff.PrettyDiff(expectedFolderList, actualFolderList); !equal {
		t.Fatalf("FolderList mismatch. Diff:\n%s", diff)
	}
	if l := len(globalList(s0)); l != 3 {
		t.Errorf("Incorrect global length %d != 3 for s0", l)
	}
	if l := len(globalList(s1)); l != 0 {
		t.Errorf("Incorrect global length %d != 0 for s1", l)
	}
}

func TestGlobalNeedWithInvalid(t *testing.T) {
	ldb := db.OpenMemory()

	s := db.NewFileSet("test1", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

	rem0 := fileList{
		protocol.FileInfo{Name: "a", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1002}}}, Blocks: genBlocks(4)},
		protocol.FileInfo{Name: "b", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1002}}}, RawInvalid: true},
		protocol.FileInfo{Name: "c", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1002}}}, Blocks: genBlocks(4)},
		protocol.FileInfo{Name: "d", Version: protocol.Vector{Counters: []protocol.Counter{{ID: remoteDevice0.Short(), Value: 1002}}}},
	}
	replace(s, remoteDevice0, rem0)

	rem1 := fileList{
		protocol.FileInfo{Name: "a", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1002}}}, Blocks: genBlocks(4)},
		protocol.FileInfo{Name: "b", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1002}}}, Blocks: genBlocks(4)},
		protocol.FileInfo{Name: "c", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1002}}}, RawInvalid: true},
		protocol.FileInfo{Name: "d", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1002}}}, RawInvalid: true, ModifiedS: 10},
	}
	replace(s, remoteDevice1, rem1)

	total := fileList{
		// There's a valid copy of each file, so it should be merged
		protocol.FileInfo{Name: "a", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1002}}}, Blocks: genBlocks(4)},
		protocol.FileInfo{Name: "b", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1002}}}, Blocks: genBlocks(4)},
		protocol.FileInfo{Name: "c", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1002}}}, Blocks: genBlocks(4)},
		// in conflict and older, but still wins as the other is invalid
		protocol.FileInfo{Name: "d", Version: protocol.Vector{Counters: []protocol.Counter{{ID: remoteDevice0.Short(), Value: 1002}}}},
	}

	need := fileList(needList(s, protocol.LocalDeviceID))
	if fmt.Sprint(need) != fmt.Sprint(total) {
		t.Errorf("Need incorrect;\n A: %v !=\n E: %v", need, total)
	}

	global := fileList(globalList(s))
	if fmt.Sprint(global) != fmt.Sprint(total) {
		t.Errorf("Global incorrect;\n A: %v !=\n E: %v", global, total)
	}
}

func TestLongPath(t *testing.T) {
	ldb := db.OpenMemory()

	s := db.NewFileSet("test", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

	var b bytes.Buffer
	for i := 0; i < 100; i++ {
		b.WriteString("012345678901234567890123456789012345678901234567890")
	}
	name := b.String() // 5000 characters

	local := []protocol.FileInfo{
		{Name: string(name), Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
	}

	replace(s, protocol.LocalDeviceID, local)

	gf := globalList(s)
	if l := len(gf); l != 1 {
		t.Fatalf("Incorrect len %d != 1 for global list", l)
	}
	if gf[0].Name != local[0].Name {
		t.Errorf("Incorrect long filename;\n%q !=\n%q",
			gf[0].Name, local[0].Name)
	}
}

func TestCommitted(t *testing.T) {
	// Verify that the Committed counter increases when we change things and
	// doesn't increase when we don't.

	ldb := db.OpenMemory()

	s := db.NewFileSet("test", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

	local := []protocol.FileInfo{
		{Name: string("file"), Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
	}

	// Adding a file should increase the counter

	c0 := ldb.Committed()

	replace(s, protocol.LocalDeviceID, local)

	c1 := ldb.Committed()
	if c1 <= c0 {
		t.Errorf("committed data didn't increase; %d <= %d", c1, c0)
	}

	// Updating with something identical should not do anything

	s.Update(protocol.LocalDeviceID, local)

	c2 := ldb.Committed()
	if c2 > c1 {
		t.Errorf("replace with same contents should do nothing but %d > %d", c2, c1)
	}
}

func BenchmarkUpdateOneFile(b *testing.B) {
	local0 := fileList{
		protocol.FileInfo{Name: "a", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(1)},
		protocol.FileInfo{Name: "b", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(2)},
		protocol.FileInfo{Name: "c", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(3)},
		protocol.FileInfo{Name: "d", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(4)},
		// A longer name is more realistic and causes more allocations
		protocol.FileInfo{Name: "zajksdhaskjdh/askjdhaskjdashkajshd/kasjdhaskjdhaskdjhaskdjash/dkjashdaksjdhaskdjahskdjh", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(8)},
	}

	ldb, err := db.Open("testdata/benchmarkupdate.db")
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		ldb.Close()
		os.RemoveAll("testdata/benchmarkupdate.db")
	}()

	m := db.NewFileSet("test", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)
	replace(m, protocol.LocalDeviceID, local0)
	l := local0[4:5]

	for i := 0; i < b.N; i++ {
		l[0].Version = l[0].Version.Update(myID)
		m.Update(protocol.LocalDeviceID, local0)
	}

	b.ReportAllocs()
}

func TestIndexID(t *testing.T) {
	ldb := db.OpenMemory()

	s := db.NewFileSet("test", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

	// The Index ID for some random device is zero by default.
	id := s.IndexID(remoteDevice0)
	if id != 0 {
		t.Errorf("index ID for remote device should default to zero, not %d", id)
	}

	// The Index ID for someone else should be settable
	s.SetIndexID(remoteDevice0, 42)
	id = s.IndexID(remoteDevice0)
	if id != 42 {
		t.Errorf("index ID for remote device should be remembered; got %d, expected %d", id, 42)
	}

	// Our own index ID should be generated randomly.
	id = s.IndexID(protocol.LocalDeviceID)
	if id == 0 {
		t.Errorf("index ID for local device should be random, not zero")
	}
	t.Logf("random index ID is 0x%016x", id)

	// But of course always the same after that.
	again := s.IndexID(protocol.LocalDeviceID)
	if again != id {
		t.Errorf("index ID changed; %d != %d", again, id)
	}
}

func TestDropFiles(t *testing.T) {
	ldb := db.OpenMemory()

	m := db.NewFileSet("test", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

	local0 := fileList{
		protocol.FileInfo{Name: "a", Sequence: 1, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(1)},
		protocol.FileInfo{Name: "b", Sequence: 2, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(2)},
		protocol.FileInfo{Name: "c", Sequence: 3, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(3)},
		protocol.FileInfo{Name: "d", Sequence: 4, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(4)},
		protocol.FileInfo{Name: "z", Sequence: 5, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(8)},
	}

	remote0 := fileList{
		protocol.FileInfo{Name: "a", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(1)},
		protocol.FileInfo{Name: "b", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(2)},
		protocol.FileInfo{Name: "c", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1001}}}, Blocks: genBlocks(5)},
	}

	// Insert files

	m.Update(protocol.LocalDeviceID, local0)
	m.Update(remoteDevice0, remote0)

	// Check that they're there

	h := haveList(m, protocol.LocalDeviceID)
	if len(h) != len(local0) {
		t.Errorf("Incorrect number of files after update, %d != %d", len(h), len(local0))
	}

	h = haveList(m, remoteDevice0)
	if len(h) != len(remote0) {
		t.Errorf("Incorrect number of files after update, %d != %d", len(h), len(local0))
	}

	g := globalList(m)
	if len(g) != len(local0) {
		// local0 covers all files
		t.Errorf("Incorrect global files after update, %d != %d", len(g), len(local0))
	}

	// Drop the local files and recheck

	m.Drop(protocol.LocalDeviceID)

	h = haveList(m, protocol.LocalDeviceID)
	if len(h) != 0 {
		t.Errorf("Incorrect number of files after drop, %d != %d", len(h), 0)
	}

	h = haveList(m, remoteDevice0)
	if len(h) != len(remote0) {
		t.Errorf("Incorrect number of files after update, %d != %d", len(h), len(local0))
	}

	g = globalList(m)
	if len(g) != len(remote0) {
		// the ones in remote0 remain
		t.Errorf("Incorrect global files after update, %d != %d", len(g), len(remote0))
	}
}

func TestIssue4701(t *testing.T) {
	ldb := db.OpenMemory()

	s := db.NewFileSet("test", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

	localHave := fileList{
		protocol.FileInfo{Name: "a", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
		protocol.FileInfo{Name: "b", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, LocalFlags: protocol.FlagLocalIgnored},
	}

	s.Update(protocol.LocalDeviceID, localHave)

	if c := s.LocalSize(); c.Files != 1 {
		t.Errorf("Expected 1 local file, got %v", c.Files)
	}
	if c := s.GlobalSize(); c.Files != 1 {
		t.Errorf("Expected 1 global file, got %v", c.Files)
	}

	localHave[1].LocalFlags = 0
	s.Update(protocol.LocalDeviceID, localHave)

	if c := s.LocalSize(); c.Files != 2 {
		t.Errorf("Expected 2 local files, got %v", c.Files)
	}
	if c := s.GlobalSize(); c.Files != 2 {
		t.Errorf("Expected 2 global files, got %v", c.Files)
	}

	localHave[0].LocalFlags = protocol.FlagLocalIgnored
	localHave[1].LocalFlags = protocol.FlagLocalIgnored
	s.Update(protocol.LocalDeviceID, localHave)

	if c := s.LocalSize(); c.Files != 0 {
		t.Errorf("Expected 0 local files, got %v", c.Files)
	}
	if c := s.GlobalSize(); c.Files != 0 {
		t.Errorf("Expected 0 global files, got %v", c.Files)
	}
}

func TestWithHaveSequence(t *testing.T) {
	ldb := db.OpenMemory()

	folder := "test"
	s := db.NewFileSet(folder, fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

	// The files must not be in alphabetical order
	localHave := fileList{
		protocol.FileInfo{Name: "e", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1003}}}, RawInvalid: true},
		protocol.FileInfo{Name: "b", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1001}}}, Blocks: genBlocks(2)},
		protocol.FileInfo{Name: "d", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1003}}}, Blocks: genBlocks(7)},
		protocol.FileInfo{Name: "a", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(1)},
		protocol.FileInfo{Name: "c", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1002}}}, Blocks: genBlocks(5), RawInvalid: true},
	}

	replace(s, protocol.LocalDeviceID, localHave)

	i := 2
	s.WithHaveSequence(int64(i), func(fi db.FileIntf) bool {
		if f := fi.(protocol.FileInfo); !f.IsEquivalent(localHave[i-1]) {
			t.Fatalf("Got %v\nExpected %v", f, localHave[i-1])
		}
		i++
		return true
	})
}

func TestStressWithHaveSequence(t *testing.T) {
	// This races two loops against each other: one that contiously does
	// updates, and one that continously does sequence walks. The test fails
	// if the sequence walker sees a discontinuity.

	if testing.Short() {
		t.Skip("Takes a long time")
	}

	ldb := db.OpenMemory()

	folder := "test"
	s := db.NewFileSet(folder, fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

	var localHave []protocol.FileInfo
	for i := 0; i < 100; i++ {
		localHave = append(localHave, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Blocks: genBlocks(i * 10)})
	}

	done := make(chan struct{})
	t0 := time.Now()
	go func() {
		for time.Since(t0) < 10*time.Second {
			for j, f := range localHave {
				localHave[j].Version = f.Version.Update(42)
			}

			s.Update(protocol.LocalDeviceID, localHave)
		}
		close(done)
	}()

	var prevSeq int64 = 0
loop:
	for {
		select {
		case <-done:
			break loop
		default:
		}
		s.WithHaveSequence(prevSeq+1, func(fi db.FileIntf) bool {
			if fi.SequenceNo() < prevSeq+1 {
				t.Fatal("Skipped ", prevSeq+1, fi.SequenceNo())
			}
			prevSeq = fi.SequenceNo()
			return true
		})
	}
}

func TestIssue4925(t *testing.T) {
	ldb := db.OpenMemory()

	folder := "test"
	s := db.NewFileSet(folder, fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

	localHave := fileList{
		protocol.FileInfo{Name: "dir"},
		protocol.FileInfo{Name: "dir.file"},
		protocol.FileInfo{Name: "dir/file"},
	}

	replace(s, protocol.LocalDeviceID, localHave)

	for _, prefix := range []string{"dir", "dir/"} {
		pl := haveListPrefixed(s, protocol.LocalDeviceID, prefix)
		if l := len(pl); l != 2 {
			t.Errorf("Expected 2, got %v local items below %v", l, prefix)
		}
		pl = globalListPrefixed(s, prefix)
		if l := len(pl); l != 2 {
			t.Errorf("Expected 2, got %v global items below %v", l, prefix)
		}
	}
}

func TestMoveGlobalBack(t *testing.T) {
	ldb := db.OpenMemory()

	folder := "test"
	file := "foo"
	s := db.NewFileSet(folder, fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

	localHave := fileList{{Name: file, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1}}}, Blocks: genBlocks(1), ModifiedS: 10, Size: 1}}
	remote0Have := fileList{{Name: file, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1}, {ID: remoteDevice0.Short(), Value: 1}}}, Blocks: genBlocks(2), ModifiedS: 0, Size: 2}}

	s.Update(protocol.LocalDeviceID, localHave)
	s.Update(remoteDevice0, remote0Have)

	if need := needList(s, protocol.LocalDeviceID); len(need) != 1 {
		t.Error("Expected 1 local need, got", need)
	} else if !need[0].IsEquivalent(remote0Have[0]) {
		t.Errorf("Local need incorrect;\n A: %v !=\n E: %v", need[0], remote0Have[0])
	}

	if need := needList(s, remoteDevice0); len(need) != 0 {
		t.Error("Expected no need for remote 0, got", need)
	}

	ls := s.LocalSize()
	if haveBytes := localHave[0].Size; ls.Bytes != haveBytes {
		t.Errorf("Incorrect LocalSize bytes; %d != %d", ls.Bytes, haveBytes)
	}

	gs := s.GlobalSize()
	if globalBytes := remote0Have[0].Size; gs.Bytes != globalBytes {
		t.Errorf("Incorrect GlobalSize bytes; %d != %d", gs.Bytes, globalBytes)
	}

	// That's what happens when something becomes unignored or something.
	// In any case it will be moved back from first spot in the global list
	// which is the scenario to be tested here.
	remote0Have[0].Version = remote0Have[0].Version.Update(remoteDevice0.Short()).DropOthers(remoteDevice0.Short())
	s.Update(remoteDevice0, remote0Have)

	if need := needList(s, remoteDevice0); len(need) != 1 {
		t.Error("Expected 1 need for remote 0, got", need)
	} else if !need[0].IsEquivalent(localHave[0]) {
		t.Errorf("Need for remote 0 incorrect;\n A: %v !=\n E: %v", need[0], localHave[0])
	}

	if need := needList(s, protocol.LocalDeviceID); len(need) != 0 {
		t.Error("Expected no local need, got", need)
	}

	ls = s.LocalSize()
	if haveBytes := localHave[0].Size; ls.Bytes != haveBytes {
		t.Errorf("Incorrect LocalSize bytes; %d != %d", ls.Bytes, haveBytes)
	}

	gs = s.GlobalSize()
	if globalBytes := localHave[0].Size; gs.Bytes != globalBytes {
		t.Errorf("Incorrect GlobalSize bytes; %d != %d", gs.Bytes, globalBytes)
	}
}

// TestIssue5007 checks, that updating the local device with an invalid file
// info with the newest version does indeed remove that file from the list of
// needed files.
// https://github.com/syncthing/syncthing/issues/5007
func TestIssue5007(t *testing.T) {
	ldb := db.OpenMemory()

	folder := "test"
	file := "foo"
	s := db.NewFileSet(folder, fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

	fs := fileList{{Name: file, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1}}}}}

	s.Update(remoteDevice0, fs)

	if need := needList(s, protocol.LocalDeviceID); len(need) != 1 {
		t.Fatal("Expected 1 local need, got", need)
	} else if !need[0].IsEquivalent(fs[0]) {
		t.Fatalf("Local need incorrect;\n A: %v !=\n E: %v", need[0], fs[0])
	}

	fs[0].LocalFlags = protocol.FlagLocalIgnored
	s.Update(protocol.LocalDeviceID, fs)

	if need := needList(s, protocol.LocalDeviceID); len(need) != 0 {
		t.Fatal("Expected no local need, got", need)
	}
}

// TestNeedDeleted checks that a file that doesn't exist locally isn't needed
// when the global file is deleted.
func TestNeedDeleted(t *testing.T) {
	ldb := db.OpenMemory()

	folder := "test"
	file := "foo"
	s := db.NewFileSet(folder, fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

	fs := fileList{{Name: file, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1}}}, Deleted: true}}

	s.Update(remoteDevice0, fs)

	if need := needList(s, protocol.LocalDeviceID); len(need) != 0 {
		t.Fatal("Expected no local need, got", need)
	}

	fs[0].Deleted = false
	fs[0].Version = fs[0].Version.Update(remoteDevice0.Short())
	s.Update(remoteDevice0, fs)

	if need := needList(s, protocol.LocalDeviceID); len(need) != 1 {
		t.Fatal("Expected 1 local need, got", need)
	} else if !need[0].IsEquivalent(fs[0]) {
		t.Fatalf("Local need incorrect;\n A: %v !=\n E: %v", need[0], fs[0])
	}

	fs[0].Deleted = true
	fs[0].Version = fs[0].Version.Update(remoteDevice0.Short())
	s.Update(remoteDevice0, fs)

	if need := needList(s, protocol.LocalDeviceID); len(need) != 0 {
		t.Fatal("Expected no local need, got", need)
	}
}

func TestReceiveOnlyAccounting(t *testing.T) {
	ldb := db.OpenMemory()

	folder := "test"
	s := db.NewFileSet(folder, fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

	local := protocol.DeviceID{1}
	remote := protocol.DeviceID{2}

	// Three files that have been created by the remote device

	version := protocol.Vector{Counters: []protocol.Counter{{ID: remote.Short(), Value: 1}}}
	files := fileList{
		protocol.FileInfo{Name: "f1", Size: 10, Sequence: 1, Version: version},
		protocol.FileInfo{Name: "f2", Size: 10, Sequence: 1, Version: version},
		protocol.FileInfo{Name: "f3", Size: 10, Sequence: 1, Version: version},
	}

	// We have synced them locally

	replace(s, protocol.LocalDeviceID, files)
	replace(s, remote, files)

	if n := s.LocalSize().Files; n != 3 {
		t.Fatal("expected 3 local files initially, not", n)
	}
	if n := s.LocalSize().Bytes; n != 30 {
		t.Fatal("expected 30 local bytes initially, not", n)
	}
	if n := s.GlobalSize().Files; n != 3 {
		t.Fatal("expected 3 global files initially, not", n)
	}
	if n := s.GlobalSize().Bytes; n != 30 {
		t.Fatal("expected 30 global bytes initially, not", n)
	}
	if n := s.ReceiveOnlyChangedSize().Files; n != 0 {
		t.Fatal("expected 0 receive only changed files initially, not", n)
	}
	if n := s.ReceiveOnlyChangedSize().Bytes; n != 0 {
		t.Fatal("expected 0 receive only changed bytes initially, not", n)
	}

	// Detected a local change in a receive only folder

	changed := files[0]
	changed.Version = changed.Version.Update(local.Short())
	changed.Size = 100
	changed.ModifiedBy = local.Short()
	changed.LocalFlags = protocol.FlagLocalReceiveOnly
	s.Update(protocol.LocalDeviceID, []protocol.FileInfo{changed})

	// Check that we see the files

	if n := s.LocalSize().Files; n != 3 {
		t.Fatal("expected 3 local files after local change, not", n)
	}
	if n := s.LocalSize().Bytes; n != 120 {
		t.Fatal("expected 120 local bytes after local change, not", n)
	}
	if n := s.GlobalSize().Files; n != 3 {
		t.Fatal("expected 3 global files after local change, not", n)
	}
	if n := s.GlobalSize().Bytes; n != 120 {
		t.Fatal("expected 120 global bytes after local change, not", n)
	}
	if n := s.ReceiveOnlyChangedSize().Files; n != 1 {
		t.Fatal("expected 1 receive only changed file after local change, not", n)
	}
	if n := s.ReceiveOnlyChangedSize().Bytes; n != 100 {
		t.Fatal("expected 100 receive only changed btyes after local change, not", n)
	}

	// Fake a revert. That's a two step process, first converting our
	// changed file into a less preferred variant, then pulling down the old
	// version.

	changed.Version = protocol.Vector{}
	changed.LocalFlags &^= protocol.FlagLocalReceiveOnly
	s.Update(protocol.LocalDeviceID, []protocol.FileInfo{changed})

	s.Update(protocol.LocalDeviceID, []protocol.FileInfo{files[0]})

	// Check that we see the files, same data as initially

	if n := s.LocalSize().Files; n != 3 {
		t.Fatal("expected 3 local files after revert, not", n)
	}
	if n := s.LocalSize().Bytes; n != 30 {
		t.Fatal("expected 30 local bytes after revert, not", n)
	}
	if n := s.GlobalSize().Files; n != 3 {
		t.Fatal("expected 3 global files after revert, not", n)
	}
	if n := s.GlobalSize().Bytes; n != 30 {
		t.Fatal("expected 30 global bytes after revert, not", n)
	}
	if n := s.ReceiveOnlyChangedSize().Files; n != 0 {
		t.Fatal("expected 0 receive only changed files after revert, not", n)
	}
	if n := s.ReceiveOnlyChangedSize().Bytes; n != 0 {
		t.Fatal("expected 0 receive only changed bytes after revert, not", n)
	}
}

func TestNeedAfterUnignore(t *testing.T) {
	ldb := db.OpenMemory()

	folder := "test"
	file := "foo"
	s := db.NewFileSet(folder, fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

	remID := remoteDevice0.Short()

	// Initial state: Devices in sync, locally ignored
	local := protocol.FileInfo{Name: file, Version: protocol.Vector{Counters: []protocol.Counter{{ID: remID, Value: 1}, {ID: myID, Value: 1}}}, ModifiedS: 10}
	local.SetIgnored(myID)
	remote := protocol.FileInfo{Name: file, Version: protocol.Vector{Counters: []protocol.Counter{{ID: remID, Value: 1}, {ID: myID, Value: 1}}}, ModifiedS: 10}
	s.Update(protocol.LocalDeviceID, fileList{local})
	s.Update(remoteDevice0, fileList{remote})

	// Unignore locally -> conflicting changes. Remote is newer, thus winning.
	local.Version = local.Version.Update(myID)
	local.Version = local.Version.DropOthers(myID)
	local.LocalFlags = 0
	local.ModifiedS = 0
	s.Update(protocol.LocalDeviceID, fileList{local})

	if need := needList(s, protocol.LocalDeviceID); len(need) != 1 {
		t.Fatal("Expected one local need, got", need)
	} else if !need[0].IsEquivalent(remote) {
		t.Fatalf("Got %v, expected %v", need[0], remote)
	}
}

func TestRemoteInvalidNotAccounted(t *testing.T) {
	// Remote files with the invalid bit should not count.

	ldb := db.OpenMemory()
	s := db.NewFileSet("test", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

	files := []protocol.FileInfo{
		{Name: "a", Size: 1234, Sequence: 42, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1003}}}},                   // valid, should count
		{Name: "b", Size: 1234, Sequence: 43, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1003}}}, RawInvalid: true}, // invalid, doesn't count
	}
	s.Update(remoteDevice0, files)

	global := s.GlobalSize()
	if global.Files != 1 {
		t.Error("Expected one file in global size, not", global.Files)
	}
	if global.Bytes != 1234 {
		t.Error("Expected 1234 bytes in global size, not", global.Bytes)
	}
}

func replace(fs *db.FileSet, device protocol.DeviceID, files []protocol.FileInfo) {
	fs.Drop(device)
	fs.Update(device, files)
}
