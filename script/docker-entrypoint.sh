#!/bin/sh

set -eu

if [ "$(id -u)" = '0' ]; then
  binary="$1"
  if [ "${PCAP:-}" == "" ] ; then
    # If Syncthing should have no extra capabilities, make sure to remove them
    # from the binary. This will fail with an error if there are no
    # capabilities to remove, hence the || true etc.
    setcap -r "$binary" 2>/dev/null || true
  else
    # Set capabilities on the Syncthing binary before launching it.
    setcap "$PCAP" "$binary"
  fi

  # Chown may fail, which may cause us to be unable to start; but maybe
  # it'll work anyway, so we the error slide.
  chown "${PUID}:${PGID}" "${HOME}" || true
  exec su-exec "${PUID}:${PGID}" \
       env HOME="$HOME" "$@"
else
  exec "$@"
fi
