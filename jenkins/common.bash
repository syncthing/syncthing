#!/bin/bash
set -euo pipefail

# Copyright (C) 2016 The Syncthing Authors.
#
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at https://mozilla.org/MPL/2.0/.

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
    rm -rf parts # created by snapcraft, contains git repo so not cleaned by git above
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
    CGO_ENABLED=1

    echo "mode: set" > coverage.out
    fail=0

    # For every package in the repo
    for dir in $(go list ./lib/... ./cmd/...) ; do
        # run the tests
        GOPATH="$(pwd)/Godeps/_workspace:$GOPATH" go test -coverprofile=profile.out $dir
        if [ -f profile.out ] ; then
            # and if there was test output, append it to coverage.out
            grep -v "mode: " profile.out >> coverage.out
            rm profile.out
        fi
    done

    gocov convert coverage.out | gocov-xml > coverage.xml

    # This is usually run from within Jenkins. If it is, we need to
    # tweak the paths in coverage.xml so cobertura finds the
    # source.
    if [[ "${WORKSPACE:-default}" != "default" ]] ; then
        sed "s#$WORKSPACE##g" < coverage.xml > coverage.xml.new && mv coverage.xml.new coverage.xml
    fi

    notCovered=$(egrep -c '\s0$' coverage.out)
    total=$(wc -l coverage.out | awk '{print $1}')
    coverPct=$(awk "BEGIN{print (1 - $notCovered / $total) * 100}")
    echo "$coverPct" > "coverage.txt"
    echo "Test coverage is $coverPct%%"
    echo

    CGO_ENABLED=0 # reset to before
}

function buildSource {
    echo Archiving source
    echo "$version" > RELEASE
    pushd .. >/dev/null
    tar c -z -f "$WORKSPACE/syncthing-source-$version.tar.gz" --exclude .git syncthing
    popd >/dev/null
    echo
}
