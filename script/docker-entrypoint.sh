#!/bin/sh

set -eu

chown -R "${PUID}:${PGID}" "/var/syncthing/config" || true \
   && exec su-exec "${PUID}:${PGID}" \
     env HOME="$HOME" "$@"
