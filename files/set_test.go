// Copyright (C) 2014 Jakob Borg and other contributors. All rights reserved.
// Use of this source code is governed by an MIT-style license that can be
// found in the LICENSE file.

package files_test

import (
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/calmh/syncthing/cid"
	"github.com/calmh/syncthing/files"
	"github.com/calmh/syncthing/lamport"
	"github.com/calmh/syncthing/protocol"
	"github.com/calmh/syncthing/scanner"
)

type fileList []scanner.File

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
	m := files.NewSet()

	local := []scanner.File{
		scanner.File{Name: "a", Version: 1000},
		scanner.File{Name: "b", Version: 1000},
		scanner.File{Name: "c", Version: 1000},
		scanner.File{Name: "d", Version: 1000},
	}

	remote0 := []scanner.File{
		scanner.File{Name: "a", Version: 1000},
		scanner.File{Name: "c", Version: 1002},
	}
	remote1 := []scanner.File{
		scanner.File{Name: "b", Version: 1001},
		scanner.File{Name: "e", Version: 1000},
	}
	remoteTot := []scanner.File{
		scanner.File{Name: "a", Version: 1000},
		scanner.File{Name: "b", Version: 1001},
		scanner.File{Name: "c", Version: 1002},
		scanner.File{Name: "e", Version: 1000},
	}

	expectedGlobal := []scanner.File{
		scanner.File{Name: "a", Version: 1000},
		scanner.File{Name: "b", Version: 1001},
		scanner.File{Name: "c", Version: 1002},
		scanner.File{Name: "d", Version: 1000},
		scanner.File{Name: "e", Version: 1000},
	}

	expectedLocalNeed := []scanner.File{
		scanner.File{Name: "b", Version: 1001},
		scanner.File{Name: "c", Version: 1002},
		scanner.File{Name: "e", Version: 1000},
	}

	expectedRemoteNeed := []scanner.File{
		scanner.File{Name: "d", Version: 1000},
	}

	m.ReplaceWithDelete(cid.LocalID, local)
	m.Replace(1, remote0)
	m.Update(1, remote1)

	g := m.Global()
	sort.Sort(fileList(g))

	if !reflect.DeepEqual(g, expectedGlobal) {
		t.Errorf("Global incorrect;\n A: %v !=\n E: %v", g, expectedGlobal)
	}

	h := m.Have(cid.LocalID)
	sort.Sort(fileList(h))

	if !reflect.DeepEqual(h, local) {
		t.Errorf("Have incorrect;\n A: %v !=\n E: %v", h, local)
	}

	h = m.Have(1)
	sort.Sort(fileList(h))

	if !reflect.DeepEqual(h, remoteTot) {
		t.Errorf("Have incorrect;\n A: %v !=\n E: %v", h, remoteTot)
	}

	n := m.Need(cid.LocalID)
	sort.Sort(fileList(n))

	if !reflect.DeepEqual(n, expectedLocalNeed) {
		t.Errorf("Need incorrect;\n A: %v !=\n E: %v", n, expectedLocalNeed)
	}

	n = m.Need(1)
	sort.Sort(fileList(n))

	if !reflect.DeepEqual(n, expectedRemoteNeed) {
		t.Errorf("Need incorrect;\n A: %v !=\n E: %v", n, expectedRemoteNeed)
	}

	f := m.Get(cid.LocalID, "b")
	if !reflect.DeepEqual(f, local[1]) {
		t.Errorf("Get incorrect;\n A: %v !=\n E: %v", f, local[1])
	}

	f = m.Get(1, "b")
	if !reflect.DeepEqual(f, remote1[0]) {
		t.Errorf("Get incorrect;\n A: %v !=\n E: %v", f, remote1[0])
	}

	f = m.GetGlobal("b")
	if !reflect.DeepEqual(f, remote1[0]) {
		t.Errorf("Get incorrect;\n A: %v !=\n E: %v", f, remote1[0])
	}

	a := int(m.Availability("a"))
	if av := 1<<0 + 1<<1; a != av {
		t.Errorf("Availability incorrect;\n A: %v !=\n E: %v", a, av)
	}
	a = int(m.Availability("b"))
	if av := 1 << 1; a != av {
		t.Errorf("Availability incorrect;\n A: %v !=\n E: %v", a, av)
	}
	a = int(m.Availability("d"))
	if av := 1 << 0; a != av {
		t.Errorf("Availability incorrect;\n A: %v !=\n E: %v", a, av)
	}
}

