// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/syncthing/syncthing/lib/db/backend"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/protocol"
)

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

func TestIgnoredFiles(t *testing.T) {
	ldb, err := openJSONS(t, "testdata/v0.14.48-ignoredfiles.db.jsons")
	if err != nil {
		t.Fatal(err)
	}
	db := newLowlevel(t, ldb)
	defer db.Close()
	if err := UpdateSchema(db); err != nil {
		t.Fatal(err)
	}

	fs := newFileSet(t, "test", db)

	// The contents of the database are like this:
	//
	// 	fs := newFileSet(t, "test", db)
	// 	fs.Update(protocol.LocalDeviceID, []protocol.FileInfo{
	// 		{ // invalid (ignored) file
	// 			Name:    "foo",
	// 			Type:    protocol.FileInfoTypeFile,
	// 			Invalid: true,
	// 			Version: protocol.Vector{Counters: []protocol.Counter{{ID: 1, Value: 1000}}},
	// 		},
	// 		{ // regular file
	// 			Name:    "bar",
	// 			Type:    protocol.FileInfoTypeFile,
	// 			Version: protocol.Vector{Counters: []protocol.Counter{{ID: 1, Value: 1001}}},
	// 		},
	// 	})
	// 	fs.Update(protocol.DeviceID{42}, []protocol.FileInfo{
	// 		{ // invalid file
	// 			Name:    "baz",
	// 			Type:    protocol.FileInfoTypeFile,
	// 			Invalid: true,
	// 			Version: protocol.Vector{Counters: []protocol.Counter{{ID: 42, Value: 1000}}},
	// 		},
	// 		{ // regular file
	// 			Name:    "quux",
	// 			Type:    protocol.FileInfoTypeFile,
	// 			Version: protocol.Vector{Counters: []protocol.Counter{{ID: 42, Value: 1002}}},
	// 		},
	// 	})

	// Local files should have the "ignored" bit in addition to just being
	// generally invalid if we want to look at the simulation of that bit.

	snap := snapshot(t, fs)
	defer snap.Release()
	fi, ok := snap.Get(protocol.LocalDeviceID, "foo")
	if !ok {
		t.Fatal("foo should exist")
	}
	if !fi.IsInvalid() {
		t.Error("foo should be invalid")
	}
	if !fi.IsIgnored() {
		t.Error("foo should be ignored")
	}

	fi, ok = snap.Get(protocol.LocalDeviceID, "bar")
	if !ok {
		t.Fatal("bar should exist")
	}
	if fi.IsInvalid() {
		t.Error("bar should not be invalid")
	}
	if fi.IsIgnored() {
		t.Error("bar should not be ignored")
	}

	// Remote files have the invalid bit as usual, and the IsInvalid() method
	// should pick this up too.

	fi, ok = snap.Get(protocol.DeviceID{42}, "baz")
	if !ok {
		t.Fatal("baz should exist")
	}
	if !fi.IsInvalid() {
		t.Error("baz should be invalid")
	}
	if !fi.IsInvalid() {
		t.Error("baz should be invalid")
	}

	fi, ok = snap.Get(protocol.DeviceID{42}, "quux")
	if !ok {
		t.Fatal("quux should exist")
	}
	if fi.IsInvalid() {
		t.Error("quux should not be invalid")
	}
	if fi.IsInvalid() {
		t.Error("quux should not be invalid")
	}
}

const myID = 1

var (
	remoteDevice0, remoteDevice1 protocol.DeviceID
	update0to3Folder             = "UpdateSchema0to3"
	invalid                      = "invalid"
	slashPrefixed                = "/notgood"
	haveUpdate0to3               map[protocol.DeviceID][]protocol.FileInfo
)

