// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package stats

import (
	"time"

	"github.com/syncthing/syncthing/lib/db"
)

type FolderStatistics struct {
	LastFile LastFile  `json:"lastFile"`
	LastScan time.Time `json:"lastScan"`
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

func NewFolderStatisticsReference(ldb *db.Instance, folder string) *FolderStatisticsReference {
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
	deleted, _ := s.ns.Bool("lastFileDeleted")
	return LastFile{
		At:       at,
		Filename: file,
		Deleted:  deleted,
	}
}

func (s *FolderStatisticsReference) ReceivedFile(file string, deleted bool) {
	l.Debugln("stats.FolderStatisticsReference.ReceivedFile:", s.folder, file)
	s.ns.PutTime("lastFileAt", time.Now())
	s.ns.PutString("lastFileName", file)
	s.ns.PutBool("lastFileDeleted", deleted)
}

func (s *FolderStatisticsReference) ScanCompleted() {
	s.ns.PutTime("lastScan", time.Now())
}

func (s *FolderStatisticsReference) GetLastScanTime() time.Time {
	lastScan, ok := s.ns.Time("lastScan")
	if !ok {
		return time.Time{}
	}
	return lastScan
}

func (s *FolderStatisticsReference) GetStatistics() FolderStatistics {
	return FolderStatistics{
		LastFile: s.GetLastFile(),
		LastScan: s.GetLastScanTime(),
	}
}
