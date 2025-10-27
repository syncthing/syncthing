// Copyright (C) 2023 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package structutil

import (
	"reflect"
	"strconv"
	"strings"
)

type defaultParser interface {
	ParseDefault(string) error
}

// SetDefaults sets default values on a struct, based on the default annotation.
func SetDefaults(data any) {
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
		} else if f.CanSet() && f.Kind() == reflect.Struct && f.CanAddr() {
			if addr := f.Addr(); addr.CanInterface() {
				SetDefaults(addr.Interface())
			}
		}
	}
}

func FillNilExceptDeprecated(data any) {
	fillNil(data, true)
}

func FillNil(data any) {
	fillNil(data, false)
}

func fillNil(data any, skipDeprecated bool) {
	s := reflect.ValueOf(data).Elem()
	t := s.Type()
	for i := range s.NumField() {
		if skipDeprecated && strings.HasPrefix(t.Field(i).Name, "Deprecated") {
			continue
		}

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

			switch f.Kind() {
			case reflect.Slice:
				if f.Type().Elem().Kind() != reflect.Struct {
					continue
				}
				for i := range f.Len() {
					fillNil(f.Index(i).Addr().Interface(), skipDeprecated)
				}
			case reflect.Struct:
				if f.CanAddr() {
					if addr := f.Addr(); addr.CanInterface() {
						fillNil(addr.Interface(), skipDeprecated)
					}
				}
			}
		}
	}
}

// FillNilSlices sets default value on slices that are still nil.
func FillNilSlices(data any) error {
	s := reflect.ValueOf(data).Elem()
	t := s.Type()

	for i := range s.NumField() {
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
