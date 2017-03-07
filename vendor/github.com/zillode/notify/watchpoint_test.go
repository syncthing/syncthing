// Copyright (c) 2014-2015 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

package notify

import (
	"fmt"
	"reflect"
	"testing"
)

func call(wp watchpoint, fn interface{}, args []interface{}) eventDiff {
	vals := []reflect.Value{reflect.ValueOf(wp)}
	for _, arg := range args {
		vals = append(vals, reflect.ValueOf(arg))
	}
	res := reflect.ValueOf(fn).Call(vals)
	if n := len(res); n != 1 {
		panic(fmt.Sprintf("unexpected len(res)=%d", n))
	}
	diff, ok := res[0].Interface().(eventDiff)
	if !ok {
		panic(fmt.Sprintf("want typeof(diff)=EventDiff; got %T", res[0].Interface()))
	}
	return diff
}

func TestWatchpoint(t *testing.T) {
	ch := NewChans(5)
	all := All | recursive
	cases := [...]struct {
		fn    interface{}
		args  []interface{}
		diff  eventDiff
		total Event
	}{
		// i=0
		{
			watchpoint.Add,
			[]interface{}{ch[0], Remove},
			eventDiff{0, Remove},
			Remove,
		},
		// i=1
		{
			watchpoint.Add,
			[]interface{}{ch[1], Create | Remove | recursive},
			eventDiff{Remove, Remove | Create},
			Create | Remove | recursive,
		},
		// i=2
		{
			watchpoint.Add,
			[]interface{}{ch[2], Create | Rename},
			eventDiff{Create | Remove, Create | Remove | Rename},
			Create | Remove | Rename | recursive,
		},
		// i=3
		{
			watchpoint.Add,
			[]interface{}{ch[0], Write | recursive},
			eventDiff{Create | Remove | Rename, Create | Remove | Rename | Write},
			Create | Remove | Rename | Write | recursive,
		},
		// i=4
		{
			watchpoint.Add,
			[]interface{}{ch[2], Remove | recursive},
			none,
			Create | Remove | Rename | Write | recursive,
		},
		// i=5
		{
			watchpoint.Del,
			[]interface{}{ch[0], all},
			eventDiff{Create | Remove | Rename | Write, Create | Remove | Rename},
			Create | Remove | Rename | recursive,
		},
		// i=6
		{
			watchpoint.Del,
			[]interface{}{ch[2], all},
			eventDiff{Create | Remove | Rename, Create | Remove},
			Create | Remove | recursive,
		},
		// i=7
		{
			watchpoint.Add,
			[]interface{}{ch[3], Create | Remove},
			none,
			Create | Remove | recursive,
		},
		// i=8
		{
			watchpoint.Del,
			[]interface{}{ch[1], all},
			none,
			Create | Remove,
		},
		// i=9
		{
			watchpoint.Add,
			[]interface{}{ch[3], recursive | Write},
			eventDiff{Create | Remove, Create | Remove | Write},
			Create | Remove | Write | recursive,
		},
		// i=10
		{
			watchpoint.Del,
			[]interface{}{ch[3], Create},
			eventDiff{Create | Remove | Write, Remove | Write},
			Remove | Write | recursive,
		},
		// i=11
		{
			watchpoint.Add,
			[]interface{}{ch[3], Create | Rename},
			eventDiff{Remove | Write, Create | Remove | Rename | Write},
			Create | Remove | Rename | Write | recursive,
		},
		// i=12
		{
			watchpoint.Add,
			[]interface{}{ch[2], Remove | Write},
			none,
			Create | Remove | Rename | Write | recursive,
		},
		// i=13
		{
			watchpoint.Del,
			[]interface{}{ch[3], Create | Remove | Write},
			eventDiff{Create | Remove | Rename | Write, Remove | Rename | Write},
			Remove | Rename | Write | recursive,
		},
		// i=14
		{
			watchpoint.Del,
			[]interface{}{ch[2], Remove},
			eventDiff{Remove | Rename | Write, Rename | Write},
			Rename | Write | recursive,
		},
		// i=15
		{
			watchpoint.Del,
			[]interface{}{ch[3], Rename | recursive},
			eventDiff{Rename | Write, Write},
			Write,
		},
	}
	wp := watchpoint{}
	for i, cas := range cases {
		if diff := call(wp, cas.fn, cas.args); diff != cas.diff {
			t.Errorf("want diff=%v; got %v (i=%d)", cas.diff, diff, i)
			continue
		}
		if total := wp[nil]; total != cas.total {
			t.Errorf("want total=%v; got %v (i=%d)", cas.total, total, i)
			continue
		}
	}
}
