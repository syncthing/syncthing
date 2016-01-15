// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build ignore

package main

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"flag"
	"go/format"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"
)

var (
	dev bool
)

var tpl = template.Must(template.New("assets").Parse(`package auto

import (
	"encoding/base64"
)

const (
	AssetsBuildDate = "{{.BuildDate}}"
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

			name, _ = filepath.Rel(basePath, name)

			filename := filepath.ToSlash(name)
			var buf bytes.Buffer
    	gw := gzip.NewWriter(&buf)

			// check for dev mode, modify api locaiton
			if filename == "syncthing/connection.js" && dev {
				gw.Write([]byte("var apiBase = 'http://localhost:3000/api/v1';"));
			} else {
				io.Copy(gw, fd)
				fd.Close()
			}

			gw.Flush()
			gw.Close()

			assets = append(assets, asset{
				Name: filename,
				Data: base64.StdEncoding.EncodeToString(buf.Bytes()),
			})
		}

		return nil
	}
}

type templateVars struct {
	Assets    []asset
	BuildDate string
}

func main() {
	flag.BoolVar(&dev, "dev", dev, "Development mode")
	flag.Parse()

	filepath.Walk(flag.Arg(0), walkerFor(flag.Arg(0)))
	var buf bytes.Buffer
	tpl.Execute(&buf, templateVars{
		Assets:    assets,
		BuildDate: time.Now().UTC().Format(http.TimeFormat),
	})
	bs, err := format.Source(buf.Bytes())
	if err != nil {
		panic(err)
	}
	os.Stdout.Write(bs)
}
