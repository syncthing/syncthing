// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

package files_test

import (
	"bytes"
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/syncthing/syncthing/internal/files"
	"github.com/syncthing/syncthing/internal/lamport"
	"github.com/syncthing/syncthing/internal/protocol"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"
)

var remoteDevice0, remoteDevice1 protocol.DeviceID

func init() {
	remoteDevice0, _ = protocol.DeviceIDFromString("AIR6LPZ-7K4PTTV-UXQSMUU-CPQ5YWH-OEDFIIQ-JUG777G-2YQXXR5-YD6AWQR")
	remoteDevice1, _ = protocol.DeviceIDFromString("I6KAH76-66SLLLB-5PFXSOA-UFJCDZC-YAOMLEK-CP2GB32-BV5RQST-3PSROAU")
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

func haveList(s *files.Set, n protocol.DeviceID) []protocol.FileInfo {
	var fs []protocol.FileInfo
	s.WithHave(n, func(fi protocol.FileIntf) bool {
		f := fi.(protocol.FileInfo)
		fs = append(fs, f)
		return true
	})
	return fs
}

func needList(s *files.Set, n protocol.DeviceID) []protocol.FileInfo {
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

	m.ReplaceWithDelete(protocol.LocalDeviceID, local0)
	m.ReplaceWithDelete(protocol.LocalDeviceID, local1)
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

	f := m.Get(protocol.LocalDeviceID, "b")
	if fmt.Sprint(f) != fmt.Sprint(localTot[1]) {
		t.Errorf("Get incorrect;\n A: %v !=\n E: %v", f, localTot[1])
	}

	f = m.Get(remoteDevice0, "b")
	if fmt.Sprint(f) != fmt.Sprint(remote1[0]) {
		t.Errorf("Get incorrect;\n A: %v !=\n E: %v", f, remote1[0])
	}

	f = m.GetGlobal("b")
	if fmt.Sprint(f) != fmt.Sprint(remote1[0]) {
		t.Errorf("GetGlobal incorrect;\n A: %v !=\n E: %v", f, remote1[0])
	}

	f = m.Get(protocol.LocalDeviceID, "zz")
	if f.Name != "" {
		t.Errorf("Get incorrect;\n A: %v !=\n E: %v", f, protocol.FileInfo{})
	}

	f = m.GetGlobal("zz")
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

	s.ReplaceWithDelete(protocol.LocalDeviceID, localHave)
	s.Replace(remoteDevice0, remote0Have)
	s.Replace(remoteDevice1, remote1Have)

	need := fileList(needList(s, protocol.LocalDeviceID))
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

	s.ReplaceWithDelete(protocol.LocalDeviceID, localHave)

	have := fileList(haveList(s, protocol.LocalDeviceID))
	sort.Sort(have)

	if fmt.Sprint(have) != fmt.Sprint(localHave) {
		t.Errorf("Have incorrect before invalidation;\n A: %v !=\n E: %v", have, localHave)
	}

	localHave[1] = protocol.FileInfo{Name: "b", Version: 1001, Flags: protocol.FlagInvalid}
	s.Update(protocol.LocalDeviceID, localHave[1:2])

	have = fileList(haveList(s, protocol.LocalDeviceID))
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

func TestLocalDeleted(t *testing.T) {
	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		t.Fatal(err)
	}
	m := files.NewSet("test", db)
	lamport.Default = lamport.Clock{}

	local1 := []protocol.FileInfo{
		{Name: "a", Version: 1000},
		{Name: "b", Version: 1000},
		{Name: "c", Version: 1000},
		{Name: "d", Version: 1000},
		{Name: "z", Version: 1000, Flags: protocol.FlagDirectory},
	}

	m.ReplaceWithDelete(protocol.LocalDeviceID, local1)

	m.ReplaceWithDelete(protocol.LocalDeviceID, []protocol.FileInfo{
		local1[0],
		// [1] removed
		local1[2],
		local1[3],
		local1[4],
	})
	m.ReplaceWithDelete(protocol.LocalDeviceID, []protocol.FileInfo{
		local1[0],
		local1[2],
		// [3] removed
		local1[4],
	})
	m.ReplaceWithDelete(protocol.LocalDeviceID, []protocol.FileInfo{
		local1[0],
		local1[2],
		// [4] removed
	})

	expectedGlobal1 := []protocol.FileInfo{
		local1[0],
		{Name: "b", Version: 1001, Flags: protocol.FlagDeleted},
		local1[2],
		{Name: "d", Version: 1002, Flags: protocol.FlagDeleted},
		{Name: "z", Version: 1003, Flags: protocol.FlagDeleted | protocol.FlagDirectory},
	}

	g := globalList(m)
	sort.Sort(fileList(g))
	sort.Sort(fileList(expectedGlobal1))

	if fmt.Sprint(g) != fmt.Sprint(expectedGlobal1) {
		t.Errorf("Global incorrect;\n A: %v !=\n E: %v", g, expectedGlobal1)
	}

	m.ReplaceWithDelete(protocol.LocalDeviceID, []protocol.FileInfo{
		local1[0],
		// [2] removed
	})

	expectedGlobal2 := []protocol.FileInfo{
		local1[0],
		{Name: "b", Version: 1001, Flags: protocol.FlagDeleted},
		{Name: "c", Version: 1004, Flags: protocol.FlagDeleted},
		{Name: "d", Version: 1002, Flags: protocol.FlagDeleted},
		{Name: "z", Version: 1003, Flags: protocol.FlagDeleted | protocol.FlagDirectory},
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
		m.ReplaceWithDelete(protocol.LocalDeviceID, local)
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
	m.Replace(remoteDevice0, remote)

	var local []protocol.FileInfo
	for i := 0; i < 10000; i++ {
		local = append(local, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: 1000})
	}

	m.ReplaceWithDelete(protocol.LocalDeviceID, local)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		for j := range local {
			local[j].Version++
		}
		b.StartTimer()
		m.Update(protocol.LocalDeviceID, local)
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
	m.Replace(remoteDevice0, remote)

	var local []protocol.FileInfo
	for i := 0; i < 10000; i++ {
		local = append(local, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: 1000})
	}

	m.ReplaceWithDelete(protocol.LocalDeviceID, local)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Update(protocol.LocalDeviceID, local)
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
	m.Replace(remoteDevice0, remote)

	var local []protocol.FileInfo
	for i := 0; i < 8000; i++ {
		local = append(local, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: 1000})
	}
	for i := 8000; i < 10000; i++ {
		local = append(local, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: 980})
	}

	m.ReplaceWithDelete(protocol.LocalDeviceID, local)

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
		remote = append(remote, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: 1000})
	}

	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		b.Fatal(err)
	}

	m := files.NewSet("test", db)
	m.Replace(remoteDevice0, remote)

	var local []protocol.FileInfo
	for i := 0; i < 2000; i++ {
		local = append(local, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: 1000})
	}
	for i := 2000; i < 10000; i++ {
		local = append(local, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: 980})
	}

	m.ReplaceWithDelete(protocol.LocalDeviceID, local)

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
		remote = append(remote, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: 1000})
	}

	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		b.Fatal(err)
	}

	m := files.NewSet("test", db)
	m.Replace(remoteDevice0, remote)

	var local []protocol.FileInfo
	for i := 0; i < 2000; i++ {
		local = append(local, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: 1000})
	}
	for i := 2000; i < 10000; i++ {
		local = append(local, protocol.FileInfo{Name: fmt.Sprintf("file%d", i), Version: 980})
	}

	m.ReplaceWithDelete(protocol.LocalDeviceID, local)

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
		{Name: "a", Version: 1000},
		{Name: "b", Version: 1000},
		{Name: "c", Version: 1000},
		{Name: "d", Version: 1000},
	}

	remote := []protocol.FileInfo{
		{Name: "a", Version: 1000},
		{Name: "b", Version: 1001},
		{Name: "c", Version: 1002},
		{Name: "e", Version: 1000},
	}

	m.ReplaceWithDelete(protocol.LocalDeviceID, local)
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
	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		t.Fatal(err)
	}

	m := files.NewSet("test", db)

	local := []protocol.FileInfo{
		{Name: "a", Version: 1000},
		{Name: "b", Version: 1000},
		{Name: "c", Version: 1000},
		{Name: "d", Version: 1000},
	}

	remote := []protocol.FileInfo{
		{Name: "a", Version: 1000},
		{Name: "b", Version: 1001},
		{Name: "c", Version: 1002},
		{Name: "e", Version: 1000},
	}

	shouldNeed := []protocol.FileInfo{
		{Name: "b", Version: 1001},
		{Name: "c", Version: 1002},
		{Name: "e", Version: 1000},
	}

	m.ReplaceWithDelete(protocol.LocalDeviceID, local)
	m.Replace(remoteDevice0, remote)

	need := needList(m, protocol.LocalDeviceID)

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
		{Name: "a", Version: 1000},
		{Name: "b", Version: 1000},
		{Name: "c", Version: 1000},
		{Name: "d", Version: 1000},
	}

	local2 := []protocol.FileInfo{
		local1[0],
		// [1] deleted
		local1[2],
		{Name: "d", Version: 1002},
		{Name: "e", Version: 1000},
	}

	m.ReplaceWithDelete(protocol.LocalDeviceID, local1)
	c0 := m.LocalVersion(protocol.LocalDeviceID)

	m.ReplaceWithDelete(protocol.LocalDeviceID, local2)
	c1 := m.LocalVersion(protocol.LocalDeviceID)
	if !(c1 > c0) {
		t.Fatal("Local version number should have incremented")
	}

	m.ReplaceWithDelete(protocol.LocalDeviceID, local2)
	c2 := m.LocalVersion(protocol.LocalDeviceID)
	if c2 != c1 {
		t.Fatal("Local version number should be unchanged")
	}
}

