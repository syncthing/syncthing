#!/bin/bash
set -euo pipefail
IFS=$'\n\t'

./test-http.sh
./test-merge.sh
./test-delupd.sh
go test -tags integration -v
