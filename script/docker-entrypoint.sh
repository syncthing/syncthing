#!/bin/sh

set -eu

chown "${PUID}:${PGID}" /var/syncthing \
  && exec su-exec "${PUID}:${PGID}" \
     env HOME=/var/syncthing \
     /bin/syncthing "$@"
