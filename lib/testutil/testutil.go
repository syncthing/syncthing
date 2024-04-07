// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package testutil

import (
	"errors"
	"slices"
	"sync"
	"testing"

	"golang.org/x/exp/constraints"
)

var ErrClosed = errors.New("closed")

// BlockingRW implements io.Reader, Writer and Closer, but only returns when closed
type BlockingRW struct {
	c         chan struct{}
	closeOnce sync.Once
}

func NewBlockingRW() *BlockingRW {
	return &BlockingRW{
		c:         make(chan struct{}),
		closeOnce: sync.Once{},
	}
}

func (rw *BlockingRW) Read(_ []byte) (int, error) {
	<-rw.c
	return 0, ErrClosed
}

func (rw *BlockingRW) Write(_ []byte) (int, error) {
	<-rw.c
	return 0, ErrClosed
}

func (rw *BlockingRW) Close() error {
	rw.closeOnce.Do(func() {
		close(rw.c)
	})
	return nil
}

// NoopRW implements io.Reader and Writer but never returns when called
type NoopRW struct{}

func (*NoopRW) Read(p []byte) (n int, err error) {
	return len(p), nil
}

func (*NoopRW) Write(p []byte) (n int, err error) {
	return len(p), nil
}

type NoopCloser struct{}

func (NoopCloser) Close() error {
	return nil
}

func IfExpr[T any](expr bool, then T, els T) T {
	if expr {
		return then
	} else {
		return els
	}
}

func AssertTrue(t *testing.T, testFailFunc func(string, ...any), a bool, sprintfArgs ...any) {
	t.Helper()
	if !a {
		if len(sprintfArgs) == 0 {
			testFailFunc("Assertion failed")
		} else if len(sprintfArgs) == 1 {
			testFailFunc("Assertion failed: %s", sprintfArgs[0])
		} else {
			testFailFunc("Assertion failed: "+sprintfArgs[0].(string), sprintfArgs[1:]...)
		}
	}
}

func AssertFalse(t *testing.T, testFailFunc func(string, ...any), a bool, sprintfArgs ...any) {
	t.Helper()
	AssertTrue(t, testFailFunc, !a, sprintfArgs)
}

func AssertEqual[T comparable](t *testing.T, testFailFunc func(string, ...any), a T, b T, sprintfArgs ...any) {
	t.Helper()
	if a != b {
		if len(sprintfArgs) == 0 {
			testFailFunc("Assertion failed: %v == %v", a, b)
		} else if len(sprintfArgs) == 1 {
			testFailFunc("Assertion failed: %v == %v: %s", a, b, sprintfArgs[0])
		} else {
			testFailFunc("Assertion failed: %v == %v: "+sprintfArgs[0].(string), slices.Concat([]any{a, b}, sprintfArgs[1:])...)
		}
	}
}

func AssertNotEqual[T comparable](t *testing.T, testFailFunc func(string, ...any), a T, b T, sprintfArgs ...any) {
	t.Helper()
	if a == b {
		if len(sprintfArgs) == 0 {
			testFailFunc("Assertion failed: %v != %v", a, b)
		} else if len(sprintfArgs) == 1 {
			testFailFunc("Assertion failed: %v != %v: %s", a, b, sprintfArgs[0])
		} else {
			testFailFunc("Assertion failed: %v != %v: "+sprintfArgs[0].(string), slices.Concat([]any{a, b}, sprintfArgs[1:])...)
		}
	}
}

func AssertGreater[T constraints.Ordered](t *testing.T, testFailFunc func(string, ...any), a T, b T, sprintfArgs ...any) {
	t.Helper()
	if a > b {
		if len(sprintfArgs) == 0 {
			testFailFunc("Assertion failed: %v > %v", a, b)
		} else if len(sprintfArgs) == 1 {
			testFailFunc("Assertion failed: %v > %v: %s", a, b, sprintfArgs[0])
		} else {
			testFailFunc("Assertion failed: %v > %v: "+sprintfArgs[0].(string), slices.Concat([]any{a, b}, sprintfArgs[1:])...)
		}
	}
}

func AssertPredicate[T any](t *testing.T, testFailFunc func(string, ...any), predicate func(T, T) bool, a T, b T, sprintfArgs ...any) {
	t.Helper()
	if !predicate(a, b) {
		if len(sprintfArgs) == 0 {
			testFailFunc("Assertion failed: %s(%v, %v) != true", predicate, a, b)
		} else if len(sprintfArgs) == 1 {
			testFailFunc("Assertion failed: %s(%v, %v) != true: %s", predicate, a, b, sprintfArgs[0])
		} else {
			testFailFunc("Assertion failed: %s(%v, %v) != true: "+sprintfArgs[0].(string), slices.Concat([]any{predicate, a, b}, sprintfArgs[1:])...)
		}
	}
}

func FatalIfErr(t *testing.T, err error, args ...any) {
	t.Helper()
	if err != nil {
		t.Fatal(err, args)
	}
}
