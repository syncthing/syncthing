#!/bin/sh

# If NOCREATE is defined, check if configuration exists.
# Missing configuration is treated as volume not yet mounted (boot time race condition)
# and container exits for Docker to handle restart.
if [ ! -z "${NOCREATE}" -a ! -f "/var/syncthing/config/config.xml" ]; then
  echo "Volume is not mounted; waiting and quitting."
  sleep 1
  exit 1
fi

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

  chown "${PUID}:${PGID}" "${HOME}" \
    && exec su-exec "${PUID}:${PGID}" \
       env HOME="$HOME" "$@"
else
  exec "$@"
fi
