// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package db_test

import (
	"bytes"
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"
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

func haveList(s *db.FileSet, n protocol.DeviceID) []protocol.FileInfo {
	var fs []protocol.FileInfo
	s.WithHave(n, func(fi db.FileIntf) bool {
		f := fi.(protocol.FileInfo)
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
		fmt.Fprintf(&b, "  %q: #%d, %d bytes, %d blocks, flags=%o\n", f.Name, f.Version, f.Size(), len(f.Blocks), f.Flags)
	}
	b.WriteString("}")
	return b.String()
}

func TestGlobalSet(t *testing.T) {

	ldb, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		t.Fatal(err)
	}

	m := db.NewFileSet("test", ldb)

	local0 := fileList{
		protocol.FileInfo{Name: "a", Version: protocol.Vector{{ID: myID, Value: 1000}}, Blocks: genBlocks(1)},
		protocol.FileInfo{Name: "b", Version: protocol.Vector{{ID: myID, Value: 1000}}, Blocks: genBlocks(2)},
		protocol.FileInfo{Name: "c", Version: protocol.Vector{{ID: myID, Value: 1000}}, Blocks: genBlocks(3)},
		protocol.FileInfo{Name: "d", Version: protocol.Vector{{ID: myID, Value: 1000}}, Blocks: genBlocks(4)},
		protocol.FileInfo{Name: "z", Version: protocol.Vector{{ID: myID, Value: 1000}}, Blocks: genBlocks(8)},
	}
	local1 := fileList{
		protocol.FileInfo{Name: "a", Version: protocol.Vector{{ID: myID, Value: 1000}}, Blocks: genBlocks(1)},
		protocol.FileInfo{Name: "b", Version: protocol.Vector{{ID: myID, Value: 1000}}, Blocks: genBlocks(2)},
		protocol.FileInfo{Name: "c", Version: protocol.Vector{{ID: myID, Value: 1000}}, Blocks: genBlocks(3)},
		protocol.FileInfo{Name: "d", Version: protocol.Vector{{ID: myID, Value: 1000}}, Blocks: genBlocks(4)},
		protocol.FileInfo{Name: "z", Version: protocol.Vector{{ID: myID, Value: 1001}}, Flags: protocol.FlagDeleted},
	}
	localTot := fileList{
		local0[0],
		local0[1],
		local0[2],
		local0[3],
		protocol.FileInfo{Name: "z", Version: protocol.Vector{{ID: myID, Value: 1001}}, Flags: protocol.FlagDeleted},
	}

	remote0 := fileList{
		protocol.FileInfo{Name: "a", Version: protocol.Vector{{ID: myID, Value: 1000}}, Blocks: genBlocks(1)},
		protocol.FileInfo{Name: "b", Version: protocol.Vector{{ID: myID, Value: 1000}}, Blocks: genBlocks(2)},
		protocol.FileInfo{Name: "c", Version: protocol.Vector{{ID: myID, Value: 1001}}, Blocks: genBlocks(5)},
	}
	remote1 := fileList{
		protocol.FileInfo{Name: "b", Version: protocol.Vector{{ID: myID, Value: 1001}}, Blocks: genBlocks(6)},
		protocol.FileInfo{Name: "e", Version: protocol.Vector{{ID: myID, Value: 1000}}, Blocks: genBlocks(7)},
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

	m.Replace(protocol.LocalDeviceID, local0)
	m.Replace(protocol.LocalDeviceID, local1)
	m.Replace(remoteDevice0, remote0)
	m.Update(remoteDevice0, remote1)

	g := fileList(globalList(m))
	sort.Sort(g)

	if fmt.Sprint(g) != fmt.Sprint(expectedGlobal) {
		t.Errorf("Global incorrect;\n A: %v !=\n E: %v", g, expectedGlobal)
	}

	h := fileList(haveList(m, protocol.LocalDeviceID))
	sort.Sort(h)

	if fmt.Sprint(h) != fmt.Sprint(localTot) {
		t.Errorf("Have incorrect;\n A: %v !=\n E: %v", h, localTot)
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
	ldb, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		t.Fatal(err)
	}

	s := db.NewFileSet("test", ldb)

	localHave := fileList{
		protocol.FileInfo{Name: "a", Version: protocol.Vector{{ID: myID, Value: 1000}}, Blocks: genBlocks(1)},
	}
	remote0Have := fileList{
		protocol.FileInfo{Name: "b", Version: protocol.Vector{{ID: myID, Value: 1001}}, Blocks: genBlocks(2)},
		protocol.FileInfo{Name: "c", Version: protocol.Vector{{ID: myID, Value: 1002}}, Blocks: genBlocks(5), Flags: protocol.FlagInvalid},
		protocol.FileInfo{Name: "d", Version: protocol.Vector{{ID: myID, Value: 1003}}, Blocks: genBlocks(7)},
	}
	remote1Have := fileList{
		protocol.FileInfo{Name: "c", Version: protocol.Vector{{ID: myID, Value: 1002}}, Blocks: genBlocks(7)},
		protocol.FileInfo{Name: "d", Version: protocol.Vector{{ID: myID, Value: 1003}}, Blocks: genBlocks(5), Flags: protocol.FlagInvalid},
		protocol.FileInfo{Name: "e", Version: protocol.Vector{{ID: myID, Value: 1004}}, Blocks: genBlocks(5), Flags: protocol.FlagInvalid},
	}

	expectedNeed := fileList{
		protocol.FileInfo{Name: "b", Version: protocol.Vector{{ID: myID, Value: 1001}}, Blocks: genBlocks(2)},
		protocol.FileInfo{Name: "c", Version: protocol.Vector{{ID: myID, Value: 1002}}, Blocks: genBlocks(7)},
		protocol.FileInfo{Name: "d", Version: protocol.Vector{{ID: myID, Value: 1003}}, Blocks: genBlocks(7)},
	}

	s.Replace(protocol.LocalDeviceID, localHave)
	s.Replace(remoteDevice0, remote0Have)
	s.Replace(remoteDevice1, remote1Have)

	need := fileList(needList(s, protocol.LocalDeviceID))
	sort.Sort(need)

	if fmt.Sprint(need) != fmt.Sprint(expectedNeed) {
		t.Errorf("Need incorrect;\n A: %v !=\n E: %v", need, expectedNeed)
	}
}

func TestUpdateToInvalid(t *testing.T) {
	ldb, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		t.Fatal(err)
	}

	s := db.NewFileSet("test", ldb)

	localHave := fileList{
		protocol.FileInfo{Name: "a", Version: protocol.Vector{{ID: myID, Value: 1000}}, Blocks: genBlocks(1)},
		protocol.FileInfo{Name: "b", Version: protocol.Vector{{ID: myID, Value: 1001}}, Blocks: genBlocks(2)},
		protocol.FileInfo{Name: "c", Version: protocol.Vector{{ID: myID, Value: 1002}}, Blocks: genBlocks(5), Flags: protocol.FlagInvalid},
		protocol.FileInfo{Name: "d", Version: protocol.Vector{{ID: myID, Value: 1003}}, Blocks: genBlocks(7)},
	}

	s.Replace(protocol.LocalDeviceID, localHave)

	have := fileList(haveList(s, protocol.LocalDeviceID))
	sort.Sort(have)

	if fmt.Sprint(have) != fmt.Sprint(localHave) {
		t.Errorf("Have incorrect before invalidation;\n A: %v !=\n E: %v", have, localHave)
	}

	localHave[1] = protocol.FileInfo{Name: "b", Version: protocol.Vector{{ID: myID, Value: 1001}}, Flags: protocol.FlagInvalid}
	s.Update(protocol.LocalDeviceID, localHave[1:2])

	have = fileList(haveList(s, protocol.LocalDeviceID))
	sort.Sort(have)

	if fmt.Sprint(have) != fmt.Sprint(localHave) {
		t.Errorf("Have incorrect after invalidation;\n A: %v !=\n E: %v", have, localHave)
	}
}

func TestInvalidAvailability(t *testing.T) {
	ldb, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		t.Fatal(err)
	}

	s := db.NewFileSet("test", ldb)

	remote0Have := fileList{
		protocol.FileInfo{Name: "both", Version: protocol.Vector{{ID: myID, Value: 1001}}, Blocks: genBlocks(2)},
		protocol.FileInfo{Name: "r1only", Version: protocol.Vector{{ID: myID, Value: 1002}}, Blocks: genBlocks(5), Flags: protocol.FlagInvalid},
		protocol.FileInfo{Name: "r0only", Version: protocol.Vector{{ID: myID, Value: 1003}}, Blocks: genBlocks(7)},
		protocol.FileInfo{Name: "none", Version: protocol.Vector{{ID: myID, Value: 1004}}, Blocks: genBlocks(5), Flags: protocol.FlagInvalid},
	}
	remote1Have := fileList{
		protocol.FileInfo{Name: "both", Version: protocol.Vector{{ID: myID, Value: 1001}}, Blocks: genBlocks(2)},
		protocol.FileInfo{Name: "r1only", Version: protocol.Vector{{ID: myID, Value: 1002}}, Blocks: genBlocks(7)},
		protocol.FileInfo{Name: "r0only", Version: protocol.Vector{{ID: myID, Value: 1003}}, Blocks: genBlocks(5), Flags: protocol.FlagInvalid},
		protocol.FileInfo{Name: "none", Version: protocol.Vector{{ID: myID, Value: 1004}}, Blocks: genBlocks(5), Flags: protocol.FlagInvalid},
	}

	s.Replace(remoteDevice0, remote0Have)
	s.Replace(remoteDevice1, remote1Have)

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
func Benchmark10kReplace(b *testing.B) {
	ldb, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		b.Fatal(err)
	}

	var local []protocol.FileInfo
	for i := 0; i < 10000; i++ {
		local = append(local, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: protocol.Vector{{ID: myID, Value: 1000}}})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := db.NewFileSet("test", ldb)
		m.Replace(protocol.LocalDeviceID, local)
	}
}