func init() {
	remoteDevice0, _ = protocol.DeviceIDFromString("AIR6LPZ-7K4PTTV-UXQSMUU-CPQ5YWH-OEDFIIQ-JUG777G-2YQXXR5-YD6AWQR")
	remoteDevice1, _ = protocol.DeviceIDFromString("I6KAH76-66SLLLB-5PFXSOA-UFJCDZC-YAOMLEK-CP2GB32-BV5RQST-3PSROAU")
	haveUpdate0to3 = map[protocol.DeviceID][]protocol.FileInfo{
		protocol.LocalDeviceID: {
			protocol.FileInfo{Name: "a", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(1)},
			protocol.FileInfo{Name: slashPrefixed, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(1)},
		},
		remoteDevice0: {
			protocol.FileInfo{Name: "b", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1001}}}, Blocks: genBlocks(2)},
			protocol.FileInfo{Name: "c", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1002}}}, Blocks: genBlocks(5), RawInvalid: true},
			protocol.FileInfo{Name: "d", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1003}}}, Blocks: genBlocks(7)},
		},
		remoteDevice1: {
			protocol.FileInfo{Name: "c", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1002}}}, Blocks: genBlocks(7)},
			protocol.FileInfo{Name: "d", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1003}}}, Blocks: genBlocks(5), RawInvalid: true},
			protocol.FileInfo{Name: invalid, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1004}}}, Blocks: genBlocks(5), RawInvalid: true},
		},
	}
}

func TestUpdate0to3(t *testing.T) {
	ldb, err := openJSONS(t, "testdata/v0.14.45-update0to3.db.jsons")

	if err != nil {
		t.Fatal(err)
	}

	db := newLowlevel(t, ldb)
	defer db.Close()
	updater := schemaUpdater{db}

	folder := []byte(update0to3Folder)

	if err := updater.updateSchema0to1(0); err != nil {
		t.Fatal(err)
	}

	trans, err := db.newReadOnlyTransaction()
	if err != nil {
		t.Fatal(err)
	}
	defer trans.Release()
	if _, ok, err := trans.getFile(folder, protocol.LocalDeviceID[:], []byte(slashPrefixed)); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Error("File prefixed by '/' was not removed during transition to schema 1")
	}

	var key []byte

	key, err = db.keyer.GenerateGlobalVersionKey(nil, folder, []byte(invalid))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Get(key); err != nil {
		t.Error("Invalid file wasn't added to global list")
	}

	if err := updater.updateSchema1to2(1); err != nil {
		t.Fatal(err)
	}

	found := false
	trans, err = db.newReadOnlyTransaction()
	if err != nil {
		t.Fatal(err)
	}
	defer trans.Release()
	_ = trans.withHaveSequence(folder, 0, func(fi protocol.FileIntf) bool {
		f := fi.(protocol.FileInfo)
		l.Infoln(f)
		if found {
			t.Error("Unexpected additional file via sequence", f.FileName())
			return true
		}
		if e := haveUpdate0to3[protocol.LocalDeviceID][0]; f.IsEquivalentOptional(e, 0, true, true, 0) {
			found = true
		} else {
			t.Errorf("Wrong file via sequence, got %v, expected %v", f, e)
		}
		return true
	})
	if !found {
		t.Error("Local file wasn't added to sequence bucket", err)
	}

	if err := updater.updateSchema2to3(2); err != nil {
		t.Fatal(err)
	}

	need := map[string]protocol.FileInfo{
		haveUpdate0to3[remoteDevice0][0].Name: haveUpdate0to3[remoteDevice0][0],
		haveUpdate0to3[remoteDevice1][0].Name: haveUpdate0to3[remoteDevice1][0],
		haveUpdate0to3[remoteDevice0][2].Name: haveUpdate0to3[remoteDevice0][2],
	}

	trans, err = db.newReadOnlyTransaction()
	if err != nil {
		t.Fatal(err)
	}
	defer trans.Release()

	key, err = trans.keyer.GenerateNeedFileKey(nil, folder, nil)
	if err != nil {
		t.Fatal(err)
	}
	dbi, err := trans.NewPrefixIterator(key)
	if err != nil {
		t.Fatal(err)
	}
	defer dbi.Release()

	for dbi.Next() {
		name := trans.keyer.NameFromGlobalVersionKey(dbi.Key())
		key, err = trans.keyer.GenerateGlobalVersionKey(key, folder, name)
		bs, err := trans.Get(key)
		if err != nil {
			t.Fatal(err)
		}
		var vl VersionListDeprecated
		if err := vl.Unmarshal(bs); err != nil {
			t.Fatal(err)
		}
		key, err = trans.keyer.GenerateDeviceFileKey(key, folder, vl.Versions[0].Device, name)
		if err != nil {
			t.Fatal(err)
		}
		fi, ok, err := trans.getFileTrunc(key, false)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			device := "<invalid>"
			if dev, err := protocol.DeviceIDFromBytes(vl.Versions[0].Device); err != nil {
				device = dev.String()
			}
			t.Fatal("surprise missing global file", string(name), device)
		}
		e, ok := need[fi.FileName()]
		if !ok {
			t.Error("Got unexpected needed file:", fi.FileName())
		}
		f := fi.(protocol.FileInfo)
		delete(need, f.Name)
		if !f.IsEquivalentOptional(e, 0, true, true, 0) {
			t.Errorf("Wrong needed file, got %v, expected %v", f, e)
		}
	}
	if dbi.Error() != nil {
		t.Fatal(err)
	}

	for n := range need {
		t.Errorf(`Missing needed file "%v"`, n)
	}
}

