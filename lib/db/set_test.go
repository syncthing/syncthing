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
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/d4l3k/messagediff"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/db/backend"
	"github.com/syncthing/syncthing/lib/events"
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
		b[i].Size = i
		b[i].Hash = h
	}
	return b
}

func globalList(s *db.FileSet) []protocol.FileInfo {
	var fs []protocol.FileInfo
	snap := s.Snapshot()
	defer snap.Release()
	snap.WithGlobal(func(fi protocol.FileIntf) bool {
		f := fi.(protocol.FileInfo)
		fs = append(fs, f)
		return true
	})
	return fs
}
func globalListPrefixed(s *db.FileSet, prefix string) []db.FileInfoTruncated {
	var fs []db.FileInfoTruncated
	snap := s.Snapshot()
	defer snap.Release()
	snap.WithPrefixedGlobalTruncated(prefix, func(fi protocol.FileIntf) bool {
		f := fi.(db.FileInfoTruncated)
		fs = append(fs, f)
		return true
	})
	return fs
}

func haveList(s *db.FileSet, n protocol.DeviceID) []protocol.FileInfo {
	var fs []protocol.FileInfo
	snap := s.Snapshot()
	defer snap.Release()
	snap.WithHave(n, func(fi protocol.FileIntf) bool {
		f := fi.(protocol.FileInfo)
		fs = append(fs, f)
		return true
	})
	return fs
}

func haveListPrefixed(s *db.FileSet, n protocol.DeviceID, prefix string) []db.FileInfoTruncated {
	var fs []db.FileInfoTruncated
	snap := s.Snapshot()
	defer snap.Release()
	snap.WithPrefixedHaveTruncated(n, prefix, func(fi protocol.FileIntf) bool {
		f := fi.(db.FileInfoTruncated)
		fs = append(fs, f)
		return true
	})
	return fs
}

