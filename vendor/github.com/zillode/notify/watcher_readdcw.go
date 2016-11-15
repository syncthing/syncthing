// Copyright (c) 2014-2015 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

// +build windows

package notify

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"
)

// readBufferSize defines the size of an array in which read statuses are stored.
// The buffer have to be DWORD-aligned and, if notify is used in monitoring a
// directory over the network, its size must not be greater than 64KB. Each of
// watched directories uses its own buffer for storing events.
const readBufferSize = 4096

// Since all operations which go through the Windows completion routine are done
// asynchronously, filter may set one of the constants belor. They were defined
// in order to distinguish whether current folder should be re-registered in
// ReadDirectoryChangesW function or some control operations need to be executed.
const (
	stateRewatch uint32 = 1 << (28 + iota)
	stateUnwatch
	stateCPClose
)

// Filter used in current implementation was split into four segments:
//  - bits  0-11 store ReadDirectoryChangesW filters,
//  - bits 12-19 store File notify actions,
//  - bits 20-27 store notify specific events and flags,
//  - bits 28-31 store states which are used in loop's FSM.
// Constants below are used as masks to retrieve only specific filter parts.
const (
	onlyNotifyChanges uint32 = 0x00000FFF
	onlyNGlobalEvents uint32 = 0x0FF00000
	onlyMachineStates uint32 = 0xF0000000
)

// grip represents a single watched directory. It stores the data required by
// ReadDirectoryChangesW function. Only the filter, recursive, and handle members
// may by modified by watcher implementation. Rest of the them have to remain
// constant since they are used by Windows completion routine. This indicates that
// grip can be removed only when all operations on the file handle are finished.
type grip struct {
	handle    syscall.Handle
	filter    uint32
	recursive bool
	pathw     []uint16
	buffer    [readBufferSize]byte
	parent    *watched
	ovlapped  *overlappedEx
}

// overlappedEx stores information used in asynchronous input and output.
// Additionally, overlappedEx contains a pointer to 'grip' item which is used in
// order to gather the structure in which the overlappedEx object was created.
type overlappedEx struct {
	syscall.Overlapped
	parent *grip
}

// newGrip creates a new file handle that can be used in overlapped operations.
// Then, the handle is associated with I/O completion port 'cph' and its value
// is stored in newly created 'grip' object.
func newGrip(cph syscall.Handle, parent *watched, filter uint32) (*grip, error) {
	g := &grip{
		handle:    syscall.InvalidHandle,
		filter:    filter,
		recursive: parent.recursive,
		pathw:     parent.pathw,
		parent:    parent,
		ovlapped:  &overlappedEx{},
	}
	if err := g.register(cph); err != nil {
		return nil, err
	}
	g.ovlapped.parent = g
	return g, nil
}

// NOTE : Thread safe
func (g *grip) register(cph syscall.Handle) (err error) {
	if g.handle, err = syscall.CreateFile(
		&g.pathw[0],
		syscall.FILE_LIST_DIRECTORY,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_BACKUP_SEMANTICS|syscall.FILE_FLAG_OVERLAPPED,
		0,
	); err != nil {
		return
	}
	if _, err = syscall.CreateIoCompletionPort(g.handle, cph, 0, 0); err != nil {
		syscall.CloseHandle(g.handle)
		return
	}
	return g.readDirChanges()
}

// readDirChanges tells the system to store file change information in grip's
// buffer. Directory changes that occur between calls to this function are added
// to the buffer and then, returned with the next call.
func (g *grip) readDirChanges() error {
	return syscall.ReadDirectoryChanges(
		g.handle,
		&g.buffer[0],
		uint32(unsafe.Sizeof(g.buffer)),
		g.recursive,
		encode(g.filter),
		nil,
		(*syscall.Overlapped)(unsafe.Pointer(g.ovlapped)),
		0,
	)
}

// encode transforms a generic filter, which contains platform independent and
// implementation specific bit fields, to value that can be used as NotifyFilter
// parameter in ReadDirectoryChangesW function.
func encode(filter uint32) uint32 {
	e := Event(filter & (onlyNGlobalEvents | onlyNotifyChanges))
	if e&dirmarker != 0 {
		return uint32(FileNotifyChangeDirName)
	}
	if e&Create != 0 {
		e = (e ^ Create) | FileNotifyChangeFileName
	}
	if e&Remove != 0 {
		e = (e ^ Remove) | FileNotifyChangeFileName
	}
	if e&Write != 0 {
		e = (e ^ Write) | FileNotifyChangeAttributes | FileNotifyChangeSize |
			FileNotifyChangeCreation | FileNotifyChangeSecurity
	}
	if e&Rename != 0 {
		e = (e ^ Rename) | FileNotifyChangeFileName
	}
	return uint32(e)
}

