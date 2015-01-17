// Copyright (C) 2014 The Syncthing Authors.
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

package model

import (
	"path/filepath"
	"sync"

	"github.com/syncthing/protocol"
)

type deviceFilePair struct {
	device protocol.DeviceID
	file   string
}

// Helper struct which helps managing temporary indexes
type tempIndex struct {
	// List of hashes a device currently holds
	deviceHashes map[protocol.DeviceID][]string
	// For a given hash, a list of devices and folder+file pairs which have this block.
	blockPairs map[string][]deviceFilePair
	mut        sync.RWMutex
}

func newTempIndex() *tempIndex {
	return &tempIndex{
		deviceHashes: make(map[protocol.DeviceID][]string),
		blockPairs:   make(map[string][]deviceFilePair),
	}
}

// Update temporary index for a given device and given folder
// Replaces the existing temporary index.
func (i *tempIndex) Update(device protocol.DeviceID, folder string, files []protocol.FileInfo) {
	i.mut.Lock()

	// Cleanup existing pairs
	if existingHash, ok := i.deviceHashes[device]; ok {
		for _, hash := range existingHash {
			candidates := i.blockPairs[hash]
			count := len(candidates)
			n := 0
		loop:
			for n < count {
				if candidates[n].device == device {
					candidates[n] = candidates[count-1]
					count--
					continue loop
				}
				n++
			}
			i.blockPairs[hash] = candidates[0:count]
		}
	}

	// Insert new pairs
	i.deviceHashes[device] = make([]string, 0)

	for _, file := range files {
		fullpath := filepath.Join(folder, file.Name)
		for _, block := range file.Blocks {
			hash := string(block.Hash)
			i.deviceHashes[device] = append(i.deviceHashes[device], hash)

			newPair := deviceFilePair{
				device: device,
				file:   fullpath,
			}

			if _, ok := i.blockPairs[hash]; ok {
				i.blockPairs[hash] = append(i.blockPairs[hash], newPair)
			} else {
				i.blockPairs[hash] = []deviceFilePair{newPair}
			}
		}
	}

	i.mut.Unlock()
}

// Looks up which devices have the given block in the given file.
func (i *tempIndex) Lookup(folder, file string, hash []byte) []protocol.DeviceID {
	i.mut.RLock()
	defer i.mut.RUnlock()

	if hash == nil {
		return nil
	}

	candidates, ok := i.blockPairs[string(hash)]
	if !ok {
		return nil
	}

	devices := make([]protocol.DeviceID, 0, len(i.deviceHashes))
	fullpath := filepath.Join(folder, file)

	for _, candidate := range candidates {
		if candidate.file == fullpath {
			devices = append(devices, candidate.device)
		}
	}

	return devices
}
