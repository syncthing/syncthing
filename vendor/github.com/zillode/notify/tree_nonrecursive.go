// Copyright (c) 2014-2015 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

package notify

import "sync"

// nonrecursiveTree TODO(rjeczalik)
type nonrecursiveTree struct {
	rw   sync.RWMutex // protects root
	root root
	w    watcher
	c    chan EventInfo
	rec  chan EventInfo
}

// newNonrecursiveTree TODO(rjeczalik)
func newNonrecursiveTree(w watcher, c, rec chan EventInfo) *nonrecursiveTree {
	if rec == nil {
		rec = make(chan EventInfo, buffer)
	}
	t := &nonrecursiveTree{
		root: root{nd: newnode("")},
		w:    w,
		c:    c,
		rec:  rec,
	}
	go t.dispatch(c)
	go t.internal(rec)
	return t
}

// dispatch TODO(rjeczalik)
func (t *nonrecursiveTree) dispatch(c <-chan EventInfo) {
	for ei := range c {
		dbgprintf("dispatching %v on %q", ei.Event(), ei.Path())
		go func(ei EventInfo) {
			var nd node
			var isrec bool
			dir, base := split(ei.Path())
			fn := func(it node, isbase bool) error {
				isrec = isrec || it.Watch.IsRecursive()
				if isbase {
					nd = it
				} else {
					it.Watch.Dispatch(ei, recursive)
				}
				return nil
			}
			t.rw.RLock()
			// Notify recursive watchpoints found on the path.
			if err := t.root.WalkPath(dir, fn); err != nil {
				dbgprint("dispatch did not reach leaf:", err)
				t.rw.RUnlock()
				return
			}
			// Notify parent watchpoint.
			nd.Watch.Dispatch(ei, 0)
			isrec = isrec || nd.Watch.IsRecursive()
			// If leaf watchpoint exists, notify it.
			if nd, ok := nd.Child[base]; ok {
				isrec = isrec || nd.Watch.IsRecursive()
				nd.Watch.Dispatch(ei, 0)
			}
			t.rw.RUnlock()
			// If the event describes newly leaf directory created within
			if !isrec || ei.Event() != Create {
				return
			}
			if ok, err := ei.(isDirer).isDir(); !ok || err != nil {
				return
			}
			t.rec <- ei
		}(ei)
	}
}

// internal TODO(rjeczalik)
func (t *nonrecursiveTree) internal(rec <-chan EventInfo) {
	for ei := range rec {
		var nd node
		var eset = internal
		t.rw.Lock()
		t.root.WalkPath(ei.Path(), func(it node, _ bool) error {
			if e := it.Watch[t.rec]; e != 0 && e > eset {
				eset = e
			}
			nd = it
			return nil
		})
		if eset == internal {
			t.rw.Unlock()
			continue
		}
		err := nd.Add(ei.Path()).AddDir(t.recFunc(eset, nil))
		t.rw.Unlock()
		if err != nil {
			dbgprintf("internal(%p) error: %v", rec, err)
		}
	}
}

// watchAdd TODO(rjeczalik)
func (t *nonrecursiveTree) watchAdd(nd node, c chan<- EventInfo, e Event) eventDiff {
	if e&recursive != 0 {
		diff := nd.Watch.Add(t.rec, e|Create|omit)
		nd.Watch.Add(c, e)
		return diff
	}
	return nd.Watch.Add(c, e)
}

// watchDelMin TODO(rjeczalik)
func (t *nonrecursiveTree) watchDelMin(min Event, nd node, c chan<- EventInfo, e Event) eventDiff {
	old, ok := nd.Watch[t.rec]
	if ok {
		nd.Watch[t.rec] = min
	}
	diff := nd.Watch.Del(c, e)
	if ok {
		switch old &^= diff[0] &^ diff[1]; {
		case old|internal == internal:
			delete(nd.Watch, t.rec)
			if set, ok := nd.Watch[nil]; ok && len(nd.Watch) == 1 && set == 0 {
				delete(nd.Watch, nil)
			}
		default:
			nd.Watch.Add(t.rec, old|Create)
			switch {
			case diff == none:
			case diff[1]|Create == diff[0]:
				diff = none
			default:
				diff[1] |= Create
			}
		}
	}
	return diff
}

// watchDel TODO(rjeczalik)
func (t *nonrecursiveTree) watchDel(nd node, c chan<- EventInfo, e Event) eventDiff {
	return t.watchDelMin(0, nd, c, e)
}

