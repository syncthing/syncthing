// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package stats

import (
	"time"

	"github.com/syncthing/syncthing/internal/db"
	"github.com/syndtr/goleveldb/leveldb"
)

type FolderStatistics struct {
	LastFile LastFile
}

type FolderStatisticsReference struct {
	ns     *db.NamespacedKV
	folder string
}

type LastFile struct {
	At       time.Time
	Filename string
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
	return LastFile{
		At:       at,
		Filename: file,
	}
}

func (s *FolderStatisticsReference) ReceivedFile(filename string) {
	if debug {
		l.Debugln("stats.FolderStatisticsReference.ReceivedFile:", s.folder, filename)
	}
	s.ns.PutTime("lastFileAt", time.Now())
	s.ns.PutString("lastFileName", filename)
}

func (s *FolderStatisticsReference) GetStatistics() FolderStatistics {
	return FolderStatistics{
		LastFile: s.GetLastFile(),
	}
}
