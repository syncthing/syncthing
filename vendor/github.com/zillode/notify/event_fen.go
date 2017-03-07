// Copyright (c) 2014-2015 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

// +build solaris

package notify

const (
	osSpecificCreate Event = 0x00000100 << iota
	osSpecificRemove
	osSpecificWrite
	osSpecificRename
	// internal
	// recursive is used to distinguish recursive eventsets from non-recursive ones
	recursive
	// omit is used for dispatching internal events; only those events are sent
	// for which both the event and the watchpoint has omit in theirs event sets.
	omit
)

const (
	FileAccess     = fileAccess
	FileModified   = fileModified
	FileAttrib     = fileAttrib
	FileDelete     = fileDelete
	FileRenameTo   = fileRenameTo
	FileRenameFrom = fileRenameFrom
	FileTrunc      = fileTrunc
	FileNoFollow   = fileNoFollow
	Unmounted      = unmounted
	MountedOver    = mountedOver
)

var osestr = map[Event]string{
	FileAccess:     "notify.FileAccess",
	FileModified:   "notify.FileModified",
	FileAttrib:     "notify.FileAttrib",
	FileDelete:     "notify.FileDelete",
	FileRenameTo:   "notify.FileRenameTo",
	FileRenameFrom: "notify.FileRenameFrom",
	FileTrunc:      "notify.FileTrunc",
	FileNoFollow:   "notify.FileNoFollow",
	Unmounted:      "notify.Unmounted",
	MountedOver:    "notify.MountedOver",
}
