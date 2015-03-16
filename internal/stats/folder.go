// Copyright (C) 2014 The Syncthing Authors.
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

package stats

import (
	"time"

	"github.com/syncthing/syncthing/internal/db"
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