func Benchmark10kUpdateChg(b *testing.B) {
	var remote []protocol.FileInfo
	for i := 0; i < 10000; i++ {
		remote = append(remote, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: protocol.Vector{{ID: myID, Value: 1000}}})
	}

	ldb, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		b.Fatal(err)
	}

	m := db.NewFileSet("test", ldb)
	m.Replace(remoteDevice0, remote)

	var local []protocol.FileInfo
	for i := 0; i < 10000; i++ {
		local = append(local, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: protocol.Vector{{ID: myID, Value: 1000}}})
	}

	m.Replace(protocol.LocalDeviceID, local)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		for j := range local {
			local[j].Version = local[j].Version.Update(myID)
		}
		b.StartTimer()
		m.Update(protocol.LocalDeviceID, local)
	}
}

func Benchmark10kUpdateSme(b *testing.B) {
	var remote []protocol.FileInfo
	for i := 0; i < 10000; i++ {
		remote = append(remote, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: protocol.Vector{{ID: myID, Value: 1000}}})
	}

	ldb, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		b.Fatal(err)
	}
	m := db.NewFileSet("test", ldb)
	m.Replace(remoteDevice0, remote)

	var local []protocol.FileInfo
	for i := 0; i < 10000; i++ {
		local = append(local, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: protocol.Vector{{ID: myID, Value: 1000}}})
	}

	m.Replace(protocol.LocalDeviceID, local)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Update(protocol.LocalDeviceID, local)
	}
}