// TestRepairSequence checks that a few hand-crafted messed-up sequence entries get fixed.
func TestRepairSequence(t *testing.T) {
	db := newLowlevelMemory(t)
	defer db.Close()

	folderStr := "test"
	folder := []byte(folderStr)
	id := protocol.LocalDeviceID
	short := protocol.LocalDeviceID.Short()

	files := []protocol.FileInfo{
		{Name: "fine", Blocks: genBlocks(1)},
		{Name: "duplicate", Blocks: genBlocks(2)},
		{Name: "missing", Blocks: genBlocks(3)},
		{Name: "overwriting", Blocks: genBlocks(4)},
		{Name: "inconsistent", Blocks: genBlocks(5)},
		{Name: "inconsistentNotIndirected", Blocks: genBlocks(2)},
	}
	for i, f := range files {
		files[i].Version = f.Version.Update(short)
	}

	trans, err := db.newReadWriteTransaction()
	if err != nil {
		t.Fatal(err)
	}
	defer trans.close()

	addFile := func(f protocol.FileInfo, seq int64) {
		dk, err := trans.keyer.GenerateDeviceFileKey(nil, folder, id[:], []byte(f.Name))
		if err != nil {
			t.Fatal(err)
		}
		if err := trans.putFile(dk, f); err != nil {
			t.Fatal(err)
		}
		sk, err := trans.keyer.GenerateSequenceKey(nil, folder, seq)
		if err != nil {
			t.Fatal(err)
		}
		if err := trans.Put(sk, dk); err != nil {
			t.Fatal(err)
		}
	}

	// Plain normal entry
	var seq int64 = 1
	files[0].Sequence = 1
	addFile(files[0], seq)

	// Second entry once updated with original sequence still in place
	f := files[1]
	f.Sequence = int64(len(files) + 1)
	addFile(f, f.Sequence)
	// Original sequence entry
	seq++
	sk, err := trans.keyer.GenerateSequenceKey(nil, folder, seq)
	if err != nil {
		t.Fatal(err)
	}
	dk, err := trans.keyer.GenerateDeviceFileKey(nil, folder, id[:], []byte(f.Name))
	if err != nil {
		t.Fatal(err)
	}
	if err := trans.Put(sk, dk); err != nil {
		t.Fatal(err)
	}

	// File later overwritten thus missing sequence entry
	seq++
	files[2].Sequence = seq
	addFile(files[2], seq)

	// File overwriting previous sequence entry (no seq bump)
	seq++
	files[3].Sequence = seq
	addFile(files[3], seq)

	// Inconistent files
	seq++
	files[4].Sequence = 101
	addFile(files[4], seq)
	seq++
	files[5].Sequence = 102
	addFile(files[5], seq)

	// And a sequence entry pointing at nothing because why not
	sk, err = trans.keyer.GenerateSequenceKey(nil, folder, 100001)
	if err != nil {
		t.Fatal(err)
	}
	dk, err = trans.keyer.GenerateDeviceFileKey(nil, folder, id[:], []byte("nonexisting"))
	if err != nil {
		t.Fatal(err)
	}
	if err := trans.Put(sk, dk); err != nil {
		t.Fatal(err)
	}

	if err := trans.Commit(); err != nil {
		t.Fatal(err)
	}

	// Loading the metadata for the first time means a "re"calculation happens,
	// along which the sequences get repaired too.
	db.gcMut.RLock()
	_, err = db.loadMetadataTracker(folderStr)
	db.gcMut.RUnlock()
	if err != nil {
		t.Fatal(err)
	}

	// Check the db
	ro, err := db.newReadOnlyTransaction()
	if err != nil {
		t.Fatal(err)
	}
	defer ro.close()

	it, err := ro.NewPrefixIterator([]byte{KeyTypeDevice})
	if err != nil {
		t.Fatal(err)
	}
	defer it.Release()
	for it.Next() {
		fi, err := ro.unmarshalTrunc(it.Value(), true)
		if err != nil {
			t.Fatal(err)
		}
		if sk, err = ro.keyer.GenerateSequenceKey(sk, folder, fi.SequenceNo()); err != nil {
			t.Fatal(err)
		}
		dk, err := ro.Get(sk)
		if backend.IsNotFound(err) {
			t.Error("Missing sequence entry for", fi.FileName())
		} else if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(it.Key(), dk) {
			t.Errorf("Wrong key for %v, expected %s, got %s", f.FileName(), it.Key(), dk)
		}
	}
	if err := it.Error(); err != nil {
		t.Fatal(err)
	}
	it.Release()

	it, err = ro.NewPrefixIterator([]byte{KeyTypeSequence})
	if err != nil {
		t.Fatal(err)
	}
	defer it.Release()
	for it.Next() {
		intf, ok, err := ro.getFileTrunc(it.Value(), false)
		if err != nil {
			t.Fatal(err)
		}
		fi := intf.(protocol.FileInfo)
		seq := ro.keyer.SequenceFromSequenceKey(it.Key())
		if !ok {
			t.Errorf("Sequence entry %v points at nothing", seq)
		} else if fi.SequenceNo() != seq {
			t.Errorf("Inconsistent sequence entry for %v: %v != %v", fi.FileName(), fi.SequenceNo(), seq)
		}
		if len(fi.Blocks) == 0 {
			t.Error("Missing blocks in", fi.FileName())
		}
	}
	if err := it.Error(); err != nil {
		t.Fatal(err)
	}
	it.Release()
}

