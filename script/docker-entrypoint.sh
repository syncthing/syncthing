#!/bin/sh

set -eu

if [ "$(id -u)" = '0' ]; then
  chown "${PUID}:${PGID}" "${HOME}" \
    && exec su-exec "${PUID}:${PGID}" \
       env HOME="$HOME" "$@"
else
  exec "$@"
fi
