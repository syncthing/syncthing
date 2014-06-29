// Copyright (C) 2014 Jakob Borg and other contributors. All rights reserved.
// Use of this source code is governed by an MIT-style license that can be
// found in the LICENSE file.

package model

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/calmh/syncthing/protocol"
	"github.com/calmh/syncthing/scanner"
)

func fileFromFileInfo(f protocol.FileInfo) scanner.File {
	var blocks = make([]scanner.Block, len(f.Blocks))
	var offset int64
	for i, b := range f.Blocks {
		blocks[i] = scanner.Block{
			Offset: offset,
			Size:   b.Size,
			Hash:   b.Hash,
		}
		offset += int64(b.Size)
	}
	return scanner.File{
		// Name is with native separator and normalization
		Name:       filepath.FromSlash(f.Name),
		Size:       offset,
		Flags:      f.Flags &^ protocol.FlagInvalid,
		Modified:   f.Modified,
		Version:    f.Version,
		Blocks:     blocks,
		Suppressed: f.Flags&protocol.FlagInvalid != 0,
	}
}

func fileInfoFromFile(f scanner.File) protocol.FileInfo {
	var blocks = make([]protocol.BlockInfo, len(f.Blocks))
	for i, b := range f.Blocks {
		blocks[i] = protocol.BlockInfo{
			Size: b.Size,
			Hash: b.Hash,
		}
	}
	pf := protocol.FileInfo{
		Name:     filepath.ToSlash(f.Name),
		Flags:    f.Flags,
		Modified: f.Modified,
		Version:  f.Version,
		Blocks:   blocks,
	}
	if f.Suppressed {
		pf.Flags |= protocol.FlagInvalid
	}
	return pf
}

func cmMap(cm protocol.ClusterConfigMessage) map[string]map[protocol.NodeID]uint32 {
	m := make(map[string]map[protocol.NodeID]uint32)
	for _, repo := range cm.Repositories {
		m[repo.ID] = make(map[protocol.NodeID]uint32)
		for _, node := range repo.Nodes {
			var id protocol.NodeID
			copy(id[:], node.ID)
			m[repo.ID][id] = node.Flags
		}
	}
	return m
}

type ClusterConfigMismatch error

// compareClusterConfig returns nil for two equivalent configurations,
// otherwise a descriptive error
func compareClusterConfig(local, remote protocol.ClusterConfigMessage) error {
	lm := cmMap(local)
	rm := cmMap(remote)

	for repo, lnodes := range lm {
		_ = lnodes
		if rnodes, ok := rm[repo]; ok {
			for node, lflags := range lnodes {
				if rflags, ok := rnodes[node]; ok {
					if lflags&protocol.FlagShareBits != rflags&protocol.FlagShareBits {
						return ClusterConfigMismatch(fmt.Errorf("remote has different sharing flags for node %q in repository %q", node, repo))
					}
				}
			}
		}
	}

	return nil
}

func deadlockDetect(mut sync.Locker, timeout time.Duration) {
	go func() {
		for {
			time.Sleep(timeout / 4)
			ok := make(chan bool, 2)

			go func() {
				mut.Lock()
				mut.Unlock()
				ok <- true
			}()

			go func() {
				time.Sleep(timeout)
				ok <- false
			}()

			if r := <-ok; !r {
				panic("deadlock detected")
			}
		}
	}()
}
