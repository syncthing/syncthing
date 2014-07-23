#!/bin/sh

./test-http.sh || exit
./test-merge.sh || exit
./test-delupd.sh || exit
# ./test-folders.sh || exit
./test-reconnect.sh || exit