// watched is made in order to check whether an action comes from a directory or
// file. This approach requires two file handlers per single monitored folder. The
// second grip handles actions which include creating or deleting a directory. If
// these processes are not monitored, only the first grip is created.
type watched struct {
	filter    uint32
	recursive bool
	count     uint8
	pathw     []uint16
	digrip    [2]*grip
}

// newWatched creates a new watched instance. It splits the filter variable into
// two parts. The first part is responsible for watching all events which can be
// created for a file in watched directory structure and the second one watches
// only directory Create/Remove actions. If all operations succeed, the Create
// message is sent to I/O completion port queue for further processing.
func newWatched(cph syscall.Handle, filter uint32, recursive bool,
	path string) (wd *watched, err error) {
	wd = &watched{
		filter:    filter,
		recursive: recursive,
	}
	if wd.pathw, err = syscall.UTF16FromString(path); err != nil {
		return
	}
	if err = wd.recreate(cph); err != nil {
		return
	}
	return wd, nil
}

// TODO : doc
func (wd *watched) recreate(cph syscall.Handle) (err error) {
	filefilter := wd.filter &^ uint32(FileNotifyChangeDirName)
	if err = wd.updateGrip(0, cph, filefilter == 0, filefilter); err != nil {
		return
	}
	dirfilter := wd.filter & uint32(FileNotifyChangeDirName|Create|Remove)
	if err = wd.updateGrip(1, cph, dirfilter == 0, wd.filter|uint32(dirmarker)); err != nil {
		return
	}
	wd.filter &^= onlyMachineStates
	return
}

// TODO : doc
func (wd *watched) updateGrip(idx int, cph syscall.Handle, reset bool,
	newflag uint32) (err error) {
	if reset {
		wd.digrip[idx] = nil
	} else {
		if wd.digrip[idx] == nil {
			if wd.digrip[idx], err = newGrip(cph, wd, newflag); err != nil {
				wd.closeHandle()
				return
			}
		} else {
			wd.digrip[idx].filter = newflag
			wd.digrip[idx].recursive = wd.recursive
			if err = wd.digrip[idx].register(cph); err != nil {
				wd.closeHandle()
				return
			}
		}
		wd.count++
	}
	return
}

// closeHandle closes handles that are stored in digrip array. Function always
// tries to close all of the handlers before it exits, even when there are errors
// returned from the operating system kernel.
func (wd *watched) closeHandle() (err error) {
	for _, g := range wd.digrip {
		if g != nil && g.handle != syscall.InvalidHandle {
			switch suberr := syscall.CloseHandle(g.handle); {
			case suberr == nil:
				g.handle = syscall.InvalidHandle
			case err == nil:
				err = suberr
			}
		}
	}
	return
}

// watcher implements Watcher interface. It stores a set of watched directories.
// All operations which remove watched objects from map `m` must be performed in
// loop goroutine since these structures are used internally by operating system.
type readdcw struct {
	sync.Mutex
	m     map[string]*watched
	cph   syscall.Handle
	start bool
	wg    sync.WaitGroup
	c     chan<- EventInfo
}

// NewWatcher creates new non-recursive watcher backed by ReadDirectoryChangesW.
func newWatcher(c chan<- EventInfo) watcher {
	r := &readdcw{
		m:   make(map[string]*watched),
		cph: syscall.InvalidHandle,
		c:   c,
	}
	runtime.SetFinalizer(r, func(r *readdcw) {
		if r.cph != syscall.InvalidHandle {
			syscall.CloseHandle(r.cph)
		}
	})
	return r
}

// Watch implements notify.Watcher interface.
func (r *readdcw) Watch(path string, event Event) error {
	return r.watch(path, event, false)
}

// RecursiveWatch implements notify.RecursiveWatcher interface.
func (r *readdcw) RecursiveWatch(path string, event Event) error {
	return r.watch(path, event, true)
}

// watch inserts a directory to the group of watched folders. If watched folder
// already exists, function tries to rewatch it with new filters(NOT VALID). Moreover,
// watch starts the main event loop goroutine when called for the first time.
func (r *readdcw) watch(path string, event Event, recursive bool) (err error) {
	if event&^(All|fileNotifyChangeAll) != 0 {
		return errors.New("notify: unknown event")
	}
	r.Lock()
	wd, ok := r.m[path]
	r.Unlock()
	if !ok {
		if err = r.lazyinit(); err != nil {
			return
		}
		r.Lock()
		if wd, ok = r.m[path]; ok {
			r.Unlock()
			return
		}
		if wd, err = newWatched(r.cph, uint32(event), recursive, path); err != nil {
			r.Unlock()
			return
		}
		r.m[path] = wd
		r.Unlock()
	}
	return nil
}

