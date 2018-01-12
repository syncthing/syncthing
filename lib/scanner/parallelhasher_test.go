// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package scanner

import (
	"context"
	"os"
	gosync "sync"
	"testing"

	"github.com/onsi/gomega"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/test"
)

func _TestMain(m *testing.M) {
	tempDir := test.NewTemporaryDirectoryForTests()
	defer tempDir.Cleanup()

	os.Exit(m.Run())
}

var (
	hConfig = &hashConfig{
		filesystem: fs.NewFilesystem(fs.FilesystemTypeBasic, "."),
		blockSize:  16,
	}
	outbox = make(chan protocol.FileInfo)
	inbox  = make(chan protocol.FileInfo)
	done   = make(chan struct{})
	h      = newParallelHasher(hConfig, 100, outbox, inbox, done)
)

func Test_shouldCallExactNumberOfWorkers(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	countedWaitGroup := &countedWaitGroup{}
	h.wg = countedWaitGroup

	h.run(context.TODO(), &noGlobalFolderScannerLimiter{})

	g.Expect(100, countedWaitGroup.count)
}

func Test_shouldHashTestFile(t *testing.T) {
	tempDir := test.NewTemporaryDirectoryForTests()
	defer tempDir.Cleanup()
	g := gomega.NewGomegaWithT(t)

	hConfig.filesystem = fs.NewFilesystem(fs.FilesystemTypeBasic, ".")

	file, _ := os.Create("test")
	defer file.Close()
	file.WriteString("test")

	inbox = make(chan protocol.FileInfo, 1)
	h := newParallelHasher(hConfig, 100, outbox, inbox, done)

	// action
	inbox <- protocol.FileInfo{Type: protocol.FileInfoTypeFile, Name: file.Name()}
	h.run(context.TODO(), &noGlobalFolderScannerLimiter{})
	finfo := <-outbox

	// verify
	firstBlock := finfo.Blocks[0]
	g.Expect(protocol.BlockInfo{
		Size: int32(4),
		Hash: []uint8{0x9f, 0x86, 0xd0, 0x81, 0x88, 0x4c, 0x7d, 0x65, 0x9a, 0x2f, 0xea, 0xa0, 0xc5, 0x5a, 0xd0, 0x15, 0xa3, 0xbf, 0x4f, 0x1b, 0x2b, 0xb, 0x82, 0x2c, 0xd1, 0x5d, 0x6c, 0x15, 0xb0, 0xf0, 0xa, 0x8},
	}, firstBlock)
}

type countedWaitGroup struct {
	gosync.WaitGroup
	count int
}

func (wg *countedWaitGroup) Add(delta int) {
	wg.WaitGroup.Add(delta)
	wg.count++
}