func TestDowngrade(t *testing.T) {
	db := newLowlevelMemory(t)
	defer db.Close()
	// sets the min version etc
	if err := UpdateSchema(db); err != nil {
		t.Fatal(err)
	}

	// Bump the database version to something newer than we actually support
	miscDB := NewMiscDataNamespace(db)
	if err := miscDB.PutInt64("dbVersion", dbVersion+1); err != nil {
		t.Fatal(err)
	}
	l.Infoln(dbVersion)

	// Pretend we just opened the DB and attempt to update it again
	err := UpdateSchema(db)

	if err, ok := err.(*databaseDowngradeError); !ok {
		t.Fatal("Expected error due to database downgrade, got", err)
	} else if err.minSyncthingVersion != dbMinSyncthingVersion {
		t.Fatalf("Error has %v as min Syncthing version, expected %v", err.minSyncthingVersion, dbMinSyncthingVersion)
	}
}

func TestCheckGlobals(t *testing.T) {
	db := newLowlevelMemory(t)
	defer db.Close()

	fs := newFileSet(t, "test", db)

	// Add any file
	name := "foo"
	fs.Update(protocol.LocalDeviceID, []protocol.FileInfo{
		{
			Name:    name,
			Type:    protocol.FileInfoTypeFile,
			Version: protocol.Vector{Counters: []protocol.Counter{{ID: 1, Value: 1001}}},
		},
	})

	// Remove just the file entry
	if err := db.dropPrefix([]byte{KeyTypeDevice}); err != nil {
		t.Fatal(err)
	}

	// Clean up global entry of the now missing file
	if repaired, err := db.checkGlobals(fs.folder); err != nil {
		t.Fatal(err)
	} else if repaired != 1 {
		t.Error("Expected 1 repaired global item, got", repaired)
	}

	// Check that the global entry is gone
	gk, err := db.keyer.GenerateGlobalVersionKey(nil, []byte(fs.folder), []byte(name))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Get(gk)
	if !backend.IsNotFound(err) {
		t.Error("Expected key missing error, got", err)
	}
}