func TestListDropFolder(t *testing.T) {
	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		t.Fatal(err)
	}

	s0 := files.NewSet("test0", db)
	local1 := []protocol.FileInfo{
		{Name: "a", Version: 1000},
		{Name: "b", Version: 1000},
		{Name: "c", Version: 1000},
	}
	s0.Replace(protocol.LocalDeviceID, local1)

	s1 := files.NewSet("test1", db)
	local2 := []protocol.FileInfo{
		{Name: "d", Version: 1002},
		{Name: "e", Version: 1002},
		{Name: "f", Version: 1002},
	}
	s1.Replace(remoteDevice0, local2)

	// Check that we have both folders and their data is in the global list

	expectedFolderList := []string{"test0", "test1"}
	if actualFolderList := files.ListFolders(db); !reflect.DeepEqual(actualFolderList, expectedFolderList) {
		t.Fatalf("FolderList mismatch\nE: %v\nA: %v", expectedFolderList, actualFolderList)
	}
	if l := len(globalList(s0)); l != 3 {
		t.Errorf("Incorrect global length %d != 3 for s0", l)
	}
	if l := len(globalList(s1)); l != 3 {
		t.Errorf("Incorrect global length %d != 3 for s1", l)
	}

	// Drop one of them and check that it's gone.

	files.DropFolder(db, "test1")

	expectedFolderList = []string{"test0"}
	if actualFolderList := files.ListFolders(db); !reflect.DeepEqual(actualFolderList, expectedFolderList) {
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
	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		t.Fatal(err)
	}

	s := files.NewSet("test1", db)

	rem0 := fileList{
		protocol.FileInfo{Name: "a", Version: 1002, Blocks: genBlocks(4)},
		protocol.FileInfo{Name: "b", Version: 1002, Flags: protocol.FlagInvalid},
		protocol.FileInfo{Name: "c", Version: 1002, Blocks: genBlocks(4)},
	}
	s.Replace(remoteDevice0, rem0)

	rem1 := fileList{
		protocol.FileInfo{Name: "a", Version: 1002, Blocks: genBlocks(4)},
		protocol.FileInfo{Name: "b", Version: 1002, Blocks: genBlocks(4)},
		protocol.FileInfo{Name: "c", Version: 1002, Flags: protocol.FlagInvalid},
	}
	s.Replace(remoteDevice1, rem1)

	total := fileList{
		// There's a valid copy of each file, so it should be merged
		protocol.FileInfo{Name: "a", Version: 1002, Blocks: genBlocks(4)},
		protocol.FileInfo{Name: "b", Version: 1002, Blocks: genBlocks(4)},
		protocol.FileInfo{Name: "c", Version: 1002, Blocks: genBlocks(4)},
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
		{Name: string(name), Version: 1000},
	}

	s.ReplaceWithDelete(protocol.LocalDeviceID, local)

	gf := globalList(s)
	if l := len(gf); l != 1 {
		t.Fatalf("Incorrect len %d != 1 for global list", l)
	}
	if gf[0].Name != local[0].Name {
		t.Errorf("Incorrect long filename;\n%q !=\n%q",
			gf[0].Name, local[0].Name)
	}
}
