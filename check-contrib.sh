#!/bin/bash

missing-authors() {
	for email in $(git log --format=%ae master | sort | uniq) ; do
		grep -q "$email" AUTHORS || echo $email
	done
}

no-docs-typos() {
	# Commits that are known to not change code
	grep -v 63bd0136fb40a91efaa279cb4b4159d82e8e6904 |\
	grep -v 4e2feb6fbc791bb8a2daf0ab8efb10775d66343e |\
	grep -v f2459ef3319b2f060dbcdacd0c35a1788a94b8bd |\
	grep -v b61f418bf2d1f7d5a9d7088a20a2a448e5e66801 |\
	grep -v f0621207e3953711f9ab86d99724f1d0faac45b1 |\
	grep -v f1120d7aa936c0658429edef0037792520b46334
}

print-missing-authors() {
	for email in $(missing-authors) ; do
		git log --author="$email" --format="%H %ae %s" | no-docs-typos
	done
}

print-missing-copyright() {
	find . -name \*.go | xargs grep -L 'Copyright (C)' | grep -v Godeps
}

print-line-blame() {
	for f in $(find . -name \*.go | grep -v Godep) gui/app.js gui/index.html ; do
		git blame --line-porcelain $f | grep author-mail
	done | sort | uniq -c | sort -n
}
echo Author emails missing in AUTHORS file:
print-missing-authors
echo

echo Files missing copyright notice:
print-missing-copyright
echo

echo Blame lines per author:
print-line-blame