// lazyinit creates an I/O completion port and starts the main event processing
// loop. This method uses Double-Checked Locking optimization.
func (r *readdcw) lazyinit() (err error) {
	invalid := uintptr(syscall.InvalidHandle)
	if atomic.LoadUintptr((*uintptr)(&r.cph)) == invalid {
		r.Lock()
		defer r.Unlock()
		if atomic.LoadUintptr((*uintptr)(&r.cph)) == invalid {
			cph := syscall.InvalidHandle
			if cph, err = syscall.CreateIoCompletionPort(cph, 0, 0, 0); err != nil {
				return
			}
			r.cph, r.start = cph, true
			go r.loop()
		}
	}
	return
}

// TODO(pknap) : doc
func (r *readdcw) loop() {
	var n, key uint32
	var overlapped *syscall.Overlapped
	for {
		err := syscall.GetQueuedCompletionStatus(r.cph, &n, &key, &overlapped, syscall.INFINITE)
		if key == stateCPClose {
			r.Lock()
			handle := r.cph
			r.cph = syscall.InvalidHandle
			r.Unlock()
			syscall.CloseHandle(handle)
			r.wg.Done()
			return
		}
		if overlapped == nil {
			// TODO: check key == rewatch delete or 0(panic)
			continue
		}
		overEx := (*overlappedEx)(unsafe.Pointer(overlapped))
		if n == 0 {
			r.loopstate(overEx)
		} else {
			r.loopevent(n, overEx)
			if err = overEx.parent.readDirChanges(); err != nil {
				// TODO: error handling
			}
		}
	}
}

// TODO(pknap) : doc
func (r *readdcw) loopstate(overEx *overlappedEx) {
	filter := atomic.LoadUint32(&overEx.parent.parent.filter)
	if filter&onlyMachineStates == 0 {
		return
	}
	if overEx.parent.parent.count--; overEx.parent.parent.count == 0 {
		switch filter & onlyMachineStates {
		case stateRewatch:
			r.Lock()
			overEx.parent.parent.recreate(r.cph)
			r.Unlock()
		case stateUnwatch:
			r.Lock()
			delete(r.m, syscall.UTF16ToString(overEx.parent.pathw))
			r.Unlock()
		case stateCPClose:
		default:
			panic(`notify: windows loopstate logic error`)
		}
	}
}

// TODO(pknap) : doc
func (r *readdcw) loopevent(n uint32, overEx *overlappedEx) {
	events := []*event{}
	var currOffset uint32
	for {
		raw := (*syscall.FileNotifyInformation)(unsafe.Pointer(&overEx.parent.buffer[currOffset]))
		name := syscall.UTF16ToString((*[syscall.MAX_LONG_PATH]uint16)(unsafe.Pointer(&raw.FileName))[:raw.FileNameLength>>1])
		events = append(events, &event{
			pathw:  overEx.parent.pathw,
			filter: overEx.parent.filter,
			action: raw.Action,
			name:   name,
		})
		if raw.NextEntryOffset == 0 {
			break
		}
		if currOffset += raw.NextEntryOffset; currOffset >= n {
			break
		}
	}
	r.send(events)
}

// TODO(pknap) : doc
func (r *readdcw) send(es []*event) {
	for _, e := range es {
		var syse Event
		if e.e, syse = decode(e.filter, e.action); e.e == 0 && syse == 0 {
			continue
		}
		switch {
		case e.action == syscall.FILE_ACTION_MODIFIED:
			e.ftype = fTypeUnknown
		case e.filter&uint32(dirmarker) != 0:
			e.ftype = fTypeDirectory
		default:
			e.ftype = fTypeFile
		}
		switch {
		case e.e == 0:
			e.e = syse
		case syse != 0:
			r.c <- &event{
				pathw:  e.pathw,
				name:   e.name,
				ftype:  e.ftype,
				action: e.action,
				filter: e.filter,
				e:      syse,
			}
		}
		r.c <- e
	}
}

// Rewatch implements notify.Rewatcher interface.
func (r *readdcw) Rewatch(path string, oldevent, newevent Event) error {
	return r.rewatch(path, uint32(oldevent), uint32(newevent), false)
}