func needList(s *db.FileSet, n protocol.DeviceID) []protocol.FileInfo {
	var fs []protocol.FileInfo
	snap := s.Snapshot()
	defer snap.Release()
	snap.WithNeed(n, func(fi protocol.FileIntf) bool {
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

func setSequence(seq int64, files fileList) int64 {
	for i := range files {
		seq++
		files[i].Sequence = seq
	}
	return seq
}

func setBlocksHash(files fileList) {
	for i, f := range files {
		files[i].BlocksHash = protocol.BlocksHash(f.Blocks)
	}
}

func TestGlobalSet(t *testing.T) {
	ldb := newLowlevelMemory(t)
	defer ldb.Close()

	m := newFileSet(t, "test", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

	local0 := fileList{
		protocol.FileInfo{Name: "a", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(1)},
		protocol.FileInfo{Name: "b", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(2)},
		protocol.FileInfo{Name: "c", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(3)},
		protocol.FileInfo{Name: "d", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(4)},
		protocol.FileInfo{Name: "z", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(8)},
	}
	localSeq := setSequence(0, local0)
	setBlocksHash(local0)
	local1 := fileList{
		protocol.FileInfo{Name: "a", Sequence: 6, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(1)},
		protocol.FileInfo{Name: "b", Sequence: 7, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(2)},
		protocol.FileInfo{Name: "c", Sequence: 8, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(3)},
		protocol.FileInfo{Name: "d", Sequence: 9, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(4)},
		protocol.FileInfo{Name: "z", Sequence: 10, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1001}}}, Deleted: true},
	}
	setSequence(localSeq, local1)
	setBlocksHash(local1)
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
	remoteSeq := setSequence(0, remote0)
	setBlocksHash(remote0)
	remote1 := fileList{
		protocol.FileInfo{Name: "b", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1001}}}, Blocks: genBlocks(6)},
		protocol.FileInfo{Name: "e", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(7)},
	}
	setSequence(remoteSeq, remote1)
	setBlocksHash(remote1)
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

	check := func() {
		t.Helper()

		g := fileList(globalList(m))
		sort.Sort(g)

		if fmt.Sprint(g) != fmt.Sprint(expectedGlobal) {
			t.Errorf("Global incorrect;\n A: %v !=\n E: %v", g, expectedGlobal)
		}

		var globalFiles, globalDirectories, globalDeleted int
		var globalBytes int64
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
		gs := globalSize(m)
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
			t.Errorf("Have incorrect (local);\n A: %v !=\n E: %v", h, localTot)
		}

		var haveFiles, haveDirectories, haveDeleted int
		var haveBytes int64
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
		ls := localSize(m)
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
			t.Errorf("Have incorrect (remote);\n A: %v !=\n E: %v", h, remoteTot)
		}

		n := fileList(needList(m, protocol.LocalDeviceID))
		sort.Sort(n)

		if fmt.Sprint(n) != fmt.Sprint(expectedLocalNeed) {
			t.Errorf("Need incorrect (local);\n A: %v !=\n E: %v", n, expectedLocalNeed)
		}

		checkNeed(t, m, protocol.LocalDeviceID, expectedLocalNeed)

		n = fileList(needList(m, remoteDevice0))
		sort.Sort(n)

		if fmt.Sprint(n) != fmt.Sprint(expectedRemoteNeed) {
			t.Errorf("Need incorrect (remote);\n A: %v !=\n E: %v", n, expectedRemoteNeed)
		}

		checkNeed(t, m, remoteDevice0, expectedRemoteNeed)

		snap := m.Snapshot()
		defer snap.Release()
		f, ok := snap.Get(protocol.LocalDeviceID, "b")
		if !ok {
			t.Error("Unexpectedly not OK")
		}
		if fmt.Sprint(f) != fmt.Sprint(localTot[1]) {
			t.Errorf("Get incorrect;\n A: %v !=\n E: %v", f, localTot[1])
		}

		f, ok = snap.Get(remoteDevice0, "b")
		if !ok {
			t.Error("Unexpectedly not OK")
		}
		if fmt.Sprint(f) != fmt.Sprint(remote1[0]) {
			t.Errorf("Get incorrect (remote);\n A: %v !=\n E: %v", f, remote1[0])
		}

		f, ok = snap.GetGlobal("b")
		if !ok {
			t.Error("Unexpectedly not OK")
		}
		if fmt.Sprint(f) != fmt.Sprint(expectedGlobal[1]) {
			t.Errorf("GetGlobal incorrect;\n A: %v !=\n E: %v", f, remote1[0])
		}

		f, ok = snap.Get(protocol.LocalDeviceID, "zz")
		if ok {
			t.Error("Unexpectedly OK")
		}
		if f.Name != "" {
			t.Errorf("Get incorrect (local);\n A: %v !=\n E: %v", f, protocol.FileInfo{})
		}

		f, ok = snap.GetGlobal("zz")
		if ok {
			t.Error("Unexpectedly OK")
		}
		if f.Name != "" {
			t.Errorf("GetGlobal incorrect;\n A: %v !=\n E: %v", f, protocol.FileInfo{})
		}
	}

	check()

	snap := m.Snapshot()

	av := []protocol.DeviceID{protocol.LocalDeviceID, remoteDevice0}
	a := snap.Availability("a")
	if !(len(a) == 2 && (a[0] == av[0] && a[1] == av[1] || a[0] == av[1] && a[1] == av[0])) {
		t.Errorf("Availability incorrect;\n A: %v !=\n E: %v", a, av)
	}
	a = snap.Availability("b")
	if len(a) != 1 || a[0] != remoteDevice0 {
		t.Errorf("Availability incorrect;\n A: %v !=\n E: %v", a, remoteDevice0)
	}
	a = snap.Availability("d")
	if len(a) != 1 || a[0] != protocol.LocalDeviceID {
		t.Errorf("Availability incorrect;\n A: %v !=\n E: %v", a, protocol.LocalDeviceID)
	}

	snap.Release()

	// Now bring another remote into play

	secRemote := fileList{
		local1[0],  // a
		remote1[0], // b
		local1[3],  // d
		remote1[1], // e
		local1[4],  // z
	}
	secRemote[0].Version = secRemote[0].Version.Update(remoteDevice1.Short())
	secRemote[1].Version = secRemote[1].Version.Update(remoteDevice1.Short())
	secRemote[4].Version = secRemote[4].Version.Update(remoteDevice1.Short())
	secRemote[4].Deleted = false
	secRemote[4].Blocks = genBlocks(1)
	setSequence(0, secRemote)

	expectedGlobal = fileList{
		secRemote[0], // a
		secRemote[1], // b
		remote0[2],   // c
		localTot[3],  // d
		secRemote[3], // e
		secRemote[4], // z
	}

	expectedLocalNeed = fileList{
		secRemote[0], // a
		secRemote[1], // b
		remote0[2],   // c
		secRemote[3], // e
		secRemote[4], // z
	}

	expectedRemoteNeed = fileList{
		secRemote[0], // a
		secRemote[1], // b
		local0[3],    // d
		secRemote[4], // z
	}

	expectedSecRemoteNeed := fileList{
		remote0[2], // c
	}

	m.Update(remoteDevice1, secRemote)

	check()

	h := fileList(haveList(m, remoteDevice1))
	sort.Sort(h)

	if fmt.Sprint(h) != fmt.Sprint(secRemote) {
		t.Errorf("Have incorrect (secRemote);\n A: %v !=\n E: %v", h, secRemote)
	}

	n := fileList(needList(m, remoteDevice1))
	sort.Sort(n)

	if fmt.Sprint(n) != fmt.Sprint(expectedSecRemoteNeed) {
		t.Errorf("Need incorrect (secRemote);\n A: %v !=\n E: %v", n, expectedSecRemoteNeed)
	}

	checkNeed(t, m, remoteDevice1, expectedSecRemoteNeed)
}

func TestNeedWithInvalid(t *testing.T) {
	ldb := newLowlevelMemory(t)
	defer ldb.Close()

	s := newFileSet(t, "test", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

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

	checkNeed(t, s, protocol.LocalDeviceID, expectedNeed)
}

func TestUpdateToInvalid(t *testing.T) {
	ldb := newLowlevelMemory(t)
	defer ldb.Close()

	folder := "test"
	s := newFileSet(t, folder, fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)
	f := db.NewBlockFinder(ldb)

	localHave := fileList{
		protocol.FileInfo{Name: "a", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(1), Size: 1},
		protocol.FileInfo{Name: "b", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1001}}}, Blocks: genBlocks(2), Size: 1},
		protocol.FileInfo{Name: "c", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1002}}}, Blocks: genBlocks(5), LocalFlags: protocol.FlagLocalIgnored, Size: 1},
		protocol.FileInfo{Name: "d", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1003}}}, Blocks: genBlocks(7), Size: 1},
		protocol.FileInfo{Name: "e", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1003}}}, LocalFlags: protocol.FlagLocalIgnored, Size: 1},
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
		return file == localHave[4].Name
	}) {
		t.Errorf("First block of un-invalidated file is missing from blockmap")
	}
}

