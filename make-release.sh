#!/bin/bash
set -euo pipefail

# Check preconditions

version="${1:-}"
if [[ $version == "" ]] ; then
	echo Usage:
	echo " " $0 "[version]"
	exit 1
fi

if [[ $version != v*.*.* ]] ; then
	echo Version has incorrect format
	exit 1
fi

branch=$(git rev-parse --abbrev-ref HEAD)
if [[ $branch != "master" ]] ; then
	echo Releases can only be made from master branch
	exit 1
fi

if [[ -n $(git status -s) ]] ; then
	echo Releases can only be made from a clean tree
	exit 1
fi

# Tag the release

cat > cmd/syncthing/version.go <<EOT
// This file is auto generated

package main

var Version = "$version"
EOT

git commit -am "Set version to $version for release"

shouldSign="no"
if hash gpg >/dev/null 2>&1 ; then
	if gpg --list-secret-keys release@syncthing.net >/dev/null 2>&1 ; then
		# We have GPG and the correct key
		shouldSign="yes"
	fi
fi

if [[ $shouldSign == "yes" ]] ; then
	git tag -a -m "$version" -u release@syncthing.net -s "$version"
else
	git tag -a -m "$version" "$version"
fi

# Start the next dev cycle with a new -dev version number

cat > cmd/syncthing/version.go <<EOT
// This file is auto generated

package main

var Version = "${version}-dev"
EOT

git commit -am "Set version to ${version}-dev for development"
