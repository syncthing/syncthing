// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//+build ignore

package proto

//go:generate go run scripts/protofmt.go .

// First generate extensions using standard proto compiler.
//go:generate protoc -I ../ -I . --gogofast_out=Mgoogle/protobuf/descriptor.proto=github.com/gogo/protobuf/protoc-gen-gogo/descriptor,paths=source_relative:ext ext.proto

// Then build our vanity compiler that uses the new extensions
//go:generate go build -o scripts/protoc-gen-gosyncthing scripts/protoc_plugin.go

// Each package needs to be generated separately, to avoid go_package clashes.
//go:generate protoc -I ../ -I . --plugin=protoc-gen-gosyncthing=scripts/protoc-gen-gosyncthing --gosyncthing_out=paths=source_relative:.. lib/config/*.proto
//go:generate protoc -I ../ -I . --plugin=protoc-gen-gosyncthing=scripts/protoc-gen-gosyncthing --gosyncthing_out=paths=source_relative:.. lib/fs/*.proto

// Use the standard compiler here. We can revisit this later, but we don't plan on exposing this via any APIs.
//go:generate protoc -I ../ -I . --gogofast_out=paths=source_relative:.. lib/protocol/*.proto
