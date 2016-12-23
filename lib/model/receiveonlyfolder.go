// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"fmt"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/versioner"
)

type receiveOnlyFolder struct {
	// WO folders are really just RW folders where we reject local changes...
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

	return f
}

func (f *receiveOnlyFolder) String() string {
	return fmt.Sprintf("receiveOnlyFolder/%s@%p", f.folderID, f)
}
