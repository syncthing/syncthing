#!/bin/bash
set -euo pipefail

# Copyright (C) 2017 The Syncthing Authors.
#
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at https://mozilla.org/MPL/2.0/.

# This script should be run by Jenkins as './src/github.com/syncthing/syncthing/jenkins/build-macos.bash',
# that is, it should be run from $GOPATH.

. src/github.com/syncthing/syncthing/jenkins/common.bash

init

# after init we are in the source directory

clean
fetchExtra

# Solaris always uses cgo, as opposed to our regular cross builds
export CGO_ENABLED=1

# Quick build, generate assets
go run build.go build syncthing

# Test the stuff we are going to build only, as discosrv etc fails.
# -race is not supported on Solaris.
echo Testing
go test ./lib/... ./cmd/syncthing
echo

# Specifically set "syncthing" target as discosrv currently doesn't build
echo Building
go run build.go tar syncthing
mv *.tar.gz "$WORKSPACE"
echo
