// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build ignore

package main

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"flag"
	"go/format"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

var tpl = template.Must(template.New("assets").Parse(`package auto

import (
	"encoding/base64"
)

func Assets() map[string][]byte {
	var assets = make(map[string][]byte, {{.Assets | len}})
{{range $asset := .Assets}}
	assets["{{$asset.Name}}"], _ = base64.StdEncoding.DecodeString("{{$asset.Data}}"){{end}}
	return assets
}

`))

type asset struct {
	Name string
	Data string
}

var assets []asset

func walkerFor(basePath string) filepath.WalkFunc {
	return func(name string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if strings.HasPrefix(filepath.Base(name), ".") {
			// Skip dotfiles
			return nil
		}

		if info.Mode().IsRegular() {
			fd, err := os.Open(name)
			if err != nil {
				return err
			}

			var buf bytes.Buffer
			gw := gzip.NewWriter(&buf)
			io.Copy(gw, fd)
			fd.Close()
			gw.Flush()
			gw.Close()

			name, _ = filepath.Rel(basePath, name)
			assets = append(assets, asset{
				Name: filepath.ToSlash(name),
				Data: base64.StdEncoding.EncodeToString(buf.Bytes()),
			})
		}

		return nil
	}
}

type templateVars struct {
	Assets []asset
}

func main() {
	flag.Parse()

	filepath.Walk(flag.Arg(0), walkerFor(flag.Arg(0)))
	var buf bytes.Buffer
	tpl.Execute(&buf, templateVars{
		Assets: assets,
	})
	bs, err := format.Source(buf.Bytes())
	if err != nil {
		panic(err)
	}
	os.Stdout.Write(bs)
}
