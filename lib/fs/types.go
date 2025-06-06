// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import "sync"

type FilesystemType string

// Option modifies a filesystem at creation. An option might be specific
// to a filesystem-type.
//
// String is used to detect options with the same effect, i.e. must be different
// for options with different effects. Meaning if an option has parameters, a
// representation of those must be part of the returned string.
type Option interface {
	String() string
	apply(Filesystem) Filesystem
}

// Factory function type for constructing a custom file system. It takes the URI
// and options as its parameters.
type FilesystemFactory func(string, ...Option) (Filesystem, error)

// For each registered file system type, a function to construct a file system.
var (
	filesystemFactories      map[FilesystemType]FilesystemFactory = make(map[FilesystemType]FilesystemFactory)
	filesystemFactoriesMutex sync.Mutex                           = sync.Mutex{}
)

// Register a function to be called when a filesystem is to be constructed with
// the specified fsType. The function will receive the URI for the file system as well
// as all options.
func RegisterFilesystemType(fsType FilesystemType, fn FilesystemFactory) {
	filesystemFactoriesMutex.Lock()
	defer filesystemFactoriesMutex.Unlock()
	filesystemFactories[fsType] = fn
}
