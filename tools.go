// This file is never built. It serves to establish dependencies on tools
// used by go generate and build.go. See
// https://github.com/golang/go/wiki/Modules#how-can-i-track-tool-dependencies-for-a-module

//go:build tools
// +build tools

package tools

import (
	_ "github.com/calmh/xdr"
	_ "github.com/gogo/protobuf/protoc-gen-gogofast"
	_ "github.com/maxbrunsfeld/counterfeiter/v6"
	_ "golang.org/x/tools/cmd/goimports"
	_ "github.com/josephspurrier/goversioninfo/cmd/goversioninfo"
)
