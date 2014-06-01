// Copyright (C) 2014 Jakob Borg and other contributors. All rights reserved.
// Use of this source code is governed by an MIT-style license that can be
// found in the LICENSE file.

package model

import (
	"fmt"
	"path/filepath"

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

func cmMap(cm protocol.ClusterConfigMessage) map[string]map[string]uint32 {
	m := make(map[string]map[string]uint32)
	for _, repo := range cm.Repositories {
		m[repo.ID] = make(map[string]uint32)
		for _, node := range repo.Nodes {
			m[repo.ID][node.ID] = node.Flags
		}
	}
	return m
}

type ClusterConfigMismatch error

// compareClusterConfig returns nil for two equivalent configurations,
// otherwise a decriptive error
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
