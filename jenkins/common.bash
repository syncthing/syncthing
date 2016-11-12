#!/bin/bash
set -euo pipefail

# Copyright (C) 2016 The Syncthing Authors.
#
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.

ulimit -t 600 || true
ulimit -d 1024000 || true
ulimit -m 1024000 || true

export CGO_ENABLED=0
export GO386=387
export GOARM=5

function init {
    echo Initializing
    export GOPATH=$(pwd)
    export WORKSPACE="${WORKSPACE:-$GOPATH}"
    go version
    rm -f *.tar.gz *.zip *.deb *.snap
    cd src/github.com/syncthing/syncthing

    version=$(go run build.go version)
    echo "Building $version"
    echo
}

function clean {
    echo Cleaning
    rm -rf "$GOPATH/pkg"
    git clean -fxd
    echo
}

function fetchExtra {
    echo Fetching extra resources
    mkdir extra
    curl -s -o extra/Getting-Started.pdf https://docs.syncthing.net/pdf/Getting-Started.pdf
    curl -s -o extra/FAQ.pdf https://docs.syncthing.net/pdf/FAQ.pdf
    echo
}

function checkAuthorsCopyright {
    echo Basic metadata checks
    go run script/check-authors.go
    go run script/check-copyright.go lib/ cmd/ script/
    echo
}

function build {
    echo Build
    go run build.go
    echo
}

function test {
    echo Test with race
    CGO_ENABLED=1 go run build.go -race test
    echo
}

function testWithCoverage {
    echo Test with coverage
    CGO_ENABLED=1 ./build.sh test-cov

    notCovered=$(egrep -c '\s0$' coverage.out)
    total=$(wc -l coverage.out | awk '{print $1}')
    coverPct=$(awk "BEGIN{print (1 - $notCovered / $total) * 100}")
    echo "$coverPct" > "coverage.txt"
    echo "Test coverage is $coverPct%%"
    echo
}

function buildSource {
    echo Archiving source
    echo "$version" > RELEASE
    pushd .. >/dev/null
    tar c -z -f "$WORKSPACE/syncthing-source-$version.tar.gz" --exclude .git syncthing
    popd >/dev/null
    echo
}
