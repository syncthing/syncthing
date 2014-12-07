#!/bin/bash

since="$1"
if [[ -z $since ]] ; then
	since="$(git describe --abbrev=0 HEAD^).."
fi

git log --pretty=format:'* %h %s (%an)' "$since" | grep '(fixes #'

