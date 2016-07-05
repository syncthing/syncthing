#!/bin/bash
set -euo pipefail

git pull
make clean html man latexpdf

rm -rf _deployed.old
[ -d _deployed ] && mv _deployed _deployed.old || true
mv _build _deployed

