// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package util

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/sync"

	"github.com/thejerf/suture"
)

type defaultParser interface {
	ParseDefault(string) error
}

// SetDefaults sets default values on a struct, based on the default annotation.
func SetDefaults(data interface{}) {
	s := reflect.ValueOf(data).Elem()
	t := s.Type()

	for i := 0; i < s.NumField(); i++ {
		f := s.Field(i)
		tag := t.Field(i).Tag

		v := tag.Get("default")
		if len(v) > 0 {
			if f.CanInterface() {
				if parser, ok := f.Interface().(defaultParser); ok {
					if err := parser.ParseDefault(v); err != nil {
						panic(err)
					}
					continue
				}
			}

			if f.CanAddr() && f.Addr().CanInterface() {
				if parser, ok := f.Addr().Interface().(defaultParser); ok {
					if err := parser.ParseDefault(v); err != nil {
						panic(err)
					}
					continue
				}
			}

			switch f.Interface().(type) {
			case string:
				f.SetString(v)

			case int:
				i, err := strconv.ParseInt(v, 10, 64)
				if err != nil {
					panic(err)
				}
				f.SetInt(i)

			case float64:
				i, err := strconv.ParseFloat(v, 64)
				if err != nil {
					panic(err)
				}
				f.SetFloat(i)

			case bool:
				f.SetBool(v == "true")

			case []string:
				// We don't do anything with string slices here. Any default
				// we set will be appended to by the XML decoder, so we fill
				// those after decoding.

			default:
				panic(f.Type())
			}
		}
	}
}

// CopyMatchingTag copies fields tagged tag:"value" from "from" struct onto "to" struct.
func CopyMatchingTag(from interface{}, to interface{}, tag string, shouldCopy func(value string) bool) {
	fromStruct := reflect.ValueOf(from).Elem()
	fromType := fromStruct.Type()

	toStruct := reflect.ValueOf(to).Elem()
	toType := toStruct.Type()

	if fromType != toType {
		panic(fmt.Sprintf("non equal types: %s != %s", fromType, toType))
	}

	for i := 0; i < toStruct.NumField(); i++ {
		fromField := fromStruct.Field(i)
		toField := toStruct.Field(i)

		if !toField.CanSet() {
			// Unexported fields
			continue
		}

		structTag := toType.Field(i).Tag

		v := structTag.Get(tag)
		if shouldCopy(v) {
			toField.Set(fromField)
		}
	}
}

// UniqueTrimmedStrings returns a list on unique strings, trimming at the same time.
func UniqueTrimmedStrings(ss []string) []string {
	// Trim all first
	for i, v := range ss {
		ss[i] = strings.Trim(v, " ")
	}

	var m = make(map[string]struct{}, len(ss))
	var us = make([]string, 0, len(ss))
	for _, v := range ss {
		if _, ok := m[v]; ok {
			continue
		}
		m[v] = struct{}{}
		us = append(us, v)
	}

	return us
}

// FillNilSlices sets default value on slices that are still nil.
func FillNilSlices(data interface{}) error {
	s := reflect.ValueOf(data).Elem()
	t := s.Type()

	for i := 0; i < s.NumField(); i++ {
		f := s.Field(i)
		tag := t.Field(i).Tag

		v := tag.Get("default")
		if len(v) > 0 {
			switch f.Interface().(type) {
			case []string:
				if f.IsNil() {
					// Treat the default as a comma separated slice
					vs := strings.Split(v, ",")
					for i := range vs {
						vs[i] = strings.TrimSpace(vs[i])
					}

					rv := reflect.MakeSlice(reflect.TypeOf([]string{}), len(vs), len(vs))
					for i, v := range vs {
						rv.Index(i).SetString(v)
					}
					f.Set(rv)
				}
			}
		}
	}
	return nil
}

// Address constructs a URL from the given network and hostname.
func Address(network, host string) string {
	u := url.URL{
		Scheme: network,
		Host:   host,
	}
	return u.String()
}

