// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package itererr

import "iter"

// Collect returns a slice of the items from the iterator, plus the error if
// any.
func Collect[T any](it iter.Seq[T], errFn func() error) ([]T, error) {
	var s []T
	for v := range it {
		s = append(s, v)
	}
	return s, errFn()
}

// Zip interleaves the iterator value with the error. The iteration ends
// after a non-nil error.
func Zip[T any](it iter.Seq[T], errFn func() error) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		for v := range it {
			if !yield(v, nil) {
				break
			}
		}
		if err := errFn(); err != nil {
			var zero T
			yield(zero, err)
		}
	}
}

// Map returns a new iterator by applying the map function, while respecting
// the error function. Additionally, the map function can return an error if
// its own.
func Map[A, B any](i iter.Seq[A], errFn func() error, mapFn func(A) (B, error)) (iter.Seq[B], func() error) {
	var retErr error
	return func(yield func(B) bool) {
			for v := range i {
				mapped, err := mapFn(v)
				if err != nil {
					retErr = err
					return
				}
				if !yield(mapped) {
					return
				}
			}
		}, func() error {
			if prevErr := errFn(); prevErr != nil {
				return prevErr
			}
			return retErr
		}
}

// Map returns a new iterator by applying the map function, while respecting
// the error function. Additionally, the map function can return an error if
// its own.
func Map2[A, B, C any](i iter.Seq[A], errFn func() error, mapFn func(A) (B, C, error)) (iter.Seq2[B, C], func() error) {
	var retErr error
	return func(yield func(B, C) bool) {
			for v := range i {
				ma, mb, err := mapFn(v)
				if err != nil {
					retErr = err
					return
				}
				if !yield(ma, mb) {
					return
				}
			}
		}, func() error {
			if prevErr := errFn(); prevErr != nil {
				return prevErr
			}
			return retErr
		}
}
