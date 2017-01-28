// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/versioner"
)

type receiveOnlyFolder struct {
	// receiveOnlyFolders are really just sendReceiveFolders,, where we reject local changes...
	sendReceiveFolder
}

func init() {
	folderFactories[config.FolderTypeReceiveOnly] = newReceiveOnlyFolder
}

func newReceiveOnlyFolder(model *Model, cfg config.FolderConfiguration, ver versioner.Versioner, mtimeFS *fs.MtimeFS) service {
	f := &receiveOnlyFolder{
		sendReceiveFolder{
			folder: folder{
				stateTracker: newStateTracker(cfg.ID),
				scan:         newFolderScanner(cfg),
				stop:         make(chan struct{}),
				model:        model,
			},
			FolderConfiguration: cfg,

			mtimeFS:   mtimeFS,
			dir:       cfg.Path(),
			versioner: ver,

			queue:       newJobQueue(),
			pullTimer:   time.NewTimer(time.Second),
			remoteIndex: make(chan struct{}, 1), // This needs to be 1-buffered so that we queue a notification if we're busy doing a pull when it comes.

			errorsMut: sync.NewMutex(),

			initialScanCompleted: make(chan struct{}),
		},
	}

	f.configureCopiersAndPullers()

	f.resetInvalidFiles()

	return f
}

func (f *receiveOnlyFolder) String() string {
	return fmt.Sprintf("receiveOnlyFolder/%s@%p", f.folderID, f)
}

// validateAndUpdateLocalChanges reverts all local changes
func (f *receiveOnlyFolder) validateAndUpdateLocalChanges(fs []protocol.FileInfo) []protocol.FileInfo {
	fileDeletions := []protocol.FileInfo{}
	dirDeletions := []protocol.FileInfo{}

	for i, file := range fs {
		if strings.Contains(file.Name, ".sync-conflict-") {
			// This is a conflict copy, let's move on to the next file
			continue
		}

		objType := "file"
		action := "modified"

		if file.IsDirectory() {
			objType = "dir"
		}

		if file.IsDeleted() {
			action = "deleted"
		}

		// Let's update the record to reflect that this is invalid, to avoid it being sent to the cluster
		fs[i].Deleted = false
		fs[i].Invalid = true
		if f.RevertLocalChanges {
			correctiveAction := "resync"

			if len(file.Version.Counters) == 1 && file.Version.Counters[0].Value == 1 {
				// A file, directory or symlink was added, which we'll have to remove again
				action = "added"
				correctiveAction = "none"
				if f.DeleteLocalChanges {
					correctiveAction = "deleted"
					if file.IsDirectory() {
						dirDeletions = append(dirDeletions, file)
					} else {
						fileDeletions = append(fileDeletions, file)
					}
				}
			}

			// resetting the version to its initial value, will trigger pulling the latest version from the cluster
			fs[i].Version = protocol.Vector{}

			// We better tell the user on the UI and in the log that we had to take corrective actions
			l.Infoln("Rejecting local change on folder", f.Description(), objType, file.Name, "was", action, "corrective action:", correctiveAction)

			// Fire the LocalChangeRejected event to notify listeners about rejected local changes.
			events.Default.Log(events.LocalChangeRejected, map[string]string{
				"folder": f.ID,
				"item":   file.Name,
				"type":   objType,
				"action": correctiveAction,
			})
		} else {
			l.Infoln("Ignoring local change on folder", f.Description(), objType, file.Name, "was", action)
		}
	}

	// Delete all the files first, so versioning and conflict managed gets applied
	for _, file := range fileDeletions {
		l.Debugln("Deleting file", file.Name)
		f.deleteRejectedFile(file, f.versioner)
	}

	// Now get rid of those pesky directories that were created
	for i := range dirDeletions {
		dir := dirDeletions[len(dirDeletions)-i-1]
		l.Debugln("Deleting dir", dir.Name)
		f.deleteRejectedDir(dir)
	}

	// Update the database
	f.model.updateLocals(f.ID, fs)

	// Trigger a pull
	f.IndexUpdated()

	// Return the update list of files
	return fs
}

// deleteRejectedDir attempts to delete the given directory
func (f *receiveOnlyFolder) deleteRejectedDir(file protocol.FileInfo) {
	err := f.folder.deleteDir(f.Path(), file, nil)
	if err != nil && !os.IsNotExist(err) {
		l.Infof("deleteRejectedDir (folder %q, file %q): delete: %v", f, file.Name, err)
	}
}

// deleteRejectedFile attempts to delete the given file
func (f *receiveOnlyFolder) deleteRejectedFile(file protocol.FileInfo, ver versioner.Versioner) {
	err := f.folder.deleteFile(f.Path(), file, ver, f.MaxConflicts)

	if err != nil && !os.IsNotExist(err) {
		l.Infof("deleteRejectedFile (folder %q, file %q): delete: %v", f, file.Name, err)
	}
}

// Find all invalid files and force a new evaluation
func (f *receiveOnlyFolder) resetInvalidFiles() {
	folderFiles := f.model.folderFiles[f.folderID]

	var invalidFiles []protocol.FileInfo

	folderFiles.WithHave(protocol.LocalDeviceID, func(fi db.FileIntf) bool {
		f := fi.(protocol.FileInfo)

		if f.IsInvalid() {
			invalidFiles = append(invalidFiles, f)
		}
		return true
	})

	// set all file sizes to 0, to force a new evaluation
	for i := range invalidFiles {
		invalidFiles[i].Size = 0
	}

	// Update the database
	folderFiles.Update(protocol.LocalDeviceID, invalidFiles)
}
