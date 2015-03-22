#!/bin/bash

since="$1"
if [[ -z $since ]] ; then
	since="$(git describe --abbrev=0 HEAD^).."
fi

case $(uname) in
	Darwin)
		grep="egrep"
		;;
	*)
		grep="grep -P"
		;;
esac

git log --reverse --pretty=format:'* %s, @%aN)' "$since" | $grep 'fixes #\d|ref #\d' | sed 's/)[,. ]*,/,/' | sed 's/fixes #/#/g' | sed 's/ref #/#/g'

git diff "$since" -- AUTHORS