func TestUpdateTo10(t *testing.T) {
	ldb, err := openJSONS(t, "./testdata/v1.4.0-updateTo10.json")
	if err != nil {
		t.Fatal(err)
	}
	db := newLowlevel(t, ldb)
	defer db.Close()

	UpdateSchema(db)

	folder := "test"

	meta, err := db.getMetaAndCheck(folder)
	if err != nil {
		t.Fatal(err)
	}

	empty := Counts{}

	c := meta.Counts(protocol.LocalDeviceID, needFlag)
	if c.Files != 1 {
		t.Error("Expected 1 needed file locally, got", c.Files)
	}
	c.Files = 0
	if c.Deleted != 1 {
		t.Error("Expected 1 needed deletion locally, got", c.Deleted)
	}
	c.Deleted = 0
	if !c.Equal(empty) {
		t.Error("Expected all counts to be zero, got", c)
	}
	c = meta.Counts(remoteDevice0, needFlag)
	if !c.Equal(empty) {
		t.Error("Expected all counts to be zero, got", c)
	}

	trans, err := db.newReadOnlyTransaction()
	if err != nil {
		t.Fatal(err)
	}
	defer trans.Release()
	// a
	vl, err := trans.getGlobalVersions(nil, []byte(folder), []byte("a"))
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range vl.RawVersions {
		if !v.Deleted {
			t.Error("Unexpected undeleted global version for a")
		}
	}
	// b
	vl, err = trans.getGlobalVersions(nil, []byte(folder), []byte("b"))
	if err != nil {
		t.Fatal(err)
	}
	if !vl.RawVersions[0].Deleted {
		t.Error("vl.Versions[0] not deleted for b")
	}
	if vl.RawVersions[1].Deleted {
		t.Error("vl.Versions[1] deleted for b")
	}
	// c
	vl, err = trans.getGlobalVersions(nil, []byte(folder), []byte("c"))
	if err != nil {
		t.Fatal(err)
	}
	if vl.RawVersions[0].Deleted {
		t.Error("vl.Versions[0] deleted for c")
	}
	if !vl.RawVersions[1].Deleted {
		t.Error("vl.Versions[1] not deleted for c")
	}
}

