#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

DOCKERIMGV=1.4-4

case "${1:-default}" in
	default)
		go run build.go
		;;

	clean)
		go run build.go "$1"
		;;

	test)
		ulimit -t 60 &>/dev/null || true
		ulimit -d 512000 &>/dev/null || true
		ulimit -m 512000 &>/dev/null || true

		go run build.go "$1"
		;;

	tar)
		go run build.go "$1"
		;;

	deps)
		go run build.go "$1"
		;;

	assets)
		go run build.go "$1"
		;;

	xdr)
		go run build.go "$1"
		;;

	translate)
		go run build.go "$1"
		;;

	transifex)
		go run build.go "$1"
		;;

	noupgrade)
		go run build.go -no-upgrade tar
		;;

	all)
		go run build.go -goos linux -goarch amd64 tar
		go run build.go -goos linux -goarch 386 tar
		go run build.go -goos linux -goarch armv5 tar
		go run build.go -goos linux -goarch armv6 tar
		go run build.go -goos linux -goarch armv7 tar

		go run build.go -goos freebsd -goarch amd64 tar
		go run build.go -goos freebsd -goarch 386 tar

		go run build.go -goos openbsd -goarch amd64 tar
		go run build.go -goos openbsd -goarch 386 tar

		go run build.go -goos darwin -goarch amd64 tar

		go run build.go -goos windows -goarch amd64 zip
		go run build.go -goos windows -goarch 386 zip
		;;

	setup)
		echo "Don't worry, just build."
		;;

	test-cov)
		ulimit -t 600 &>/dev/null || true
		ulimit -d 512000 &>/dev/null || true
		ulimit -m 512000 &>/dev/null || true

		go get github.com/axw/gocov/gocov
		go get github.com/AlekSi/gocov-xml

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

	docker-init)
		docker build -q -t syncthing/build:$DOCKERIMGV docker >/dev/null
		;;

	docker-all)
		docker run --rm -h syncthing-builder -u $(id -u) -t \
			-v $(pwd):/go/src/github.com/syncthing/syncthing \
			-w /go/src/github.com/syncthing/syncthing \
			syncthing/build:$DOCKERIMGV \
			sh -c './build.sh clean \
				&& go vet ./cmd/... ./internal/... \
				&& ( golint ./cmd/... ; golint ./internal/... ) | egrep -v "comment on exported|should have comment" \
				&& ./build.sh all \
				&& STTRACE=all ./build.sh test-cov'
		;;

	docker-test)
		docker run --rm -h syncthing-builder -u $(id -u) -t \
			-v $(pwd):/tmp/syncthing \
			syncthing/build:$DOCKERIMGV \
			sh -euxc 'mkdir -p /go/src/github.com/syncthing \
				&& cd /go/src/github.com/syncthing \
				&& cp -r /tmp/syncthing syncthing \
				&& cd syncthing \
				&& ./build.sh clean \
				&& go run build.go -race \
				&& export GOPATH=$(pwd)/Godeps/_workspace:$GOPATH \
				&& cd test \
				&& go test -tags integration -v -timeout 60m -short'
		;;

	*)
		echo "Unknown build command $1"
		;;
esac
