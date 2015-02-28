#!/bin/bash

missing-authors() {
	for email in $(git log --format=%ae HEAD | sort | uniq) ; do
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
	grep -v f1120d7aa936c0658429edef0037792520b46334 |\
	grep -v a9339d0627fff439879d157c75077f02c9fac61b |\
	grep -v 254c63763a3ad42fd82259f1767db526cff94a14 |\
	grep -v 4b76ec40c07078beaa2c5e250ed7d9bd6276a718 |\
	grep -v ffc39dfbcb34eacc3ea12327a02b6e7741a2c207
}

print-missing-authors() {
	for email in $(missing-authors) ; do
		git log --author="$email" --format="%H %ae %s" | no-docs-typos
	done
}

print-missing-copyright() {
	find . -name \*.go | xargs egrep -L 'Copyright \(C\)|automatically generated' | grep -v Godeps | grep -v internal/auto/
}

authors=$(print-missing-authors)
if [[ ! -z $authors ]] ; then
	echo '***'
	echo Author emails not in AUTHORS:
	echo $authors
	echo '***'
	exit 1
fi

copy=$(print-missing-copyright)
if [[ ! -z $copy ]] ; then
	echo ***
	echo Files missing copyright notice:
	echo $copy
	echo ***
	exit 1
fi

