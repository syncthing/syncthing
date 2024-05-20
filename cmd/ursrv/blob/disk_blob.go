// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package blob

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Disk struct {
	path string
}

func NewDisk(path string) *Disk {
	return &Disk{path: path}
}

func (d *Disk) Put(key string, data []byte) error {
	path := filepath.Join(d.path, key)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func (d *Disk) Get(key string) ([]byte, error) {
	path := filepath.Join(d.path, key)
	return os.ReadFile(path)
}

func (d *Disk) Delete(key string) error {
	path := filepath.Join(d.path, key)
	return os.Remove(path)
}

func (d *Disk) IterateFromDate(_ context.Context, reportType string, from time.Time, fn func([]byte) bool) error {
	prefix := fmt.Sprintf("%s/%s", reportType, commonTimestampPrefix(from, time.Now()))
	matches, err := filepath.Glob(filepath.Join(d.path, prefix+"*"))
	if err != nil {
		return err
	}

	for _, file := range matches {
		if !hasValidDate(filepath.Base(file), from) {
			continue
		}
		stat, err := os.Lstat(file)
		if err != nil {
			continue
		}
		if stat.IsDir() {
			continue
		}
		content, err := os.ReadFile(file)
		if err != nil {
			continue
		}

		if !fn(content) {
			break
		}
	}
	return err
}

func (d *Disk) Iterate(_ context.Context, key string, fn func([]byte) bool) error {
	matches, err := filepath.Glob(filepath.Join(d.path, key+"*"))
	if err != nil {
		return err
	}
loop:
	for _, file := range matches {
		stat, err := os.Lstat(file)
		if err != nil {
			continue
		}
		if stat.IsDir() {
			continue
		}
		content, err := os.ReadFile(file)
		if err != nil {
			continue
		}

		if !fn(content) {
			break loop
		}
	}

	return err
}

func (d *Disk) CountFromDate(reportType string, from time.Time) (int, error) {
	prefix := fmt.Sprintf("%s/%s", reportType, commonTimestampPrefix(from, time.Now()))
	matches, err := filepath.Glob(filepath.Join(d.path, prefix+"*"))
	if err != nil {
		return 0, err
	}

	var count = 0
	for _, match := range matches {
		if !hasValidDate(match, from) {
			continue
		}
		count++
	}
	return count, nil
}
