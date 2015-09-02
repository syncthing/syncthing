#!/bin/bash
set -euo pipefail

[ -d ~/go1.5 ] && exit

# Install the version of Go that we want

curl -s https://storage.googleapis.com/golang/go1.5.linux-amd64.tar.gz \
	| tar -C ~ --transform s/go/go1.5/ -zx

# Build the standard library for all our cross compilation targets. We do that
# here so that it gets cached and we don't need to repeat it for every build.

for GOOS in darwin dragonfly solaris; do
	export GOOS
	export GOARCH=amd64
	echo $GOOS $GOARCH
	go install std
done

for GOOS in freebsd linux netbsd openbsd windows; do
	for GOARCH in amd64 386; do
		export GOOS
		export GOARCH
		echo $GOOS $GOARCH
		go install std
	done
done

export GOOS=linux
export GOARCH=arm
echo $GOOS $GOARCH
go install std