func Benchmark10kNeed2k(b *testing.B) {
	var remote []protocol.FileInfo
	for i := 0; i < 10000; i++ {
		remote = append(remote, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: protocol.Vector{{ID: myID, Value: 1000}}})
	}

	ldb, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		b.Fatal(err)
	}

	m := db.NewFileSet("test", ldb)
	m.Replace(remoteDevice0, remote)

	var local []protocol.FileInfo
	for i := 0; i < 8000; i++ {
		local = append(local, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: protocol.Vector{{ID: myID, Value: 1000}}})
	}
	for i := 8000; i < 10000; i++ {
		local = append(local, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: protocol.Vector{{1, 980}}})
	}

	m.Replace(protocol.LocalDeviceID, local)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fs := needList(m, protocol.LocalDeviceID)
		if l := len(fs); l != 2000 {
			b.Errorf("wrong length %d != 2k", l)
		}
	}
}

func Benchmark10kHaveFullList(b *testing.B) {
	var remote []protocol.FileInfo
	for i := 0; i < 10000; i++ {
		remote = append(remote, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: protocol.Vector{{ID: myID, Value: 1000}}})
	}

	ldb, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		b.Fatal(err)
	}

	m := db.NewFileSet("test", ldb)
	m.Replace(remoteDevice0, remote)

	var local []protocol.FileInfo
	for i := 0; i < 2000; i++ {
		local = append(local, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: protocol.Vector{{ID: myID, Value: 1000}}})
	}
	for i := 2000; i < 10000; i++ {
		local = append(local, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: protocol.Vector{{1, 980}}})
	}

	m.Replace(protocol.LocalDeviceID, local)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fs := haveList(m, protocol.LocalDeviceID)
		if l := len(fs); l != 10000 {
			b.Errorf("wrong length %d != 10k", l)
		}
	}
}

func Benchmark10kGlobal(b *testing.B) {
	var remote []protocol.FileInfo
	for i := 0; i < 10000; i++ {
		remote = append(remote, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: protocol.Vector{{ID: myID, Value: 1000}}})
	}

	ldb, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		b.Fatal(err)
	}

	m := db.NewFileSet("test", ldb)
	m.Replace(remoteDevice0, remote)

	var local []protocol.FileInfo
	for i := 0; i < 2000; i++ {
		local = append(local, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: protocol.Vector{{ID: myID, Value: 1000}}})
	}
	for i := 2000; i < 10000; i++ {
		local = append(local, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: protocol.Vector{{1, 980}}})
	}

	m.Replace(protocol.LocalDeviceID, local)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fs := globalList(m)
		if l := len(fs); l != 10000 {
			b.Errorf("wrong length %d != 10k", l)
		}
	}
}

