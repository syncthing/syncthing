// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import ()
import "time"

type folder struct {
	stateTracker
	scan folderscan
	stop chan struct{}
}

func (f *folder) IndexUpdated() {
}

func (f *folder) DelayScan(next time.Duration) {
	f.scan.Delay(next)
}

func (f *folder) Scan(subdirs []string) error {
	return f.scan.Scan(subdirs)
}
func (f *folder) Stop() {
	close(f.stop)
}

func (f *folder) Jobs() ([]string, []string) {
	return nil, nil
}

func (f *folder) BringToFront(string) {}
