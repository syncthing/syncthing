#!/bin/bash
set -euo pipefail

# Copyright (C) 2016 The Syncthing Authors.
#
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.

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
	linux-amd64 linux-386 linux-arm linux-arm64 linux-ppc64 linux-ppc64le
	netbsd-amd64 netbsd-386
	openbsd-amd64 openbsd-386
	solaris-amd64
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

go run build.go -goarch amd64 deb
fakeroot sh -c 'chown -R root:root deb ; dpkg-deb -b deb .'
mv *.deb "$WORKSPACE"

go run build.go -goarch i386 deb
fakeroot sh -c 'chown -R root:root deb ; dpkg-deb -b deb .'
mv *.deb "$WORKSPACE"

go run build.go -goarch armel deb
fakeroot sh -c 'chown -R root:root deb ; dpkg-deb -b deb .'
mv *.deb "$WORKSPACE"

go run build.go -goarch armhf deb
fakeroot sh -c 'chown -R root:root deb ; dpkg-deb -b deb .'
mv *.deb "$WORKSPACE"