func TestGlobalReset(t *testing.T) {
	ldb, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		t.Fatal(err)
	}

	m := db.NewFileSet("test", ldb)

	local := []protocol.FileInfo{
		{Name: "a", Version: protocol.Vector{{ID: myID, Value: 1000}}},
		{Name: "b", Version: protocol.Vector{{ID: myID, Value: 1000}}},
		{Name: "c", Version: protocol.Vector{{ID: myID, Value: 1000}}},
		{Name: "d", Version: protocol.Vector{{ID: myID, Value: 1000}}},
	}

	remote := []protocol.FileInfo{
		{Name: "a", Version: protocol.Vector{{ID: myID, Value: 1000}}},
		{Name: "b", Version: protocol.Vector{{ID: myID, Value: 1001}}},
		{Name: "c", Version: protocol.Vector{{ID: myID, Value: 1002}}},
		{Name: "e", Version: protocol.Vector{{ID: myID, Value: 1000}}},
	}

	m.Replace(protocol.LocalDeviceID, local)
	g := globalList(m)
	sort.Sort(fileList(g))

	if fmt.Sprint(g) != fmt.Sprint(local) {
		t.Errorf("Global incorrect;\n%v !=\n%v", g, local)
	}

	m.Replace(remoteDevice0, remote)
	m.Replace(remoteDevice0, nil)

	g = globalList(m)
	sort.Sort(fileList(g))

	if fmt.Sprint(g) != fmt.Sprint(local) {
		t.Errorf("Global incorrect;\n%v !=\n%v", g, local)
	}
}

func TestNeed(t *testing.T) {
	ldb, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		t.Fatal(err)
	}

	m := db.NewFileSet("test", ldb)

	local := []protocol.FileInfo{
		{Name: "a", Version: protocol.Vector{{ID: myID, Value: 1000}}},
		{Name: "b", Version: protocol.Vector{{ID: myID, Value: 1000}}},
		{Name: "c", Version: protocol.Vector{{ID: myID, Value: 1000}}},
		{Name: "d", Version: protocol.Vector{{ID: myID, Value: 1000}}},
	}

	remote := []protocol.FileInfo{
		{Name: "a", Version: protocol.Vector{{ID: myID, Value: 1000}}},
		{Name: "b", Version: protocol.Vector{{ID: myID, Value: 1001}}},
		{Name: "c", Version: protocol.Vector{{ID: myID, Value: 1002}}},
		{Name: "e", Version: protocol.Vector{{ID: myID, Value: 1000}}},
	}

	shouldNeed := []protocol.FileInfo{
		{Name: "b", Version: protocol.Vector{{ID: myID, Value: 1001}}},
		{Name: "c", Version: protocol.Vector{{ID: myID, Value: 1002}}},
		{Name: "e", Version: protocol.Vector{{ID: myID, Value: 1000}}},
	}

	m.Replace(protocol.LocalDeviceID, local)
	m.Replace(remoteDevice0, remote)

	need := needList(m, protocol.LocalDeviceID)

	sort.Sort(fileList(need))
	sort.Sort(fileList(shouldNeed))

	if fmt.Sprint(need) != fmt.Sprint(shouldNeed) {
		t.Errorf("Need incorrect;\n%v !=\n%v", need, shouldNeed)
	}
}

func TestLocalVersion(t *testing.T) {
	ldb, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		t.Fatal(err)
	}

	m := db.NewFileSet("test", ldb)

	local1 := []protocol.FileInfo{
		{Name: "a", Version: protocol.Vector{{ID: myID, Value: 1000}}},
		{Name: "b", Version: protocol.Vector{{ID: myID, Value: 1000}}},
		{Name: "c", Version: protocol.Vector{{ID: myID, Value: 1000}}},
		{Name: "d", Version: protocol.Vector{{ID: myID, Value: 1000}}},
	}

	local2 := []protocol.FileInfo{
		local1[0],
		// [1] deleted
		local1[2],
		{Name: "d", Version: protocol.Vector{{ID: myID, Value: 1002}}},
		{Name: "e", Version: protocol.Vector{{ID: myID, Value: 1000}}},
	}

	m.Replace(protocol.LocalDeviceID, local1)
	c0 := m.LocalVersion(protocol.LocalDeviceID)

	m.Replace(protocol.LocalDeviceID, local2)
	c1 := m.LocalVersion(protocol.LocalDeviceID)
	if !(c1 > c0) {
		t.Fatal("Local version number should have incremented")
	}
}

