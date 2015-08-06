// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package stats

import (
	"time"

	"github.com/syncthing/protocol"

	"github.com/syncthing/syncthing/lib/db"
	"github.com/syndtr/goleveldb/leveldb"
)

type FolderStatistics struct {
	LastFile LastFile `json:"lastFile"`
}

type FolderStatisticsReference struct {
	ns     *db.NamespacedKV
	folder string
}

type LastFile struct {
	At       time.Time `json:"at"`
	Filename string    `json:"filename"`
	Deleted  bool      `json:"deleted"`
}

func NewFolderStatisticsReference(ldb *leveldb.DB, folder string) *FolderStatisticsReference {
	prefix := string(db.KeyTypeFolderStatistic) + folder
	return &FolderStatisticsReference{
		ns:     db.NewNamespacedKV(ldb, prefix),
		folder: folder,
	}
}

func (s *FolderStatisticsReference) GetLastFile() LastFile {
	at, ok := s.ns.Time("lastFileAt")
	if !ok {
		return LastFile{}
	}
	file, ok := s.ns.String("lastFileName")
	if !ok {
		return LastFile{}
	}
	deleted, ok := s.ns.Bool("lastFileDeleted")
	return LastFile{
		At:       at,
		Filename: file,
		Deleted:  deleted,
	}
}

func (s *FolderStatisticsReference) ReceivedFile(file protocol.FileInfo) {
	if debug {
		l.Debugln("stats.FolderStatisticsReference.ReceivedFile:", s.folder, file)
	}
	s.ns.PutTime("lastFileAt", time.Now())
	s.ns.PutString("lastFileName", file.Name)
	s.ns.PutBool("lastFileDeleted", file.IsDeleted())
}

func (s *FolderStatisticsReference) GetStatistics() FolderStatistics {
	return FolderStatistics{
		LastFile: s.GetLastFile(),
	}
}
