#!/bin/bash

export COPYFILE_DISABLE=true

version=$(git describe --always)
buildDir=dist

if [[ $fast != yes ]] ; then
	./assets.sh | gofmt > auto/gui.files.go
	go get -d
	go test ./...
fi

if [[ -z $1 ]] ; then
	go build -ldflags "-X main.Version $version" ./cmd/syncthing
elif [[ $1 == "tar" ]] ; then
	go build -ldflags "-X main.Version $version" ./cmd/syncthing \
	&& mkdir syncthing-dist \
	&& cp syncthing README.md LICENSE syncthing-dist \
	&& tar zcvf syncthing-dist.tar.gz syncthing-dist \
	&& rm -rf syncthing-dist
elif [[ $1 == "all" ]] ; then
	rm -rf "$buildDir"
	mkdir -p "$buildDir" || exit 1

	export GOARM=7
	for os in darwin-amd64 linux-amd64 linux-arm freebsd-amd64 windows-amd64 ; do
		echo "$os"
		export name="syncthing-$os"
		export GOOS=${os%-*}
		export GOARCH=${os#*-}
		go build -ldflags "-X main.Version $version" ./cmd/syncthing
		mkdir -p "$name"
		cp README.md LICENSE "$name"
		case $GOOS in
			windows)
				cp syncthing.exe "$buildDir/$name.exe"
				mv syncthing.exe "$name"
				zip -qr "$buildDir/$name.zip" "$name"
				;;
			*)
				cp syncthing "$buildDir/$name"
				mv syncthing "$name"
				tar zcf "$buildDir/$name.tar.gz" "$name"
				;;
		esac
		rm -r  "$name"
	done
fi