func TestLocalDeleted(t *testing.T) {
	m := files.NewSet()
	lamport.Default = lamport.Clock{}

	local1 := []scanner.File{
		scanner.File{Name: "a", Version: 1000},
		scanner.File{Name: "b", Version: 1000},
		scanner.File{Name: "c", Version: 1000},
		scanner.File{Name: "d", Version: 1000},
		scanner.File{Name: "z", Version: 1000, Flags: protocol.FlagDirectory},
	}

	m.ReplaceWithDelete(cid.LocalID, local1)

	m.ReplaceWithDelete(cid.LocalID, []scanner.File{
		local1[0],
		// [1] removed
		local1[2],
		local1[3],
		local1[4],
	})
	m.ReplaceWithDelete(cid.LocalID, []scanner.File{
		local1[0],
		local1[2],
		// [3] removed
		local1[4],
	})
	m.ReplaceWithDelete(cid.LocalID, []scanner.File{
		local1[0],
		local1[2],
		// [4] removed
	})

	expectedGlobal1 := []scanner.File{
		local1[0],
		scanner.File{Name: "b", Version: 1001, Flags: protocol.FlagDeleted},
		local1[2],
		scanner.File{Name: "d", Version: 1002, Flags: protocol.FlagDeleted},
		scanner.File{Name: "z", Version: 1003, Flags: protocol.FlagDeleted | protocol.FlagDirectory},
	}

	g := m.Global()
	sort.Sort(fileList(g))
	sort.Sort(fileList(expectedGlobal1))

	if !reflect.DeepEqual(g, expectedGlobal1) {
		t.Errorf("Global incorrect;\n A: %v !=\n E: %v", g, expectedGlobal1)
	}

	m.ReplaceWithDelete(cid.LocalID, []scanner.File{
		local1[0],
		// [2] removed
	})

	expectedGlobal2 := []scanner.File{
		local1[0],
		scanner.File{Name: "b", Version: 1001, Flags: protocol.FlagDeleted},
		scanner.File{Name: "c", Version: 1004, Flags: protocol.FlagDeleted},
		scanner.File{Name: "d", Version: 1002, Flags: protocol.FlagDeleted},
		scanner.File{Name: "z", Version: 1003, Flags: protocol.FlagDeleted | protocol.FlagDirectory},
	}

	g = m.Global()
	sort.Sort(fileList(g))
	sort.Sort(fileList(expectedGlobal2))

	if !reflect.DeepEqual(g, expectedGlobal2) {
		t.Errorf("Global incorrect;\n A: %v !=\n E: %v", g, expectedGlobal2)
	}
}

func Benchmark10kReplace(b *testing.B) {
	var local []scanner.File
	for i := 0; i < 10000; i++ {
		local = append(local, scanner.File{Name: fmt.Sprintf("file%d", i), Version: 1000})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := files.NewSet()
		m.ReplaceWithDelete(cid.LocalID, local)
	}
}

func Benchmark10kUpdateChg(b *testing.B) {
	var remote []scanner.File
	for i := 0; i < 10000; i++ {
		remote = append(remote, scanner.File{Name: fmt.Sprintf("file%d", i), Version: 1000})
	}

	m := files.NewSet()
	m.Replace(1, remote)

	var local []scanner.File
	for i := 0; i < 10000; i++ {
		local = append(local, scanner.File{Name: fmt.Sprintf("file%d", i), Version: 1000})
	}

	m.ReplaceWithDelete(cid.LocalID, local)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		for j := range local {
			local[j].Version++
		}
		b.StartTimer()
		m.Update(cid.LocalID, local)
	}
}

func Benchmark10kUpdateSme(b *testing.B) {
	var remote []scanner.File
	for i := 0; i < 10000; i++ {
		remote = append(remote, scanner.File{Name: fmt.Sprintf("file%d", i), Version: 1000})
	}

	m := files.NewSet()
	m.Replace(1, remote)

	var local []scanner.File
	for i := 0; i < 10000; i++ {
		local = append(local, scanner.File{Name: fmt.Sprintf("file%d", i), Version: 1000})
	}

	m.ReplaceWithDelete(cid.LocalID, local)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Update(cid.LocalID, local)
	}
}

func Benchmark10kNeed2k(b *testing.B) {
	var remote []scanner.File
	for i := 0; i < 10000; i++ {
		remote = append(remote, scanner.File{Name: fmt.Sprintf("file%d", i), Version: 1000})
	}

	m := files.NewSet()
	m.Replace(cid.LocalID+1, remote)

	var local []scanner.File
	for i := 0; i < 8000; i++ {
		local = append(local, scanner.File{Name: fmt.Sprintf("file%d", i), Version: 1000})
	}
	for i := 8000; i < 10000; i++ {
		local = append(local, scanner.File{Name: fmt.Sprintf("file%d", i), Version: 980})
	}

	m.ReplaceWithDelete(cid.LocalID, local)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fs := m.Need(cid.LocalID)
		if l := len(fs); l != 2000 {
			b.Errorf("wrong length %d != 2k", l)
		}
	}
}