// AddressUnspecifiedLess is a comparator function preferring least specific network address (most widely listening,
// namely preferring 0.0.0.0 over some IP), if both IPs are equal, it prefers the less restrictive network (prefers tcp
// over tcp4)
func AddressUnspecifiedLess(a, b net.Addr) bool {
	aIsUnspecified := false
	bIsUnspecified := false
	if host, _, err := net.SplitHostPort(a.String()); err == nil {
		aIsUnspecified = host == "" || net.ParseIP(host).IsUnspecified()
	}
	if host, _, err := net.SplitHostPort(b.String()); err == nil {
		bIsUnspecified = host == "" || net.ParseIP(host).IsUnspecified()
	}

	if aIsUnspecified == bIsUnspecified {
		return len(a.Network()) < len(b.Network())
	}
	return aIsUnspecified
}

// AsService wraps the given function to implement suture.Service by calling
// that function on serve and closing the passed channel when Stop is called.
func AsService(fn func(ctx context.Context), creator string) suture.Service {
	return asServiceWithError(func(ctx context.Context) error {
		fn(ctx)
		return nil
	}, creator)
}

type ServiceWithError interface {
	suture.Service
	fmt.Stringer
	Error() error
	SetError(error)
}

// AsServiceWithError does the same as AsService, except that it keeps track
// of an error returned by the given function.
func AsServiceWithError(fn func(ctx context.Context) error, creator string) ServiceWithError {
	return asServiceWithError(fn, creator)
}

func asServiceWithError(fn func(ctx context.Context) error, creator string) ServiceWithError {
	ctx, cancel := context.WithCancel(context.Background())
	s := &service{
		serve:   fn,
		ctx:     ctx,
		cancel:  cancel,
		stopped: make(chan struct{}),
		creator: creator,
		mut:     sync.NewMutex(),
	}
	close(s.stopped) // not yet started, don't block on Stop()
	return s
}

type service struct {
	creator string
	serve   func(ctx context.Context) error
	ctx     context.Context
	cancel  context.CancelFunc
	stopped chan struct{}
	err     error
	mut     sync.Mutex
}

func (s *service) Serve() {
	s.mut.Lock()
	select {
	case <-s.ctx.Done():
		s.mut.Unlock()
		return
	default:
	}
	s.err = nil
	s.stopped = make(chan struct{})
	s.mut.Unlock()

	var err error
	defer func() {
		if err == context.Canceled {
			err = nil
		}
		s.mut.Lock()
		s.err = err
		close(s.stopped)
		s.mut.Unlock()
	}()
	err = s.serve(s.ctx)
}

func (s *service) Stop() {
	s.mut.Lock()
	select {
	case <-s.ctx.Done():
		s.mut.Unlock()
		panic(fmt.Sprintf("Stop called more than once on %v", s))
	default:
		s.cancel()
	}

	// Cache s.stopped in a variable while we hold the mutex
	// to prevent a data race with Serve's resetting it.
	stopped := s.stopped
	s.mut.Unlock()
	<-stopped
}

func (s *service) Error() error {
	s.mut.Lock()
	defer s.mut.Unlock()
	return s.err
}

func (s *service) SetError(err error) {
	s.mut.Lock()
	s.err = err
	s.mut.Unlock()
}

func (s *service) String() string {
	return fmt.Sprintf("Service@%p created by %v", s, s.creator)
}

func CallWithContext(ctx context.Context, fn func() error) error {
	var err error
	done := make(chan struct{})
	go func() {
		err = fn()
		close(done)
	}()
	select {
	case <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func NiceDurationString(d time.Duration) string {
	switch {
	case d > 24*time.Hour:
		d = d.Round(time.Hour)
	case d > time.Hour:
		d = d.Round(time.Minute)
	case d > time.Minute:
		d = d.Round(time.Second)
	case d > time.Second:
		d = d.Round(time.Millisecond)
	case d > time.Millisecond:
		d = d.Round(time.Microsecond)
	}
	return d.String()
}
