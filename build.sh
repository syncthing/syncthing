#!/bin/bash

version=$(git describe --always)
buildDir=dist

if [[ -z $1 ]] ; then
	go test ./...
	go build -ldflags "-X main.Version $version" \
	&& nrsc syncthing gui
elif [[ $1 == "tar" ]] ; then
	go test ./...
	go build -ldflags "-X main.Version $version" \
	&& nrsc syncthing gui \
	&& mkdir syncthing-dist \
	&& cp syncthing README.md LICENSE syncthing-dist \
	&& tar zcvf syncthing-dist.tar.gz syncthing-dist \
	&& rm -rf syncthing-dist
else
	go test ./... || exit 1

	rm -rf "$buildDir"
	mkdir -p "$buildDir" || exit 1

	for goos in darwin linux freebsd ; do
		for goarch in amd64 386 ; do
			echo "$goos-$goarch"
			export GOOS="$goos"
			export GOARCH="$goarch"
			export name="syncthing-$goos-$goarch"
			go build -ldflags "-X main.Version $version" \
				&& nrsc syncthing gui \
				&& mkdir -p "$name" \
				&& mv syncthing "$buildDir/$name" \
				&& cp README.md LICENSE "$name" \
				&& mv syncthing "$name" \
				&& tar zcf "$buildDir/$name.tar.gz" "$name" \
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
				&& nrsc syncthing.exe gui \
				&& mkdir -p "$name" \
				&& mv syncthing.exe "$buildDir/$name.exe" \
				&& cp README.md LICENSE "$name" \
				&& zip -qr "$buildDir/$name.zip" "$name" \
				&& rm -r  "$name"
		done
	done
fi
