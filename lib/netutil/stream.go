// Copyright (C) 2023 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package netutil

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
)

// Stream is the underlying connection we use for wire communication. Mostly
// this is a ReadWriteCloser, i.e. a regular network socket of some kind.
// This is referred to as the "primary" stream, and it is used for all
// metadata messages.
//
// We also support "secondary" streams, which are used for sending data
// requests and responses (only). Connection types that do not support this
// should return ErrSecondaryStreamsUnsupported to indicate that all
// communication should happen on the primary stream.
//
// When secondary streams are supported we will use one or more such streams
// for data requests, for the purpose of increasing concurrency and
// bandwidth.
type Stream interface {
	io.ReadWriteCloser

	// CreateSubstream requests a new secondary stream. The returned
	// ReadWriteCloser is the new stream, and the error is any error that
	// occurred while creating the stream. An error in creating a secondary
	// stream is not fatal -- the connection will continue to operate
	// normally, using the primary stream instead. Creating a stream may
	// have a certain overhead (e.g. TLS handshakes), so it is recommended
	// to reuse streams for multiple requests. The stream should be closed
	// once it is no longer required. Returning ErrSecondaryStreamsUnsupported
	// from this method indicates that the connection does not support
	// secondary streams.
	CreateSubstream(context.Context) (io.ReadWriteCloser, error)

	// AcceptSubstream accepts a new secondary stream. The returned
	// ReadWriteCloser is the new stream, and the error is any error that
	// occurred while accepting the stream. An error in accepting a
	// secondary stream is not fatal -- the connection will continue to
	// operate normally, using the primary stream instead. If the underlying
	// connection does not support secondary streams, this method should
	// return ErrSecondaryStreamsUnsupported, in which case the accept call
	// will not be retried for this connection.
	AcceptSubstream(context.Context) (io.ReadWriteCloser, error)
}

var ErrSubstreamsUnsupported = errors.New("secondary streams not supported")

type readWriteCloser struct {
	io.Reader
	io.Writer
	io.Closer
}

type rwcStream readWriteCloser

func (rwcStream) CreateSubstream(_ context.Context) (io.ReadWriteCloser, error) {
	return nil, ErrSubstreamsUnsupported
}

func (rwcStream) AcceptSubstream(_ context.Context) (io.ReadWriteCloser, error) {
	return nil, ErrSubstreamsUnsupported
}

func NewRWStream(r io.Reader, w io.Writer) Stream {
	return &rwcStream{
		Reader: r,
		Writer: w,
		Closer: io.NopCloser(r),
	}
}

func NewRWCStream(r io.Reader, w io.Writer, c io.Closer) Stream {
	return &rwcStream{
		Reader: r,
		Writer: w,
		Closer: c,
	}
}

// TLSConnStream is a trivial Stream implementation for a TLS connection. It
// supports all the methods of a *tls.Conn, and adds the Stream methods
// (returning ErrSubstreamsUnsupported).
type TLSConnStream struct {
	*tls.Conn
}

func NewTLSConnStream(c *tls.Conn) *TLSConnStream {
	return &TLSConnStream{c}
}

func (TLSConnStream) CreateSubstream(_ context.Context) (io.ReadWriteCloser, error) {
	return nil, ErrSubstreamsUnsupported
}

func (TLSConnStream) AcceptSubstream(_ context.Context) (io.ReadWriteCloser, error) {
	return nil, ErrSubstreamsUnsupported
}
