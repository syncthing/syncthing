// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build ignore
// +build ignore

package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/syncthing/syncthing/lib/config"
)

func main() {
	new, err := os.Create("tags.csv")
	if err != nil {
		panic(err)
	}
	fmt.Println(filepath.Abs(new.Name()))
	w := csv.NewWriter(new)
	w.Write([]string{
		"path", "json", "xml", "default", "restart",
	})
	walk(w, "", &config.Configuration{})
	w.Flush()
	new.Close()
}

func walk(w *csv.Writer, prefix string, data interface{}) {
	s := reflect.ValueOf(data).Elem()
	t := s.Type()
	for i := 0; i < s.NumField(); i++ {
		f := s.Field(i)
		ft := t.Field(i)

		for f.Kind() == reflect.Ptr {
			f = f.Elem()
		}

		pfx := prefix + "." + s.Type().Field(i).Name
		if f.Kind() == reflect.Slice {
			slc := reflect.MakeSlice(f.Type(), 1, 1)
			f = slc.Index(0)
			pfx = prefix + "." + s.Type().Field(i).Name + "[]"
		}

		if f.Kind() == reflect.Struct && strings.HasPrefix(f.Type().PkgPath(), "github.com/syncthing/syncthing") {
			walk(w, pfx, f.Addr().Interface())
		} else {
			jsonTag := ft.Tag.Get("json")
			xmlTag := ft.Tag.Get("xml")
			defaultTag := ft.Tag.Get("default")
			restartTag := ft.Tag.Get("restart")
			w.Write([]string{
				strings.ToLower(pfx), jsonTag, xmlTag, defaultTag, restartTag,
			})
		}
	}

}
