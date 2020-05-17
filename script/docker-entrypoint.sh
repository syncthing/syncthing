#!/bin/sh

set -eu

mkdir -p "/var/syncthing/config" \
  && chown "${PUID}:${PGID}" "/var/syncthing" \
  && chown -R "${PUID}:${PGID}" "/var/syncthing/config" \
  && exec su-exec "${PUID}:${PGID}" \
     env HOME="$HOME" "$@"
