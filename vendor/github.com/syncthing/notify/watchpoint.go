// Copyright (c) 2014-2015 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

package notify

// EventDiff describes a change to an event set - EventDiff[0] is an old state,
// while EventDiff[1] is a new state. If event set has not changed (old == new),
// functions typically return the None value.
type eventDiff [2]Event

func (diff eventDiff) Event() Event {
	return diff[1] &^ diff[0]
}

// Watchpoint
//
// The nil key holds total event set - logical sum for all registered events.
// It speeds up computing EventDiff for Add method.
//
// The rec key holds an event set for a watchpoints created by RecursiveWatch
// for a Watcher implementation which is not natively recursive.
type watchpoint map[chan<- EventInfo]Event

// None is an empty event diff, think null object.
var none eventDiff

// rec is just a placeholder
var rec = func() (ch chan<- EventInfo) {
	ch = make(chan<- EventInfo)
	close(ch)
	return
}()

func (wp watchpoint) dryAdd(ch chan<- EventInfo, e Event) eventDiff {
	if e &^= internal; wp[ch]&e == e {
		return none
	}
	total := wp[ch] &^ internal
	return eventDiff{total, total | e}
}

// Add assumes neither c nor e are nil or zero values.
func (wp watchpoint) Add(c chan<- EventInfo, e Event) (diff eventDiff) {
	wp[c] |= e
	diff[0] = wp[nil]
	diff[1] = diff[0] | e
	wp[nil] = diff[1] &^ omit
	// Strip diff from internal events.
	diff[0] &^= internal
	diff[1] &^= internal
	if diff[0] == diff[1] {
		return none
	}
	return
}

func (wp watchpoint) Del(c chan<- EventInfo, e Event) (diff eventDiff) {
	wp[c] &^= e
	if wp[c] == 0 {
		delete(wp, c)
	}
	diff[0] = wp[nil]
	delete(wp, nil)
	if len(wp) != 0 {
		// Recalculate total event set.
		for _, e := range wp {
			diff[1] |= e
		}
		wp[nil] = diff[1] &^ omit
	}
	// Strip diff from internal events.
	diff[0] &^= internal
	diff[1] &^= internal
	if diff[0] == diff[1] {
		return none
	}
	return
}

func (wp watchpoint) Dispatch(ei EventInfo, extra Event) {
	e := eventmask(ei, extra)
	if !matches(wp[nil], e) {
		return
	}
	for ch, eset := range wp {
		if ch != nil && matches(eset, e) {
			select {
			case ch <- ei:
			default: // Drop event if receiver is too slow
				dbgprintf("dropped %s on %q: receiver too slow", ei.Event(), ei.Path())
			}
		}
	}
}

func (wp watchpoint) Total() Event {
	return wp[nil] &^ internal
}

func (wp watchpoint) IsRecursive() bool {
	return wp[nil]&recursive != 0
}
