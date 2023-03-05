#!/bin/sh

set -eu

if [ "$MAXMIND_KEY" != "" ] ; then
	curl "https://download.maxmind.com/app/geoip_download?edition_id=GeoLite2-City&license_key=${MAXMIND_KEY}&suffix=tar.gz" \
	| tar --strip-components 1 -zxv
fi

exec "$@"
