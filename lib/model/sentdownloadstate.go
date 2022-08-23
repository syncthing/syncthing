// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
)

// sentFolderFileDownloadState represents a state of what we've announced as available
// to some remote device for a specific file.
type sentFolderFileDownloadState struct {
	blockIndexes []int
	version      protocol.Vector
	updated      time.Time
	created      time.Time
	blockSize    int
}

// sentFolderDownloadState represents a state of what we've announced as available
// to some remote device for a specific folder.
type sentFolderDownloadState struct {
	files map[string]*sentFolderFileDownloadState
}

// update takes a set of currently active sharedPullerStates, and returns a list
// of updates which we need to send to the client to become up to date.
func (s *sentFolderDownloadState) update(pullers []*sharedPullerState) []protocol.FileDownloadProgressUpdate {
	var name string
	var updates []protocol.FileDownloadProgressUpdate
	seen := make(map[string]struct{}, len(pullers))

	for _, puller := range pullers {
		name = puller.file.Name

		seen[name] = struct{}{}

		pullerBlockIndexes := puller.Available()
		pullerVersion := puller.file.Version
		pullerBlockIndexesUpdated := puller.AvailableUpdated()
		pullerCreated := puller.created
		pullerBlockSize := puller.file.BlockSize()

		localFile, ok := s.files[name]

		// New file we haven't seen before
		if !ok {
			// Only send an update if the file actually has some blocks.
			if len(pullerBlockIndexes) > 0 {
				s.files[name] = &sentFolderFileDownloadState{
					blockIndexes: pullerBlockIndexes,
					updated:      pullerBlockIndexesUpdated,
					version:      pullerVersion,
					created:      pullerCreated,
					blockSize:    pullerBlockSize,
				}

				updates = append(updates, protocol.FileDownloadProgressUpdate{
					Name:         name,
					Version:      pullerVersion,
					UpdateType:   protocol.FileDownloadProgressUpdateTypeAppend,
					BlockIndexes: pullerBlockIndexes,
					BlockSize:    pullerBlockSize,
				})
			}
			continue
		}

		// Existing file we've already sent an update for.
		if pullerBlockIndexesUpdated.Equal(localFile.updated) && pullerVersion.Equal(localFile.version) {
			// The file state hasn't changed, go to next.
			continue
		}

		if !pullerVersion.Equal(localFile.version) || !pullerCreated.Equal(localFile.created) {
			// The version has changed or the puller was reconstructed due to failure.
			// Clean up whatever we had for the old file, and advertise the new file.
			updates = append(updates,
				protocol.FileDownloadProgressUpdate{
					Name:       name,
					Version:    localFile.version,
					UpdateType: protocol.FileDownloadProgressUpdateTypeForget,
				},
				protocol.FileDownloadProgressUpdate{
					Name:         name,
					Version:      pullerVersion,
					UpdateType:   protocol.FileDownloadProgressUpdateTypeAppend,
					BlockIndexes: pullerBlockIndexes,
					BlockSize:    pullerBlockSize,
				})
			localFile.blockIndexes = pullerBlockIndexes
			localFile.updated = pullerBlockIndexesUpdated
			localFile.version = pullerVersion
			localFile.created = pullerCreated
			localFile.blockSize = int(pullerBlockSize)
			continue
		}

		// Relies on the fact that sharedPullerState.Available() should always
		// append.
		newBlocks := pullerBlockIndexes[len(localFile.blockIndexes):]

		localFile.blockIndexes = append(localFile.blockIndexes, newBlocks...)
		localFile.updated = pullerBlockIndexesUpdated

		// If there are new blocks, send the update.
		if len(newBlocks) > 0 {
			updates = append(updates, protocol.FileDownloadProgressUpdate{
				Name:         name,
				Version:      localFile.version,
				UpdateType:   protocol.FileDownloadProgressUpdateTypeAppend,
				BlockIndexes: newBlocks,
				BlockSize:    pullerBlockSize,
			})
		}
	}

	// For each file that we are tracking, see if there still is a puller for it
	// if not, the file completed or errored out.
	for name, info := range s.files {
		_, ok := seen[name]
		if !ok {
			updates = append(updates, protocol.FileDownloadProgressUpdate{
				Name:       name,
				Version:    info.version,
				UpdateType: protocol.FileDownloadProgressUpdateTypeForget,
			})
			delete(s.files, name)
		}
	}

	return updates
}

// destroy removes all stored state, and returns a set of updates we need to
// dispatch to clean up the state on the remote end.
func (s *sentFolderDownloadState) destroy() []protocol.FileDownloadProgressUpdate {
	updates := make([]protocol.FileDownloadProgressUpdate, 0, len(s.files))
	for name, info := range s.files {
		updates = append(updates, protocol.FileDownloadProgressUpdate{
			Name:       name,
			Version:    info.version,
			UpdateType: protocol.FileDownloadProgressUpdateTypeForget,
		})
		delete(s.files, name)
	}
	return updates
}

// sentDownloadState represents a state of what we've announced as available
// to some remote device. It is used from within the progress emitter
// which only has one routine, hence is deemed threadsafe.
type sentDownloadState struct {
	folderStates map[string]*sentFolderDownloadState
}

// update receives a folder, and a slice of pullers that are currently available
// for the given folder, and according to the state of what we've seen before
// returns a set of updates which we should send to the remote device to make
// it aware of everything that we currently have available.
func (s *sentDownloadState) update(folder string, pullers []*sharedPullerState) []protocol.FileDownloadProgressUpdate {
	fs, ok := s.folderStates[folder]
	if !ok {
		fs = &sentFolderDownloadState{
			files: make(map[string]*sentFolderFileDownloadState),
		}
		s.folderStates[folder] = fs
	}
	return fs.update(pullers)
}

// folders returns a set of folders this state is currently aware off.
func (s *sentDownloadState) folders() []string {
	folders := make([]string, 0, len(s.folderStates))
	for key := range s.folderStates {
		folders = append(folders, key)
	}
	return folders
}

// cleanup cleans up all state related to a folder, and returns a set of updates
// which would clean up the state on the remote device.
func (s *sentDownloadState) cleanup(folder string) []protocol.FileDownloadProgressUpdate {
	fs, ok := s.folderStates[folder]
	if ok {
		updates := fs.destroy()
		delete(s.folderStates, folder)
		return updates
	}
	return nil
}
