// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package changeset

import (
	"io"
	"sync"

	"github.com/syncthing/syncthing/lib/protocol"
)

// An AsyncRequester wraps a Requester to parallelize calls to it.
type AsyncRequester struct {
	reqs chan internalRequest
	next Requester
}

// The internalRequest is what we queue internally when a request is made.
type internalRequest struct {
	file   string
	offset int64
	hash   []byte
	size   int
	bufC   chan []byte
	errC   chan error
}

// This manages buffers for the background requests. It's better we do it
// than the user of the API as we can minimize the number of active buffers
// to that needed to handle the number of currently outstanding requests.
var bufferPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, protocol.BlockSize)
	},
}

// NewAsyncRequester returns a new AsyncRequester wrapping r and performing
// up to p requests in parallell in the background.
func NewAsyncRequester(r Requester, p int) *AsyncRequester {
	// The requests channel has a buffer size of p, and we'll start p
	// processors in the background. This means we will have up to p requests
	// outstanding on the network, and up to p more ready in the queue to
	// fire off as soon as we get a response to something else. When we have
	// p requests outstanding and p requests in the queue, further calls to
	// Request() will block.
	reqs := make(chan internalRequest, p)

	async := &AsyncRequester{
		reqs: reqs,
		next: r,
	}
	for i := 0; i < p; i++ {
		go async.processor(reqs)
	}

	return async
}

// Request resturns an asynchronous response and performs the request in the
// background. This call may block if the request queue is full.
func (r *AsyncRequester) Request(file string, offset int64, hash []byte, size int) *AsyncResponse {
	bufC := make(chan []byte)
	errC := make(chan error)
	r.reqs <- internalRequest{
		file:   file,
		offset: offset,
		hash:   hash,
		size:   size,
		bufC:   bufC,
		errC:   errC,
	}
	return &AsyncResponse{
		done: make(chan struct{}),
		bufC: bufC,
		errC: errC,
	}
}

// Close stops the AsyncRequester. No further requests are possible after
// closing.
func (r *AsyncRequester) Close() {
	close(r.reqs)
}

// processor processes requests from the request channel until it closes,
// then returns.
func (r *AsyncRequester) processor(reqs <-chan internalRequest) {
	for req := range reqs {
		buf := bufferPool.Get().([]byte)[:req.size]
		err := r.next.Request(req.file, req.offset, req.hash, buf)
		if err != nil {
			bufferPool.Put(buf) // we don't need the buffer any more
			req.errC <- err
		} else {
			req.bufC <- buf
		}
	}
}

// An AsyncResponse is the pending response to a possibly still ongoing
// background request. The Error(), WriteAt(), WriteTo() and Close() methods
// will block until the request has completed and the response is available.
type AsyncResponse struct {
	buf  []byte        // contains the requested block, or nil if error
	err  error         // contains the request error, or nil on success
	done chan struct{} // closes when buf/error is available
	bufC <-chan []byte // this is where we get successfull responses
	errC <-chan error  // this gets us errors
}

// Error returns the request error, or nil
func (r *AsyncResponse) Error() error {
	r.awaitResponse()
	return r.err
}

// WriteAt writes the request buffer at the given offset.
func (r *AsyncResponse) WriteAt(w io.WriterAt, offset int64) (int, error) {
	r.awaitResponse()
	return w.WriteAt(r.buf, offset)
}

// WriteTo writes the request buffer to the given io.Writer.
func (r *AsyncResponse) WriteTo(w io.Writer) (int64, error) {
	r.awaitResponse()
	n, err := w.Write(r.buf)
	return int64(n), err
}

func (r *AsyncResponse) Bytes() []byte {
	r.awaitResponse()
	return r.buf
}

// Close marks the response as handled and recycles the internal buffer.
func (r *AsyncResponse) Close() {
	r.awaitResponse()
	if r.buf != nil {
		bufferPool.Put(r.buf)
	}
}

// awaitResponse blocks until the request resolves into either a filled out
// buffer or an error. Returns immediately if the response is already
// complete.
func (r *AsyncResponse) awaitResponse() {
	select {
	case <-r.done:
		// We're already done reading responses.

	case buf := <-r.bufC:
		// There's a successfull response waiting for us.
		r.buf = buf
		close(r.done) // so the next call knows we're done

	case err := <-r.errC:
		// There's an error waiting for us.
		r.err = err
		close(r.done) // so the next call knows we're done
	}
}