// Watch TODO(rjeczalik)
func (t *nonrecursiveTree) Watch(path string, c chan<- EventInfo,
	doNotWatch func(string) bool, events ...Event) error {
	if c == nil {
		panic("notify: Watch using nil channel")
	}
	// Expanding with empty event set is a nop.
	if len(events) == 0 {
		return nil
	}
	path, isrec, err := cleanpath(path)
	if err != nil {
		return err
	}
	eset := joinevents(events)
	t.rw.Lock()
	defer t.rw.Unlock()
	nd := t.root.Add(path)
	if isrec {
		return t.watchrec(nd, c, eset|recursive, doNotWatch)
	}
	return t.watch(nd, c, eset)
}

func (t *nonrecursiveTree) watch(nd node, c chan<- EventInfo, e Event) (err error) {
	diff := nd.Watch.Add(c, e)
	switch {
	case diff == none:
		return nil
	case diff[1] == 0:
		// TODO(rjeczalik): cleanup this panic after implementation is stable
		panic("eset is empty: " + nd.Name)
	case diff[0] == 0:
		err = t.w.Watch(nd.Name, diff[1])
	default:
		err = t.w.Rewatch(nd.Name, diff[0], diff[1])
	}
	if err != nil {
		nd.Watch.Del(c, diff.Event())
		return err
	}
	return nil
}

func (t *nonrecursiveTree) recFunc(e Event, doNotWatch func(string) bool) walkFunc {
	addWatch := func(nd node) (err error) {
		switch diff := nd.Watch.Add(t.rec, e|omit|Create); {
		case diff == none:
		case diff[1] == 0:
			// TODO(rjeczalik): cleanup this panic after implementation is stable
			panic("eset is empty: " + nd.Name)
		case diff[0] == 0:
			err = t.w.Watch(nd.Name, diff[1])
		default:
			err = t.w.Rewatch(nd.Name, diff[0], diff[1])
		}
		return
	}
	if doNotWatch != nil {
		return func(nd node) (err error) {
			if doNotWatch(nd.Name) {
				return errSkip
			}
			return addWatch(nd)
		}
	}
	return addWatch
}

func (t *nonrecursiveTree) watchrec(nd node, c chan<- EventInfo, e Event,
	doNotWatch func(string) bool) error {
	var traverse func(walkFunc) error
	// Non-recursive tree listens on Create event for every recursive
	// watchpoint in order to automagically set a watch for every
	// created directory.
	switch diff := nd.Watch.dryAdd(t.rec, e|Create); {
	case diff == none:
		t.watchAdd(nd, c, e)
		nd.Watch.Add(t.rec, e|omit|Create)
		return nil
	case diff[1] == 0:
		// TODO(rjeczalik): cleanup this panic after implementation is stable
		panic("eset is empty: " + nd.Name)
	case diff[0] == 0:
		// TODO(rjeczalik): BFS into directories and skip subtree as soon as first
		// recursive watchpoint is encountered.
		traverse = nd.AddDir
	default:
		traverse = nd.Walk
	}
	// TODO(rjeczalik): account every path that failed to be (re)watched
	// and retry.
	if err := traverse(t.recFunc(e, doNotWatch)); err != nil {
		return err
	}
	t.watchAdd(nd, c, e)
	return nil
}

type walkWatchpointFunc func(Event, node) error

func (t *nonrecursiveTree) walkWatchpoint(nd node, fn walkWatchpointFunc) error {
	type minode struct {
		min Event
		nd  node
	}
	mnd := minode{nd: nd}
	stack := []minode{mnd}
Traverse:
	for n := len(stack); n != 0; n = len(stack) {
		mnd, stack = stack[n-1], stack[:n-1]
		// There must be no recursive watchpoints if the node has no watchpoints
		// itself (every node in subtree rooted at recursive watchpoints must
		// have at least nil (total) and t.rec watchpoints).
		if len(mnd.nd.Watch) != 0 {
			switch err := fn(mnd.min, mnd.nd); err {
			case nil:
			case errSkip:
				continue Traverse
			default:
				return err
			}
		}
		for _, nd := range mnd.nd.Child {
			stack = append(stack, minode{mnd.nd.Watch[t.rec], nd})
		}
	}
	return nil
}

// Stop TODO(rjeczalik)
func (t *nonrecursiveTree) Stop(c chan<- EventInfo) {
	fn := func(min Event, nd node) error {
		// TODO(rjeczalik): aggregate watcher errors and retry; in worst case
		// forward to the user.
		switch diff := t.watchDelMin(min, nd, c, all); {
		case diff == none:
			return nil
		case diff[1] == 0:
			t.w.Unwatch(nd.Name)
		default:
			t.w.Rewatch(nd.Name, diff[0], diff[1])
		}
		return nil
	}
	t.rw.Lock()
	err := t.walkWatchpoint(t.root.nd, fn) // TODO(rjeczalik): store max root per c
	t.rw.Unlock()
	dbgprintf("Stop(%p) error: %v\n", c, err)
}

// Close TODO(rjeczalik)
func (t *nonrecursiveTree) Close() error {
	err := t.w.Close()
	close(t.c)
	return err
}
