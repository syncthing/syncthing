#!/bin/bash

set -e

echo '# linux arm7'
GOARM=7 GOARCH=arm GOOS=linux go build
echo '# linux arm5'
GOARM=5 GOARCH=arm GOOS=linux go build
echo '# windows 386'
GOARCH=386 GOOS=windows go build
echo '# windows amd64'
GOARCH=amd64 GOOS=windows go build
echo '# darwin'
GOARCH=amd64 GOOS=darwin go build
echo '# freebsd'
GOARCH=amd64 GOOS=freebsd go build
