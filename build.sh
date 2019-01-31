#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

STTRACE=${STTRACE:-}

script() {
	name="$1"
	shift
	go run "script/$name.go" "$@"
}

build() {
	go run build.go "$@"
}

case "${1:-default}" in
	default)
		build
		;;

	clean)
		build "$@"
		;;

	tar)
		build "$@"
		;;

	assets)
		build "$@"
		;;

	xdr)
		build "$@"
		;;

	translate)
		build "$@"
		;;

	deb)
		build "$@"
		;;

	setup)
		build "$@"
		;;

	test)
		LOGGER_DISCARD=1 build test
		;;

	bench)
		LOGGER_DISCARD=1 build bench | script benchfilter
		;;

	prerelease)
		go run script/authors.go
		build transifex
		pushd man ; ./refresh.sh ; popd
		git add -A gui man AUTHORS
		git commit -m 'gui, man, authors: Update docs, translations, and contributors'
		;;

	noupgrade)
		build -no-upgrade tar
		;;

	all)
		platforms=( # format: GOOS-GOARCH[-GOARM]
			# Amd64 architecture
			darwin-amd64 dragonfly-amd64 freebsd-amd64 linux-amd64 netbsd-amd64 openbsd-amd64 solaris-amd64 windows-amd64

			# 386 architecture
			freebsd-386 linux-386 netbsd-386 openbsd-386 windows-386

			# ARM architecture
			linux-arm linux-arm-5 linux-arm-6 linux-arm-7 linux-arm64

			# PPC64 Architecture
			linux-ppc64 linux-ppc64le
		)

		for plat in "${platforms[@]}"; do
			echo Building "$plat"

			goos="${plat%%-*}"
			goarm="${plat#$goos-}"
			goarch="${goarm%%-*}"
			goarm="${goarm#$goarch-}"
			build_params="-goos $goos -goarch $goarch"

			if [[ $goarch == "arm" && $goarm != "arm" ]]; then
				build_params="$build_params -goarm $goarm"
			fi

			dist="tar"
			if [[ $goos == "windows" ]]; then
				dist="zip"
			fi

			build "$build_params" "$dist"
			echo
		done
		;;

	test-xunit)

		(GOPATH="$(pwd)/Godeps/_workspace:$GOPATH" go test -v -race ./lib/... ./cmd/... || true) > tests.out
		go2xunit -output tests.xml -fail < tests.out
		;;

	*)
		echo "Unknown build command $1"
		;;
esac
