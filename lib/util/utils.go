// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package util

import (
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"
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
