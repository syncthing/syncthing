#!/bin/bash

iterations=${1:-5}

id1=I6KAH7666SLLL5PFXSOAUFJCDZYAOMLEKCP2GB3BV5RQST3PSROA
id2=JMFJCXBGZDE4BOCJE3VF65GYZNAIVJRET3J6HMRAUQIGJOFKNHMQ
id3=373HSRPQLPNLIJYKZVQFP4PKZ6R2ZE6K3YD442UJHBGBQGWWXAHA

go build json.go

start() {
	echo "Starting..."
	for i in 1 2 ; do
		STPROFILER=":909$i" syncthing -home "f$i" &
	done
}

stop() {
	echo "Stopping..."
	for i in 1 2 ; do
		curl -X POST "http://localhost:808$i/rest/shutdown"
	done
}

setup() {
	echo "Setting up dirs..."
	mkdir -p s1
	pushd s1 >/dev/null
	rmdir */*[02468] 2>/dev/null
	rm -rf *2
	for ((i = 0; i < 1000; i++)) ; do
		mkdir -p $RANDOM/$RANDOM
	done
	popd >/dev/null
}

testConvergence() {
	while true ; do
		sleep 5
		s1comp=$(curl -s "http://localhost:8082/rest/connections" | ./json "$id1/Completion")
		s2comp=$(curl -s "http://localhost:8081/rest/connections" | ./json "$id2/Completion")
		s1comp=${s1comp:-0}
		s2comp=${s2comp:-0}
		tot=$(($s1comp + $s2comp))
		echo $tot / 200
		if [[ $tot == 200 ]] ; then
			break
		fi
	done

	echo "Verifying..."

	pushd s1 >/dev/null
	find . -type d | sort > ../dirs-1
	popd >/dev/null

	pushd s2 >/dev/null
	find . -type d | sort > ../dirs-2
	popd >/dev/null

	if ! cmp dirs-1 dirs-2 ; then
		echo Repos differ
		stop
		exit 1
	fi
}

rm -rf s? s??-?
rm -f h?/*.idx.gz

setup
start

for ((j = 0; j < 10; j++)) ; do
	echo "#$j..."
	testConvergence
	setup
	echo "Waiting..."
	sleep 30
done

stop