func Benchmark10kHave(b *testing.B) {
	var remote []scanner.File
	for i := 0; i < 10000; i++ {
		remote = append(remote, scanner.File{Name: fmt.Sprintf("file%d", i), Version: 1000})
	}

	m := files.NewSet()
	m.Replace(cid.LocalID+1, remote)

	var local []scanner.File
	for i := 0; i < 2000; i++ {
		local = append(local, scanner.File{Name: fmt.Sprintf("file%d", i), Version: 1000})
	}
	for i := 2000; i < 10000; i++ {
		local = append(local, scanner.File{Name: fmt.Sprintf("file%d", i), Version: 980})
	}

	m.ReplaceWithDelete(cid.LocalID, local)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fs := m.Have(cid.LocalID)
		if l := len(fs); l != 10000 {
			b.Errorf("wrong length %d != 10k", l)
		}
	}
}

func Benchmark10kGlobal(b *testing.B) {
	var remote []scanner.File
	for i := 0; i < 10000; i++ {
		remote = append(remote, scanner.File{Name: fmt.Sprintf("file%d", i), Version: 1000})
	}

	m := files.NewSet()
	m.Replace(cid.LocalID+1, remote)

	var local []scanner.File
	for i := 0; i < 2000; i++ {
		local = append(local, scanner.File{Name: fmt.Sprintf("file%d", i), Version: 1000})
	}
	for i := 2000; i < 10000; i++ {
		local = append(local, scanner.File{Name: fmt.Sprintf("file%d", i), Version: 980})
	}

	m.ReplaceWithDelete(cid.LocalID, local)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fs := m.Global()
		if l := len(fs); l != 10000 {
			b.Errorf("wrong length %d != 10k", l)
		}
	}
}

func TestGlobalReset(t *testing.T) {
	m := files.NewSet()

	local := []scanner.File{
		scanner.File{Name: "a", Version: 1000},
		scanner.File{Name: "b", Version: 1000},
		scanner.File{Name: "c", Version: 1000},
		scanner.File{Name: "d", Version: 1000},
	}

	remote := []scanner.File{
		scanner.File{Name: "a", Version: 1000},
		scanner.File{Name: "b", Version: 1001},
		scanner.File{Name: "c", Version: 1002},
		scanner.File{Name: "e", Version: 1000},
	}

	m.ReplaceWithDelete(cid.LocalID, local)
	g := m.Global()
	sort.Sort(fileList(g))

	if !reflect.DeepEqual(g, local) {
		t.Errorf("Global incorrect;\n%v !=\n%v", g, local)
	}

	m.Replace(1, remote)
	m.Replace(1, nil)

	g = m.Global()
	sort.Sort(fileList(g))

	if !reflect.DeepEqual(g, local) {
		t.Errorf("Global incorrect;\n%v !=\n%v", g, local)
	}
}

func TestNeed(t *testing.T) {
	m := files.NewSet()

	local := []scanner.File{
		scanner.File{Name: "a", Version: 1000},
		scanner.File{Name: "b", Version: 1000},
		scanner.File{Name: "c", Version: 1000},
		scanner.File{Name: "d", Version: 1000},
	}

	remote := []scanner.File{
		scanner.File{Name: "a", Version: 1000},
		scanner.File{Name: "b", Version: 1001},
		scanner.File{Name: "c", Version: 1002},
		scanner.File{Name: "e", Version: 1000},
	}

	shouldNeed := []scanner.File{
		scanner.File{Name: "b", Version: 1001},
		scanner.File{Name: "c", Version: 1002},
		scanner.File{Name: "e", Version: 1000},
	}

	m.ReplaceWithDelete(cid.LocalID, local)
	m.Replace(1, remote)

	need := m.Need(0)

	sort.Sort(fileList(need))
	sort.Sort(fileList(shouldNeed))

	if !reflect.DeepEqual(need, shouldNeed) {
		t.Errorf("Need incorrect;\n%v !=\n%v", need, shouldNeed)
	}
}

func TestChanges(t *testing.T) {
	m := files.NewSet()

	local1 := []scanner.File{
		scanner.File{Name: "a", Version: 1000},
		scanner.File{Name: "b", Version: 1000},
		scanner.File{Name: "c", Version: 1000},
		scanner.File{Name: "d", Version: 1000},
	}

	local2 := []scanner.File{
		local1[0],
		// [1] deleted
		local1[2],
		scanner.File{Name: "d", Version: 1002},
		scanner.File{Name: "e", Version: 1000},
	}

	m.ReplaceWithDelete(cid.LocalID, local1)
	c0 := m.Changes(cid.LocalID)

	m.ReplaceWithDelete(cid.LocalID, local2)
	c1 := m.Changes(cid.LocalID)
	if !(c1 > c0) {
		t.Fatal("Change number should have incremented")
	}

	m.ReplaceWithDelete(cid.LocalID, local2)
	c2 := m.Changes(cid.LocalID)
	if c2 != c1 {
		t.Fatal("Change number should be unchanged")
	}
}
