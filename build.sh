#!/usr/bin/env bash

export COPYFILE_DISABLE=true

distFiles=(README.md LICENSE) # apart from the binary itself
version=$(git describe --always)

build() {
	go build -ldflags "-w -X main.Version $version" ./cmd/syncthing	
}

prepare() {
	./assets.sh | gofmt > auto/gui.files.go
	go get -d
}

test() {
	go test ./...
}	

tarDist() {
	name="$1"
	mkdir -p "$name"
	cp syncthing "${distFiles[@]}" "$name"
	tar zcvf "$name.tar.gz" "$name"
	rm -rf "$name"
}

zipDist() {
	name="$1"
	mkdir -p "$name"
	cp syncthing.exe "${distFiles[@]}" "$name"
	zip -r "$name.zip" "$name"
	rm -rf "$name"
}

case "$1" in
	"")
		build
		;;

	tar)
		rm -f *.tar.gz *.zip
		prepare
		test || exit 1
		build

		eval $(go env)
		name="syncthing-$GOOS-$GOARCH-$version"

		tarDist "$name"
		;;

	all)
		rm -f *.tar.gz *.zip
		prepare
		test || exit 1

		export GOARM=7
		for os in darwin-amd64 linux-amd64 linux-arm freebsd-amd64 windows-amd64 ; do
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
		;;

	upload)
		tag=$(git describe)
		shopt -s nullglob
		for f in *gz *zip ; do
			relup calmh/syncthing "$tag" "$f"
		done
		;;

	*)
		echo "Unknown build parameter $1"
		;;
esac
