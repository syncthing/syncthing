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

	"github.com/thejerf/suture/v4"
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

			case int, uint32, int32, int64, uint64:
				i, err := strconv.ParseInt(v, 10, 64)
				if err != nil {
					panic(err)
				}
				f.SetInt(i)

			case float64, float32:
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

func FillNil(data interface{}) {
	s := reflect.ValueOf(data).Elem()
	for i := 0; i < s.NumField(); i++ {
		f := s.Field(i)

		for f.Kind() == reflect.Ptr && f.IsZero() && f.CanSet() {
			newValue := reflect.New(f.Type().Elem())
			f.Set(newValue)
			f = f.Elem()
		}

		if f.CanSet() {
			if f.IsZero() {
				switch f.Kind() {
				case reflect.Map:
					f.Set(reflect.MakeMap(f.Type()))
				case reflect.Slice:
					f.Set(reflect.MakeSlice(f.Type(), 0, 0))
				case reflect.Chan:
					f.Set(reflect.MakeChan(f.Type(), 0))
				}
			}

			if f.Kind() == reflect.Struct && f.CanAddr() {
				if addr := f.Addr(); addr.CanInterface() {
					FillNil(addr.Interface())
				}
			}
		}
	}
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

type FatalErr struct {
	Err    error
	Status ExitStatus
}

func (e *FatalErr) Error() string {
	return e.Err.Error()
}

func (e *FatalErr) Unwrap() error {
	return e.Err
}

func (e *FatalErr) Is(target error) bool {
	return target == suture.ErrTerminateSupervisorTree
}

type ExitStatus int

const (
	ExitSuccess            ExitStatus = 0
	ExitError              ExitStatus = 1
	ExitNoUpgradeAvailable ExitStatus = 2
	ExitRestart            ExitStatus = 3
	ExitUpgrade            ExitStatus = 4
)

func (s ExitStatus) AsInt() int {
	return int(s)
}

type ServiceWithError interface {
	suture.Service
	fmt.Stringer
	Error() error
	SetError(error)
}

// AsService wraps the given function to implement suture.Service. In addition
// it keeps track of the returned error and allows querying and setting that error.
func AsService(fn func(ctx context.Context) error, creator string) ServiceWithError {
	return &service{
		creator: creator,
		serve:   fn,
		mut:     sync.NewMutex(),
	}
}

type service struct {
	creator string
	serve   func(ctx context.Context) error
	err     error
	mut     sync.Mutex
}

func (s *service) Serve(ctx context.Context) error {
	s.mut.Lock()
	s.err = nil
	s.mut.Unlock()

	err := s.serve(ctx)

	s.mut.Lock()
	s.err = err
	s.mut.Unlock()

	return err
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

// OnDone calls fn when ctx is cancelled.
func OnDone(ctx context.Context, fn func()) {
	go func() {
		<-ctx.Done()
		fn()
	}()
}

type doneService struct {
	fn func()
}

func (s *doneService) Serve(ctx context.Context) error {
	<-ctx.Done()
	s.fn()
	return nil
}

// OnSupervisorDone calls fn when sup is done.
func OnSupervisorDone(sup *suture.Supervisor, fn func()) {
	sup.Add(&doneService{fn})
}

func Spec() suture.Spec {
	return suture.Spec{
		PassThroughPanics:        true,
		DontPropagateTermination: false,
	}
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
