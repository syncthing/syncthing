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
		git add -A gui man
		git commit -m 'gui, man: Update docs & translations'
		;;

	noupgrade)
		build -no-upgrade tar
		;;

	all)
		platforms=(
			darwin-amd64 dragonfly-amd64 freebsd-amd64 linux-amd64 netbsd-amd64 openbsd-amd64 solaris-amd64 windows-amd64
			freebsd-386 linux-386 netbsd-386 openbsd-386 windows-386
			linux-arm linux-arm64 linux-ppc64 linux-ppc64le
		)

		for plat in "${platforms[@]}"; do
			echo Building "$plat"

			goos="${plat%-*}"
			goarch="${plat#*-}"
			dist="tar"

			if [[ $goos == "windows" ]]; then
				dist="zip"
			fi

			build -goos "$goos" -goarch "$goarch" "$dist"
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
