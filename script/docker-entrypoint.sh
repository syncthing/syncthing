#!/bin/sh

set -eu

chown "${PUID}:${PGID}" "${HOME}" \
  && exec su-exec "${PUID}:${PGID}" \
     env HOME="$HOME" "$@"
