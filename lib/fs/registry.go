// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import "errors"

func init() {
	registry["basic"] = NewBasicFilesystem
}

type filesystemFactory func(string) Filesystem

var (
	registry map[string]filesystemFactory
)

func NewFilesystem(fsType, uri string) Filesystem {
	factory, ok := registry[fsType]
	if !ok {
		return &errorFilesystem{
			fsType: fsType,
			uri:    uri,
			err:    errors.New("filesystem with type " + fsType + " does not exist."),
		}
	}
	return factory(uri)
}
