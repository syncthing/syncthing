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
		ulimit -t 600 &>/dev/null || true
		ulimit -d 512000 &>/dev/null || true
		ulimit -m 512000 &>/dev/null || true
		LOGGER_DISCARD=1 build test
		;;

	bench)
		LOGGER_DISCARD=1 build bench | script benchfilter
		;;

	prerelease)
		go run script/authors.go
		build transifex
		git add -A gui/default/assets/ lib/auto/
		pushd man ; ./refresh.sh ; popd
		git add -A man
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

	test-cov)
		ulimit -t 600 &>/dev/null || true
		ulimit -d 512000 &>/dev/null || true
		ulimit -m 512000 &>/dev/null || true

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

		notCovered=$(egrep -c '\s0$' coverage.out)
		total=$(wc -l coverage.out | awk '{print $1}')
		coverPct=$(awk "BEGIN{print (1 - $notCovered / $total) * 100}")
		echo "Total coverage is $coverPct%"

		gocov convert coverage.out | gocov-xml > coverage.xml

		# This is usually run from within Jenkins. If it is, we need to
		# tweak the paths in coverage.xml so cobertura finds the
		# source.
		if [[ "${WORKSPACE:-default}" != "default" ]] ; then
			sed "s#$WORKSPACE##g" < coverage.xml > coverage.xml.new && mv coverage.xml.new coverage.xml
		fi
		;;

	test-xunit)
		ulimit -t 600 &>/dev/null || true
		ulimit -d 512000 &>/dev/null || true
		ulimit -m 512000 &>/dev/null || true

		(GOPATH="$(pwd)/Godeps/_workspace:$GOPATH" go test -v -race ./lib/... ./cmd/... || true) > tests.out
		go2xunit -output tests.xml -fail < tests.out
		;;

	*)
		echo "Unknown build command $1"
		;;
esac
