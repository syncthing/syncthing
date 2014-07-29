#!/bin/bash

# Copyright (C) 2014 Jakob Borg and other contributors. All rights reserved.
# Use of this source code is governed by an MIT-style license that can be
# found in the LICENSE file.

iterations=${1:-5}

id1=I6KAH76-66SLLLB-5PFXSOA-UFJCDZC-YAOMLEK-CP2GB32-BV5RQST-3PSROAU
id2=JMFJCXB-GZDE4BN-OCJE3VF-65GYZNU-AIVJRET-3J6HMRQ-AUQIGJO-FKNHMQU

go build json.go

start() {
	echo "Starting..."
	STTRACE=model,scanner STPROFILER=":9091" syncthing -home "f1" > 1.out 2>&1 &
	STTRACE=model,scanner STPROFILER=":9092" syncthing -home "f2" > 2.out 2>&1 &
	sleep 1
}

stop() {
	echo "Stopping..."
	for i in 1 2 ; do
		curl -HX-API-Key:abc123 -X POST "http://localhost:808$i/rest/shutdown"
	done
}

setup() {
	echo "Setting up dirs..."
	mkdir -p s1
	pushd s1 >/dev/null
	rm -r */*[02468] 2>/dev/null
	rm -rf *2
	for ((i = 0; i < 500; i++)) ; do
		mkdir -p "$RANDOM/$RANDOM"
	done
	for ((i = 0; i < 500; i++)) ; do
		d="$RANDOM/$RANDOM"
		mkdir -p "$d"
		touch "$d/foo"
	done
	../md5r -d | grep -v ' . ' > ../dirs-1
	popd >/dev/null
}

testConvergence() {
	while true ; do
		sleep 5
		s1comp=$(curl -HX-API-Key:abc123 -s "http://localhost:8082/rest/debug/peerCompletion" | ./json "$id1")
		s2comp=$(curl -HX-API-Key:abc123 -s "http://localhost:8081/rest/debug/peerCompletion" | ./json "$id2")
		s1comp=${s1comp:-0}
		s2comp=${s2comp:-0}
		tot=$(($s1comp + $s2comp))
		echo $tot / 200
		if [[ $tot == 200 ]] ; then
			# when fixing up directories, a node will announce completion
			# slightly before it's actually complete. this is arguably a bug,
			# but we let it slide for the moment as long as it gets there
			# eventually.
			sleep 5
			break
		fi
	done

	echo "Verifying..."

	pushd s2 >/dev/null
	../md5r -d | grep -v ' . ' | grep -v .stversions > ../dirs-2
	popd >/dev/null

	if ! cmp dirs-1 dirs-2 ; then
		echo Repos differ
		stop
		exit 1
	fi
}

rm -rf s? s??-?
rm -rf f?/*.idx.gz f?/index

setup
start

for ((j = 0; j < iterations; j++)) ; do
	echo "#$j..."
	testConvergence
	setup
	echo "Waiting..."
	sleep 30
done

stop
