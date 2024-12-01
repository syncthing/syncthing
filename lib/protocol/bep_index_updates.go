// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package protocol

import "github.com/syncthing/syncthing/internal/gen/bep"

type Index struct {
	Folder       string
	Files        []FileInfo
	LastSequence int64
}

func (i *Index) toWire() *bep.Index {
	files := make([]*bep.FileInfo, len(i.Files))
	for j, f := range i.Files {
		files[j] = f.ToWire(false)
	}
	return &bep.Index{
		Folder:       i.Folder,
		Files:        files,
		LastSequence: i.LastSequence,
	}
}

func indexFromWire(w *bep.Index) *Index {
	if w == nil {
		return nil
	}
	i := &Index{
		Folder:       w.Folder,
		LastSequence: w.LastSequence,
	}
	i.Files = make([]FileInfo, len(w.Files))
	for j, f := range w.Files {
		i.Files[j] = FileInfoFromWire(f)
	}
	return i
}

type IndexUpdate struct {
	Folder       string
	Files        []FileInfo
	LastSequence int64
	PrevSequence int64
}

func (i *IndexUpdate) toWire() *bep.IndexUpdate {
	files := make([]*bep.FileInfo, len(i.Files))
	for j, f := range i.Files {
		files[j] = f.ToWire(false)
	}
	return &bep.IndexUpdate{
		Folder:       i.Folder,
		Files:        files,
		LastSequence: i.LastSequence,
		PrevSequence: i.PrevSequence,
	}
}

func indexUpdateFromWire(w *bep.IndexUpdate) *IndexUpdate {
	if w == nil {
		return nil
	}
	i := &IndexUpdate{
		Folder:       w.Folder,
		LastSequence: w.LastSequence,
		PrevSequence: w.PrevSequence,
	}
	i.Files = make([]FileInfo, len(w.Files))
	for j, f := range w.Files {
		i.Files[j] = FileInfoFromWire(f)
	}
	return i
}
