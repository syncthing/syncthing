#!/bin/bash
set -euo pipefail
IFS=$'\n\t'

# Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
#
# This program is free software: you can redistribute it and/or modify it
# under the terms of the GNU General Public License as published by the Free
# Software Foundation, either version 3 of the License, or (at your option)
# any later version.
#
# This program is distributed in the hope that it will be useful, but WITHOUT
# ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
# FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
# more details.
#
# You should have received a copy of the GNU General Public License along
# with this program. If not, see <http://www.gnu.org/licenses/>.

iterations=${1:-5}

id1=I6KAH76-66SLLLB-5PFXSOA-UFJCDZC-YAOMLEK-CP2GB32-BV5RQST-3PSROAU
id2=JMFJCXB-GZDE4BN-OCJE3VF-65GYZNU-AIVJRET-3J6HMRQ-AUQIGJO-FKNHMQU

go build json.go

start() {
	echo "Starting..."
	STTRACE=model,scanner STPROFILER=":9091" ../bin/syncthing -home "f1" > 1.out 2>&1 &
	STTRACE=model,scanner STPROFILER=":9092" ../bin/syncthing -home "f2" > 2.out 2>&1 &
	sleep 1
}

stop() {
	echo "Stopping..."
	for i in 1 2 ; do
		curl -s -o /dev/null -HX-API-Key:abc123 -X POST "http://127.0.0.1:808$i/rest/shutdown"
	done
}

setup() {
	echo "Setting up dirs..."
	mkdir -p s1
	pushd s1 >/dev/null
	rm -r */*[02468] 2>/dev/null || true
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
		s1comp=$(curl -HX-API-Key:abc123 -s "http://127.0.0.1:8082/rest/debug/peerCompletion" | ./json "$id1")
		s2comp=$(curl -HX-API-Key:abc123 -s "http://127.0.0.1:8081/rest/debug/peerCompletion" | ./json "$id2")
		s1comp=${s1comp:-0}
		s2comp=${s2comp:-0}
		tot=$(($s1comp + $s2comp))
		echo $tot / 200
		if [[ $tot == 200 ]] ; then
			# when fixing up directories, a device will announce completion
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
		echo Folders differ
		stop
		exit 1
	fi
}

chmod -R +w s? s??-? || true
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