// RecursiveRewatch implements notify.RecursiveRewatcher interface.
func (r *readdcw) RecursiveRewatch(oldpath, newpath string, oldevent,
	newevent Event) error {
	if oldpath != newpath {
		if err := r.unwatch(oldpath); err != nil {
			return err
		}
		return r.watch(newpath, newevent, true)
	}
	return r.rewatch(newpath, uint32(oldevent), uint32(newevent), true)
}

// TODO : (pknap) doc.
func (r *readdcw) rewatch(path string, oldevent, newevent uint32, recursive bool) (err error) {
	if Event(newevent)&^(All|fileNotifyChangeAll) != 0 {
		return errors.New("notify: unknown event")
	}
	var wd *watched
	r.Lock()
	if wd, err = r.nonStateWatched(path); err != nil {
		r.Unlock()
		return
	}
	if wd.filter&(onlyNotifyChanges|onlyNGlobalEvents) != oldevent {
		panic(`notify: windows re-watcher logic error`)
	}
	wd.filter = stateRewatch | newevent
	wd.recursive, recursive = recursive, wd.recursive
	if err = wd.closeHandle(); err != nil {
		wd.filter = oldevent
		wd.recursive = recursive
		r.Unlock()
		return
	}
	r.Unlock()
	return
}

// TODO : pknap
func (r *readdcw) nonStateWatched(path string) (wd *watched, err error) {
	wd, ok := r.m[path]
	if !ok || wd == nil {
		err = errors.New(`notify: ` + path + ` path is unwatched`)
		return
	}
	if filter := atomic.LoadUint32(&wd.filter); filter&onlyMachineStates != 0 {
		err = errors.New(`notify: another re/unwatching operation in progress`)
		return
	}
	return
}

// Unwatch implements notify.Watcher interface.
func (r *readdcw) Unwatch(path string) error {
	return r.unwatch(path)
}

// RecursiveUnwatch implements notify.RecursiveWatcher interface.
func (r *readdcw) RecursiveUnwatch(path string) error {
	return r.unwatch(path)
}

// TODO : pknap
func (r *readdcw) unwatch(path string) (err error) {
	var wd *watched
	r.Lock()
	if wd, err = r.nonStateWatched(path); err != nil {
		r.Unlock()
		return
	}
	wd.filter |= stateUnwatch
	if err = wd.closeHandle(); err != nil {
		wd.filter &^= stateUnwatch
		r.Unlock()
		return
	}
	r.Unlock()
	return
}

// Close resets the whole watcher object, closes all existing file descriptors,
// and sends stateCPClose state as completion key to the main watcher's loop.
func (r *readdcw) Close() (err error) {
	r.Lock()
	if !r.start {
		r.Unlock()
		return nil
	}
	for _, wd := range r.m {
		wd.filter &^= onlyMachineStates
		wd.filter |= stateCPClose
		if e := wd.closeHandle(); e != nil && err == nil {
			err = e
		}
	}
	r.start = false
	r.Unlock()
	r.wg.Add(1)
	if e := syscall.PostQueuedCompletionStatus(r.cph, 0, stateCPClose, nil); e != nil && err == nil {
		return e
	}
	r.wg.Wait()
	return
}

// decode creates a notify event from both non-raw filter and action which was
// returned from completion routine. Function may return Event(0) in case when
// filter was replaced by a new value which does not contain fields that are
// valid with passed action.
func decode(filter, action uint32) (Event, Event) {
	switch action {
	case syscall.FILE_ACTION_ADDED:
		return gensys(filter, Create, FileActionAdded)
	case syscall.FILE_ACTION_REMOVED:
		return gensys(filter, Remove, FileActionRemoved)
	case syscall.FILE_ACTION_MODIFIED:
		return gensys(filter, Write, FileActionModified)
	case syscall.FILE_ACTION_RENAMED_OLD_NAME:
		return gensys(filter, Rename, FileActionRenamedOldName)
	case syscall.FILE_ACTION_RENAMED_NEW_NAME:
		return gensys(filter, Rename, FileActionRenamedNewName)
	}
	panic(`notify: cannot decode internal mask`)
}

// gensys decides whether the Windows action, system-independent event or both
// of them should be returned. Since the grip's filter may be atomically changed
// during watcher lifetime, it is possible that neither Windows nor notify masks
// are watched by the user when this function is called.
func gensys(filter uint32, ge, se Event) (gene, syse Event) {
	isdir := filter&uint32(dirmarker) != 0
	if isdir && filter&uint32(FileNotifyChangeDirName) != 0 ||
		!isdir && filter&uint32(FileNotifyChangeFileName) != 0 ||
		filter&uint32(fileNotifyChangeModified) != 0 {
		syse = se
	}
	if filter&uint32(ge) != 0 {
		gene = ge
	}
	return
}
