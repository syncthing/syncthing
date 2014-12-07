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
	"encoding/binary"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
)

const (
	folderStatisticTypeLastFile = iota
)

var folderStatisticsTypes = []byte{
	folderStatisticTypeLastFile,
}

type FolderStatistics struct {
	LastFile *LastFile
}

type FolderStatisticsReference struct {
	db     *leveldb.DB
	folder string
}

func NewFolderStatisticsReference(db *leveldb.DB, folder string) *FolderStatisticsReference {
	return &FolderStatisticsReference{
		db:     db,
		folder: folder,
	}
}

func (s *FolderStatisticsReference) key(stat byte) []byte {
	k := make([]byte, 1+1+64)
	k[0] = keyTypeFolderStatistic
	k[1] = stat
	copy(k[1+1:], s.folder[:])
	return k
}

func (s *FolderStatisticsReference) GetLastFile() *LastFile {
	value, err := s.db.Get(s.key(folderStatisticTypeLastFile), nil)
	if err != nil {
		if err != leveldb.ErrNotFound {
			l.Warnln("FolderStatisticsReference: Failed loading last file filename value for", s.folder, ":", err)
		}
		return nil
	}

	file := LastFile{}
	err = file.UnmarshalBinary(value)
	if err != nil {
		l.Warnln("FolderStatisticsReference: Failed loading last file value for", s.folder, ":", err)
		return nil
	}
	return &file
}

func (s *FolderStatisticsReference) ReceivedFile(filename string) {
	f := LastFile{
		Filename: filename,
		At:       time.Now(),
	}
	if debug {
		l.Debugln("stats.FolderStatisticsReference.ReceivedFile:", s.folder)
	}

	value, err := f.MarshalBinary()
	if err != nil {
		l.Warnln("FolderStatisticsReference: Failed serializing last file value for", s.folder, ":", err)
		return
	}

	err = s.db.Put(s.key(folderStatisticTypeLastFile), value, nil)
	if err != nil {
		l.Warnln("Failed update last file value for", s.folder, ":", err)
	}
}

// Never called, maybe because it's worth while to keep the data
// or maybe because we have no easy way of knowing that a folder has been removed.
func (s *FolderStatisticsReference) Delete() error {
	for _, stype := range folderStatisticsTypes {
		err := s.db.Delete(s.key(stype), nil)
		if debug && err == nil {
			l.Debugln("stats.FolderStatisticsReference.Delete:", s.folder, stype)
		}
		if err != nil && err != leveldb.ErrNotFound {
			return err
		}
	}
	return nil
}

func (s *FolderStatisticsReference) GetStatistics() FolderStatistics {
	return FolderStatistics{
		LastFile: s.GetLastFile(),
	}
}

type LastFile struct {
	At       time.Time
	Filename string
}

func (f *LastFile) MarshalBinary() ([]byte, error) {
	buf := make([]byte, 8+len(f.Filename))
	binary.BigEndian.PutUint64(buf[:8], uint64(f.At.Unix()))
	copy(buf[8:], []byte(f.Filename))
	return buf, nil
}

func (f *LastFile) UnmarshalBinary(buf []byte) error {
	f.At = time.Unix(int64(binary.BigEndian.Uint64(buf[:8])), 0)
	f.Filename = string(buf[8:])
	return nil
}
