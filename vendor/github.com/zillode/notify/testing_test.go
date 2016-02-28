// Copyright (c) 2014-2015 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

package notify

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// NOTE(rjeczalik): some useful environment variables:
//
//   - NOTIFY_DEBUG gives some extra information about generated events
//   - NOTIFY_TIMEOUT allows for changing default wait time for watcher's
//     events
//   - NOTIFY_TMP allows for changing location of temporary directory trees
//     created for test purpose

var wd string

func init() {
	var err error
	if wd, err = os.Getwd(); err != nil {
		panic("Getwd()=" + err.Error())
	}
}

func timeout() time.Duration {
	if s := os.Getenv("NOTIFY_TIMEOUT"); s != "" {
		if t, err := time.ParseDuration(s); err == nil {
			return t
		}
	}
	return 2 * time.Second
}

func vfs() (string, string) {
	if s := os.Getenv("NOTIFY_TMP"); s != "" {
		return filepath.Split(s)
	}
	return "testdata", ""
}

func isDir(path string) bool {
	r := path[len(path)-1]
	return r == '\\' || r == '/'
}

func tmpcreateall(tmp string, path string) error {
	isdir := isDir(path)
	path = filepath.Join(tmp, filepath.FromSlash(path))
	if isdir {
		if err := os.MkdirAll(path, 0755); err != nil {
			return err
		}
	} else {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		if err := nonil(f.Sync(), f.Close()); err != nil {
			return err
		}
	}
	return nil
}

func tmpcreate(root, path string) (bool, error) {
	isdir := isDir(path)
	path = filepath.Join(root, filepath.FromSlash(path))
	if isdir {
		if err := os.Mkdir(path, 0755); err != nil {
			return false, err
		}
	} else {
		f, err := os.Create(path)
		if err != nil {
			return false, err
		}
		if err := nonil(f.Sync(), f.Close()); err != nil {
			return false, err
		}
	}
	return isdir, nil
}