func TestListDropFolder(t *testing.T) {
	ldb, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		t.Fatal(err)
	}

	s0 := db.NewFileSet("test0", ldb)
	local1 := []protocol.FileInfo{
		{Name: "a", Version: protocol.Vector{{ID: myID, Value: 1000}}},
		{Name: "b", Version: protocol.Vector{{ID: myID, Value: 1000}}},
		{Name: "c", Version: protocol.Vector{{ID: myID, Value: 1000}}},
	}
	s0.Replace(protocol.LocalDeviceID, local1)

	s1 := db.NewFileSet("test1", ldb)
	local2 := []protocol.FileInfo{
		{Name: "d", Version: protocol.Vector{{ID: myID, Value: 1002}}},
		{Name: "e", Version: protocol.Vector{{ID: myID, Value: 1002}}},
		{Name: "f", Version: protocol.Vector{{ID: myID, Value: 1002}}},
	}
	s1.Replace(remoteDevice0, local2)

	// Check that we have both folders and their data is in the global list

	expectedFolderList := []string{"test0", "test1"}
	if actualFolderList := db.ListFolders(ldb); !reflect.DeepEqual(actualFolderList, expectedFolderList) {
		t.Fatalf("FolderList mismatch\nE: %v\nA: %v", expectedFolderList, actualFolderList)
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
	if actualFolderList := db.ListFolders(ldb); !reflect.DeepEqual(actualFolderList, expectedFolderList) {
		t.Fatalf("FolderList mismatch\nE: %v\nA: %v", expectedFolderList, actualFolderList)
	}
	if l := len(globalList(s0)); l != 3 {
		t.Errorf("Incorrect global length %d != 3 for s0", l)
	}
	if l := len(globalList(s1)); l != 0 {
		t.Errorf("Incorrect global length %d != 0 for s1", l)
	}
}

func TestGlobalNeedWithInvalid(t *testing.T) {
	ldb, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		t.Fatal(err)
	}

	s := db.NewFileSet("test1", ldb)

	rem0 := fileList{
		protocol.FileInfo{Name: "a", Version: protocol.Vector{{ID: myID, Value: 1002}}, Blocks: genBlocks(4)},
		protocol.FileInfo{Name: "b", Version: protocol.Vector{{ID: myID, Value: 1002}}, Flags: protocol.FlagInvalid},
		protocol.FileInfo{Name: "c", Version: protocol.Vector{{ID: myID, Value: 1002}}, Blocks: genBlocks(4)},
	}
	s.Replace(remoteDevice0, rem0)

	rem1 := fileList{
		protocol.FileInfo{Name: "a", Version: protocol.Vector{{ID: myID, Value: 1002}}, Blocks: genBlocks(4)},
		protocol.FileInfo{Name: "b", Version: protocol.Vector{{ID: myID, Value: 1002}}, Blocks: genBlocks(4)},
		protocol.FileInfo{Name: "c", Version: protocol.Vector{{ID: myID, Value: 1002}}, Flags: protocol.FlagInvalid},
	}
	s.Replace(remoteDevice1, rem1)

	total := fileList{
		// There's a valid copy of each file, so it should be merged
		protocol.FileInfo{Name: "a", Version: protocol.Vector{{ID: myID, Value: 1002}}, Blocks: genBlocks(4)},
		protocol.FileInfo{Name: "b", Version: protocol.Vector{{ID: myID, Value: 1002}}, Blocks: genBlocks(4)},
		protocol.FileInfo{Name: "c", Version: protocol.Vector{{ID: myID, Value: 1002}}, Blocks: genBlocks(4)},
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
	ldb, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		t.Fatal(err)
	}

	s := db.NewFileSet("test", ldb)

	var b bytes.Buffer
	for i := 0; i < 100; i++ {
		b.WriteString("012345678901234567890123456789012345678901234567890")
	}
	name := b.String() // 5000 characters

	local := []protocol.FileInfo{
		{Name: string(name), Version: protocol.Vector{{ID: myID, Value: 1000}}},
	}

	s.Replace(protocol.LocalDeviceID, local)

	gf := globalList(s)
	if l := len(gf); l != 1 {
		t.Fatalf("Incorrect len %d != 1 for global list", l)
	}
	if gf[0].Name != local[0].Name {
		t.Errorf("Incorrect long filename;\n%q !=\n%q",
			gf[0].Name, local[0].Name)
	}
}
