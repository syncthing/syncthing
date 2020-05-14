#!/bin/sh

set -eu

chown -R "${PUID}:${PGID}" "${HOME}" \
  && exec su-exec "${PUID}:${PGID}" \
     env HOME="$HOME" "$@"
