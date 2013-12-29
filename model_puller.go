package main

/*

Locking
=======

These methods are never called from the outside so don't follow the locking
policy in model.go.

TODO(jb): Refactor this into smaller and cleaner pieces.
TODO(jb): Increase performance by taking apparent peer bandwidth into account.

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

	// One local copy routine

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
	var remoteBlocks = blockIterator{blocks: remote}
	for i := 0; i < opts.Advanced.RequestsInFlight; i++ {
		curNode := nodeIDs[i%len(nodeIDs)]
		fetchDone.Add(1)

		go func(nodeID string) {
			for {
				block, ok := remoteBlocks.Next()
				if !ok {
					break
				}
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
		}(curNode)
	}

	fetchDone.Wait()
	close(contentChan)
	applyDone.Wait()

	err = hashCheck(tmpFilename, globalFile.Blocks)
	if err != nil {
		return err
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
		time.Sleep(time.Second)

		var ns []string
		m.RLock()
		for n := range m.need {
			ns = append(ns, n)
		}
		m.RUnlock()

		if len(ns) == 0 {
			continue
		}

		var limiter = make(chan bool, opts.Advanced.FilesInFlight)

		for _, n := range ns {
			limiter <- true

			f, ok := m.GlobalFile(n)
			if !ok {
				continue
			}

			var err error
			if f.Flags&FlagDeleted == 0 {
				if opts.Debug.TraceFile {
					debugf("FILE: Pull %q", n)
				}
				err = m.pullFile(n)
			} else {
				if opts.Debug.TraceFile {
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

			<-limiter
		}
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

func hashCheck(name string, correct []Block) error {
	rf, err := os.Open(name)
	if err != nil {
		return err
	}
	defer rf.Close()

	current, err := Blocks(rf, BlockSize)
	if err != nil {
		return err
	}
	if len(current) != len(correct) {
		return fmt.Errorf("%s: incorrect number of blocks after sync", name)
	}
	for i := range current {
		if bytes.Compare(current[i].Hash, correct[i].Hash) != 0 {
			return fmt.Errorf("%s: hash mismatch after sync\n  %v\n  %v", name, current[i], correct[i])
		}
	}

	return nil
}

type blockIterator struct {
	sync.Mutex
	blocks []Block
}

func (i *blockIterator) Next() (b Block, ok bool) {
	i.Lock()
	defer i.Unlock()

	if len(i.blocks) == 0 {
		return
	}

	b, i.blocks = i.blocks[0], i.blocks[1:]
	ok = true

	return
}
