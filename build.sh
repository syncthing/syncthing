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

	deps)
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
		build transifex
		git add -A gui/assets/ lib/auto/
		pushd man ; ./refresh.sh ; popd
		git add -A man
		;;

	noupgrade)
		build -no-upgrade tar
		;;

	all)
		build -goos darwin -goarch amd64 tar

		build -goos dragonfly -goarch amd64 tar

		build -goos freebsd -goarch 386 tar
		build -goos freebsd -goarch amd64 tar

		build -goos linux -goarch 386 tar
		build -goos linux -goarch amd64 tar
		build -goos linux -goarch arm tar

		build -goos netbsd -goarch 386 tar
		build -goos netbsd -goarch amd64 tar

		build -goos openbsd -goarch 386 tar
		build -goos openbsd -goarch amd64 tar

		build -goos solaris -goarch amd64 tar

		build -goos windows -goarch 386 zip
		build -goos windows -goarch amd64 zip
		;;

	test-cov)
		ulimit -t 600 &>/dev/null || true
		ulimit -d 512000 &>/dev/null || true
		ulimit -m 512000 &>/dev/null || true

		echo "mode: set" > coverage.out
		fail=0

		# For every package in the repo
		for dir in $(go list ./...) ; do
			# run the tests
			godep go test -coverprofile=profile.out $dir
			if [ -f profile.out ] ; then
				# and if there was test output, append it to coverage.out
				grep -v "mode: set" profile.out >> coverage.out
				rm profile.out
			fi
		done

		gocov convert coverage.out | gocov-xml > coverage.xml

		# This is usually run from within Jenkins. If it is, we need to
		# tweak the paths in coverage.xml so cobertura finds the
		# source.
		if [[ "${WORKSPACE:-default}" != "default" ]] ; then
			sed "s#$WORKSPACE##g" < coverage.xml > coverage.xml.new && mv coverage.xml.new coverage.xml
		fi
		;;

	docker-all)
		img=${DOCKERIMG:-syncthing/build:latest}
		docker run --rm -h syncthing-builder -u $(id -u) -t \
			-v $(pwd):/go/src/github.com/syncthing/syncthing \
			-w /go/src/github.com/syncthing/syncthing \
			-e "STTRACE=$STTRACE" \
			"$img" \
			sh -c './build.sh clean \
				&& ./build.sh test-cov \
				&& ./build.sh bench \
				&& ./build.sh all'
		;;

	docker-test)
		img=${DOCKERIMG:-syncthing/build:latest}
		docker run --rm -h syncthing-builder -u $(id -u) -t \
			-v $(pwd):/go/src/github.com/syncthing/syncthing \
			-w /go/src/github.com/syncthing/syncthing \
			-e "STTRACE=$STTRACE" \
			"$img" \
			sh -euxc './build.sh clean \
				&& go run build.go -race \
				&& export GOPATH=$(pwd)/Godeps/_workspace:$GOPATH \
				&& cd test \
				&& go test -tags integration -v -timeout 90m -short \
				&& git clean -fxd .'
		;;

	docker-lint)
		img=${DOCKERIMG:-syncthing/build:latest}
		docker run --rm -h syncthing-builder -u $(id -u) -t \
			-v $(pwd):/go/src/github.com/syncthing/syncthing \
			-w /go/src/github.com/syncthing/syncthing \
			-e "STTRACE=$STTRACE" \
			"$img" \
			sh -euxc 'go run build.go lint'
		;;


	docker-vet)
		img=${DOCKERIMG:-syncthing/build:latest}
		docker run --rm -h syncthing-builder -u $(id -u) -t \
			-v $(pwd):/go/src/github.com/syncthing/syncthing \
			-w /go/src/github.com/syncthing/syncthing \
			-e "STTRACE=$STTRACE" \
			"$img" \
			sh -euxc 'go run build.go vet'
		;;

	*)
		echo "Unknown build command $1"
		;;
esac
