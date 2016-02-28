// Copyright (c) 2014-2015 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

package notify

import "sync"

// watchAdd TODO(rjeczalik)
func watchAdd(nd node, c chan<- EventInfo, e Event) eventDiff {
	diff := nd.Watch.Add(c, e)
	if wp := nd.Child[""].Watch; len(wp) != 0 {
		e = wp.Total()
		diff[0] |= e
		diff[1] |= e
		if diff[0] == diff[1] {
			return none
		}
	}
	return diff
}

// watchAddInactive TODO(rjeczalik)
func watchAddInactive(nd node, c chan<- EventInfo, e Event) eventDiff {
	wp := nd.Child[""].Watch
	if wp == nil {
		wp = make(watchpoint)
		nd.Child[""] = node{Watch: wp}
	}
	diff := wp.Add(c, e)
	e = nd.Watch.Total()
	diff[0] |= e
	diff[1] |= e
	if diff[0] == diff[1] {
		return none
	}
	return diff
}

// watchCopy TODO(rjeczalik)
func watchCopy(src, dst node) {
	for c, e := range src.Watch {
		if c == nil {
			continue
		}
		watchAddInactive(dst, c, e)
	}
	if wpsrc := src.Child[""].Watch; len(wpsrc) != 0 {
		wpdst := dst.Child[""].Watch
		for c, e := range wpsrc {
			if c == nil {
				continue
			}
			wpdst.Add(c, e)
		}
	}
}

// watchDel TODO(rjeczalik)
func watchDel(nd node, c chan<- EventInfo, e Event) eventDiff {
	diff := nd.Watch.Del(c, e)
	if wp := nd.Child[""].Watch; len(wp) != 0 {
		diffInactive := wp.Del(c, e)
		e = wp.Total()
		// TODO(rjeczalik): add e if e != all?
		diff[0] |= diffInactive[0] | e
		diff[1] |= diffInactive[1] | e
		if diff[0] == diff[1] {
			return none
		}
	}
	return diff
}

// watchTotal TODO(rjeczalik)
func watchTotal(nd node) Event {
	e := nd.Watch.Total()
	if wp := nd.Child[""].Watch; len(wp) != 0 {
		e |= wp.Total()
	}
	return e
}

// watchIsRecursive TODO(rjeczalik)
func watchIsRecursive(nd node) bool {
	ok := nd.Watch.IsRecursive()
	// TODO(rjeczalik): add a test for len(wp) != 0 change the condition.
	if wp := nd.Child[""].Watch; len(wp) != 0 {
		// If a watchpoint holds inactive watchpoints, it means it's a parent
		// one, which is recursive by nature even though it may be not recursive
		// itself.
		ok = true
	}
	return ok
}

// recursiveTree TODO(rjeczalik)
type recursiveTree struct {
	rw   sync.RWMutex // protects root
	root root
	// TODO(rjeczalik): merge watcher + recursiveWatcher after #5 and #6
	w interface {
		watcher
		recursiveWatcher
	}
	c chan EventInfo
}

// newRecursiveTree TODO(rjeczalik)
func newRecursiveTree(w recursiveWatcher, c chan EventInfo) *recursiveTree {
	t := &recursiveTree{
		root: root{nd: newnode("")},
		w: struct {
			watcher
			recursiveWatcher
		}{w.(watcher), w},
		c: c,
	}
	go t.dispatch()
	return t
}

// dispatch TODO(rjeczalik)
func (t *recursiveTree) dispatch() {
	for ei := range t.c {
		dbgprintf("dispatching %v on %q", ei.Event(), ei.Path())
		go func(ei EventInfo) {
			nd, ok := node{}, false
			dir, base := split(ei.Path())
			fn := func(it node, isbase bool) error {
				if isbase {
					nd = it
				} else {
					it.Watch.Dispatch(ei, recursive)
				}
				return nil
			}
			t.rw.RLock()
			defer t.rw.RUnlock()
			// Notify recursive watchpoints found on the path.
			if err := t.root.WalkPath(dir, fn); err != nil {
				dbgprint("dispatch did not reach leaf:", err)
				return
			}
			// Notify parent watchpoint.
			nd.Watch.Dispatch(ei, 0)
			// If leaf watchpoint exists, notify it.
			if nd, ok = nd.Child[base]; ok {
				nd.Watch.Dispatch(ei, 0)
			}
		}(ei)
	}
}

