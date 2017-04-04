#!/bin/bash
set -euo pipefail

# Copyright (C) 2016 The Syncthing Authors.
#
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at https://mozilla.org/MPL/2.0/.

# This script should be run by Jenkins as './src/github.com/syncthing/syncthing/jenkins/build-linux.bash',
# that is, it should be run from $GOPATH.

. src/github.com/syncthing/syncthing/jenkins/common.bash

init

# after init we are in the source directory

clean
fetchExtra
buildSource
build
test
testWithCoverage

platforms=(
	dragonfly-amd64
	freebsd-amd64 freebsd-386
	linux-amd64 linux-386 linux-arm linux-arm64 linux-ppc64 linux-ppc64le linux-mips linux-mipsle
	netbsd-amd64 netbsd-386
	openbsd-amd64 openbsd-386
)

echo Building
for plat in "${platforms[@]}"; do
	echo Building "$plat"

	goos="${plat%-*}"
	goarch="${plat#*-}"
	go run build.go -goos "$goos" -goarch "$goarch" tar
	mv *.tar.gz "$WORKSPACE"
	echo
done

export BUILD_USER=deb
go run build.go -goarch amd64 deb
go run build.go -goarch i386 deb
go run build.go -goarch armel deb
go run build.go -goarch armhf deb
go run build.go -goarch arm64 deb

mv *.deb "$WORKSPACE"

export BUILD_USER=snap
go run build.go -goarch amd64 snap
go run build.go -goarch armhf snap
go run build.go -goarch arm64 snap

mv *.snap "$WORKSPACE"

if [[ -d /usr/local/oldgo ]]; then
	echo
	echo Building with minimum supported Go version
	export GOROOT=/usr/local/oldgo
	export PATH="$GOROOT/bin:$PATH"
	go version
	echo

	rm -rf "$GOPATH/pkg"
	go run build.go install all # only compile, don't run lints and stuff
fi
