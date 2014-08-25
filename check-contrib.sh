#!/bin/bash

missing-contribs() {
	for email in $(git log --format=%ae master | grep -v jakob@nym.se | sort | uniq) ; do
		grep -q "$email" CONTRIBUTORS || echo $email
	done
}

no-docs-typos() {
	# Commits that are known to not change code
	grep -v f2459ef3319b2f060dbcdacd0c35a1788a94b8bd |\
	grep -v b61f418bf2d1f7d5a9d7088a20a2a448e5e66801 |\
	grep -v f0621207e3953711f9ab86d99724f1d0faac45b1 |\
	grep -v f1120d7aa936c0658429edef0037792520b46334
}

for email in $(missing-contribs) ; do
	git log --author="$email" --format="%H %ae %s" | no-docs-typos
done

