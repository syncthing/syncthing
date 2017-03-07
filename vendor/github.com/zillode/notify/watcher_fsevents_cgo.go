// Copyright (c) 2014-2015 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

// +build darwin,!kqueue

package notify

/*
#include <CoreServices/CoreServices.h>

typedef void (*CFRunLoopPerformCallBack)(void*);

void gosource(void *);
void gostream(uintptr_t, uintptr_t, size_t, uintptr_t, uintptr_t, uintptr_t);

static FSEventStreamRef EventStreamCreate(FSEventStreamContext * context, uintptr_t info, CFArrayRef paths, FSEventStreamEventId since, CFTimeInterval latency, FSEventStreamCreateFlags flags) {
	context->info = (void*) info;
	return FSEventStreamCreate(NULL, (FSEventStreamCallback) gostream, context, paths, since, latency, flags);
}

#cgo LDFLAGS: -framework CoreServices
*/
import "C"

import (
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

var nilstream C.FSEventStreamRef

// Default arguments for FSEventStreamCreate function.
var (
	latency C.CFTimeInterval
	flags   = C.FSEventStreamCreateFlags(C.kFSEventStreamCreateFlagFileEvents | C.kFSEventStreamCreateFlagNoDefer)
	since   = uint64(C.FSEventsGetCurrentEventId())
)

var runloop C.CFRunLoopRef // global runloop which all streams are registered with
var wg sync.WaitGroup      // used to wait until the runloop starts

// source is used for synchronization purposes - it signals when runloop has
// started and is ready via the wg. It also serves purpose of a dummy source,
// thanks to it the runloop does not return as it also has at least one source
// registered.
var source = C.CFRunLoopSourceCreate(nil, 0, &C.CFRunLoopSourceContext{
	perform: (C.CFRunLoopPerformCallBack)(C.gosource),
})

// Errors returned when FSEvents functions fail.
var (
	errCreate = os.NewSyscallError("FSEventStreamCreate", errors.New("NULL"))
	errStart  = os.NewSyscallError("FSEventStreamStart", errors.New("false"))
)

// initializes the global runloop and ensures any created stream awaits its
// readiness.
func init() {
	wg.Add(1)
	go func() {
		runloop = C.CFRunLoopGetCurrent()
		C.CFRunLoopAddSource(runloop, source, C.kCFRunLoopDefaultMode)
		C.CFRunLoopRun()
		panic("runloop has just unexpectedly stopped")
	}()
	C.CFRunLoopSourceSignal(source)
}

//export gosource
func gosource(unsafe.Pointer) {
	time.Sleep(time.Second)
	wg.Done()
}

//export gostream
func gostream(_, info uintptr, n C.size_t, paths, flags, ids uintptr) {
	const (
		offchar = unsafe.Sizeof((*C.char)(nil))
		offflag = unsafe.Sizeof(C.FSEventStreamEventFlags(0))
		offid   = unsafe.Sizeof(C.FSEventStreamEventId(0))
	)
	if n == 0 {
		return
	}
	ev := make([]FSEvent, 0, int(n))
	for i := uintptr(0); i < uintptr(n); i++ {
		switch flags := *(*uint32)(unsafe.Pointer((flags + i*offflag))); {
		case flags&uint32(FSEventsEventIdsWrapped) != 0:
			atomic.StoreUint64(&since, uint64(C.FSEventsGetCurrentEventId()))
		default:
			ev = append(ev, FSEvent{
				Path:  C.GoString(*(**C.char)(unsafe.Pointer(paths + i*offchar))),
				Flags: flags,
				ID:    *(*uint64)(unsafe.Pointer(ids + i*offid)),
			})
		}

	}
	streamFuncs.get(info)(ev)
}

// StreamFunc is a callback called when stream receives file events.
type streamFunc func([]FSEvent)

var streamFuncs = streamFuncRegistry{m: map[uintptr]streamFunc{}}

type streamFuncRegistry struct {
	mu sync.Mutex
	m  map[uintptr]streamFunc
	i  uintptr
}

func (r *streamFuncRegistry) get(id uintptr) streamFunc {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.m[id]
}

func (r *streamFuncRegistry) add(fn streamFunc) uintptr {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.i++
	r.m[r.i] = fn
	return r.i
}

func (r *streamFuncRegistry) delete(id uintptr) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.m, id)
}

// Stream represents single watch-point which listens for events scheduled by
// the global runloop.
type stream struct {
	path string
	ref  C.FSEventStreamRef
	info uintptr
}

// NewStream creates a stream for given path, listening for file events and
// calling fn upon receving any.
func newStream(path string, fn streamFunc) *stream {
	return &stream{
		path: path,
		info: streamFuncs.add(fn),
	}
}

// Start creates a FSEventStream for the given path and schedules it with
// global runloop. It's a nop if the stream was already started.
func (s *stream) Start() error {
	if s.ref != nilstream {
		return nil
	}
	wg.Wait()
	p := C.CFStringCreateWithCStringNoCopy(nil, C.CString(s.path), C.kCFStringEncodingUTF8, nil)
	path := C.CFArrayCreate(nil, (*unsafe.Pointer)(unsafe.Pointer(&p)), 1, nil)
	ctx := C.FSEventStreamContext{}
	ref := C.EventStreamCreate(&ctx, C.uintptr_t(s.info), path, C.FSEventStreamEventId(atomic.LoadUint64(&since)), latency, flags)
	if ref == nilstream {
		return errCreate
	}
	C.FSEventStreamScheduleWithRunLoop(ref, runloop, C.kCFRunLoopDefaultMode)
	if C.FSEventStreamStart(ref) == C.Boolean(0) {
		C.FSEventStreamInvalidate(ref)
		return errStart
	}
	C.CFRunLoopWakeUp(runloop)
	s.ref = ref
	return nil
}

// Stop stops underlying FSEventStream and unregisters it from global runloop.
func (s *stream) Stop() {
	if s.ref == nilstream {
		return
	}
	wg.Wait()
	C.FSEventStreamStop(s.ref)
	C.FSEventStreamInvalidate(s.ref)
	C.CFRunLoopWakeUp(runloop)
	s.ref = nilstream
	streamFuncs.delete(s.info)
}
