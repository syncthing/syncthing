#!/bin/bash

set -e

# check there are no formatting issues
GOFMT_LINES=`gofmt -l . | wc -l | xargs`
test $GOFMT_LINES -eq 0 || echo "gofmt needs to be run, ${GOFMT_LINES} files have issues"

# run the tests for the root package
go test .
