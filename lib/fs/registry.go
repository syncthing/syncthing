// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import "errors"

func init() {
	registry[FilesystemTypeBasic] = func(root string) Filesystem {
		return NewWalkFilesystem(NewBasicFilesystem(root))
	}
}

type filesystemFactory func(string) Filesystem

var (
	registry = map[FilesystemType]filesystemFactory{}
)

func NewFilesystem(fsType FilesystemType, uri string) (fs Filesystem) {
	factory, ok := registry[fsType]

	if !ok {
		l.Debugln("Unknown filesystem", fsType, uri)
		fs = &errorFilesystem{
			fsType: fsType,
			uri:    uri,
			err:    errors.New("filesystem with type " + fsType.String() + " does not exist."),
		}
	} else {
		fs = factory(uri)
	}

	if l.ShouldDebug("filesystem") {
		fs = &logFilesystem{fs}
	}
	return fs
}
