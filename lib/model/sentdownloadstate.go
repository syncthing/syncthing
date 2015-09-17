// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
)

// sentFolderDownloadState represents a state of what we've announced as available
// to some remote device for a specific folder.
type sentFolderDownloadState struct {
	fileBlockIndexes map[string][]int32
	fileVersion      map[string]protocol.Vector
	fileUpdated      map[string]time.Time
}

// update takes a set of currently active sharedPullerStates, and returns a list
// of updates which we need to send to the client to become up to date.
// Any pullers that
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

		localBlockIndexes := s.fileBlockIndexes[name]
		localVersion := s.fileVersion[name]
		localBlockIndexesUpdated, ok := s.fileUpdated[name]

		// New file we haven't seen before
		if !ok {
			s.fileBlockIndexes[name] = pullerBlockIndexes
			s.fileUpdated[name] = pullerBlockIndexesUpdated
			s.fileVersion[name] = pullerVersion

			updates = append(updates, protocol.FileDownloadProgressUpdate{
				Name:         name,
				Version:      pullerVersion,
				UpdateType:   protocol.UpdateTypeAppend,
				BlockIndexes: pullerBlockIndexes,
			})
			continue
		}

		// Existing file we've already sent an update for.
		if pullerBlockIndexesUpdated.Equal(localBlockIndexesUpdated) && pullerVersion.Equal(localVersion) {
			// The file state hasn't changed, go to next.
			continue
		}

		if !pullerVersion.Equal(localVersion) {
			// The version has changed, clean up whatever we had for the old
			// file, and advertise the new file.
			updates = append(updates, protocol.FileDownloadProgressUpdate{
				Name:       name,
				Version:    localVersion,
				UpdateType: protocol.UpdateTypeForget,
			})
			updates = append(updates, protocol.FileDownloadProgressUpdate{
				Name:         name,
				Version:      pullerVersion,
				UpdateType:   protocol.UpdateTypeAppend,
				BlockIndexes: pullerBlockIndexes,
			})
			continue
		}

		// Relies on the fact that sharedPullerState.Available() should always
		// append.
		newBlocks := pullerBlockIndexes[len(localBlockIndexes):]

		s.fileBlockIndexes[name] = append(localBlockIndexes, newBlocks...)
		s.fileUpdated[name] = pullerBlockIndexesUpdated

		updates = append(updates, protocol.FileDownloadProgressUpdate{
			Name:         name,
			Version:      localVersion,
			UpdateType:   protocol.UpdateTypeAppend,
			BlockIndexes: newBlocks,
		})
	}

	// For each file that we are tracking, see if there still is a puller for it
	// if not, the file completed or errored out.
	for name := range s.fileBlockIndexes {
		_, ok := seen[name]
		if !ok {
			updates = append(updates, protocol.FileDownloadProgressUpdate{
				Name:       name,
				Version:    s.fileVersion[name],
				UpdateType: protocol.UpdateTypeForget,
			})
		}
	}

	return updates
}

// destroy removes all stored state, and returns a set of updates we need to
// dispatch to clean up the state on the remote end.
func (s *sentFolderDownloadState) destroy() []protocol.FileDownloadProgressUpdate {
	updates := make([]protocol.FileDownloadProgressUpdate, 0, len(s.fileBlockIndexes))
	for name := range s.fileBlockIndexes {
		updates = append(updates, protocol.FileDownloadProgressUpdate{
			Name:       name,
			Version:    s.fileVersion[name],
			UpdateType: protocol.UpdateTypeForget,
		})
		delete(s.fileBlockIndexes, name)
		delete(s.fileVersion, name)
		delete(s.fileUpdated, name)
	}
	return updates
}

// sentDownloadState represents a state of what we've announced as available
// to some remote device.
type sentDownloadState struct {
	folderStates map[string]sentFolderDownloadState
}

// update receives a folder, and a slice of pullers that are currently available
// for the given folder, and according to the state of what we've seen before
// returns a set of updates which we should send to the remote device to make
// it aware of everything that we currently have available.
func (s *sentDownloadState) update(folder string, pullers []*sharedPullerState) []protocol.FileDownloadProgressUpdate {
	fs, ok := s.folderStates[folder]
	if !ok {
		fs = sentFolderDownloadState{
			fileBlockIndexes: make(map[string][]int32),
			fileUpdated:      make(map[string]time.Time),
			fileVersion:      make(map[string]protocol.Vector),
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
