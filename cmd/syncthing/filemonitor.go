package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"sync"
	"time"

	"github.com/calmh/syncthing/buffers"
)

type fileMonitor struct {
	name        string // in-repo name
	path        string // full path
	writeDone   sync.WaitGroup
	model       *Model
	global      File
	localBlocks []Block
	copyError   error
	writeError  error
}

func (m *fileMonitor) FileBegins(cc <-chan content) error {
	if m.model.trace["file"] {
		log.Printf("FILE: FileBegins: " + m.name)
	}

	tmp := tempName(m.path, m.global.Modified)

	dir := path.Dir(tmp)
	_, err := os.Stat(dir)
	if err != nil && os.IsNotExist(err) {
		err = os.MkdirAll(dir, 0777)
		if err != nil {
			return err
		}
	}

	outFile, err := os.Create(tmp)
	if err != nil {
		return err
	}

	m.writeDone.Add(1)

	var writeWg sync.WaitGroup
	if len(m.localBlocks) > 0 {
		writeWg.Add(1)
		inFile, err := os.Open(m.path)
		if err != nil {
			return err
		}

		// Copy local blocks, close infile when done
		go m.copyLocalBlocks(inFile, outFile, &writeWg)
	}

	// Write remote blocks,
	writeWg.Add(1)
	go m.copyRemoteBlocks(cc, outFile, &writeWg)

	// Wait for both writing routines, then close the outfile
	go func() {
		writeWg.Wait()
		outFile.Close()
		m.writeDone.Done()
	}()

	return nil
}

func (m *fileMonitor) copyLocalBlocks(inFile, outFile *os.File, writeWg *sync.WaitGroup) {
	defer inFile.Close()
	defer writeWg.Done()

	var buf = buffers.Get(BlockSize)
	defer buffers.Put(buf)

	for _, lb := range m.localBlocks {
		buf = buf[:lb.Size]
		_, err := inFile.ReadAt(buf, lb.Offset)
		if err != nil {
			m.copyError = err
			return
		}
		_, err = outFile.WriteAt(buf, lb.Offset)
		if err != nil {
			m.copyError = err
			return
		}
	}
}

func (m *fileMonitor) copyRemoteBlocks(cc <-chan content, outFile *os.File, writeWg *sync.WaitGroup) {
	defer writeWg.Done()

	for content := range cc {
		_, err := outFile.WriteAt(content.data, content.offset)
		buffers.Put(content.data)
		if err != nil {
			m.writeError = err
			return
		}
	}
}

func (m *fileMonitor) FileDone() error {
	if m.model.trace["file"] {
		log.Printf("FILE: FileDone: " + m.name)
	}

	m.writeDone.Wait()

	tmp := tempName(m.path, m.global.Modified)
	defer os.Remove(tmp)

	if m.copyError != nil {
		return m.copyError
	}
	if m.writeError != nil {
		return m.writeError
	}

	err := hashCheck(tmp, m.global.Blocks)
	if err != nil {
		return err
	}

	err = os.Chtimes(tmp, time.Unix(m.global.Modified, 0), time.Unix(m.global.Modified, 0))
	if err != nil {
		return err
	}

	err = os.Chmod(tmp, os.FileMode(m.global.Flags&0777))
	if err != nil {
		return err
	}

	err = os.Rename(tmp, m.path)
	if err != nil {
		return err
	}

	m.model.updateLocal(m.global)
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
		return errors.New("incorrect number of blocks")
	}
	for i := range current {
		if bytes.Compare(current[i].Hash, correct[i].Hash) != 0 {
			return fmt.Errorf("hash mismatch: %x != %x", current[i], correct[i])
		}
	}

	return nil
}
