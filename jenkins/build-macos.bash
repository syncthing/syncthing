#!/bin/bash
set -euo pipefail

# Copyright (C) 2016 The Syncthing Authors.
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
build
test

platforms=(
	darwin-amd64 darwin-386
)

# Mac builds always require cgo
export CGO_ENABLED=1

echo Building
for plat in "${platforms[@]}"; do
	echo Building "$plat"

	goos="${plat%-*}"
	goarch="${plat#*-}"
	go run build.go -goos "$goos" -goarch "$goarch" tar
	mv *.tar.gz "$WORKSPACE"
	echo
done
