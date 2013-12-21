#!/bin/bash

version=$(git describe --always)

for goos in darwin linux freebsd ; do
	for goarch in amd64 386 ; do
		export GOOS="$goos"
		export GOARCH="$goarch"
		go build -ldflags "-X main.Version $version" \
		&& mkdir -p "syncthing-$goos-$goarch" \
		&& mv syncthing "syncthing-$goos-$goarch" \
		&& cp syncthing.ini "syncthing-$goos-$goarch" \
		&& tar zcf "syncthing-$goos-$goarch.tar.gz" "syncthing-$goos-$goarch" \
		&& rm -r  "syncthing-$goos-$goarch"
	done
done
