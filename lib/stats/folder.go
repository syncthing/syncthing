// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package stats

import (
	"time"

	"github.com/syncthing/syncthing/internal/db"
)

type FolderStatistics struct {
	LastFile LastFile  `json:"lastFile"`
	LastScan time.Time `json:"lastScan"`
}

type FolderStatisticsReference struct {
	kv *db.Typed
}

type LastFile struct {
	At       time.Time `json:"at"`
	Filename string    `json:"filename"`
	Deleted  bool      `json:"deleted"`
}

func NewFolderStatisticsReference(kv *db.Typed) *FolderStatisticsReference {
	return &FolderStatisticsReference{
		kv: kv,
	}
}

func (s *FolderStatisticsReference) GetLastFile() (LastFile, error) {
	at, ok, err := s.kv.Time("lastFileAt")
	if err != nil {
		return LastFile{}, err
	} else if !ok {
		return LastFile{}, nil
	}
	file, ok, err := s.kv.String("lastFileName")
	if err != nil {
		return LastFile{}, err
	} else if !ok {
		return LastFile{}, nil
	}
	deleted, _, err := s.kv.Bool("lastFileDeleted")
	if err != nil {
		return LastFile{}, err
	}
	return LastFile{
		At:       at,
		Filename: file,
		Deleted:  deleted,
	}, nil
}

func (s *FolderStatisticsReference) ReceivedFile(file string, deleted bool) error {
	if err := s.kv.PutTime("lastFileAt", time.Now().Truncate(time.Second)); err != nil {
		return err
	}
	if err := s.kv.PutString("lastFileName", file); err != nil {
		return err
	}
	if err := s.kv.PutBool("lastFileDeleted", deleted); err != nil {
		return err
	}
	return nil
}

func (s *FolderStatisticsReference) ScanCompleted() error {
	return s.kv.PutTime("lastScan", time.Now().Truncate(time.Second))
}

func (s *FolderStatisticsReference) GetLastScanTime() (time.Time, error) {
	lastScan, ok, err := s.kv.Time("lastScan")
	if err != nil {
		return time.Time{}, err
	} else if !ok {
		return time.Time{}, nil
	}
	return lastScan, nil
}

func (s *FolderStatisticsReference) GetStatistics() (FolderStatistics, error) {
	lastFile, err := s.GetLastFile()
	if err != nil {
		return FolderStatistics{}, err
	}
	lastScanTime, err := s.GetLastScanTime()
	if err != nil {
		return FolderStatistics{}, err
	}
	return FolderStatistics{
		LastFile: lastFile,
		LastScan: lastScanTime,
	}, nil
}
