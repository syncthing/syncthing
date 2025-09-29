# Docker Container for Syncthing

Use the Dockerfile in this repo, or pull the `syncthing/syncthing` image
from Docker Hub.

Use the `/var/syncthing` volume to have the synchronized files available on the
host. You can add more folders and map them as you prefer.

Note that Syncthing runs as UID 1000 and GID 1000 by default. These may be
altered with the `PUID` and `PGID` environment variables. In addition
the name of the Syncthing instance can be optionally defined by using
`--hostname=syncthing` parameter.

To grant Syncthing additional capabilities without running as root, use the
`PCAP` environment variable with the same syntax as that for `setcap(8)`.
For example, `PCAP=cap_chown,cap_fowner+ep`.

To set a different umask value, use the `UMASK` environment variable. For
example `UMASK=002`.

## Example Usage

**Docker cli**
```
$ docker pull syncthing/syncthing
$ docker run --network=host  -e STGUIADDRESS= \
    -v /wherever/st-sync:/var/syncthing \
    syncthing/syncthing:latest
```

**Docker compose**
```yml
---
version: "3"
services:
  syncthing:
    image: syncthing/syncthing
    container_name: syncthing
    hostname: my-syncthing
    environment:
      - PUID=1000
      - PGID=1000
      - STGUIADDRESS=
    volumes:
      - /wherever/st-sync:/var/syncthing
    network_mode: host
    restart: unless-stopped
    healthcheck:
      test: curl -fkLsS -m 2 127.0.0.1:8384/rest/noauth/health | grep -o --color=never OK || exit 1
      interval: 1m
      timeout: 10s
      retries: 3
```

## Discovery

Please note that Docker's default network mode prevents local IP addresses
from being discovered, as Syncthing can only see the internal IP address of
the container on the `172.17.0.0/16` subnet. This would likely break the ability
for nodes to establish LAN connections properly, resulting in poor transfer
rates unless local device addresses are configured manually.

It is therefore strongly recommended to stick to the [host network mode](https://docs.docker.com/network/host/),
as shown above.

Be aware that syncthing alone is now in control of what interfaces and ports it
listens on. You can edit the syncthing configuration to change the defaults if
there are conflicts.

## GUI Security

By default Syncthing inside the Docker image listens on `0.0.0.0:8384`. This
allows GUI connections when running without host network mode. The example
above unsets the `STGUIADDRESS` environment variable to have Syncthing fall
back to listening on what has been configured in the configuration file or the
GUI settings dialog. By default this is the localhost IP address `127.0.0.1`.
If you configure your GUI to be externally reachable, make sure you set up
authentication and enable TLS.
