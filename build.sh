#!/usr/bin/env bash

export COPYFILE_DISABLE=true
export GO386=387 # Don't use SSE on 32 bit builds

distFiles=(README.md LICENSE CONTRIBUTORS) # apart from the binary itself
version=$(git describe --always --dirty)
date=$(git show -s --format=%ct)
user=$(whoami)
host=$(hostname)
host=${host%%.*}
bldenv=${ENVIRONMENT:-default}
ldflags="-w -X main.Version $version -X main.BuildStamp $date -X main.BuildUser $user -X main.BuildHost $host -X main.BuildEnv $bldenv"

check() {
	if ! command -v godep >/dev/null ; then
		echo "Error: no godep. Try \"$0 setup\"."
		exit 1
	fi
}

build() {
	check

	go vet ./...

	godep go build $* -ldflags "$ldflags" ./cmd/syncthing
}

assets() {
	check
	godep go run cmd/assets/assets.go gui > auto/gui.files.go
}

test-cov() {
	echo "mode: set" > coverage.out
	fail=0

	for dir in $(go list ./...) ; do
		godep go test -coverprofile=profile.out $dir || fail=1
		if [ -f profile.out ] ; then
			grep -v "mode: set" profile.out >> coverage.out
		rm profile.out
        fi
    done

   exit $fail
}

test() {
	check
	godep go test -cpu=1,2,4 ./...
}

sign() {
	if git describe --exact-match 2>/dev/null >/dev/null ; then
		# HEAD is a tag
		id=BCE524C7
		if gpg --list-keys "$id" >/dev/null 2>&1 ; then
			gpg -ab -u "$id" "$1"
		fi
	fi
}

tarDist() {
	name="$1"
	rm -rf "$name"
	mkdir -p "$name"
	cp syncthing "${distFiles[@]}" "$name"
	sign "$name/syncthing"
	tar zcvf "$name.tar.gz" "$name"
	rm -rf "$name"
}

zipDist() {
	name="$1"
	rm -rf "$name"
	mkdir -p "$name"
	for f in "${distFiles[@]}" ; do
		sed 's/$/
/' < "$f" > "$name/$f.txt"
	done
	cp syncthing.exe "$name"
	sign "$name/syncthing.exe"
	zip -r "$name.zip" "$name"
	rm -rf "$name"
}

deps() {
	check
	godep save ./cmd/syncthing ./cmd/assets ./discover/cmd/discosrv
}

setup() {
	echo Installing godep...
	go get -u github.com/tools/godep
	echo Installing go vet...
	go get -u code.google.com/p/go.tools/cmd/vet
}

case "$1" in
	"")
		shift
		build $*
		;;

	race)
		build -race
		;;

	guidev)
		echo "Syncthing is already built for GUI developments. Try:"
		echo "    STGUIASSETS=~/someDir/gui syncthing"
		;;

	test)
		test
		;;

	test-cov)
		test-cov
		;;

	tar)
		rm -f *.tar.gz *.zip
		test || exit 1
		assets
		build

		eval $(go env)
		name="syncthing-$GOOS-$GOARCH-$version"

		tarDist "$name"
		;;

	all)
		rm -f *.tar.gz *.zip
		test || exit 1
		assets

		godep go build ./discover/cmd/discosrv
		godep go build ./cmd/stpidx
		godep go build ./cmd/stcli

		for os in darwin-amd64 linux-386 linux-amd64 freebsd-amd64 windows-amd64 windows-386 solaris-amd64 ; do
			export GOOS=${os%-*}
			export GOARCH=${os#*-}

			build

			name="syncthing-$os-$version"
			case $GOOS in
				windows)
					zipDist "$name"
					rm -f syncthing.exe
					;;
				*)
					tarDist "$name"
					rm -f syncthing
					;;
			esac
		done

		export GOOS=linux
		export GOARCH=arm

		origldflags="$ldflags"

		export GOARM=7
		ldflags="$origldflags -X main.GoArchExtra v7"
		build
		tarDist "syncthing-linux-armv7-$version"

		export GOARM=6
		ldflags="$origldflags -X main.GoArchExtra v6"
		build
		tarDist "syncthing-linux-armv6-$version"

		export GOARM=5
		ldflags="$origldflags -X main.GoArchExtra v5"
		build
		tarDist "syncthing-linux-armv5-$version"

		;;

	upload)
		tag=$(git describe)
		shopt -s nullglob
		for f in *.tar.gz *.zip *.asc ; do
			relup calmh/syncthing "$tag" "$f"
		done
		;;

	deps)
		deps
		;;

	assets)
		assets
		;;

	setup)
		setup
		;;

	*)
		echo "Unknown build parameter $1"
		;;
esac
