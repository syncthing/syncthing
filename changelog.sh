#!/bin/bash

since="$1"
if [[ -z $since ]] ; then
	since="$(git describe --abbrev=0 HEAD^).."
fi

git log --reverse --pretty=format:'* %s, @%aN)' "$since" | egrep 'fixes #\d|ref #\d' | sed 's/)[,. ]*,/,/' | sed 's/fixes #/#/g' | sed 's/ref #/#/g'

git diff "$since" -- AUTHORS

