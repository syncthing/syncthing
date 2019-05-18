// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package testutils

// BlockingRW implements io.Reader and Writer but never returns when called
type BlockingRW struct{ nilChan chan struct{} }

func (rw *BlockingRW) Read(p []byte) (n int, err error) {
	<-rw.nilChan
	return
}

func (rw *BlockingRW) Write(p []byte) (n int, err error) {
	<-rw.nilChan
	return
}

// NoopRW implements io.Reader and Writer but never returns when called
type NoopRW struct{}

func (rw *NoopRW) Read(p []byte) (n int, err error) {
	return len(p), nil
}

func (rw *NoopRW) Write(p []byte) (n int, err error) {
	return len(p), nil
}
