#!/bin/bash

# Copyright (C) 2014 Jakob Borg and other contributors. All rights reserved.
# Use of this source code is governed by an MIT-style license that can be
# found in the LICENSE file.

id1=I6KAH76-66SLLLB-5PFXSOA-UFJCDZC-YAOMLEK-CP2GB32-BV5RQST-3PSROAU
id2=JMFJCXB-GZDE4BN-OCJE3VF-65GYZNU-AIVJRET-3J6HMRQ-AUQIGJO-FKNHMQU

go build json.go
go build md5r.go
go build genfiles.go

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
	sleep 1
}

setup() {
	echo "Setting up..."
	rm -rf s? s??-?
	rm -rf f?/*.idx.gz f?/index
	mkdir -p s1
	pushd s1 >/dev/null
	../genfiles
	../md5r > ../md5-1
	popd >/dev/null
}

testConvergence() {
	torestart="$1"
	prevcomp=0

	while true ; do
		sleep 5
		comp=$(curl -HX-API-Key:abc123 -s "http://localhost:8081/rest/debug/peerCompletion" | ./json "$id2")
		comp=${comp:-0}
		echo $comp / 100

		if [[ $comp == 100 ]] ; then
			echo Done
			break
		fi

		# Restart if the destination has made some progress
		if [[ $comp -gt $prevcomp ]] ; then
			prevcomp=$comp
			curl -HX-API-Key:abc123 -X POST "http://localhost:$torestart/rest/restart"
		fi
	done

	echo "Verifying..."

	pushd s2 >/dev/null
	../md5r | grep -v .stversions > ../md5-2
	popd >/dev/null

	if ! cmp md5-1 md5-2 ; then
		echo Repos differ
		stop
		exit 1
	fi
}

echo Testing reconnects during pull where the source node restarts
setup
start
testConvergence 8081
stop

echo Testing reconnects during pull where the destination node restarts
setup
start
testConvergence 8082
stop

exit 0
