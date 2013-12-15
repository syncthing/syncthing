package main

/*

Locking
=======

These methods are never called from the outside so don't follow the locking
policy in model.go. Instead, appropriate locks are acquired when needed and
held for as short a time as possible.

TODO(jb): Refactor this into smaller and cleaner pieces.

TODO(jb): Some kind of coalescing / rate limiting of index sending, so we don't
send hundreds of index updates in a short period if time when deleting files
etc.

*/

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"sync"
	"time"

	"github.com/calmh/syncthing/buffers"
)

func (m *Model) pullFile(name string) error {
	m.RLock()
	var localFile = m.local[name]
	var globalFile = m.global[name]
	m.RUnlock()

	filename := path.Join(m.dir, name)
	sdir := path.Dir(filename)

	_, err := os.Stat(sdir)
	if err != nil && os.IsNotExist(err) {
		os.MkdirAll(sdir, 0777)
	}

	tmpFilename := tempName(filename, globalFile.Modified)
	tmpFile, err := os.Create(tmpFilename)
	if err != nil {
		return err
	}
	defer tmpFile.Close()

	contentChan := make(chan content, 32)
	var applyDone sync.WaitGroup
	applyDone.Add(1)
	go func() {
		applyContent(contentChan, tmpFile)
		applyDone.Done()
	}()

	local, remote := localFile.Blocks.To(globalFile.Blocks)
	var fetchDone sync.WaitGroup

	// One local copy routing

	fetchDone.Add(1)
	go func() {
		for _, block := range local {
			data, err := m.Request("<local>", name, block.Offset, block.Length, block.Hash)
			if err != nil {
				break
			}
			contentChan <- content{
				offset: int64(block.Offset),
				data:   data,
			}
		}
		fetchDone.Done()
	}()

	// N remote copy routines

	m.RLock()
	var nodeIDs = m.whoHas(name)
	m.RUnlock()
	var remoteBlocksChan = make(chan Block)
	go func() {
		for _, block := range remote {
			remoteBlocksChan <- block
		}
		close(remoteBlocksChan)
	}()

	// XXX: This should be rewritten into something nicer that takes differing
	// peer performance into account.

	for i := 0; i < RemoteFetchers; i++ {
		for _, nodeID := range nodeIDs {
			fetchDone.Add(1)
			go func(nodeID string) {
				for block := range remoteBlocksChan {
					data, err := m.RequestGlobal(nodeID, name, block.Offset, block.Length, block.Hash)
					if err != nil {
						break
					}
					contentChan <- content{
						offset: int64(block.Offset),
						data:   data,
					}
				}
				fetchDone.Done()
			}(nodeID)
		}
	}

	fetchDone.Wait()
	close(contentChan)
	applyDone.Wait()

	rf, err := os.Open(tmpFilename)
	if err != nil {
		return err
	}
	defer rf.Close()

	writtenBlocks, err := Blocks(rf, BlockSize)
	if err != nil {
		return err
	}
	if len(writtenBlocks) != len(globalFile.Blocks) {
		return fmt.Errorf("%s: incorrect number of blocks after sync", tmpFilename)
	}
	for i := range writtenBlocks {
		if bytes.Compare(writtenBlocks[i].Hash, globalFile.Blocks[i].Hash) != 0 {
			return fmt.Errorf("%s: hash mismatch after sync\n  %v\n  %v", tmpFilename, writtenBlocks[i], globalFile.Blocks[i])
		}
	}

	err = os.Chtimes(tmpFilename, time.Unix(globalFile.Modified, 0), time.Unix(globalFile.Modified, 0))
	if err != nil {
		return err
	}

	err = os.Rename(tmpFilename, filename)
	if err != nil {
		return err
	}

	return nil
}

func (m *Model) puller() {
	for {
		for {
			var n string
			var f File

			m.RLock()
			for n = range m.need {
				break // just pick first name
			}
			if len(n) != 0 {
				f = m.global[n]
			}
			m.RUnlock()

			if len(n) == 0 {
				// we got nothing
				break
			}

			var err error
			if f.Flags&FlagDeleted == 0 {
				if traceFile {
					debugf("FILE: Pull %q", n)
				}
				err = m.pullFile(n)
			} else {
				if traceFile {
					debugf("FILE: Remove %q", n)
				}
				// Cheerfully ignore errors here
				_ = os.Remove(path.Join(m.dir, n))
			}
			if err == nil {
				m.UpdateLocal(f)
			} else {
				warnln(err)
			}
		}
		time.Sleep(time.Second)
	}
}

type content struct {
	offset int64
	data   []byte
}

func applyContent(cc <-chan content, dst io.WriterAt) error {
	var err error

	for c := range cc {
		_, err = dst.WriteAt(c.data, c.offset)
		if err != nil {
			return err
		}
		buffers.Put(c.data)
	}

	return nil
}
