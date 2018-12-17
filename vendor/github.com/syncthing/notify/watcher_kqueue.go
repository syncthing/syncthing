// Copyright (c) 2014-2015 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

// +build darwin,kqueue dragonfly freebsd netbsd openbsd

package notify

import (
	"fmt"
	"os"
	"syscall"
)

// newTrigger returns implementation of trigger.
func newTrigger(pthLkp map[string]*watched) trigger {
	return &kq{
		pthLkp: pthLkp,
		idLkp:  make(map[int]*watched),
	}
}

// kq is a structure implementing trigger for kqueue.
type kq struct {
	// fd is a kqueue file descriptor
	fd int
	// pipefds are file descriptors used to stop `Kevent` call.
	pipefds [2]int
	// idLkp is a data structure mapping file descriptors with data about watching
	// represented by them files/directories.
	idLkp map[int]*watched
	// pthLkp is a structure mapping monitored files/dir with data about them,
	// shared with parent trg structure
	pthLkp map[string]*watched
}

// watched is a data structure representing watched file/directory.
type watched struct {
	trgWatched
	// fd is a file descriptor for watched file/directory.
	fd int
}

// Stop implements trigger.
func (k *kq) Stop() (err error) {
	// trigger event used to interrupt Kevent call.
	_, err = syscall.Write(k.pipefds[1], []byte{0x00})
	return
}

// Close implements trigger.
func (k *kq) Close() error {
	return syscall.Close(k.fd)
}

// NewWatched implements trigger.
func (*kq) NewWatched(p string, fi os.FileInfo) (*watched, error) {
	fd, err := syscall.Open(p, syscall.O_NONBLOCK|syscall.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	return &watched{
		trgWatched: trgWatched{p: p, fi: fi},
		fd:         fd,
	}, nil
}

// Record implements trigger.
func (k *kq) Record(w *watched) {
	k.idLkp[w.fd], k.pthLkp[w.p] = w, w
}

// Del implements trigger.
func (k *kq) Del(w *watched) {
	syscall.Close(w.fd)
	delete(k.idLkp, w.fd)
	delete(k.pthLkp, w.p)
}

func inter2kq(n interface{}) syscall.Kevent_t {
	kq, ok := n.(syscall.Kevent_t)
	if !ok {
		panic(fmt.Sprintf("kqueue: type should be Kevent_t, %T instead", n))
	}
	return kq
}

// Init implements trigger.
func (k *kq) Init() (err error) {
	if k.fd, err = syscall.Kqueue(); err != nil {
		return
	}
	// Creates pipe used to stop `Kevent` call by registering it,
	// watching read end and writing to other end of it.
	if err = syscall.Pipe(k.pipefds[:]); err != nil {
		return nonil(err, k.Close())
	}
	var kevn [1]syscall.Kevent_t
	syscall.SetKevent(&kevn[0], k.pipefds[0], syscall.EVFILT_READ, syscall.EV_ADD)
	if _, err = syscall.Kevent(k.fd, kevn[:], nil, nil); err != nil {
		return nonil(err, k.Close())
	}
	return
}

// Unwatch implements trigger.
func (k *kq) Unwatch(w *watched) (err error) {
	var kevn [1]syscall.Kevent_t
	syscall.SetKevent(&kevn[0], w.fd, syscall.EVFILT_VNODE, syscall.EV_DELETE)

	_, err = syscall.Kevent(k.fd, kevn[:], nil, nil)
	return
}

// Watch implements trigger.
func (k *kq) Watch(fi os.FileInfo, w *watched, e int64) (err error) {
	var kevn [1]syscall.Kevent_t
	syscall.SetKevent(&kevn[0], w.fd, syscall.EVFILT_VNODE,
		syscall.EV_ADD|syscall.EV_CLEAR)
	kevn[0].Fflags = uint32(e)

	_, err = syscall.Kevent(k.fd, kevn[:], nil, nil)
	return
}

// Wait implements trigger.
func (k *kq) Wait() (interface{}, error) {
	var (
		kevn [1]syscall.Kevent_t
		err  error
	)
	kevn[0] = syscall.Kevent_t{}
	_, err = syscall.Kevent(k.fd, nil, kevn[:], nil)

	return kevn[0], err
}

// Watched implements trigger.
func (k *kq) Watched(n interface{}) (*watched, int64, error) {
	kevn, ok := n.(syscall.Kevent_t)
	if !ok {
		panic(fmt.Sprintf("kq: type should be syscall.Kevent_t, %T instead", kevn))
	}
	if _, ok = k.idLkp[int(kevn.Ident)]; !ok {
		return nil, 0, errNotWatched
	}
	return k.idLkp[int(kevn.Ident)], int64(kevn.Fflags), nil
}

// IsStop implements trigger.
func (k *kq) IsStop(n interface{}, err error) bool {
	return int(inter2kq(n).Ident) == k.pipefds[0]
}

func init() {
	encode = func(e Event, dir bool) (o int64) {
		// Create event is not supported by kqueue. Instead NoteWrite event will
		// be registered for a directory. If this event will be reported on dir
		// which is to be monitored for Create, dir will be rescanned
		// and Create events will be generated and returned for new files.
		// In case of files, if not requested NoteRename event is reported,
		// it will be ignored.
		o = int64(e &^ Create)
		if (e&Create != 0 && dir) || e&Write != 0 {
			o = (o &^ int64(Write)) | int64(NoteWrite)
		}
		if e&Rename != 0 {
			o = (o &^ int64(Rename)) | int64(NoteRename)
		}
		if e&Remove != 0 {
			o = (o &^ int64(Remove)) | int64(NoteDelete)
		}
		return
	}
	nat2not = map[Event]Event{
		NoteWrite:  Write,
		NoteRename: Rename,
		NoteDelete: Remove,
		NoteExtend: Event(0),
		NoteAttrib: Event(0),
		NoteRevoke: Event(0),
		NoteLink:   Event(0),
	}
	not2nat = map[Event]Event{
		Write:  NoteWrite,
		Rename: NoteRename,
		Remove: NoteDelete,
	}
}