// Watch TODO(rjeczalik)
func (t *recursiveTree) Watch(path string, c chan<- EventInfo, events ...Event) error {
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
	eventset := joinevents(events)
	if isrec {
		eventset |= recursive
	}
	t.rw.Lock()
	defer t.rw.Unlock()
	// case 1: cur is a child
	//
	// Look for parent watch which already covers the given path.
	parent := node{}
	self := false
	err = t.root.WalkPath(path, func(nd node, isbase bool) error {
		if watchTotal(nd) != 0 {
			parent = nd
			self = isbase
			return errSkip
		}
		return nil
	})
	cur := t.root.Add(path) // add after the walk, so it's less to traverse
	if err == nil && parent.Watch != nil {
		// Parent watch found. Register inactive watchpoint, so we have enough
		// information to shrink the eventset on eventual Stop.
		// return t.resetwatchpoint(parent, parent, c, eventset|inactive)
		var diff eventDiff
		if self {
			diff = watchAdd(cur, c, eventset)
		} else {
			diff = watchAddInactive(parent, c, eventset)
		}
		switch {
		case diff == none:
			// the parent watchpoint already covers requested subtree with its
			// eventset
		case diff[0] == 0:
			// TODO(rjeczalik): cleanup this panic after implementation is stable
			panic("dangling watchpoint: " + parent.Name)
		default:
			if isrec || watchIsRecursive(parent) {
				err = t.w.RecursiveRewatch(parent.Name, parent.Name, diff[0], diff[1])
			} else {
				err = t.w.Rewatch(parent.Name, diff[0], diff[1])
			}
			if err != nil {
				watchDel(parent, c, diff.Event())
				return err
			}
			watchAdd(cur, c, eventset)
			// TODO(rjeczalik): account top-most path for c
			return nil
		}
		if !self {
			watchAdd(cur, c, eventset)
		}
		return nil
	}
	// case 2: cur is new parent
	//
	// Look for children nodes, unwatch n-1 of them and rewatch the last one.
	var children []node
	fn := func(nd node) error {
		if len(nd.Watch) == 0 {
			return nil
		}
		children = append(children, nd)
		return errSkip
	}
	switch must(cur.Walk(fn)); len(children) {
	case 0:
		// no child watches, cur holds a new watch
	case 1:
		watchAdd(cur, c, eventset) // TODO(rjeczalik): update cache c subtree root?
		watchCopy(children[0], cur)
		err = t.w.RecursiveRewatch(children[0].Name, cur.Name, watchTotal(children[0]),
			watchTotal(cur))
		if err != nil {
			// Clean inactive watchpoint. The c chan did not exist before.
			cur.Child[""] = node{}
			delete(cur.Watch, c)
			return err
		}
		return nil
	default:
		watchAdd(cur, c, eventset)
		// Copy children inactive watchpoints to the new parent.
		for _, nd := range children {
			watchCopy(nd, cur)
		}
		// Watch parent subtree.
		if err = t.w.RecursiveWatch(cur.Name, watchTotal(cur)); err != nil {
			// Clean inactive watchpoint. The c chan did not exist before.
			cur.Child[""] = node{}
			delete(cur.Watch, c)
			return err
		}
		// Unwatch children subtrees.
		var e error
		for _, nd := range children {
			if watchIsRecursive(nd) {
				e = t.w.RecursiveUnwatch(nd.Name)
			} else {
				e = t.w.Unwatch(nd.Name)
			}
			if e != nil {
				err = nonil(err, e)
				// TODO(rjeczalik): child is still watched, warn all its watchpoints
				// about possible duplicate events via Error event
			}
		}
		return err
	}
	// case 3: cur is new, alone node
	switch diff := watchAdd(cur, c, eventset); {
	case diff == none:
		// TODO(rjeczalik): cleanup this panic after implementation is stable
		panic("watch requested but no parent watchpoint found: " + cur.Name)
	case diff[0] == 0:
		if isrec {
			err = t.w.RecursiveWatch(cur.Name, diff[1])
		} else {
			err = t.w.Watch(cur.Name, diff[1])
		}
		if err != nil {
			watchDel(cur, c, diff.Event())
			return err
		}
	default:
		// TODO(rjeczalik): cleanup this panic after implementation is stable
		panic("watch requested but no parent watchpoint found: " + cur.Name)
	}
	return nil
}

// Stop TODO(rjeczalik)
//
// TODO(rjeczalik): Split parent watchpoint - transfer watches to children
// if parent is no longer needed. This carries a risk that underlying
// watcher calls could fail - reconsider if it's worth the effort.
func (t *recursiveTree) Stop(c chan<- EventInfo) {
	var err error
	fn := func(nd node) (e error) {
		diff := watchDel(nd, c, all)
		switch {
		case diff == none && watchTotal(nd) == 0:
			// TODO(rjeczalik): There's no watchpoints deeper in the tree,
			// probably we should remove the nodes as well.
			return nil
		case diff == none:
			// Removing c from nd does not require shrinking its eventset.
		case diff[1] == 0:
			if watchIsRecursive(nd) {
				e = t.w.RecursiveUnwatch(nd.Name)
			} else {
				e = t.w.Unwatch(nd.Name)
			}
		default:
			if watchIsRecursive(nd) {
				e = t.w.RecursiveRewatch(nd.Name, nd.Name, diff[0], diff[1])
			} else {
				e = t.w.Rewatch(nd.Name, diff[0], diff[1])
			}
		}
		fn := func(nd node) error {
			watchDel(nd, c, all)
			return nil
		}
		err = nonil(err, e, nd.Walk(fn))
		// TODO(rjeczalik): if e != nil store dummy chan in nd.Watch just to
		// retry un/rewatching next time and/or let the user handle the failure
		// vie Error event?
		return errSkip
	}
	t.rw.Lock()
	e := t.root.Walk("", fn) // TODO(rjeczalik): use max root per c
	t.rw.Unlock()
	if e != nil {
		err = nonil(err, e)
	}
	dbgprintf("Stop(%p) error: %v\n", c, err)
}

// Close TODO(rjeczalik)
func (t *recursiveTree) Close() error {
	err := t.w.Close()
	close(t.c)
	return err
}
