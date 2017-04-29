// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"context"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/versioner"
)

func init() {
	folderFactories[config.FolderTypeSendOnly] = newSendOnlyFolder
}

type sendOnlyFolder struct {
	*folderScanner
	*stateTracker
	ctx    context.Context
	cancel context.CancelFunc
}

func newSendOnlyFolder(model *Model, cfg config.FolderConfiguration, _ versioner.Versioner, mtimeFS *fs.MtimeFS) service {
	ctx, cancel := context.WithCancel(context.Background())

	st := newStateTracker(cfg.ID)
	fsCfg := folderScannerConfig{
		shortID:          model.id.Short(),
		currentFiler:     cFiler{model, cfg.ID},
		filesystem:       mtimeFS,
		ignores:          nil, // XXX
		stateTracker:     st,
		dbUpdater:        nil, // XXX
		dbPrefixIterator: nil, // XXX
	}

	return &sendOnlyFolder{
		folderScanner: newFolderScanner(ctx, cfg, fsCfg),
		stateTracker:  st,
		ctx:           ctx,
		cancel:        cancel,
	}
}

func (f *sendOnlyFolder) BringToFront(string) {
	panic("bug: BringToFront on send only folder")
}

func (f *sendOnlyFolder) IndexUpdated() {
}

func (f *sendOnlyFolder) Jobs() ([]string, []string) {
	return nil, nil
}
