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
	for i in 1 2 3 4 ; do
		STTRACE=files,model,puller,versioner STPROFILER=":909$i" syncthing -home "h$i" > "$i.out" 2>&1 &
	done
}

stop() {
	for i in 1 2 3 4 ; do
		curl -s -o /dev/null -HX-API-Key:abc123 -X POST "http://127.0.0.1:808$i/rest/shutdown"
	done
	exit $1
}

clean() {
	if [[ $(uname -s) == "Linux" ]] ; then
		grep -v .stversions | grep -v utf8-nfd
	else
		grep -v .stversions
	fi
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
	cat md5-? | sort | clean | uniq > md5-tot
	cat md5-12-? | sort | clean | uniq > md5-12-tot
	cat md5-23-? | sort | clean | uniq > md5-23-tot

	for i in 1 2 3 12-1 12-2 23-2 23-3; do
		pushd "s$i" >/dev/null
		../md5r -l | sort | clean > ../md5-$i
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

	# Create some new files and alter existing ones
	for i in 1 2 3 12-1 12-2 23-2 23-3 ; do
		pushd "s$i" >/dev/null

		echo "  $i: random nonoverlapping"
		../genfiles -maxexp 22 -files 200
		echo "  $i: append to large file"
		dd if=large-$i bs=1024k count=4 >> large-$i 2>/dev/null
		../md5r -l > ../md5-tmp
		(grep -v large ../md5-tmp ; grep "large-$i" ../md5-tmp) | grep -v '/.syncthing.' > ../md5-$i
		popd >/dev/null
	done

	pkill -CONT syncthing
}

rm -rf h?/*.idx.gz h?/index
chmod -R +w s? s??-? s4d || true
rm -rf s? s??-? s4d

echo "Setting up files..."
for i in 1 2 3 12-1 12-2 23-2 23-3; do
	mkdir "s$i"
	pushd "s$i" >/dev/null
	echo "  $i: random nonoverlapping"
	../genfiles -maxexp 22 -files 200
	echo "  $i: empty file"
	touch "empty-$i"
	echo "  $i: large file"
	dd if=/dev/urandom of=large-$i bs=1024k count=15 2>/dev/null
	echo "  $i: weird encodings"
	echo somedata > "$(echo -e utf8-nfc-\\xc3\\xad)-$i"
	echo somedata > "$(echo -e utf8-nfd-i\\xcc\\x81)-$i"
	echo somedata > "$(echo -e cp850-\\xa1)-$i"
	touch "empty-$i"
	popd >/dev/null
done

mkdir s4d
echo somerandomdata > s4d/extrafile

echo "MD5-summing..."
for i in 1 2 3 12-1 12-2 23-2 23-3 ; do
	pushd "s$i" >/dev/null
	../md5r -l > ../md5-$i
	popd >/dev/null
done

start
testConvergence

for ((t = 1; t <= $iterations; t++)) ; do
	echo "Add and alter random files ($t / $iterations)..."
	alterFiles

	echo "Waiting..."
	sleep 30
	testConvergence
done

stop 0
