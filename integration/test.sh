#!/bin/bash

export STNORESTART=1

iterations=5

id1=I6KAH7666SLLL5PFXSOAUFJCDZYAOMLEKCP2GB3BV5RQST3PSROA
id2=JMFJCXBGZDE4BOCJE3VF65GYZNAIVJRET3J6HMRAUQIGJOFKNHMQ
id3=373HSRPQLPNLIJYKZVQFP4PKZ6R2ZE6K3YD442UJHBGBQGWWXAHA

go build genfiles.go
go build md5r.go
go build json.go

testConvergence() {
	echo "Starting..."
	for i in 1 2 3 ; do
		STPROFILER=":909$i" syncthing -home "h$i" &
	done

	while true ; do
		sleep 5
		s1comp=$(curl -s "http://localhost:8082/rest/connections" | ./json "$id1/Completion")
		s2comp=$(curl -s "http://localhost:8083/rest/connections" | ./json "$id2/Completion")
		s3comp=$(curl -s "http://localhost:8081/rest/connections" | ./json "$id3/Completion")
		s1comp=${s1comp:-0}
		s2comp=${s2comp:-0}
		s3comp=${s3comp:-0}
		tot=$(($s1comp + $s2comp + $s3comp))
		echo $tot / 300
		if [[ $tot == 300 ]] ; then
			echo "Stopping..."
			pkill syncthing
			break
		fi
	done

	echo "Verifying..."
	cat md5-? | sort | uniq > md5-tot
	cat md5-12-? | sort | uniq > md5-12-tot
	cat md5-23-? | sort | uniq > md5-23-tot

	for i in 1 2 3 12-1 12-2 23-2 23-3; do
		pushd "s$i" >/dev/null
		../md5r -l | sort > ../md5-$i
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
		exit 1
	fi
}

echo "Setting up files..."
for i in 1 2 3 12-1 12-2 23-2 23-3; do
	rm -f h$i/*.idx.gz
	rm -rf "s$i"
	mkdir "s$i"
	pushd "s$i" >/dev/null
	echo "  $i: random nonoverlapping"
	../genfiles -maxexp 22 -files 600
	echo "  $i: empty file"
	touch "empty-$i"
	echo "  $i: large file"
	dd if=/dev/urandom of=large-$i bs=1024k count=55 2>/dev/null
	popd >/dev/null
done

echo "MD5-summing..."
for i in 1 2 3 12-1 12-2 23-2 23-3 ; do
	pushd "s$i" >/dev/null
	../md5r -l > ../md5-$i
	popd >/dev/null
done

testConvergence

for ((t = 0; t < $iterations; t++)) ; do
	echo "Add and remove random files ($((t+1)) / $iterations)..."
	for i in 1 2 3 12-1 12-2 23-2 23-3 ; do
		pushd "s$i" >/dev/null
		rm -rf */?[02468ace]
		../genfiles -maxexp 22 -files 600
		echo "  $i: append to large file"
		dd if=/dev/urandom bs=1024k count=4 >> large-$i 2>/dev/null
		../md5r -l > ../md5-tmp
		(grep -v large ../md5-tmp ; grep "large-$i" ../md5-tmp) > ../md5-$i
		popd >/dev/null
	done

	testConvergence
done