func TestDropDuplicates(t *testing.T) {
	names := []string{
		"foo",
		"bar",
		"dcxvoijnds",
		"3d/dsfase/4/ss2",
	}
	tcs := []struct{ in, out []int }{
		{[]int{0}, []int{0}},
		{[]int{0, 1}, []int{0, 1}},
		{[]int{0, 1, 0, 1}, []int{0, 1}},
		{[]int{0, 1, 1, 1, 1}, []int{0, 1}},
		{[]int{0, 0, 0, 1}, []int{0, 1}},
		{[]int{0, 1, 2, 3}, []int{0, 1, 2, 3}},
		{[]int{3, 2, 1, 0, 0, 1, 2, 3}, []int{0, 1, 2, 3}},
		{[]int{0, 1, 1, 3, 0, 1, 0, 1, 2, 3}, []int{0, 1, 2, 3}},
	}

	for tci, tc := range tcs {
		inp := make([]protocol.FileInfo, len(tc.in))
		expSeq := make(map[string]int)
		for i, j := range tc.in {
			inp[i] = protocol.FileInfo{Name: names[j], Sequence: int64(i)}
			expSeq[names[j]] = i
		}
		outp := normalizeFilenamesAndDropDuplicates(inp)
		if len(outp) != len(tc.out) {
			t.Errorf("tc %v: Expected %v entries, got %v", tci, len(tc.out), len(outp))
			continue
		}
		for i, f := range outp {
			if exp := names[tc.out[i]]; exp != f.Name {
				t.Errorf("tc %v: Got file %v at pos %v, expected %v", tci, f.Name, i, exp)
			}
			if exp := int64(expSeq[outp[i].Name]); exp != f.Sequence {
				t.Errorf("tc %v: Got sequence %v at pos %v, expected %v", tci, f.Sequence, i, exp)
			}
		}
	}
}

func TestGCIndirect(t *testing.T) {
	// Verify that the gcIndirect run actually removes block lists.

	db := newLowlevelMemory(t)
	defer db.Close()
	meta := newMetadataTracker(db.keyer, events.NoopLogger)

	// Add three files with different block lists

	files := []protocol.FileInfo{
		{Name: "a", Blocks: genBlocks(100)},
		{Name: "b", Blocks: genBlocks(200)},
		{Name: "c", Blocks: genBlocks(300)},
	}

	db.updateLocalFiles([]byte("folder"), files, meta)

	// Run a GC pass

	db.gcIndirect(context.Background())

	// Verify that we have three different block lists

	n, err := numBlockLists(db)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(files) {
		t.Fatal("expected each file to have a block list")
	}

	// Change the block lists for each file

	for i := range files {
		files[i].Version = files[i].Version.Update(42)
		files[i].Blocks = genBlocks(len(files[i].Blocks) + 1)
	}

	db.updateLocalFiles([]byte("folder"), files, meta)

	// Verify that we now have *six* different block lists

	n, err = numBlockLists(db)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2*len(files) {
		t.Fatal("expected both old and new block lists to exist")
	}

	// Run a GC pass

	db.gcIndirect(context.Background())

	// Verify that we now have just the three we need, again

	n, err = numBlockLists(db)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(files) {
		t.Fatal("expected GC to collect all but the needed ones")
	}

	// Double check the correctness by loading the block lists and comparing with what we stored

	tr, err := db.newReadOnlyTransaction()
	if err != nil {
		t.Fatal()
	}
	defer tr.Release()
	for _, f := range files {
		fi, ok, err := tr.getFile([]byte("folder"), protocol.LocalDeviceID[:], []byte(f.Name))
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatal("mysteriously missing")
		}
		if len(fi.Blocks) != len(f.Blocks) {
			t.Fatal("block list mismatch")
		}
		for i := range fi.Blocks {
			if !bytes.Equal(fi.Blocks[i].Hash, f.Blocks[i].Hash) {
				t.Fatal("hash mismatch")
			}
		}
	}
}

