// Copyright (C) 2023 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type diskStore struct {
	dir      string
	inbox    chan diskEntry
	maxBytes int64
	maxFiles int

	currentFiles []currentFile
	currentSize  int64
}

type diskEntry struct {
	path string
	data []byte
}

type currentFile struct {
	path  string
	size  int64
	mtime int64
}

func (d *diskStore) Serve(ctx context.Context) {
	if err := os.MkdirAll(d.dir, 0750); err != nil {
		log.Println("Creating directory:", err)
		return
	}

	if err := d.inventory(); err != nil {
		log.Println("Failed to inventory disk store:", err)
	}
	d.clean()

	cleanTimer := time.NewTicker(time.Minute)
	inventoryTimer := time.NewTicker(24 * time.Hour)

	buf := new(bytes.Buffer)
	gw := gzip.NewWriter(buf)
	for {
		select {
		case entry := <-d.inbox:
			path := d.fullPath(entry.path)

			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				log.Println("Creating directory:", err)
				continue
			}

			buf.Reset()
			gw.Reset(buf)
			if _, err := gw.Write(entry.data); err != nil {
				log.Println("Failed to compress crash report:", err)
				continue
			}
			if err := gw.Close(); err != nil {
				log.Println("Failed to compress crash report:", err)
				continue
			}
			if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
				log.Printf("Failed to write %s: %v", entry.path, err)
				_ = os.Remove(path)
				continue
			}

			d.currentSize += int64(buf.Len())
			d.currentFiles = append(d.currentFiles, currentFile{
				size: int64(len(entry.data)),
				path: path,
			})

		case <-cleanTimer.C:
			d.clean()

		case <-inventoryTimer.C:
			if err := d.inventory(); err != nil {
				log.Println("Failed to inventory disk store:", err)
			}

		case <-ctx.Done():
			return
		}
	}
}

func (d *diskStore) Put(path string, data []byte) bool {
	select {
	case d.inbox <- diskEntry{
		path: path,
		data: data,
	}:
		return true
	default:
		return false
	}
}

func (d *diskStore) Get(path string) ([]byte, error) {
	path = d.fullPath(path)
	bs, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	gr, err := gzip.NewReader(bytes.NewReader(bs))
	if err != nil {
		return nil, err
	}
	defer gr.Close()
	return io.ReadAll(gr)
}

func (d *diskStore) Exists(path string) bool {
	path = d.fullPath(path)
	_, err := os.Lstat(path)
	return err == nil
}

func (d *diskStore) clean() {
	for len(d.currentFiles) > 0 && (len(d.currentFiles) > d.maxFiles || d.currentSize > d.maxBytes) {
		f := d.currentFiles[0]
		log.Println("Removing", f.path)
		if err := os.Remove(f.path); err != nil {
			log.Println("Failed to remove file:", err)
		}
		d.currentFiles = d.currentFiles[1:]
		d.currentSize -= f.size
	}
	var oldest time.Duration
	if len(d.currentFiles) > 0 {
		oldest = time.Since(time.Unix(d.currentFiles[0].mtime, 0)).Truncate(time.Minute)
	}
	log.Printf("Clean complete: %d files, %d MB, oldest is %v ago", len(d.currentFiles), d.currentSize>>20, oldest)
}

func (d *diskStore) inventory() error {
	d.currentFiles = nil
	d.currentSize = 0
	err := filepath.Walk(d.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".gz" {
			return nil
		}
		d.currentSize += info.Size()
		d.currentFiles = append(d.currentFiles, currentFile{
			path:  path,
			size:  info.Size(),
			mtime: info.ModTime().Unix(),
		})
		return nil
	})
	sort.Slice(d.currentFiles, func(i, j int) bool {
		return d.currentFiles[i].mtime < d.currentFiles[j].mtime
	})
	var oldest time.Duration
	if len(d.currentFiles) > 0 {
		oldest = time.Since(time.Unix(d.currentFiles[0].mtime, 0)).Truncate(time.Minute)
	}
	log.Printf("Inventory complete: %d files, %d MB, oldest is %v ago", len(d.currentFiles), d.currentSize>>20, oldest)
	return err
}

func (d *diskStore) fullPath(path string) string {
	return filepath.Join(d.dir, path[0:2], path[2:]) + ".gz"
}
