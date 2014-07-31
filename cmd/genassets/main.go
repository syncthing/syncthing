// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// +build ignore

package main

import (
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"go/format"
	"io"
	"os"
	"path/filepath"
	"text/template"
)

var tpl = template.Must(template.New("assets").Parse(`package auto

import (
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"io/ioutil"
)

var Assets = make(map[string][]byte)

func init() {
	var bs []byte
	var gr *gzip.Reader
{{range $asset := .assets}}
	bs, _ = hex.DecodeString("{{$asset.HexData}}")
	gr, _ = gzip.NewReader(bytes.NewBuffer(bs))
	bs, _ = ioutil.ReadAll(gr)
	Assets["{{$asset.Name}}"] = bs
{{end}}
}
`))

type asset struct {
	Name    string
	HexData string
}

var assets []asset

func walkerFor(basePath string) filepath.WalkFunc {
	return func(name string, info os.FileInfo, err error) error {
		if err != nil {
			return err
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
				Name:    filepath.ToSlash(name),
				HexData: fmt.Sprintf("%x", buf.Bytes()),
			})
		}

		return nil
	}
}

func main() {
	flag.Parse()

	filepath.Walk(flag.Arg(0), walkerFor(flag.Arg(0)))
	var buf bytes.Buffer
	tpl.Execute(&buf, map[string][]asset{"assets": assets})
	bs, err := format.Source(buf.Bytes())
	if err != nil {
		panic(err)
	}
	os.Stdout.Write(bs)
}