func TestUpdateTo14(t *testing.T) {
	db := newLowlevelMemory(t)
	defer db.Close()

	folderStr := "default"
	folder := []byte(folderStr)
	name := []byte("foo")
	file := protocol.FileInfo{Name: string(name), Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(blocksIndirectionCutoff - 1)}
	file.BlocksHash = protocol.BlocksHash(file.Blocks)
	fileWOBlocks := file
	fileWOBlocks.Blocks = nil
	meta, err := db.loadMetadataTracker(folderStr)
	if err != nil {
		t.Fatal(err)
	}

	// Initally add the correct file the usual way, all good here.
	if err := db.updateLocalFiles(folder, []protocol.FileInfo{file}, meta); err != nil {
		t.Fatal(err)
	}

	// Simulate the previous bug, where .putFile could write a file info without
	// blocks, even though the file has them (and thus a non-nil BlocksHash).
	trans, err := db.newReadWriteTransaction()
	if err != nil {
		t.Fatal(err)
	}
	defer trans.close()
	key, err := db.keyer.GenerateDeviceFileKey(nil, folder, protocol.LocalDeviceID[:], name)
	if err != nil {
		t.Fatal(err)
	}
	fiBs := mustMarshal(&fileWOBlocks)
	if err := trans.Put(key, fiBs); err != nil {
		t.Fatal(err)
	}
	if err := trans.Commit(); err != nil {
		t.Fatal(err)
	}
	trans.close()

	// Run migration, pretending were still on schema 13.
	if err := (&schemaUpdater{db}).updateSchemaTo14(13); err != nil {
		t.Fatal(err)
	}

	// checks
	ro, err := db.newReadOnlyTransaction()
	if err != nil {
		t.Fatal(err)
	}
	defer ro.close()
	if f, ok, err := ro.getFileByKey(key); err != nil {
		t.Fatal(err)
	} else if !ok {
		t.Error("file missing")
	} else if !f.MustRescan() {
		t.Error("file not marked as MustRescan")
	}

	if vl, err := ro.getGlobalVersions(nil, folder, name); err != nil {
		t.Fatal(err)
	} else if fv, ok := vl.GetGlobal(); !ok {
		t.Error("missing global")
	} else if !fv.IsInvalid() {
		t.Error("global not marked as invalid")
	}
}

func TestFlushRecursion(t *testing.T) {
	// Verify that a commit hook can write to the transaction without
	// causing another flush and thus recursion.

	db := newLowlevelMemory(t)
	defer db.Close()

	// A commit hook that writes a small piece of data to the transaction.
	hookFired := 0
	hook := func(tx backend.WriteTransaction) error {
		err := tx.Put([]byte(fmt.Sprintf("hook-key-%d", hookFired)), []byte(fmt.Sprintf("hook-value-%d", hookFired)))
		if err != nil {
			t.Fatal(err)
		}
		hookFired++
		return nil
	}

	// A transaction.
	tx, err := db.NewWriteTransaction(hook)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Release()

	// Write stuff until the transaction flushes, thus firing the hook.
	i := 0
	for hookFired == 0 {
		err := tx.Put([]byte(fmt.Sprintf("key-%d", i)), []byte(fmt.Sprintf("value-%d", i)))
		if err != nil {
			t.Fatal(err)
		}
		i++
	}

	// The hook should have fired precisely once.
	if hookFired != 1 {
		t.Error("expect one hook fire, not", hookFired)
	}
}

func TestCheckLocalNeed(t *testing.T) {
	db := newLowlevelMemory(t)
	defer db.Close()

	folderStr := "test"
	fs := newFileSet(t, folderStr, db)

	// Add files such that we are in sync for a and b, and need c and d.
	files := []protocol.FileInfo{
		{Name: "a", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1}}}},
		{Name: "b", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1}}}},
		{Name: "c", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1}}}},
		{Name: "d", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1}}}},
	}
	fs.Update(protocol.LocalDeviceID, files)
	files[2].Version = files[2].Version.Update(remoteDevice0.Short())
	files[3].Version = files[2].Version.Update(remoteDevice0.Short())
	fs.Update(remoteDevice0, files)

	checkNeed := func() {
		snap := snapshot(t, fs)
		defer snap.Release()
		c := snap.NeedSize(protocol.LocalDeviceID)
		if c.Files != 2 {
			t.Errorf("Expected 2 needed files locally, got %v in meta", c.Files)
		}
		needed := make([]protocol.FileInfo, 0, 2)
		snap.WithNeed(protocol.LocalDeviceID, func(fi protocol.FileIntf) bool {
			needed = append(needed, fi.(protocol.FileInfo))
			return true
		})
		if l := len(needed); l != 2 {
			t.Errorf("Expected 2 needed files locally, got %v in db", l)
		} else if needed[0].Name != "c" || needed[1].Name != "d" {
			t.Errorf("Expected files c and d to be needed, got %v and %v", needed[0].Name, needed[1].Name)
		}
	}

	checkNeed()

	trans, err := db.newReadWriteTransaction()
	if err != nil {
		t.Fatal(err)
	}
	defer trans.close()

	// Add "b" to needed and remove "d"
	folder := []byte(folderStr)
	key, err := trans.keyer.GenerateNeedFileKey(nil, folder, []byte(files[1].Name))
	if err != nil {
		t.Fatal(err)
	}
	if err = trans.Put(key, nil); err != nil {
		t.Fatal(err)
	}
	key, err = trans.keyer.GenerateNeedFileKey(nil, folder, []byte(files[3].Name))
	if err != nil {
		t.Fatal(err)
	}
	if err = trans.Delete(key); err != nil {
		t.Fatal(err)
	}
	if err := trans.Commit(); err != nil {
		t.Fatal(err)
	}

	if repaired, err := db.checkLocalNeed(folder); err != nil {
		t.Fatal(err)
	} else if repaired != 2 {
		t.Error("Expected 2 repaired local need items, got", repaired)
	}

	checkNeed()
}

