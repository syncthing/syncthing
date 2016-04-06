// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package changeset

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
)

type hashedBlock struct {
	data []byte
	hash []byte
}

const numTestBlocks = 8

var (
	testBlocks  [numTestBlocks]hashedBlock
	testFile    protocol.FileInfo
	testFile2   protocol.FileInfo
	testSymlink protocol.FileInfo
	testDir     protocol.FileInfo
)

func init() {
	// Create a number of data blocks available for testing.
	for i := 0; i < numTestBlocks-1; i++ {
		buf := new(bytes.Buffer)
		lr := io.LimitReader(rand.Reader, protocol.BlockSize)
		io.Copy(buf, lr)
		hash := sha256.Sum256(buf.Bytes())
		testBlocks[i] = hashedBlock{
			data: buf.Bytes(),
			hash: hash[:],
		}
	}

	// The test file consists of blocks 1, 2 and 3
	testFile = protocol.FileInfo{
		Name:  "test",
		Flags: 0666,
		Blocks: []protocol.BlockInfo{
			protocol.BlockInfo{
				Offset: 0 * protocol.BlockSize,
				Size:   protocol.BlockSize,
				Hash:   testBlocks[1].hash,
			},
			protocol.BlockInfo{
				Offset: 1 * protocol.BlockSize,
				Size:   protocol.BlockSize,
				Hash:   testBlocks[2].hash,
			},
			protocol.BlockInfo{
				Offset: 2 * protocol.BlockSize,
				Size:   protocol.BlockSize,
				Hash:   testBlocks[3].hash,
			},
		},
	}

	// The other test file consists of blocks 3, 4 and 5
	testFile2 = protocol.FileInfo{
		Name:  "test2",
		Flags: 0666,
		Blocks: []protocol.BlockInfo{
			protocol.BlockInfo{
				Offset: 0 * protocol.BlockSize,
				Size:   protocol.BlockSize,
				Hash:   testBlocks[3].hash,
			},
			protocol.BlockInfo{
				Offset: 1 * protocol.BlockSize,
				Size:   protocol.BlockSize,
				Hash:   testBlocks[4].hash,
			},
			protocol.BlockInfo{
				Offset: 2 * protocol.BlockSize,
				Size:   protocol.BlockSize,
				Hash:   testBlocks[5].hash,
			},
		},
	}

	symlinkTarget := []byte("target/of/symlink")
	symlinkHash := sha256.Sum256(symlinkTarget)
	testBlocks[numTestBlocks-1] = hashedBlock{
		data: symlinkTarget,
		hash: symlinkHash[:],
	}
	testSymlink = protocol.FileInfo{
		Name:  "symlink",
		Flags: 0666 | protocol.FlagSymlink,
		Blocks: []protocol.BlockInfo{
			protocol.BlockInfo{
				Size: int32(len(symlinkTarget)),
				Hash: symlinkHash[:],
			},
		},
	}

	testDir = protocol.FileInfo{
		Name:  "dir",
		Flags: 0777 | protocol.FlagDirectory,
	}
}

type fakeRequester []hashedBlock

func (s fakeRequester) Request(name string, offset int64, hash []byte, buf []byte) error {
	for _, b := range s {
		if bytes.Equal(b.hash, hash) {
			copy(buf, b.data)
			return nil
		}
	}

	return fmt.Errorf("No such block %x", hash)
}

type slowRequester []hashedBlock

func (s slowRequester) Request(name string, offset int64, hash []byte, buf []byte) error {
	time.Sleep(500 * time.Millisecond)

	for _, b := range s {
		if bytes.Equal(b.hash, hash) {
			copy(buf, b.data)
			return nil
		}
	}

	return fmt.Errorf("No such block %x", hash)
}

type errorRequester struct {
	t *testing.T
}

func (s errorRequester) Request(name string, offset int64, hash []byte, buf []byte) error {
	s.t.Error("Called with name", name, "and offset", offset)
	return fmt.Errorf("no such block")
}

type tempNamer struct {
	prefix string
}

var defTempNamer = tempNamer{".syncthing."}

func (t tempNamer) IsTemporary(name string) bool {
	return strings.HasPrefix(filepath.Base(name), t.prefix)
}

func (t tempNamer) TempName(name string) string {
	tdir := filepath.Dir(name)
	tbase := filepath.Base(name)
	if len(tbase) > 240 {
		hash := md5.New()
		hash.Write([]byte(name))
		tbase = fmt.Sprintf("%x", hash.Sum(nil))
	}
	tname := fmt.Sprintf("%s%s.tmp", t.prefix, tbase)
	return filepath.Join(tdir, tname)
}

type countingProgresser struct {
	started    int
	completed  int
	copied     int
	requested  int
	downloaded int
	mut        sync.Mutex
}

func (c *countingProgresser) Started(f protocol.FileInfo) {
	c.mut.Lock()
	c.started++
	c.mut.Unlock()
}

func (c *countingProgresser) Progress(f protocol.FileInfo, copied, requested, downloaded int) {
	c.mut.Lock()
	c.copied += copied
	c.requested += requested
	c.downloaded += downloaded
	c.mut.Unlock()
}

func (c *countingProgresser) Completed(_ protocol.FileInfo, _ error) {
	c.mut.Lock()
	c.completed++
	c.mut.Unlock()
}
