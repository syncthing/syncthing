#!/bin/bash
set -euo pipefail
IFS=$'\n\t'

# Copyright (C) 2014 Jakob Borg and other contributors. All rights reserved.
# Use of this source code is governed by an MIT-style license that can be
# found in the LICENSE file.

id1=I6KAH76-66SLLLB-5PFXSOA-UFJCDZC-YAOMLEK-CP2GB32-BV5RQST-3PSROAU
id2=JMFJCXB-GZDE4BN-OCJE3VF-65GYZNU-AIVJRET-3J6HMRQ-AUQIGJO-FKNHMQU
id3=373HSRP-QLPNLIE-JYKZVQF-P4PKZ63-R2ZE6K3-YD442U2-JHBGBQG-WWXAHAU

stop() {
	echo Stopping
	curl -s -o/dev/null -HX-API-Key:abc123 -X POST http://127.0.0.1:8081/rest/shutdown
	curl -s -o/dev/null -HX-API-Key:abc123 -X POST http://127.0.0.1:8082/rest/shutdown
	exit $1
}

echo Building
go build http.go

echo Starting
chmod -R +w s1 s2 || true
rm -rf s1 s2 h1/index h2/index
syncthing -home h1 > 1.out 2>&1 &
syncthing -home h2 > 2.out 2>&1 &
sleep 1

echo Fetching CSRF tokens
curl -s -o /dev/null http://testuser:testpass@127.0.0.1:8081/index.html
curl -s -o /dev/null http://127.0.0.1:8082/index.html
sleep 1

echo Testing
./http -target 127.0.0.1:8081 -user testuser -pass testpass -csrf h1/csrftokens.txt || stop 1
./http -target 127.0.0.1:8081 -api abc123 || stop 1
./http -target 127.0.0.1:8082 -csrf h2/csrftokens.txt || stop 1
./http -target 127.0.0.1:8082 -api abc123 || stop 1

stop 0