func TestDuplicateNeedCount(t *testing.T) {
	db := newLowlevelMemory(t)
	defer db.Close()

	folder := "test"

	fs := newFileSet(t, folder, db)
	files := []protocol.FileInfo{{Name: "foo", Version: protocol.Vector{}.Update(myID), Sequence: 1}}
	fs.Update(protocol.LocalDeviceID, files)
	files[0].Version = files[0].Version.Update(remoteDevice0.Short())
	fs.Update(remoteDevice0, files)

	db.checkRepair()

	fs = newFileSet(t, folder, db)
	found := false
	for _, c := range fs.meta.counts.Counts {
		if bytes.Equal(protocol.LocalDeviceID[:], c.DeviceID) && c.LocalFlags == needFlag {
			if found {
				t.Fatal("second need count for local device encountered")
			}
			found = true
		}
	}
	if !found {
		t.Fatal("no need count for local device encountered")
	}
}

func TestNeedAfterDropGlobal(t *testing.T) {
	db := newLowlevelMemory(t)
	defer db.Close()

	folder := "test"

	fs := newFileSet(t, folder, db)

	// Initial:
	// Three devices and a file "test": local has Version 1, remoteDevice0
	// Version 2 and remoteDevice2 doesn't have it.
	// All of them have "bar", just so the db knows about remoteDevice2.
	files := []protocol.FileInfo{
		{Name: "foo", Version: protocol.Vector{}.Update(myID), Sequence: 1},
		{Name: "bar", Version: protocol.Vector{}.Update(myID), Sequence: 2},
	}
	fs.Update(protocol.LocalDeviceID, files)
	files[0].Version = files[0].Version.Update(myID)
	fs.Update(remoteDevice0, files)
	fs.Update(remoteDevice1, files[1:])

	// remoteDevice1 needs one file: test
	snap := snapshot(t, fs)
	c := snap.NeedSize(remoteDevice1)
	if c.Files != 1 {
		t.Errorf("Expected 1 needed files initially, got %v", c.Files)
	}
	snap.Release()

	// Drop remoteDevice0, i.e. remove all their files from db.
	// That changes the global file, which is now what local has.
	fs.Drop(remoteDevice0)

	// remoteDevice1 still needs test.
	snap = snapshot(t, fs)
	c = snap.NeedSize(remoteDevice1)
	if c.Files != 1 {
		t.Errorf("Expected still 1 needed files, got %v", c.Files)
	}
	snap.Release()
}

func numBlockLists(db *Lowlevel) (int, error) {
	it, err := db.Backend.NewPrefixIterator([]byte{KeyTypeBlockList})
	if err != nil {
		return 0, err
	}
	defer it.Release()
	n := 0
	for it.Next() {
		n++
	}
	if err := it.Error(); err != nil {
		return 0, err
	}
	return n, nil
}
