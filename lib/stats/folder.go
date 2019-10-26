// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package stats

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/db"
)

type FolderStatistics struct {
	LastFile            LastFile            `json:"lastFile"`
	LastScan            time.Time           `json:"lastScan"`
	FolderTreeStructure FolderTreeStructure `json:"folderTreeStructure"`
}

type FolderTreeStructure struct {
	Name     string                `json:"name"`
	IsFolder bool                  `json:"isFolder"`
	Children []FolderTreeStructure `json:"children"`
}

const currentFolder = "."

func (o FolderTreeStructure) Equal(o1 FolderTreeStructure) bool {
	if o1.Name != o.Name || o1.IsFolder != o.IsFolder || len(o1.Children) != len(o.Children) {
		return false
	}

	namedChildren := make(map[string][]FolderTreeStructure)

	for _, child := range o.Children {
		namedChildren[child.Name] = append(namedChildren[child.Name], child)
	}
	for _, child := range o1.Children {
		namedChildren[child.Name] = append(namedChildren[child.Name], child)
	}

	for _, children := range namedChildren {
		if len(children) != 2 {
			return false
		}
		if !children[0].Equal(children[1]) {
			return false
		}
	}

	return true
}

type FolderStatisticsReference struct {
	ns     *db.NamespacedKV
	folder string
	path   string
}

type LastFile struct {
	At       time.Time `json:"at"`
	Filename string    `json:"filename"`
	Deleted  bool      `json:"deleted"`
}

func NewFolderStatisticsReference(ldb *db.Lowlevel, folder, path string) *FolderStatisticsReference {
	return &FolderStatisticsReference{
		ns:     db.NewFolderStatisticsNamespace(ldb, folder),
		folder: folder,
		path:   path,
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

func listFiles(dir string) []string {
	var res []string
	filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.Mode().IsRegular() {
			res = append(res, path)
		}
		return nil
	})
	return res
}

func removePathSubstring(paths []string, basePath string) (trimmed []string) {
	for _, path := range paths {
		trimmed = append(trimmed, strings.TrimPrefix(path, basePath+"/"))
	}
	return
}

func getFolderDivider() byte {
	switch runtime.GOOS {
	case "windows":
		return '\\'
	default:
		return '/'
	}
}

func pathsToFolderTreeStructure(paths []string) FolderTreeStructure {
	fs := make(map[string][]string)
	for _, path := range paths {
		if strings.Contains(path, string(getFolderDivider())) {
			outerFolderName := path[:strings.IndexByte(path, getFolderDivider())]
			folderItem := path[strings.IndexByte(path, getFolderDivider())+1:]
			fs[outerFolderName] = append(fs[outerFolderName], folderItem)
		} else {
			fs[currentFolder] = append(fs[currentFolder], path)
		}
	}

	var result FolderTreeStructure
	result.IsFolder = true
	for outerFolderName, folderItems := range fs {
		if outerFolderName == currentFolder {
			for _, file := range folderItems {
				var child FolderTreeStructure
				child.Name = file
				child.IsFolder = false
				child.Children = make([]FolderTreeStructure, 0)
				result.Children = append(result.Children, child)
			}
		} else {
			var child FolderTreeStructure
			child = pathsToFolderTreeStructure(folderItems)
			child.IsFolder = true
			child.Name = outerFolderName
			result.Children = append(result.Children, child)
		}
	}

	return result
}

func (s *FolderStatisticsReference) GetStatistics() FolderStatistics {
	return FolderStatistics{
		LastFile:            s.GetLastFile(),
		LastScan:            s.GetLastScanTime(),
		FolderTreeStructure: pathsToFolderTreeStructure(removePathSubstring(listFiles(s.path), s.path)),
	}
}
