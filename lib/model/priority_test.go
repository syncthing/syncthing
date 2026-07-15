// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/fs"
)

func TestPrioritizePullQueue(t *testing.T) {
	queue := newJobQueue()
	for _, name := range []string{
		"archive/data.bin",
		"docs/notes.txt",
		"docs/report.pdf",
		"ROOT.PDF",
		"urgent/first.bin",
		"urgent/second.bin",
	} {
		queue.Push(name, 0, time.Time{})
	}

	patterns := []string{
		"urgent/**",
		"(?i)/*.pdf",
		"docs/*.txt",
	}
	filesystem := fs.NewFilesystem(fs.FilesystemTypeFake, strings.ReplaceAll(t.Name(), "/", "-"))
	if err := prioritizePullQueue(queue, filesystem, patterns); err != nil {
		t.Fatal(err)
	}

	_, got, _ := queue.Jobs(1, 100)
	want := []string{
		"urgent/first.bin",
		"urgent/second.bin",
		"ROOT.PDF",
		"docs/notes.txt",
		"archive/data.bin",
		"docs/report.pdf",
	}
	if !slices.Equal(got, want) {
		t.Errorf("unexpected prioritized order\n got: %v\nwant: %v", got, want)
	}
}

func TestPrioritizePullQueueRejectsInvalidGlob(t *testing.T) {
	queue := newJobQueue()
	queue.Push("file.txt", 0, time.Time{})
	filesystem := fs.NewFilesystem(fs.FilesystemTypeFake, strings.ReplaceAll(t.Name(), "/", "-"))

	err := prioritizePullQueue(queue, filesystem, []string{"["})
	if err == nil || !strings.Contains(err.Error(), "parsing priority glob") {
		t.Fatalf("expected a contextual parse error, got %v", err)
	}
}
