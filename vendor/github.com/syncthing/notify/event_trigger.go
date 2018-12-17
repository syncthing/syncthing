// Copyright (c) 2014-2015 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

// +build darwin,kqueue dragonfly freebsd netbsd openbsd solaris

package notify

type event struct {
	p  string
	e  Event
	d  bool
	pe interface{}
}

func (e *event) Event() Event { return e.e }

func (e *event) Path() string { return e.p }

func (e *event) Sys() interface{} { return e.pe }

func (e *event) isDir() (bool, error) { return e.d, nil }
