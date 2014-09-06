#!/bin/bash
set -euo pipefail
IFS=$'\n\t'

#go test -tags integration -v
./test-http.sh
./test-merge.sh
./test-delupd.sh
