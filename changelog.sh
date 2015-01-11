#!/bin/bash

since="$1"
if [[ -z $since ]] ; then
	since="$(git describe --abbrev=0 HEAD^).."
fi

git log --pretty=format:'* %s  \n  (%h, @%aN)' "$since" | egrep 'fixes #\d|ref #\d' | sed s/\\\\n/\\$'\n'/

