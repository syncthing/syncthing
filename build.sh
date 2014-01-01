#!/bin/bash

version=$(git describe --always)

go test ./... || exit 1

rm -rf build
mkdir -p build || exit 1

for goos in darwin linux freebsd ; do
	for goarch in amd64 386 ; do
		echo "$goos-$goarch"
		export GOOS="$goos"
		export GOARCH="$goarch"
		export name="syncthing-$goos-$goarch"
		go build -ldflags "-X main.Version $version" \
		&& mkdir -p "$name" \
		&& cp syncthing "build/$name" \
		&& cp README.md LICENSE "$name" \
		&& mv syncthing "$name" \
		&& tar zcf "build/$name.tar.gz" "$name" \
		&& rm -r  "$name"
	done
done

for goos in windows ; do
	for goarch in amd64 386 ; do
		echo "$goos-$goarch"
		export GOOS="$goos"
		export GOARCH="$goarch"
		export name="syncthing-$goos-$goarch"
		go build -ldflags "-X main.Version $version" \
		&& mkdir -p "$name" \
		&& cp syncthing.exe "build/$name.exe" \
		&& cp README.md LICENSE "$name" \
		&& zip -qr "build/$name.zip" "$name" \
		&& rm -r  "$name"
	done
done