func tmptree(root, list string) (string, error) {
	f, err := os.Open(list)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if root == "" {
		if root, err = ioutil.TempDir(vfs()); err != nil {
			return "", err
		}
	}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if err := tmpcreateall(root, scanner.Text()); err != nil {
			return "", err
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return root, nil
}

func callern(n int) string {
	_, file, line, ok := runtime.Caller(n)
	if !ok {
		return "<unknown>"
	}
	return filepath.Base(file) + ":" + strconv.Itoa(line)
}

func caller() string {
	return callern(3)
}

type WCase struct {
	Action func()
	Events []EventInfo
}

func (cas WCase) String() string {
	s := make([]string, 0, len(cas.Events))
	for _, ei := range cas.Events {
		s = append(s, "Event("+ei.Event().String()+")@"+filepath.FromSlash(ei.Path()))
	}
	return strings.Join(s, ", ")
}

type W struct {
	Watcher watcher
	C       chan EventInfo
	Timeout time.Duration

	t    *testing.T
	root string
}

func newWatcherTest(t *testing.T, tree string) *W {
	root, err := tmptree("", filepath.FromSlash(tree))
	if err != nil {
		t.Fatalf(`tmptree("", %q)=%v`, tree, err)
	}
	Sync()
	return &W{
		t:    t,
		root: root,
	}
}

func NewWatcherTest(t *testing.T, tree string, events ...Event) *W {
	w := newWatcherTest(t, tree)
	if len(events) == 0 {
		events = []Event{Create, Remove, Write, Rename}
	}
	if rw, ok := w.watcher().(recursiveWatcher); ok {
		if err := rw.RecursiveWatch(w.root, joinevents(events)); err != nil {
			t.Fatalf("RecursiveWatch(%q, All)=%v", w.root, err)
		}
	} else {
		fn := func(path string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if fi.IsDir() {
				if err := w.watcher().Watch(path, joinevents(events)); err != nil {
					return err
				}
			}
			return nil
		}
		if err := filepath.Walk(w.root, fn); err != nil {
			t.Fatalf("Walk(%q, fn)=%v", w.root, err)
		}
	}
	drainall(w.C)
	return w
}

func (w *W) clean(path string) string {
	path, isrec, err := cleanpath(filepath.Join(w.root, path))
	if err != nil {
		w.Fatalf("cleanpath(%q)=%v", path, err)
	}
	if isrec {
		path = path + "..."
	}
	return path
}

func (w *W) Fatal(v interface{}) {
	w.t.Fatalf("%s: %v", caller(), v)
}

func (w *W) Fatalf(format string, v ...interface{}) {
	w.t.Fatalf("%s: %s", caller(), fmt.Sprintf(format, v...))
}

func (w *W) Watch(path string, e Event) {
	if err := w.watcher().Watch(w.clean(path), e); err != nil {
		w.Fatalf("Watch(%s, %v)=%v", path, e, err)
	}
}

func (w *W) Rewatch(path string, olde, newe Event) {
	if err := w.watcher().Rewatch(w.clean(path), olde, newe); err != nil {
		w.Fatalf("Rewatch(%s, %v, %v)=%v", path, olde, newe, err)
	}
}

func (w *W) Unwatch(path string) {
	if err := w.watcher().Unwatch(w.clean(path)); err != nil {
		w.Fatalf("Unwatch(%s)=%v", path, err)
	}
}

func (w *W) RecursiveWatch(path string, e Event) {
	rw, ok := w.watcher().(recursiveWatcher)
	if !ok {
		w.Fatal("watcher does not implement recursive watching on this platform")
	}
	if err := rw.RecursiveWatch(w.clean(path), e); err != nil {
		w.Fatalf("RecursiveWatch(%s, %v)=%v", path, e, err)
	}
}

func (w *W) RecursiveRewatch(oldp, newp string, olde, newe Event) {
	rw, ok := w.watcher().(recursiveWatcher)
	if !ok {
		w.Fatal("watcher does not implement recursive watching on this platform")
	}
	if err := rw.RecursiveRewatch(w.clean(oldp), w.clean(newp), olde, newe); err != nil {
		w.Fatalf("RecursiveRewatch(%s, %s, %v, %v)=%v", oldp, newp, olde, newe, err)
	}
}

func (w *W) RecursiveUnwatch(path string) {
	rw, ok := w.watcher().(recursiveWatcher)
	if !ok {
		w.Fatal("watcher does not implement recursive watching on this platform")
	}
	if err := rw.RecursiveUnwatch(w.clean(path)); err != nil {
		w.Fatalf("RecursiveUnwatch(%s)=%v", path, err)
	}
}

func (w *W) initwatcher(buffer int) {
	c := make(chan EventInfo, buffer)
	w.Watcher = newWatcher(c)
	w.C = c
}

func (w *W) watcher() watcher {
	if w.Watcher == nil {
		w.initwatcher(512)
	}
	return w.Watcher
}

func (w *W) c() chan EventInfo {
	if w.C == nil {
		w.initwatcher(512)
	}
	return w.C
}

func (w *W) timeout() time.Duration {
	if w.Timeout != 0 {
		return w.Timeout
	}
	return timeout()
}

func (w *W) Close() error {
	defer os.RemoveAll(w.root)
	if err := w.watcher().Close(); err != nil {
		w.Fatalf("w.Watcher.Close()=%v", err)
	}
	return nil
}

func EqualEventInfo(want, got EventInfo) error {
	if got.Event() != want.Event() {
		return fmt.Errorf("want Event()=%v; got %v (path=%s)", want.Event(),
			got.Event(), want.Path())
	}
	path := strings.TrimRight(filepath.FromSlash(want.Path()), `/\`)
	if !strings.HasSuffix(got.Path(), path) {
		return fmt.Errorf("want Path()=%s; got %s (event=%v)", path, got.Path(),
			want.Event())
	}
	return nil
}

func HasEventInfo(want, got Event, p string) error {
	if got&want != want {
		return fmt.Errorf("want Event=%v; got %v (path=%s)", want,
			got, p)
	}
	return nil
}

func EqualCall(want, got Call) error {
	if want.F != got.F {
		return fmt.Errorf("want F=%v; got %v (want.P=%q, got.P=%q)", want.F, got.F, want.P, got.P)
	}
	if got.E != want.E {
		return fmt.Errorf("want E=%v; got %v (want.P=%q, got.P=%q)", want.E, got.E, want.P, got.P)
	}
	if got.NE != want.NE {
		return fmt.Errorf("want NE=%v; got %v (want.P=%q, got.P=%q)", want.NE, got.NE, want.P, got.P)
	}
	if want.C != got.C {
		return fmt.Errorf("want C=%p; got %p (want.P=%q, got.P=%q)", want.C, got.C, want.P, got.P)
	}
	if want := filepath.FromSlash(want.P); !strings.HasSuffix(got.P, want) {
		return fmt.Errorf("want P=%s; got %s", want, got.P)
	}
	if want := filepath.FromSlash(want.NP); !strings.HasSuffix(got.NP, want) {
		return fmt.Errorf("want NP=%s; got %s", want, got.NP)
	}
	return nil
}

func create(w *W, path string) WCase {
	return WCase{
		Action: func() {
			isdir, err := tmpcreate(w.root, filepath.FromSlash(path))
			if err != nil {
				w.Fatalf("tmpcreate(%q, %q)=%v", w.root, path, err)
			}
			if isdir {
				dbgprintf("[FS] os.Mkdir(%q)\n", path)
			} else {
				dbgprintf("[FS] os.Create(%q)\n", path)
			}
		},
		Events: []EventInfo{
			&Call{P: path, E: Create},
		},
	}
}

func remove(w *W, path string) WCase {
	return WCase{
		Action: func() {
			if err := os.RemoveAll(filepath.Join(w.root, filepath.FromSlash(path))); err != nil {
				w.Fatal(err)
			}
			dbgprintf("[FS] os.Remove(%q)\n", path)
		},
		Events: []EventInfo{
			&Call{P: path, E: Remove},
		},
	}
}

func rename(w *W, oldpath, newpath string) WCase {
	return WCase{
		Action: func() {
			err := os.Rename(filepath.Join(w.root, filepath.FromSlash(oldpath)),
				filepath.Join(w.root, filepath.FromSlash(newpath)))
			if err != nil {
				w.Fatal(err)
			}
			dbgprintf("[FS] os.Rename(%q, %q)\n", oldpath, newpath)
		},
		Events: []EventInfo{
			&Call{P: newpath, E: Rename},
		},
	}
}

func write(w *W, path string, p []byte) WCase {
	return WCase{
		Action: func() {
			f, err := os.OpenFile(filepath.Join(w.root, filepath.FromSlash(path)),
				os.O_WRONLY, 0644)
			if err != nil {
				w.Fatalf("OpenFile(%q)=%v", path, err)
			}
			if _, err := f.Write(p); err != nil {
				w.Fatalf("Write(%q)=%v", path, err)
			}
			if err := nonil(f.Sync(), f.Close()); err != nil {
				w.Fatalf("Sync(%q)/Close(%q)=%v", path, path, err)
			}
			dbgprintf("[FS] Write(%q)\n", path)
		},
		Events: []EventInfo{
			&Call{P: path, E: Write},
		},
	}
}

func drainall(c chan EventInfo) (ei []EventInfo) {
	time.Sleep(50 * time.Millisecond)
	for {
		select {
		case e := <-c:
			ei = append(ei, e)
			runtime.Gosched()
		default:
			return
		}
	}
}

type WCaseFunc func(i int, cas WCase, ei EventInfo) error

func (w *W) ExpectAnyFunc(cases []WCase, fn WCaseFunc) {
	UpdateWait() // Wait some time before starting the test.
Test:
	for i, cas := range cases {
		dbgprintf("ExpectAny: i=%d\n", i)
		cas.Action()
		Sync()
		switch cas.Events {
		case nil:
			if ei := drainall(w.C); len(ei) != 0 {
				w.Fatalf("unexpected dangling events: %v (i=%d)", ei, i)
			}
		default:
			select {
			case ei := <-w.C:
				dbgprintf("received: path=%q, event=%v, sys=%v (i=%d)", ei.Path(),
					ei.Event(), ei.Sys(), i)
				for j, want := range cas.Events {
					if err := EqualEventInfo(want, ei); err != nil {
						dbgprint(err, j)
						continue
					}
					if fn != nil {
						if err := fn(i, cas, ei); err != nil {
							w.Fatalf("ExpectAnyFunc(%d, %v)=%v", i, ei, err)
						}
					}
					drainall(w.C) // TODO(rjeczalik): revisit
					continue Test
				}
				w.Fatalf("ExpectAny received an event which does not match any of "+
					"the expected ones (i=%d): want one of %v; got %v", i, cas.Events, ei)
			case <-time.After(w.timeout()):
				w.Fatalf("timed out after %v waiting for one of %v (i=%d)", w.timeout(),
					cas.Events, i)
			}
			drainall(w.C) // TODO(rjeczalik): revisit
		}
	}
}

func (w *W) ExpectAny(cases []WCase) {
	w.ExpectAnyFunc(cases, nil)
}

func (w *W) aggregate(ei []EventInfo, pf string) (evs map[string]Event) {
	evs = make(map[string]Event)
	for _, cas := range ei {
		p := cas.Path()
		if pf != "" {
			p = filepath.Join(pf, p)
		}
		evs[p] |= cas.Event()
	}
	return
}

func (w *W) ExpectAllFunc(cases []WCase) {
	UpdateWait() // Wait some time before starting the test.
	for i, cas := range cases {
		exp := w.aggregate(cas.Events, w.root)
		dbgprintf("ExpectAll: i=%d\n", i)
		cas.Action()
		Sync()
		got := w.aggregate(drainall(w.C), "")
		for ep, ee := range exp {
			ge, ok := got[ep]
			if !ok {
				w.Fatalf("missing events for %q (%v)", ep, ee)
				continue
			}
			delete(got, ep)
			if err := HasEventInfo(ee, ge, ep); err != nil {
				w.Fatalf("ExpectAll received an event which does not match "+
					"the expected ones for %q: want %v; got %v", ep, ee, ge)
				continue
			}
		}
		if len(got) != 0 {
			w.Fatalf("ExpectAll received unexpected events: %v", got)
		}
	}
}

// ExpectAll requires all requested events to be send.
// It does not require events to be send in the same order or in the same
// chunks (e.g. NoteWrite and NoteExtend reported as independent events are
// treated the same as one NoteWrite|NoteExtend event).
func (w *W) ExpectAll(cases []WCase) {
	w.ExpectAllFunc(cases)
}

// FuncType represents enums for Watcher interface.
type FuncType string

const (
	FuncWatch            = FuncType("Watch")
	FuncUnwatch          = FuncType("Unwatch")
	FuncRewatch          = FuncType("Rewatch")
	FuncRecursiveWatch   = FuncType("RecursiveWatch")
	FuncRecursiveUnwatch = FuncType("RecursiveUnwatch")
	FuncRecursiveRewatch = FuncType("RecursiveRewatch")
	FuncStop             = FuncType("Stop")
)

type Chans []chan EventInfo

func NewChans(n int) Chans {
	ch := make([]chan EventInfo, n)
	for i := range ch {
		ch[i] = make(chan EventInfo, buffer)
	}
	return ch
}

func (c Chans) Foreach(fn func(chan<- EventInfo, node)) {
	for i, ch := range c {
		fn(ch, node{Name: strconv.Itoa(i)})
	}
}

func (c Chans) Drain() (ei []EventInfo) {
	n := len(c)
	stop := make(chan struct{})
	eich := make(chan EventInfo, n*buffer)
	go func() {
		defer close(eich)
		cases := make([]reflect.SelectCase, n+1)
		for i := range c {
			cases[i].Chan = reflect.ValueOf(c[i])
			cases[i].Dir = reflect.SelectRecv
		}
		cases[n].Chan = reflect.ValueOf(stop)
		cases[n].Dir = reflect.SelectRecv
		for {
			i, v, ok := reflect.Select(cases)
			if i == n {
				return
			}
			if !ok {
				panic("(Chans).Drain(): unexpected chan close")
			}
			eich <- v.Interface().(EventInfo)
		}
	}()
	<-time.After(50 * time.Duration(n) * time.Millisecond)
	close(stop)
	for e := range eich {
		ei = append(ei, e)
	}
	return
}

// Call represents single call to Watcher issued by the Tree
// and recorded by a spy Watcher mock.
type Call struct {
	F   FuncType       // denotes type of function to call, for both watcher and notifier interface
	C   chan EventInfo // user channel being an argument to either Watch or Stop function
	P   string         // regular Path argument and old path from RecursiveRewatch call
	NP  string         // new Path argument from RecursiveRewatch call
	E   Event          // regular Event argument and old Event from a Rewatch call
	NE  Event          // new Event argument from Rewatch call
	S   interface{}    // when Call is used as EventInfo, S is a value of Sys()
	Dir bool           // when Call is used as EventInfo, Dir is a value of isDir()
}

// Call implements the EventInfo interface.
func (c *Call) Event() Event         { return c.E }
func (c *Call) Path() string         { return c.P }
func (c *Call) String() string       { return fmt.Sprintf("%#v", c) }
func (c *Call) Sys() interface{}     { return c.S }
func (c *Call) isDir() (bool, error) { return c.Dir, nil }

// CallSlice is a conveniance wrapper for a slice of Call values, which allows
// to sort them in ascending order.
type CallSlice []Call

// CallSlice implements sort.Interface inteface.
func (cs CallSlice) Len() int           { return len(cs) }
func (cs CallSlice) Less(i, j int) bool { return cs[i].P < cs[j].P }
func (cs CallSlice) Swap(i, j int)      { cs[i], cs[j] = cs[j], cs[i] }
func (cs CallSlice) Sort()              { sort.Sort(cs) }

// Spy is a mock for Watcher interface, which records every call.
type Spy []Call

func (s *Spy) Close() (_ error) { return }

func (s *Spy) Watch(p string, e Event) (_ error) {
	dbgprintf("%s: (*Spy).Watch(%q, %v)", caller(), p, e)
	*s = append(*s, Call{F: FuncWatch, P: p, E: e})
	return
}

func (s *Spy) Unwatch(p string) (_ error) {
	dbgprintf("%s: (*Spy).Unwatch(%q)", caller(), p)
	*s = append(*s, Call{F: FuncUnwatch, P: p})
	return
}

func (s *Spy) Rewatch(p string, olde, newe Event) (_ error) {
	dbgprintf("%s: (*Spy).Rewatch(%q, %v, %v)", caller(), p, olde, newe)
	*s = append(*s, Call{F: FuncRewatch, P: p, E: olde, NE: newe})
	return
}

func (s *Spy) RecursiveWatch(p string, e Event) (_ error) {
	dbgprintf("%s: (*Spy).RecursiveWatch(%q, %v)", caller(), p, e)
	*s = append(*s, Call{F: FuncRecursiveWatch, P: p, E: e})
	return
}

func (s *Spy) RecursiveUnwatch(p string) (_ error) {
	dbgprintf("%s: (*Spy).RecursiveUnwatch(%q)", caller(), p)
	*s = append(*s, Call{F: FuncRecursiveUnwatch, P: p})
	return
}

func (s *Spy) RecursiveRewatch(oldp, newp string, olde, newe Event) (_ error) {
	dbgprintf("%s: (*Spy).RecursiveRewatch(%q, %q, %v, %v)", caller(), oldp, newp, olde, newe)
	*s = append(*s, Call{F: FuncRecursiveRewatch, P: oldp, NP: newp, E: olde, NE: newe})
	return
}

type RCase struct {
	Call   Call
	Record []Call
}

type TCase struct {
	Event    Call
	Receiver Chans
}

type NCase struct {
	Event    WCase
	Receiver Chans
}

type N struct {
	Timeout time.Duration

	t    *testing.T
	tree tree
	w    *W
	spy  *Spy
	c    chan EventInfo
	j    int // spy offset

	realroot string
}

func newN(t *testing.T, tree string) *N {
	n := &N{
		t: t,
		w: newWatcherTest(t, tree),
	}
	realroot, err := canonical(n.w.root)
	if err != nil {
		t.Fatalf("%s: unexpected fixture failure: %v", caller(), err)
	}
	n.realroot = realroot
	return n
}

func newTreeN(t *testing.T, tree string) *N {
	c := make(chan EventInfo, buffer)
	n := newN(t, tree)
	n.spy = &Spy{}
	n.w.Watcher = n.spy
	n.w.C = c
	n.c = c
	return n
}

func NewNotifyTest(t *testing.T, tree string) *N {
	n := newN(t, tree)
	if rw, ok := n.w.watcher().(recursiveWatcher); ok {
		n.tree = newRecursiveTree(rw, n.w.c())
	} else {
		n.tree = newNonrecursiveTree(n.w.watcher(), n.w.c(), nil)
	}
	return n
}

func NewRecursiveTreeTest(t *testing.T, tree string) *N {
	n := newTreeN(t, tree)
	n.tree = newRecursiveTree(n.spy, n.c)
	return n
}

func NewNonrecursiveTreeTest(t *testing.T, tree string) *N {
	n := newTreeN(t, tree)
	n.tree = newNonrecursiveTree(n.spy, n.c, nil)
	return n
}

func NewNonrecursiveTreeTestC(t *testing.T, tree string) (*N, chan EventInfo) {
	rec := make(chan EventInfo, buffer)
	recinternal := make(chan EventInfo, buffer)
	recuser := make(chan EventInfo, buffer)
	go func() {
		for ei := range rec {
			select {
			case recinternal <- ei:
			default:
				t.Fatalf("failed to send ei to recinternal: not ready")
			}
			select {
			case recuser <- ei:
			default:
				t.Fatalf("failed to send ei to recuser: not ready")
			}
		}
	}()
	n := newTreeN(t, tree)
	tr := newNonrecursiveTree(n.spy, n.c, recinternal)
	tr.rec = rec
	n.tree = tr
	return n, recuser
}

func (n *N) timeout() time.Duration {
	if n.Timeout != 0 {
		return n.Timeout
	}
	return n.w.timeout()
}

func (n *N) W() *W {
	return n.w
}

func (n *N) Close() error {
	defer os.RemoveAll(n.w.root)
	if err := n.tree.Close(); err != nil {
		n.w.Fatalf("(notifier).Close()=%v", err)
	}
	return nil
}

func (n *N) Watch(path string, c chan<- EventInfo, events ...Event) {
	UpdateWait() // we need to wait on Windows because of its asynchronous watcher.
	path = filepath.Join(n.w.root, path)
	if err := n.tree.Watch(path, c, events...); err != nil {
		n.t.Errorf("Watch(%s, %p, %v)=%v", path, c, events, err)
	}
}

func (n *N) WatchErr(path string, c chan<- EventInfo, err error, events ...Event) {
	path = filepath.Join(n.w.root, path)
	switch e := n.tree.Watch(path, c, events...); {
	case err == nil && e == nil:
		n.t.Errorf("Watch(%s, %p, %v)=nil", path, c, events)
	case err != nil && e != err:
		n.t.Errorf("Watch(%s, %p, %v)=%v != %v", path, c, events, e, err)
	}
}

func (n *N) Stop(c chan<- EventInfo) {
	n.tree.Stop(c)
}

func (n *N) Call(calls ...Call) {
	for i := range calls {
		switch calls[i].F {
		case FuncWatch:
			n.Watch(calls[i].P, calls[i].C, calls[i].E)
		case FuncStop:
			n.Stop(calls[i].C)
		default:
			panic("unsupported call type: " + string(calls[i].F))
		}
	}
}

func (n *N) expectDry(ch Chans, i int) {
	if ei := ch.Drain(); len(ei) != 0 {
		n.w.Fatalf("unexpected dangling events: %v (i=%d)", ei, i)
	}
}

func (n *N) ExpectRecordedCalls(cases []RCase) {
	for i, cas := range cases {
		dbgprintf("ExpectRecordedCalls: i=%d\n", i)
		n.Call(cas.Call)
		record := (*n.spy)[n.j:]
		if len(cas.Record) == 0 && len(record) == 0 {
			continue
		}
		n.j = len(*n.spy)
		if len(record) != len(cas.Record) {
			n.t.Fatalf("%s: want len(record)=%d; got %d [%+v] (i=%d)", caller(),
				len(cas.Record), len(record), record, i)
		}
		CallSlice(record).Sort()
		for j := range cas.Record {
			if err := EqualCall(cas.Record[j], record[j]); err != nil {
				n.t.Fatalf("%s: %v (i=%d, j=%d)", caller(), err, i, j)
			}
		}
	}
}

func (n *N) collect(ch Chans) <-chan []EventInfo {
	done := make(chan []EventInfo)
	go func() {
		cases := make([]reflect.SelectCase, len(ch))
		unique := make(map[<-chan EventInfo]EventInfo, len(ch))
		for i := range ch {
			cases[i].Chan = reflect.ValueOf(ch[i])
			cases[i].Dir = reflect.SelectRecv
		}
		for i := len(cases); i != 0; i = len(cases) {
			j, v, ok := reflect.Select(cases)
			if !ok {
				n.t.Fatal("unexpected chan close")
			}
			ch := cases[j].Chan.Interface().(chan EventInfo)
			got := v.Interface().(EventInfo)
			if ei, ok := unique[ch]; ok {
				n.t.Fatalf("duplicated event %v (previous=%v) received on collect", got, ei)
			}
			unique[ch] = got
			cases[j], cases = cases[i-1], cases[:i-1]
		}
		collected := make([]EventInfo, 0, len(ch))
		for _, ch := range unique {
			collected = append(collected, ch)
		}
		done <- collected
	}()
	return done
}

func (n *N) abs(rel Call) *Call {
	rel.P = filepath.Join(n.realroot, filepath.FromSlash(rel.P))
	if !filepath.IsAbs(rel.P) {
		rel.P = filepath.Join(wd, rel.P)
	}
	return &rel
}

func (n *N) ExpectTreeEvents(cases []TCase, all Chans) {
	for i, cas := range cases {
		dbgprintf("ExpectTreeEvents: i=%d\n", i)
		// Ensure there're no dangling event left by previous test-case.
		n.expectDry(all, i)
		n.c <- n.abs(cas.Event)
		switch cas.Receiver {
		case nil:
			n.expectDry(all, i)
		default:
			ch := n.collect(cas.Receiver)
			select {
			case collected := <-ch:
				for _, got := range collected {
					if err := EqualEventInfo(&cas.Event, got); err != nil {
						n.w.Fatalf("%s: %s (i=%d)", caller(), err, i)
					}
				}
			case <-time.After(n.timeout()):
				n.w.Fatalf("ExpectTreeEvents has timed out after %v waiting for"+
					" %v on %s (i=%d)", n.timeout(), cas.Event.E, cas.Event.P, i)
			}

		}
	}
	n.expectDry(all, -1)
}

func (n *N) ExpectNotifyEvents(cases []NCase, all Chans) {
	UpdateWait() // Wait some time before starting the test.
	for i, cas := range cases {
		dbgprintf("ExpectNotifyEvents: i=%d\n", i)
		cas.Event.Action()
		Sync()
		switch cas.Receiver {
		case nil:
			n.expectDry(all, i)
		default:
			ch := n.collect(cas.Receiver)
			select {
			case collected := <-ch:
			Compare:
				for j, ei := range collected {
					dbgprintf("received: path=%q, event=%v, sys=%v (i=%d, j=%d)", ei.Path(),
						ei.Event(), ei.Sys(), i, j)
					for _, want := range cas.Event.Events {
						if err := EqualEventInfo(want, ei); err != nil {
							dbgprint(err, j)
							continue
						}
						continue Compare
					}
					n.w.Fatalf("ExpectNotifyEvents received an event which does not"+
						" match any of the expected ones (i=%d): want one of %v; got %v", i,
						cas.Event.Events, ei)
				}
			case <-time.After(n.timeout()):
				n.w.Fatalf("ExpectNotifyEvents did not receive any of the expected events [%v] "+
					"after %v (i=%d)", cas.Event, n.timeout(), i)
			}
		}
	}
	n.expectDry(all, -1)
}

func (n *N) Walk(fn walkFunc) {
	switch t := n.tree.(type) {
	case *recursiveTree:
		if err := t.root.Walk("", fn); err != nil {
			n.w.Fatal(err)
		}
	case *nonrecursiveTree:
		if err := t.root.Walk("", fn); err != nil {
			n.w.Fatal(err)
		}
	default:
		n.t.Fatal("unknown tree type")
	}
}
