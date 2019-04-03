#!/bin/sh

set -eu

chown "${PUID}:${PGID}" /var/syncthing \
  && exec su-exec "${PUID}:${PGID}" \
     env HOME=/var/syncthing \
     /bin/syncthing \
     -home /var/syncthing/config \
     -gui-address http://0.0.0.0:8443/ \
     "$@"
