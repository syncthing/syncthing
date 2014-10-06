#!/bin/bash
set -euo pipefail
IFS=$'\n\t'

go test -tags integration -v -short
./test-merge.sh
./test-delupd.sh
