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
	"sync/atomic"
	"testing"
	"time"

	"github.com/abiosoft/semaphore"
	"github.com/stretchr/testify/assert"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/test"
)

var (
	event = protocol.FileInfo{Type: protocol.FileInfoTypeFile, Name: "test"}
)

func setup() (context.Context, *ParallelHasher, chan protocol.FileInfo, chan protocol.FileInfo) {
	hConfig := &hashConfig{
		filesystem: fs.NewFilesystem(fs.FilesystemTypeBasic, "."),
		blockSize:  16,
	}
	createTestFile()

	inbox := make(chan protocol.FileInfo)
	outbox := make(chan protocol.FileInfo)
	h := newParallelHasher(hConfig, outbox, inbox, make(chan struct{}))

	ctx, _ := context.WithTimeout(context.Background(), time.Second*1)
	return ctx, h, inbox, outbox
}

func Test_shouldCallExactNumberOfWorkers(t *testing.T) {
	tempDir := test.NewTemporaryDirectoryForTests()
	defer tempDir.Cleanup()
	_, h, _, _ := setup()

	countedWaitGroup := &countedWaitGroup{}
	h.wg = countedWaitGroup

	h.run(context.TODO(), 100, &noopScannerLimiter{})

	assert.Equal(t, 100, countedWaitGroup.count)
}

func Test_shouldHashTestFile(t *testing.T) {
	tempDir := test.NewTemporaryDirectoryForTests()
	defer tempDir.Cleanup()
	_, h, inbox, outbox := setup()

	h.run(context.TODO(), 100, &noopScannerLimiter{})

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
	ctx, h, inbox, outbox := setup()

	limiter := newCountingLimiter(ctx)
	h.run(context.TODO(), 100, limiter)

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
	assert.Equal(t, int32(3), limiter.max)
}

func Test_shouldRunInSequential(t *testing.T) {
	tempDir := test.NewTemporaryDirectoryForTests()
	defer tempDir.Cleanup()
	ctx, h, inbox, outbox := setup()

	limiter := newCountingSingleLimiter(ctx)
	h.run(context.TODO(), 100, limiter)

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
	assert.Equal(t, int32(1), limiter.counter.max)
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
	counter int32
	max     int32
	count   chan int32
}

func newCountingLimiter(ctx context.Context) *countingLimiter {
	l := &countingLimiter{
		count: make(chan int32),
	}

	go func() {
		for {
			select {
			case step := <-l.count:
				counter := atomic.LoadInt32(&l.counter)
				counter += step
				atomic.StoreInt32(&l.counter, counter)

				max := atomic.LoadInt32(&l.max)
				if counter > max {
					atomic.StoreInt32(&l.max, counter)
				}
				fmt.Println("max: ", max)
			case <-ctx.Done():
				return
			}
		}
	}()

	return l
}

func (c *countingLimiter) Aquire(ctx context.Context) {
	c.count <- 1
	// work to have a chance to keep them busy at the same time
	time.Sleep(time.Millisecond * 300)
}

func (c *countingLimiter) Release() {
	c.count <- -1
}

type countingSingleLimiter struct {
	singleGlobalScannerLimiter
	counter *countingLimiter
}

func newCountingSingleLimiter(ctx context.Context) *countingSingleLimiter {
	l := &countingSingleLimiter{
		counter: newCountingLimiter(ctx),
	}

	l.singleGlobalScannerLimiter.sem = semaphore.New(1)
	return l
}

func (c *countingSingleLimiter) Aquire(ctx context.Context) {
	c.singleGlobalScannerLimiter.Aquire(ctx)
	c.counter.Aquire(ctx)
}

func (c *countingSingleLimiter) Release() {
	c.singleGlobalScannerLimiter.Release()
	c.counter.Release()
}
