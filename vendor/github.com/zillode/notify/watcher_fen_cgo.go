// Copyright (c) 2014-2015 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

// +build solaris

package notify

// #include <port.h>
// #include <stdio.h>
// #include <stdlib.h>
// struct file_obj* newFo() { return (struct file_obj*) malloc(sizeof(struct file_obj)); }
// port_event_t* newPe() { return (port_event_t*) malloc(sizeof(port_event_t)); }
// uintptr_t conv(struct file_obj* fo) { return (uintptr_t) fo; }
// struct file_obj* dconv(uintptr_t fo) { return (struct file_obj*) fo; }
import "C"

import (
	"syscall"
	"unsafe"
)

const (
	fileAccess     = Event(C.FILE_ACCESS)
	fileModified   = Event(C.FILE_MODIFIED)
	fileAttrib     = Event(C.FILE_ATTRIB)
	fileDelete     = Event(C.FILE_DELETE)
	fileRenameTo   = Event(C.FILE_RENAME_TO)
	fileRenameFrom = Event(C.FILE_RENAME_FROM)
	fileTrunc      = Event(C.FILE_TRUNC)
	fileNoFollow   = Event(C.FILE_NOFOLLOW)
	unmounted      = Event(C.UNMOUNTED)
	mountedOver    = Event(C.MOUNTEDOVER)
)

// PortEvent is a notify's equivalent of port_event_t.
type PortEvent struct {
	PortevEvents int         // PortevEvents is an equivalent of portev_events.
	PortevSource uint8       // PortevSource is an equivalent of portev_source.
	PortevPad    uint8       // Portevpad is an equivalent of portev_pad.
	PortevObject interface{} // PortevObject is an equivalent of portev_object.
	PortevUser   uintptr     // PortevUser is an equivalent of portev_user.
}

// FileObj is a notify's equivalent of file_obj.
type FileObj struct {
	Atim syscall.Timespec // Atim is an equivalent of fo_atime.
	Mtim syscall.Timespec // Mtim is an equivalent of fo_mtime.
	Ctim syscall.Timespec // Ctim is an equivalent of fo_ctime.
	Pad  [3]uintptr       // Pad is an equivalent of fo_pad.
	Name string           // Name is an equivalent of fo_name.
}

type cfen struct {
	p2pe map[string]*C.port_event_t
	p2fo map[string]*C.struct_file_obj
}

func newCfen() cfen {
	return cfen{
		p2pe: make(map[string]*C.port_event_t),
		p2fo: make(map[string]*C.struct_file_obj),
	}
}

func unix2C(sec int64, nsec int64) (C.time_t, C.long) {
	return C.time_t(sec), C.long(nsec)
}

func (c *cfen) port_associate(p int, fo FileObj, e int) (err error) {
	cfo := C.newFo()
	cfo.fo_atime.tv_sec, cfo.fo_atime.tv_nsec = unix2C(fo.Atim.Unix())
	cfo.fo_mtime.tv_sec, cfo.fo_mtime.tv_nsec = unix2C(fo.Mtim.Unix())
	cfo.fo_ctime.tv_sec, cfo.fo_ctime.tv_nsec = unix2C(fo.Ctim.Unix())
	cfo.fo_name = C.CString(fo.Name)
	c.p2fo[fo.Name] = cfo
	_, err = C.port_associate(C.int(p), srcFile, C.conv(cfo), C.int(e), nil)
	return
}

func (c *cfen) port_dissociate(port int, fo FileObj) (err error) {
	cfo, ok := c.p2fo[fo.Name]
	if !ok {
		return errNotWatched
	}
	_, err = C.port_dissociate(C.int(port), srcFile, C.conv(cfo))
	C.free(unsafe.Pointer(cfo.fo_name))
	C.free(unsafe.Pointer(cfo))
	delete(c.p2fo, fo.Name)
	return
}

const srcAlert = C.PORT_SOURCE_ALERT
const srcFile = C.PORT_SOURCE_FILE
const alertSet = C.PORT_ALERT_SET

func cfo2fo(cfo *C.struct_file_obj) *FileObj {
	// Currently remaining attributes are not used.
	if cfo == nil {
		return nil
	}
	var fo FileObj
	fo.Name = C.GoString(cfo.fo_name)
	return &fo
}

func (c *cfen) port_get(port int, pe *PortEvent) (err error) {
	cpe := C.newPe()
	if _, err = C.port_get(C.int(port), cpe, nil); err != nil {
		C.free(unsafe.Pointer(cpe))
		return
	}
	pe.PortevEvents, pe.PortevSource, pe.PortevPad =
		int(cpe.portev_events), uint8(cpe.portev_source), uint8(cpe.portev_pad)
	pe.PortevObject = cfo2fo(C.dconv(cpe.portev_object))
	pe.PortevUser = uintptr(cpe.portev_user)
	C.free(unsafe.Pointer(cpe))
	return
}

func (c *cfen) port_create() (int, error) {
	p, err := C.port_create()
	return int(p), err
}

func (c *cfen) port_alert(p int) (err error) {
	_, err = C.port_alert(C.int(p), alertSet, C.int(666), nil)
	return
}

func (c *cfen) free() {
	for i := range c.p2fo {
		C.free(unsafe.Pointer(c.p2fo[i].fo_name))
		C.free(unsafe.Pointer(c.p2fo[i]))
	}
	for i := range c.p2pe {
		C.free(unsafe.Pointer(c.p2pe[i]))
	}
	c.p2fo = make(map[string]*C.struct_file_obj)
	c.p2pe = make(map[string]*C.port_event_t)
}
