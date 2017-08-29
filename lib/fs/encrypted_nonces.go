// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"bytes"
	"crypto/rand"
	"encoding/gob"
	"io"
	"sync"

	"github.com/syncthing/syncthing/lib/protocol"
)

type nonceManager interface {
	getNameNonces(string) []byte
	setNameNonce(string, []byte)
	getContentNonceStorage(string) *nonceStorage
	discardContentNonces(string)
	populate() error
	flush()
}

func newNonceManager(fs Filesystem, name string) (nonceManager, error) {
	mgr := &fileNonceManager{
		name:          name,
		fs:            fs,
		nameNonces:    make(map[string][]byte),
		contentNonces: make(map[string]*nonceStorage),
	}

	return mgr, mgr.populate()
}

type fileNonceManager struct {
	name          string
	exists        bool
	fs            Filesystem
	nameNonces    map[string][]byte
	nameMut       sync.Mutex
	contentNonces map[string]*nonceStorage
	contentMut    sync.Mutex
}

func (m *fileNonceManager) getNameNonces(name string) []byte {
	m.nameMut.Lock()
	var created bool
	nonce, ok := m.nameNonces[name]
	if !ok {
		nonce = make([]byte, aesBlockSize)
		if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
			panic(err.Error())
		}
		m.nameNonces[name] = nonce
		created = true
	}
	m.nameMut.Unlock()
	if created {
		m.flush()
	}
	return nonce
}

func (m *fileNonceManager) setNameNonce(name string, nonce []byte) {
	m.nameMut.Lock()
	var created bool
	if existing, ok := m.nameNonces[name]; !ok || !bytes.Equal(existing, nonce) {
		m.nameNonces[name] = nonce
		created = true
	}
	m.nameMut.Unlock()
	if created {
		m.flush()
	}
}

func (m *fileNonceManager) getContentNonceStorage(name string) *nonceStorage {
	m.contentMut.Lock()
	storage, ok := m.contentNonces[name]
	if !ok {
		storage = &nonceStorage{
			manager: m,
		}
		m.contentNonces[name] = storage
	}
	m.contentMut.Unlock()
	return storage
}

func (m *fileNonceManager) populate() error {
	fd, err := m.fs.Open(m.name)
	if err != nil {
		// Technically this is not ok, as we could create the file for the user
		// which is what we don't want, but we track exists value in flush, and
		// only start flushing when the file actually exists
		if IsNotExist(err) {
			return nil
		}
		return err
	}
	m.exists = true

	decoder := gob.NewDecoder(fd)

	contentGob := make(map[string][][]byte)
	contentNonces := make(map[string]*nonceStorage)
	err = decoder.Decode(&contentGob)
	for name, nonces := range contentGob {
		contentNonces[name] = &nonceStorage{
			nonces:  nonces,
			manager: m,
		}
	}

	nameNonces := make(map[string][]byte)
	if err == nil {
		err = decoder.Decode(&nameNonces)
	}

	fd.Close()

	// If we had no errors, load both.
	if err == nil {
		// Technically don't need to lock as population happens only on instantiation.
		m.contentMut.Lock()
		m.contentNonces = contentNonces
		m.contentMut.Unlock()
		m.nameMut.Lock()
		m.nameNonces = nameNonces
		m.nameMut.Unlock()
	}

	// Empty file is fine, perhaps it's the first time we are initializing.
	if err == io.EOF {
		return nil
	}

	return err
}

func (m *fileNonceManager) flush() {
	// TODO
	// Will this cause between folder restarts, as in, some operation has a ref
	// to the old filesystem that is getting writes and the new one is getting
	// writes too?

	// If the file did not exist before hand, do not create it for the user
	// wait for it to appear in natural means.
	if !m.exists {
		_, err := m.fs.Lstat(m.name)
		if err == nil {
			m.exists = true
		} else {
			l.Debugf("nonce manager for %s failed to flush: %s", m.fs.URI(), err.Error())
			return
		}
	}
	m.contentMut.Lock()
	contentGob := make(map[string][][]byte)
	for file, nonces := range m.contentNonces {
		nonces.mut.Lock()
		buf := make([][]byte, len(nonces.nonces))
		for i, nonce := range nonces.nonces {
			buf[i] = make([]byte, len(nonce))
			copy(buf[i], nonce)
		}
		contentGob[file] = buf
		nonces.mut.Unlock()
	}
	m.contentMut.Unlock()

	m.nameMut.Lock()
	nameGob := make(map[string][]byte)
	for name, nonce := range m.nameNonces {
		buf := make([]byte, len(nonce))
		copy(buf, nonce)
		nameGob[name] = buf
	}
	m.nameMut.Unlock()

	// TODO
	// This needs to be hidden from walks, opens, scans etc.
	m.fs.Remove(m.name + ".tmp")
	fd, err := m.fs.Create(m.name + ".tmp")
	if err != nil {
		panic(err)
	}

	encoder := gob.NewEncoder(fd)

	if err := encoder.Encode(contentGob); err != nil {
		panic(err)
	}
	if err := encoder.Encode(nameGob); err != nil {
		panic(err)
	}
	if err := fd.Sync(); err != nil {
		panic(err)
	}
	if err := fd.Close(); err != nil {
		panic(err)
	}
	if err := m.fs.Rename(m.name+".tmp", m.name); err != nil {
		panic(err)
	}
}

func (m *fileNonceManager) discardContentNonces(name string) {
	var removed bool
	m.contentMut.Lock()
	if _, ok := m.contentNonces[name]; ok {
		delete(m.contentNonces, name)
		removed = true
	}
	m.contentMut.Unlock()

	if removed {
		m.flush()
	}
}

type nonceStorage struct {
	nonces  [][]byte
	mut     sync.Mutex
	manager *fileNonceManager
}

func (s *nonceStorage) set(block int, nonce []byte) {
	s.mut.Lock()
	if len(s.nonces) < (block + 1) {
		s.grow(block + 1)
	}

	var changed bool
	if !bytes.Equal(s.nonces[block], nonce) {
		s.nonces[block] = nonce
		changed = true
	}
	s.mut.Unlock()
	if changed {
		s.manager.flush()
	}
}

func (s *nonceStorage) grow(block int) {
	// Always under lock
	// ref: https://github.com/go-sql-driver/mysql/pull/55/files#diff-c5f7bf6980b6b3b699ddc715cd7e7f7dR61
	if block > 2*cap(s.nonces) {
		newNonces := make([][]byte, block)
		copy(newNonces, s.nonces)
		s.nonces = newNonces
		return
	}
	for cap(s.nonces) < block {
		s.nonces = append(s.nonces[:cap(s.nonces)], nil)
	}
	s.nonces = s.nonces[:cap(s.nonces)]
}

func (s *nonceStorage) get(block int) []byte {
	s.mut.Lock()
	if len(s.nonces) < (block + 1) {
		s.grow(block + 1)
	}
	nonce := s.nonces[block]
	var created bool
	if nonce == nil {
		nonce = make([]byte, nonceSize)
		if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
			panic(err.Error())
		}
		s.nonces[block] = nonce
		created = true
	}
	s.mut.Unlock()
	if created {
		s.manager.flush()
	}
	return nonce
}

func (s *nonceStorage) reset(size int64) {
	blocks := size / protocol.BlockSize
	if size%protocol.BlockSize != 0 {
		blocks++
	}
	s.mut.Lock()
	s.nonces = make([][]byte, blocks)
	s.mut.Unlock()
	s.manager.flush()
}