func TestInvalidAvailability(t *testing.T) {
	ldb := newLowlevelMemory(t)
	defer ldb.Close()

	s := newFileSet(t, "test", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

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

	snap := s.Snapshot()
	defer snap.Release()

	if av := snap.Availability("both"); len(av) != 2 {
		t.Error("Incorrect availability for 'both':", av)
	}

	if av := snap.Availability("r0only"); len(av) != 1 || av[0] != remoteDevice0 {
		t.Error("Incorrect availability for 'r0only':", av)
	}

	if av := snap.Availability("r1only"); len(av) != 1 || av[0] != remoteDevice1 {
		t.Error("Incorrect availability for 'r1only':", av)
	}

	if av := snap.Availability("none"); len(av) != 0 {
		t.Error("Incorrect availability for 'none':", av)
	}
}

func TestGlobalReset(t *testing.T) {
	ldb := newLowlevelMemory(t)
	defer ldb.Close()

	m := newFileSet(t, "test", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

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
	ldb := newLowlevelMemory(t)
	defer ldb.Close()

	m := newFileSet(t, "test", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

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

	checkNeed(t, m, protocol.LocalDeviceID, shouldNeed)
}

func TestSequence(t *testing.T) {
	ldb := newLowlevelMemory(t)
	defer ldb.Close()

	m := newFileSet(t, "test", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

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
	ldb := newLowlevelMemory(t)
	defer ldb.Close()

	s0 := newFileSet(t, "test0", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)
	local1 := []protocol.FileInfo{
		{Name: "a", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
		{Name: "b", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
		{Name: "c", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
	}
	replace(s0, protocol.LocalDeviceID, local1)

	s1 := newFileSet(t, "test1", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)
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
	ldb := newLowlevelMemory(t)
	defer ldb.Close()

	s := newFileSet(t, "test1", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

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
	checkNeed(t, s, protocol.LocalDeviceID, total)

	global := fileList(globalList(s))
	if fmt.Sprint(global) != fmt.Sprint(total) {
		t.Errorf("Global incorrect;\n A: %v !=\n E: %v", global, total)
	}
}

func TestLongPath(t *testing.T) {
	ldb := newLowlevelMemory(t)
	defer ldb.Close()

	s := newFileSet(t, "test", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

	var b bytes.Buffer
	for i := 0; i < 100; i++ {
		b.WriteString("012345678901234567890123456789012345678901234567890")
	}
	name := b.String() // 5000 characters

	local := []protocol.FileInfo{
		{Name: name, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
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

func BenchmarkUpdateOneFile(b *testing.B) {
	local0 := fileList{
		protocol.FileInfo{Name: "a", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(1)},
		protocol.FileInfo{Name: "b", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(2)},
		protocol.FileInfo{Name: "c", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(3)},
		protocol.FileInfo{Name: "d", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(4)},
		// A longer name is more realistic and causes more allocations
		protocol.FileInfo{Name: "zajksdhaskjdh/askjdhaskjdashkajshd/kasjdhaskjdhaskdjhaskdjash/dkjashdaksjdhaskdjahskdjh", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(8)},
	}

	be, err := backend.Open("testdata/benchmarkupdate.db", backend.TuningAuto)
	if err != nil {
		b.Fatal(err)
	}
	ldb := newLowlevel(b, be)
	defer func() {
		ldb.Close()
		os.RemoveAll("testdata/benchmarkupdate.db")
	}()

	m := newFileSet(b, "test", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)
	replace(m, protocol.LocalDeviceID, local0)
	l := local0[4:5]

	for i := 0; i < b.N; i++ {
		l[0].Version = l[0].Version.Update(myID)
		m.Update(protocol.LocalDeviceID, local0)
	}

	b.ReportAllocs()
}

func TestIndexID(t *testing.T) {
	ldb := newLowlevelMemory(t)
	defer ldb.Close()

	s := newFileSet(t, "test", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

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
	ldb := newLowlevelMemory(t)

	m := newFileSet(t, "test", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

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
	ldb := newLowlevelMemory(t)
	defer ldb.Close()

	s := newFileSet(t, "test", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

	localHave := fileList{
		protocol.FileInfo{Name: "a", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
		protocol.FileInfo{Name: "b", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, LocalFlags: protocol.FlagLocalIgnored},
	}

	s.Update(protocol.LocalDeviceID, localHave)

	if c := localSize(s); c.Files != 1 {
		t.Errorf("Expected 1 local file, got %v", c.Files)
	}
	if c := globalSize(s); c.Files != 1 {
		t.Errorf("Expected 1 global file, got %v", c.Files)
	}

	localHave[1].LocalFlags = 0
	s.Update(protocol.LocalDeviceID, localHave)

	if c := localSize(s); c.Files != 2 {
		t.Errorf("Expected 2 local files, got %v", c.Files)
	}
	if c := globalSize(s); c.Files != 2 {
		t.Errorf("Expected 2 global files, got %v", c.Files)
	}

	localHave[0].LocalFlags = protocol.FlagLocalIgnored
	localHave[1].LocalFlags = protocol.FlagLocalIgnored
	s.Update(protocol.LocalDeviceID, localHave)

	if c := localSize(s); c.Files != 0 {
		t.Errorf("Expected 0 local files, got %v", c.Files)
	}
	if c := globalSize(s); c.Files != 0 {
		t.Errorf("Expected 0 global files, got %v", c.Files)
	}
}

func TestWithHaveSequence(t *testing.T) {
	ldb := newLowlevelMemory(t)
	defer ldb.Close()

	folder := "test"
	s := newFileSet(t, folder, fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

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
	snap := s.Snapshot()
	defer snap.Release()
	snap.WithHaveSequence(int64(i), func(fi protocol.FileIntf) bool {
		if f := fi.(protocol.FileInfo); !f.IsEquivalent(localHave[i-1], 0) {
			t.Fatalf("Got %v\nExpected %v", f, localHave[i-1])
		}
		i++
		return true
	})
}

func TestStressWithHaveSequence(t *testing.T) {
	// This races two loops against each other: one that contiously does
	// updates, and one that continuously does sequence walks. The test fails
	// if the sequence walker sees a discontinuity.

	if testing.Short() {
		t.Skip("Takes a long time")
	}

	ldb := newLowlevelMemory(t)
	defer ldb.Close()

	folder := "test"
	s := newFileSet(t, folder, fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

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

	var prevSeq int64
loop:
	for {
		select {
		case <-done:
			break loop
		default:
		}
		snap := s.Snapshot()
		snap.WithHaveSequence(prevSeq+1, func(fi protocol.FileIntf) bool {
			if fi.SequenceNo() < prevSeq+1 {
				t.Fatal("Skipped ", prevSeq+1, fi.SequenceNo())
			}
			prevSeq = fi.SequenceNo()
			return true
		})
		snap.Release()
	}
}

func TestIssue4925(t *testing.T) {
	ldb := newLowlevelMemory(t)
	defer ldb.Close()

	folder := "test"
	s := newFileSet(t, folder, fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

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
	ldb := newLowlevelMemory(t)
	defer ldb.Close()

	folder := "test"
	file := "foo"
	s := newFileSet(t, folder, fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

	localHave := fileList{{Name: file, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1}}}, Blocks: genBlocks(1), ModifiedS: 10, Size: 1}}
	remote0Have := fileList{{Name: file, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1}, {ID: remoteDevice0.Short(), Value: 1}}}, Blocks: genBlocks(2), ModifiedS: 0, Size: 2}}

	s.Update(protocol.LocalDeviceID, localHave)
	s.Update(remoteDevice0, remote0Have)

	if need := needList(s, protocol.LocalDeviceID); len(need) != 1 {
		t.Error("Expected 1 local need, got", need)
	} else if !need[0].IsEquivalent(remote0Have[0], 0) {
		t.Errorf("Local need incorrect;\n A: %v !=\n E: %v", need[0], remote0Have[0])
	}
	checkNeed(t, s, protocol.LocalDeviceID, remote0Have[:1])

	if need := needList(s, remoteDevice0); len(need) != 0 {
		t.Error("Expected no need for remote 0, got", need)
	}
	checkNeed(t, s, remoteDevice0, nil)

	ls := localSize(s)
	if haveBytes := localHave[0].Size; ls.Bytes != haveBytes {
		t.Errorf("Incorrect LocalSize bytes; %d != %d", ls.Bytes, haveBytes)
	}

	gs := globalSize(s)
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
	} else if !need[0].IsEquivalent(localHave[0], 0) {
		t.Errorf("Need for remote 0 incorrect;\n A: %v !=\n E: %v", need[0], localHave[0])
	}
	checkNeed(t, s, remoteDevice0, localHave[:1])

	if need := needList(s, protocol.LocalDeviceID); len(need) != 0 {
		t.Error("Expected no local need, got", need)
	}
	checkNeed(t, s, protocol.LocalDeviceID, nil)

	ls = localSize(s)
	if haveBytes := localHave[0].Size; ls.Bytes != haveBytes {
		t.Errorf("Incorrect LocalSize bytes; %d != %d", ls.Bytes, haveBytes)
	}

	gs = globalSize(s)
	if globalBytes := localHave[0].Size; gs.Bytes != globalBytes {
		t.Errorf("Incorrect GlobalSize bytes; %d != %d", gs.Bytes, globalBytes)
	}
}

// TestIssue5007 checks, that updating the local device with an invalid file
// info with the newest version does indeed remove that file from the list of
// needed files.
// https://github.com/syncthing/syncthing/issues/5007
func TestIssue5007(t *testing.T) {
	ldb := newLowlevelMemory(t)
	defer ldb.Close()

	folder := "test"
	file := "foo"
	s := newFileSet(t, folder, fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

	fs := fileList{{Name: file, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1}}}}}

	s.Update(remoteDevice0, fs)

	if need := needList(s, protocol.LocalDeviceID); len(need) != 1 {
		t.Fatal("Expected 1 local need, got", need)
	} else if !need[0].IsEquivalent(fs[0], 0) {
		t.Fatalf("Local need incorrect;\n A: %v !=\n E: %v", need[0], fs[0])
	}
	checkNeed(t, s, protocol.LocalDeviceID, fs[:1])

	fs[0].LocalFlags = protocol.FlagLocalIgnored
	s.Update(protocol.LocalDeviceID, fs)

	if need := needList(s, protocol.LocalDeviceID); len(need) != 0 {
		t.Fatal("Expected no local need, got", need)
	}
	checkNeed(t, s, protocol.LocalDeviceID, nil)
}

// TestNeedDeleted checks that a file that doesn't exist locally isn't needed
// when the global file is deleted.
func TestNeedDeleted(t *testing.T) {
	ldb := newLowlevelMemory(t)
	defer ldb.Close()

	folder := "test"
	file := "foo"
	s := newFileSet(t, folder, fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

	fs := fileList{{Name: file, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1}}}, Deleted: true}}

	s.Update(remoteDevice0, fs)

	if need := needList(s, protocol.LocalDeviceID); len(need) != 0 {
		t.Fatal("Expected no local need, got", need)
	}
	checkNeed(t, s, protocol.LocalDeviceID, nil)

	fs[0].Deleted = false
	fs[0].Version = fs[0].Version.Update(remoteDevice0.Short())
	s.Update(remoteDevice0, fs)

	if need := needList(s, protocol.LocalDeviceID); len(need) != 1 {
		t.Fatal("Expected 1 local need, got", need)
	} else if !need[0].IsEquivalent(fs[0], 0) {
		t.Fatalf("Local need incorrect;\n A: %v !=\n E: %v", need[0], fs[0])
	}
	checkNeed(t, s, protocol.LocalDeviceID, fs[:1])

	fs[0].Deleted = true
	fs[0].Version = fs[0].Version.Update(remoteDevice0.Short())
	s.Update(remoteDevice0, fs)

	if need := needList(s, protocol.LocalDeviceID); len(need) != 0 {
		t.Fatal("Expected no local need, got", need)
	}
	checkNeed(t, s, protocol.LocalDeviceID, nil)
}

func TestReceiveOnlyAccounting(t *testing.T) {
	ldb := newLowlevelMemory(t)
	defer ldb.Close()

	folder := "test"
	s := newFileSet(t, folder, fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

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

	if n := localSize(s).Files; n != 3 {
		t.Fatal("expected 3 local files initially, not", n)
	}
	if n := localSize(s).Bytes; n != 30 {
		t.Fatal("expected 30 local bytes initially, not", n)
	}
	if n := globalSize(s).Files; n != 3 {
		t.Fatal("expected 3 global files initially, not", n)
	}
	if n := globalSize(s).Bytes; n != 30 {
		t.Fatal("expected 30 global bytes initially, not", n)
	}
	if n := receiveOnlyChangedSize(s).Files; n != 0 {
		t.Fatal("expected 0 receive only changed files initially, not", n)
	}
	if n := receiveOnlyChangedSize(s).Bytes; n != 0 {
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

	if n := localSize(s).Files; n != 3 {
		t.Fatal("expected 3 local files after local change, not", n)
	}
	if n := localSize(s).Bytes; n != 120 {
		t.Fatal("expected 120 local bytes after local change, not", n)
	}
	if n := globalSize(s).Files; n != 3 {
		t.Fatal("expected 3 global files after local change, not", n)
	}
	if n := globalSize(s).Bytes; n != 30 {
		t.Fatal("expected 30 global files after local change, not", n)
	}
	if n := receiveOnlyChangedSize(s).Files; n != 1 {
		t.Fatal("expected 1 receive only changed file after local change, not", n)
	}
	if n := receiveOnlyChangedSize(s).Bytes; n != 100 {
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

	if n := localSize(s).Files; n != 3 {
		t.Fatal("expected 3 local files after revert, not", n)
	}
	if n := localSize(s).Bytes; n != 30 {
		t.Fatal("expected 30 local bytes after revert, not", n)
	}
	if n := globalSize(s).Files; n != 3 {
		t.Fatal("expected 3 global files after revert, not", n)
	}
	if n := globalSize(s).Bytes; n != 30 {
		t.Fatal("expected 30 global bytes after revert, not", n)
	}
	if n := receiveOnlyChangedSize(s).Files; n != 0 {
		t.Fatal("expected 0 receive only changed files after revert, not", n)
	}
	if n := receiveOnlyChangedSize(s).Bytes; n != 0 {
		t.Fatal("expected 0 receive only changed bytes after revert, not", n)
	}
}

func TestNeedAfterUnignore(t *testing.T) {
	ldb := newLowlevelMemory(t)
	defer ldb.Close()

	folder := "test"
	file := "foo"
	s := newFileSet(t, folder, fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

	remID := remoteDevice0.Short()

	// Initial state: Devices in sync, locally ignored
	local := protocol.FileInfo{Name: file, Version: protocol.Vector{Counters: []protocol.Counter{{ID: remID, Value: 1}, {ID: myID, Value: 1}}}, ModifiedS: 10}
	local.SetIgnored()
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
	} else if !need[0].IsEquivalent(remote, 0) {
		t.Fatalf("Got %v, expected %v", need[0], remote)
	}
	checkNeed(t, s, protocol.LocalDeviceID, []protocol.FileInfo{remote})
}

func TestRemoteInvalidNotAccounted(t *testing.T) {
	// Remote files with the invalid bit should not count.

	ldb := newLowlevelMemory(t)
	defer ldb.Close()
	s := newFileSet(t, "test", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

	files := []protocol.FileInfo{
		{Name: "a", Size: 1234, Sequence: 42, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1003}}}},                   // valid, should count
		{Name: "b", Size: 1234, Sequence: 43, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1003}}}, RawInvalid: true}, // invalid, doesn't count
	}
	s.Update(remoteDevice0, files)

	global := globalSize(s)
	if global.Files != 1 {
		t.Error("Expected one file in global size, not", global.Files)
	}
	if global.Bytes != 1234 {
		t.Error("Expected 1234 bytes in global size, not", global.Bytes)
	}
}

func TestNeedWithNewerInvalid(t *testing.T) {
	ldb := newLowlevelMemory(t)
	defer ldb.Close()

	s := newFileSet(t, "default", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

	rem0ID := remoteDevice0.Short()
	rem1ID := remoteDevice1.Short()

	// Initial state: file present on rem0 and rem1, but not locally.
	file := protocol.FileInfo{Name: "foo"}
	file.Version = file.Version.Update(rem0ID)
	s.Update(remoteDevice0, fileList{file})
	s.Update(remoteDevice1, fileList{file})

	need := needList(s, protocol.LocalDeviceID)
	if len(need) != 1 {
		t.Fatal("Locally missing file should be needed")
	}
	if !need[0].IsEquivalent(file, 0) {
		t.Fatalf("Got needed file %v, expected %v", need[0], file)
	}
	checkNeed(t, s, protocol.LocalDeviceID, []protocol.FileInfo{file})

	// rem1 sends an invalid file with increased version
	inv := file
	inv.Version = inv.Version.Update(rem1ID)
	inv.RawInvalid = true
	s.Update(remoteDevice1, fileList{inv})

	// We still have an old file, we need the newest valid file
	need = needList(s, protocol.LocalDeviceID)
	if len(need) != 1 {
		t.Fatal("Locally missing file should be needed regardless of invalid files")
	}
	if !need[0].IsEquivalent(file, 0) {
		t.Fatalf("Got needed file %v, expected %v", need[0], file)
	}
	checkNeed(t, s, protocol.LocalDeviceID, []protocol.FileInfo{file})
}

func TestNeedAfterDeviceRemove(t *testing.T) {
	ldb := newLowlevelMemory(t)
	defer ldb.Close()

	file := "foo"
	s := newFileSet(t, "test", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

	fs := fileList{{Name: file, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1}}}}}

	s.Update(protocol.LocalDeviceID, fs)

	fs[0].Version = fs[0].Version.Update(myID)

	s.Update(remoteDevice0, fs)

	if need := needList(s, protocol.LocalDeviceID); len(need) != 1 {
		t.Fatal("Expected one local need, got", need)
	}

	s.Drop(remoteDevice0)

	if need := needList(s, protocol.LocalDeviceID); len(need) != 0 {
		t.Fatal("Expected no local need, got", need)
	}
	checkNeed(t, s, protocol.LocalDeviceID, nil)
}

func TestCaseSensitive(t *testing.T) {
	// Normal case sensitive lookup should work

	ldb := newLowlevelMemory(t)
	defer ldb.Close()
	s := newFileSet(t, "test", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

	local := []protocol.FileInfo{
		{Name: filepath.FromSlash("D1/f1"), Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
		{Name: filepath.FromSlash("F1"), Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
		{Name: filepath.FromSlash("d1/F1"), Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
		{Name: filepath.FromSlash("d1/f1"), Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
		{Name: filepath.FromSlash("f1"), Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
	}

	replace(s, protocol.LocalDeviceID, local)

	gf := globalList(s)
	if l := len(gf); l != len(local) {
		t.Fatalf("Incorrect len %d != %d for global list", l, len(local))
	}
	for i := range local {
		if gf[i].Name != local[i].Name {
			t.Errorf("Incorrect  filename;\n%q !=\n%q",
				gf[i].Name, local[i].Name)
		}
	}
}

func TestSequenceIndex(t *testing.T) {
	// This test attempts to verify correct operation of the sequence index.

	// It's a stress test and needs to run for a long time, but we don't
	// really have time for that in normal builds.
	runtime := time.Minute
	if testing.Short() {
		runtime = time.Second
	}

	// Set up a db and a few files that we will manipulate.

	ldb := newLowlevelMemory(t)
	defer ldb.Close()
	s := newFileSet(t, "test", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

	local := []protocol.FileInfo{
		{Name: filepath.FromSlash("banana"), Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
		{Name: filepath.FromSlash("pineapple"), Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
		{Name: filepath.FromSlash("orange"), Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
		{Name: filepath.FromSlash("apple"), Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
		{Name: filepath.FromSlash("jackfruit"), Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}},
	}

	// Start a  background routine that makes updates to these files as fast
	// as it can. We always update the same files in the same order.

	done := make(chan struct{})
	defer close(done)

	go func() {
		for {
			select {
			case <-done:
				return
			default:
			}

			for i := range local {
				local[i].Version = local[i].Version.Update(42)
			}
			s.Update(protocol.LocalDeviceID, local)
		}
	}()

	// Start a routine to walk the sequence index and inspect the result.

	seen := make(map[string]protocol.FileIntf)
	latest := make([]protocol.FileIntf, 0, len(local))
	var seq int64
	t0 := time.Now()

	for time.Since(t0) < runtime {
		// Walk the changes since our last iteration. This should give is
		// one instance each of the files that are changed all the time, or
		// a subset of those files if we manage to run before a complete
		// update has happened since our last iteration.
		latest = latest[:0]
		snap := s.Snapshot()
		snap.WithHaveSequence(seq+1, func(f protocol.FileIntf) bool {
			seen[f.FileName()] = f
			latest = append(latest, f)
			seq = f.SequenceNo()
			return true
		})
		snap.Release()

		// Calculate the spread in sequence number.
		var max, min int64
		for _, v := range seen {
			s := v.SequenceNo()
			if max == 0 || max < s {
				max = s
			}
			if min == 0 || min > s {
				min = s
			}
		}

		// We shouldn't see a spread larger than the number of files, as
		// that would mean we have missed updates. For example, if we were
		// to see the following:
		//
		// banana    N
		// pineapple N+1
		// orange    N+2
		// apple     N+10
		// jackfruit N+11
		//
		// that would mean that there have been updates to banana, pineapple
		// and orange that we didn't see in this pass. If those files aren't
		// updated again, those updates are permanently lost.
		if max-min > int64(len(local)) {
			for _, v := range seen {
				t.Log("seen", v.FileName(), v.SequenceNo())
			}
			for _, v := range latest {
				t.Log("latest", v.FileName(), v.SequenceNo())
			}
			t.Fatal("large spread")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestIgnoreAfterReceiveOnly(t *testing.T) {
	ldb := newLowlevelMemory(t)
	defer ldb.Close()

	file := "foo"
	s := newFileSet(t, "test", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)

	fs := fileList{{
		Name:       file,
		Version:    protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1}}},
		LocalFlags: protocol.FlagLocalReceiveOnly,
	}}

	s.Update(protocol.LocalDeviceID, fs)

	fs[0].LocalFlags = protocol.FlagLocalIgnored

	s.Update(protocol.LocalDeviceID, fs)

	snap := s.Snapshot()
	defer snap.Release()
	if f, ok := snap.Get(protocol.LocalDeviceID, file); !ok {
		t.Error("File missing in db")
	} else if f.IsReceiveOnlyChanged() {
		t.Error("File is still receive-only changed")
	} else if !f.IsIgnored() {
		t.Error("File is not ignored")
	}
}

// https://github.com/syncthing/syncthing/issues/6650
func TestUpdateWithOneFileTwice(t *testing.T) {
	ldb := newLowlevelMemory(t)
	defer ldb.Close()

	file := "foo"
	s := newFileSet(t, "test", fs.NewFilesystem(fs.FilesystemTypeFake, ""), ldb)

	fs := fileList{{
		Name:     file,
		Version:  protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1}}},
		Sequence: 1,
	}}

	s.Update(protocol.LocalDeviceID, fs)

	fs = append(fs, fs[0])
	for i := range fs {
		fs[i].Sequence++
		fs[i].Version = fs[i].Version.Update(myID)
	}
	fs[1].Sequence++
	fs[1].Version = fs[1].Version.Update(myID)

	s.Update(protocol.LocalDeviceID, fs)

	snap := s.Snapshot()
	defer snap.Release()
	count := 0
	snap.WithHaveSequence(0, func(f protocol.FileIntf) bool {
		count++
		return true
	})
	if count != 1 {
		t.Error("Expected to have one file, got", count)
	}
}

// https://github.com/syncthing/syncthing/issues/6668
func TestNeedRemoteOnly(t *testing.T) {
	ldb := newLowlevelMemory(t)
	defer ldb.Close()

	s := newFileSet(t, "test", fs.NewFilesystem(fs.FilesystemTypeFake, ""), ldb)

	remote0Have := fileList{
		protocol.FileInfo{Name: "b", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1001}}}, Blocks: genBlocks(2)},
	}
	s.Update(remoteDevice0, remote0Have)

	need := needSize(s, remoteDevice0)
	if !need.Equal(db.Counts{}) {
		t.Error("Expected nothing needed, got", need)
	}
}

// https://github.com/syncthing/syncthing/issues/6784
func TestNeedRemoteAfterReset(t *testing.T) {
	ldb := newLowlevelMemory(t)
	defer ldb.Close()

	s := newFileSet(t, "test", fs.NewFilesystem(fs.FilesystemTypeFake, ""), ldb)

	files := fileList{
		protocol.FileInfo{Name: "b", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1001}}}, Blocks: genBlocks(2)},
	}
	s.Update(protocol.LocalDeviceID, files)
	s.Update(remoteDevice0, files)

	need := needSize(s, remoteDevice0)
	if !need.Equal(db.Counts{}) {
		t.Error("Expected nothing needed, got", need)
	}

	s.Drop(remoteDevice0)

	need = needSize(s, remoteDevice0)
	if exp := (db.Counts{Files: 1}); !need.Equal(exp) {
		t.Errorf("Expected %v, got %v", exp, need)
	}
}

// https://github.com/syncthing/syncthing/issues/6850
func TestIgnoreLocalChanged(t *testing.T) {
	ldb := newLowlevelMemory(t)
	defer ldb.Close()

	s := newFileSet(t, "test", fs.NewFilesystem(fs.FilesystemTypeFake, ""), ldb)

	// Add locally changed file
	files := fileList{
		protocol.FileInfo{Name: "b", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1001}}}, Blocks: genBlocks(2), LocalFlags: protocol.FlagLocalReceiveOnly},
	}
	s.Update(protocol.LocalDeviceID, files)

	if c := globalSize(s).Files; c != 0 {
		t.Error("Expected no global file, got", c)
	}
	if c := localSize(s).Files; c != 1 {
		t.Error("Expected one local file, got", c)
	}

	// Change file to ignored
	files[0].LocalFlags = protocol.FlagLocalIgnored
	s.Update(protocol.LocalDeviceID, files)

	if c := globalSize(s).Files; c != 0 {
		t.Error("Expected no global file, got", c)
	}
	if c := localSize(s).Files; c != 0 {
		t.Error("Expected no local file, got", c)
	}
}

// Dropping the index ID on Drop is bad, because Drop gets called when receiving
// an Index (as opposed to an IndexUpdate), and we don't want to loose the index
// ID when that happens.
func TestNoIndexIDResetOnDrop(t *testing.T) {
	ldb := newLowlevelMemory(t)
	defer ldb.Close()

	s := newFileSet(t, "test", fs.NewFilesystem(fs.FilesystemTypeFake, ""), ldb)

	s.SetIndexID(remoteDevice0, 1)
	s.Drop(remoteDevice0)
	if got := s.IndexID(remoteDevice0); got != 1 {
		t.Errorf("Expected unchanged (%v), got %v", 1, got)
	}
}

func TestConcurrentIndexID(t *testing.T) {
	done := make(chan struct{})
	var ids [2]protocol.IndexID
	setID := func(s *db.FileSet, i int) {
		ids[i] = s.IndexID(protocol.LocalDeviceID)
		done <- struct{}{}
	}

	max := 100
	if testing.Short() {
		max = 10
	}
	for i := 0; i < max; i++ {
		ldb := newLowlevelMemory(t)
		s := newFileSet(t, "test", fs.NewFilesystem(fs.FilesystemTypeFake, ""), ldb)
		go setID(s, 0)
		go setID(s, 1)
		<-done
		<-done
		ldb.Close()
		if ids[0] != ids[1] {
			t.Fatalf("IDs differ after %v rounds", i)
		}
	}
}

func replace(fs *db.FileSet, device protocol.DeviceID, files []protocol.FileInfo) {
	fs.Drop(device)
	fs.Update(device, files)
}

func localSize(fs *db.FileSet) db.Counts {
	snap := fs.Snapshot()
	defer snap.Release()
	return snap.LocalSize()
}

func globalSize(fs *db.FileSet) db.Counts {
	snap := fs.Snapshot()
	defer snap.Release()
	return snap.GlobalSize()
}

func needSize(fs *db.FileSet, id protocol.DeviceID) db.Counts {
	snap := fs.Snapshot()
	defer snap.Release()
	return snap.NeedSize(id)
}

func receiveOnlyChangedSize(fs *db.FileSet) db.Counts {
	snap := fs.Snapshot()
	defer snap.Release()
	return snap.ReceiveOnlyChangedSize()
}

func filesToCounts(files []protocol.FileInfo) db.Counts {
	cp := db.Counts{}
	for _, f := range files {
		switch {
		case f.IsDeleted():
			cp.Deleted++
		case f.IsDirectory() && !f.IsSymlink():
			cp.Directories++
		case f.IsSymlink():
			cp.Symlinks++
		default:
			cp.Files++
		}
		cp.Bytes += f.FileSize()
	}
	return cp
}

func checkNeed(t testing.TB, s *db.FileSet, dev protocol.DeviceID, expected []protocol.FileInfo) {
	t.Helper()
	counts := needSize(s, dev)
	if exp := filesToCounts(expected); !exp.Equal(counts) {
		t.Errorf("Count incorrect (%v): expected %v, got %v", dev, exp, counts)
	}
}

func newLowlevel(t testing.TB, backend backend.Backend) *db.Lowlevel {
	t.Helper()
	ll, err := db.NewLowlevel(backend, events.NoopLogger)
	if err != nil {
		t.Fatal(err)
	}
	return ll
}

func newLowlevelMemory(t testing.TB) *db.Lowlevel {
	return newLowlevel(t, backend.OpenMemory())
}

func newFileSet(t testing.TB, folder string, fs fs.Filesystem, ll *db.Lowlevel) *db.FileSet {
	t.Helper()
	fset, err := db.NewFileSet(folder, fs, ll)
	if err != nil {
		t.Fatal(err)
	}
	return fset
}
