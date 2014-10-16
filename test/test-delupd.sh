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
id3=373HSRP-QLPNLIE-JYKZVQF-P4PKZ63-R2ZE6K3-YD442U2-JHBGBQG-WWXAHAU

go build genfiles.go
go build md5r.go
go build json.go

start() {
	echo "Starting..."
	for i in 1 2 3 ; do
		STTRACE=model,scanner STPROFILER=":909$i" ../bin/syncthing -home "h$i" > "$i.out" 2>&1 &
	done
}

stop() {
	for i in 1 2 3 ; do
		curl -s -o /dev/null -HX-API-Key:abc123 -X POST "http://127.0.0.1:808$i/rest/shutdown"
	done
	exit $1
}

testConvergence() {
	while true ; do
		sleep 5
		s1comp=$(curl -HX-API-Key:abc123 -s "http://127.0.0.1:8082/rest/debug/peerCompletion" | ./json "$id1")
		s2comp=$(curl -HX-API-Key:abc123 -s "http://127.0.0.1:8083/rest/debug/peerCompletion" | ./json "$id2")
		s3comp=$(curl -HX-API-Key:abc123 -s "http://127.0.0.1:8081/rest/debug/peerCompletion" | ./json "$id3")
		s1comp=${s1comp:-0}
		s2comp=${s2comp:-0}
		s3comp=${s3comp:-0}
		tot=$(($s1comp + $s2comp + $s3comp))
		echo $tot / 300
		if [[ $tot == 300 ]] ; then
			break
		fi
	done

	echo "Verifying..."
	cp md5-1 md5-tot
	cp md5-12-2 md5-12-tot
	cp md5-23-3 md5-23-tot

	for i in 1 2 3 12-1 12-2 23-2 23-3; do
		pushd "s$i" >/dev/null
		../md5r -l | sort | grep -v .stversions | grep -v .stfolder > ../md5-$i
		popd >/dev/null
	done

	ok=0
	for i in 1 2 3 ; do
		if ! cmp "md5-$i" md5-tot >/dev/null ; then
			echo "Fail: instance $i unconverged for default"
		else
			ok=$(($ok + 1))
			echo "OK: instance $i converged for default"
		fi
	done
	for i in 12-1 12-2 ; do
		if ! cmp "md5-$i" md5-12-tot >/dev/null ; then
			echo "Fail: instance $i unconverged for s12"
		else
			ok=$(($ok + 1))
			echo "OK: instance $i converged for s12"
		fi
	done
	for i in 23-2 23-3 ; do
		if ! cmp "md5-$i" md5-23-tot >/dev/null ; then
			echo "Fail: instance $i unconverged for s23"
		else
			ok=$(($ok + 1))
			echo "OK: instance $i converged for s23"
		fi
	done
	if [[ $ok != 7 ]] ; then
		stop 1
	fi
}

alterFiles() {
	pkill -STOP syncthing

	for i in 1 12-2 23-3 ; do
		# Delete some files
		pushd "s$i" >/dev/null
		chmod 755 ro-test
		nfiles=$(find . -type f | wc -l)
		if [[ $nfiles -ge 300 ]] ; then
			todelete=$(( $nfiles - 300 ))
			echo "  $i: deleting $todelete files..."
			set +o pipefail
			find . -type f \
				| grep -v timechanged \
				| grep -v .stfolder \
				| sort -k 1.16 \
				| head -n "$todelete" \
				| xargs rm -f
			set -o pipefail
		fi

		# Create some new files and alter existing ones
		echo "  $i: random nonoverlapping"
		../genfiles -maxexp 22 -files 200
		echo "  $i: new files in ro directory"
		uuidgen > ro-test/$(uuidgen)
		chmod 500 ro-test
		touch "timechanged-$i"

		../md5r -l | sort | grep -v .stversions | grep -v .stfolder > ../md5-$i
		popd >/dev/null
	done

	pkill -CONT syncthing

	echo "Restarting instance 2"
	curl -s -o /dev/null -HX-API-Key:abc123 -X POST "http://127.0.0.1:8082/rest/restart"
}

rm -rf h?/*.idx.gz h?/index
chmod -R u+w s? s??-? || true
rm -rf s? s??-?
mkdir s1 s2 s3 s12-1 s12-2 s23-2 s23-3

echo "Setting up files..."
for i in 1 12-2 23-3; do
	pushd "s$i" >/dev/null
	echo "  $i: random nonoverlapping"
	../genfiles -maxexp 22 -files 400
	echo "  $i: ro directory"
	mkdir ro-test
	uuidgen > ro-test/$(uuidgen)
	chmod 500 ro-test
	dd if=/dev/urandom of="timechanged-$i" bs=1024k count=1
	popd >/dev/null
done

echo "MD5-summing..."
for i in 1 12-2 23-3 ; do
	pushd "s$i" >/dev/null
	../md5r -l | grep -v .stfolder | sort > ../md5-$i
	popd >/dev/null
done

start
testConvergence

for ((t = 1; t <= $iterations; t++)) ; do
	echo "Add and remove random files ($t / $iterations)..."
	alterFiles

	echo "Waiting..."
	sleep 30
	testConvergence
done

stop 0
