#!/bin/bash

version=$(git describe --always)
buildDir=dist

if [[ $fast != yes ]] ; then
	go get -d
	go test ./...
fi

if [[ -z $1 ]] ; then
	go build -ldflags "-X main.Version $version"
elif [[ $1 == "embed" ]] ; then
	embedder main gui > gui.files.go
elif [[ $1 == "tar" ]] ; then
	go build -ldflags "-X main.Version $version" \
	&& mkdir syncthing-dist \
	&& cp syncthing README.md LICENSE syncthing-dist \
	&& tar zcvf syncthing-dist.tar.gz syncthing-dist \
	&& rm -rf syncthing-dist
elif [[ $1 == "all" ]] ; then
	rm -rf "$buildDir"
	mkdir -p "$buildDir" || exit 1

	for goos in darwin linux freebsd ; do
		for goarch in amd64 386 ; do
			echo "$goos-$goarch"
			export GOOS="$goos"
			export GOARCH="$goarch"
			export name="syncthing-$goos-$goarch"
			go build -ldflags "-X main.Version $version" \
				&& mkdir -p "$name" \
				&& cp syncthing "$buildDir/$name" \
				&& cp README.md LICENSE "$name" \
				&& mv syncthing "$name" \
				&& tar zcf "$buildDir/$name.tar.gz" "$name" \
				&& rm -r  "$name"
		done
	done

	for goos in linux ; do
		for goarm in 5 6 7 ; do
			for goarch in arm ; do
				echo "$goos-${goarch}v$goarm"
				export GOARM="$goarm"
				export GOOS="$goos"
				export GOARCH="$goarch"
				export name="syncthing-$goos-${goarch}v$goarm"
				go build -ldflags "-X main.Version $version" \
					&& mkdir -p "$name" \
					&& cp syncthing "$buildDir/$name" \
					&& cp README.md LICENSE "$name" \
					&& mv syncthing "$name" \
					&& tar zcf "$buildDir/$name.tar.gz" "$name" \
					&& rm -r  "$name"
			done
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
				&& cp syncthing.exe "$buildDir/$name.exe" \
				&& cp README.md LICENSE "$name" \
				&& mv syncthing.exe "$name" \
				&& zip -qr "$buildDir/$name.zip" "$name" \
				&& rm -r  "$name"
		done
	done
fi
