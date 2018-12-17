// Copyright (c) 2014-2018 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

package notify

type watcherStub struct{ error }

// Following methods implement notify.watcher interface.
func (s watcherStub) Watch(string, Event) error          { return s }
func (s watcherStub) Rewatch(string, Event, Event) error { return s }
func (s watcherStub) Unwatch(string) (err error)         { return s }
func (s watcherStub) Close() error                       { return s }
