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
	// FileAccess is an event reported when monitored file/directory was accessed.
	FileAccess = fileAccess
	// FileModified is an event reported when monitored file/directory was modified.
	FileModified = fileModified
	// FileAttrib is an event reported when monitored file/directory's ATTRIB
	// was changed.
	FileAttrib = fileAttrib
	// FileDelete is an event reported when monitored file/directory was deleted.
	FileDelete = fileDelete
	// FileRenameTo to is an event reported when monitored file/directory was renamed.
	FileRenameTo = fileRenameTo
	// FileRenameFrom is an event reported when monitored file/directory was renamed.
	FileRenameFrom = fileRenameFrom
	// FileTrunc is an event reported when monitored file/directory was truncated.
	FileTrunc = fileTrunc
	// FileNoFollow is an flag to indicate not to follow symbolic links.
	FileNoFollow = fileNoFollow
	// Unmounted is an event reported when monitored filesystem was unmounted.
	Unmounted = unmounted
	// MountedOver is an event reported when monitored file/directory was mounted on.
	MountedOver = mountedOver
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
