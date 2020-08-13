// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// Package versioner implements common interfaces for file versioning and a
// simple default versioning scheme.

//+build ignore

package proto

//go:generate go run ../script/protofmt.go .


//=
// First generate extensions using standard proto compiler.
//go:generate protoc -I ../ -I . --gogofast_out=Mgoogle/protobuf/descriptor.proto=github.com/gogo/protobuf/protoc-gen-gogo/descriptor,paths=source_relative:ext ext.proto

// Then build our vanity compiler that uses the new extensions
//go:generate go build -o plugin/protoc-gen-gosyncthing plugin/main.go

// Each package needs to be generated separately, to avoid go_package clashes.
//go:generate protoc -I ../ -I . --plugin=protoc-gen-gosyncthing=plugin/protoc-gen-gosyncthing --gosyncthing_out=paths=source_relative:out lib/config/*.proto
///go:generate protoc -I ../ -I . --gogofast_out=paths=source_relative:.. lib/fs/*.proto
///go:generate protoc -I ../ -I . --gogofast_out=paths=source_relative:.. lib/protocol/*.proto
