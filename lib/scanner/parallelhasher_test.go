// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package scanner

import (
	"context"
	"fmt"
	"os"
	gosync "sync"
	"testing"
	"time"

	"github.com/abiosoft/semaphore"
	"github.com/cznic/mathutil"
	"github.com/stretchr/testify/assert"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/test"
)

var (
	hConfig = &hashConfig{
		filesystem: fs.NewFilesystem(fs.FilesystemTypeBasic, "."),
		blockSize:  16,
	}
	outbox = make(chan protocol.FileInfo)
	inbox  = make(chan protocol.FileInfo)
	done   = make(chan struct{})
	h      = newParallelHasher(hConfig, 100, outbox, inbox, done)

	event = protocol.FileInfo{Type: protocol.FileInfoTypeFile, Name: "test"}
)

func setup() context.Context {

	hConfig.filesystem = fs.NewFilesystem(fs.FilesystemTypeBasic, ".")
	createTestFile()

	outbox = make(chan protocol.FileInfo)
	inbox = make(chan protocol.FileInfo)
	h = newParallelHasher(hConfig, 100, outbox, inbox, done)

	ctx, _ := context.WithTimeout(context.Background(), time.Second*1)
	return ctx
}

func Test_shouldCallExactNumberOfWorkers(t *testing.T) {
	countedWaitGroup := &countedWaitGroup{}
	h.wg = countedWaitGroup

	h.run(context.TODO(), &noGlobalFolderScannerLimiter{})

	assert.Equal(t, 100, countedWaitGroup.count)
}

func Test_shouldHashTestFile(t *testing.T) {
	tempDir := test.NewTemporaryDirectoryForTests()
	defer tempDir.Cleanup()

	setup()

	h.run(context.TODO(), &noGlobalFolderScannerLimiter{})

	inbox <- event
	finfo := <-outbox

	// verify
	firstBlock := finfo.Blocks[0]
	assert.Equal(t, protocol.BlockInfo{
		Size: int32(4),
		Hash: []uint8{0x9f, 0x86, 0xd0, 0x81, 0x88, 0x4c, 0x7d, 0x65, 0x9a, 0x2f, 0xea, 0xa0, 0xc5, 0x5a, 0xd0, 0x15, 0xa3, 0xbf, 0x4f, 0x1b, 0x2b, 0xb, 0x82, 0x2c, 0xd1, 0x5d, 0x6c, 0x15, 0xb0, 0xf0, 0xa, 0x8},
	}, firstBlock)
}

func Test_shouldRunInParallel(t *testing.T) {
	tempDir := test.NewTemporaryDirectoryForTests()
	defer tempDir.Cleanup()

	ctx := setup()

	limiter := newCountingLimiter(ctx)
	h.run(context.TODO(), limiter)

	go func() {
		inbox <- event
		inbox <- event
		inbox <- event
		for {
			select {
			case <-outbox:
			case <-ctx.Done():
				return
			}
		}
	}()

	<-ctx.Done()
	assert.Equal(t, 3, limiter.max)
}

func Test_shouldRunInSequential(t *testing.T) {
	tempDir := test.NewTemporaryDirectoryForTests()
	defer tempDir.Cleanup()

	ctx := setup()

	limiter := newCountingSingleLimiter(ctx)
	h.run(context.TODO(), limiter)

	go func() {
		inbox <- event
		inbox <- event
		inbox <- event
		for {
			select {
			case <-outbox:
			case <-ctx.Done():
				return
			}
		}
	}()

	<-ctx.Done()
	assert.Equal(t, 1, limiter.counter.max)
}
func createTestFile() {
	file, _ := os.Create("test")
	defer file.Close()
	file.WriteString("test")
}

type countedWaitGroup struct {
	gosync.WaitGroup
	count int
}

func (wg *countedWaitGroup) Add(delta int) {
	wg.WaitGroup.Add(delta)
	wg.count++
}

type countingLimiter struct {
	counter int
	max     int
	count   chan int
}

func newCountingLimiter(ctx context.Context) *countingLimiter {
	limiter := &countingLimiter{
		count: make(chan int),
	}

	go func() {
		for {
			select {
			case step := <-limiter.count:
				limiter.counter += step
				limiter.max = mathutil.Max(limiter.max, limiter.counter)
				fmt.Println("max: ", limiter.max)
			case <-ctx.Done():
				return
			}
		}
	}()

	return limiter
}

func (c *countingLimiter) Aquire() {
	time.Sleep(time.Millisecond * 300)
	c.count <- 1
}

func (c *countingLimiter) Release() {
	c.count <- -1
}

type countingSingleLimiter struct {
	singleGlobalFolderScannerLimiter
	counter *countingLimiter
}

func newCountingSingleLimiter(ctx context.Context) *countingSingleLimiter {
	l := &countingSingleLimiter{
		counter: newCountingLimiter(ctx),
	}

	l.singleGlobalFolderScannerLimiter.sem = semaphore.New(1)
	l.singleGlobalFolderScannerLimiter.ctx = ctx
	return l
}

func (c *countingSingleLimiter) Aquire() {
	c.singleGlobalFolderScannerLimiter.Aquire()
	c.counter.Aquire()
}

func (c *countingSingleLimiter) Release() {
	c.singleGlobalFolderScannerLimiter.Release()
	c.counter.Release()
}
