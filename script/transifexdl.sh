#!/bin/bash
set -euo pipefail

# Download all the translations from Transifex. For some reason they don't
# ship a working CA bundle in the container so we need to hope one exists in
# the default place and map it into the container. An access token must be
# in $TRANSIFEX_TOKEN.
docker run -it --rm \
    -v $(pwd):/src -w /src \
    -v /etc/ssl/certs/ca-certificates.crt:/etc/ssl/certs/ca-certificates.crt \
    -e TX_TOKEN=$TRANSIFEX_TOKEN \
    transifex/txcli pull -a -f

# Rename ab_CD language codes to ab-CD
pushd gui/default/assets/lang
for f in *_* ; do
    mv "$f" "${f/_/-}"
done
popd
