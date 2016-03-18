#!/bin/bash
set -euo pipefail

build() {
	export GOOS="$1"
	export GOARCH="$2"
	export GO15VENDOREXPERIMENT="1"
	target="discosrv-$GOOS-$GOARCH"
	go build -i -v -tags purego -ldflags -w
	mkdir "$target"
	if [ -f discosrv ] ; then
		mv discosrv "$target"
		tar zcvf "$target.tar.gz" "$target"
		rm -r "$target"
	fi
	if [ -f discosrv.exe ] ; then
		mv discosrv.exe "$target"
		zip -r "$target.zip" "$target"
		rm -r "$target"
	fi
}

buildpkg() {
	echo Get dependencies
	rm -rf discosrv-*-*

	for goos in linux darwin windows freebsd openbsd netbsd solaris ; do
		build "$goos" amd64
	done
	for goos in linux windows freebsd openbsd netbsd ; do
		build "$goos" 386
	done
	build linux arm

	# Hack used because we run as root under Docker
	if [[ ${CHOWN_USER:-} != "" ]] ; then
		chown -R $CHOWN_USER .
	fi
}

case "${1:-default}" in
	pkg)
		buildpkg
		;;

	default)
		go install -v
		;;
esac

