#!/bin/sh

./test-http.sh || exit
./test-merge.sh || exit
./test-delupd.sh || exit
