# Docker Container for Syncthing

Use the Dockerfile in this repo, or pull the `syncthing/syncthing` image
from Docker Hub.

Use the `/var/syncthing` volume to have the synchronized files available on the
host. You can add more folders and map them as you prefer.

Note that Syncthing runs as UID 1000 and GID 1000 by default. These may be
altered with the ``PUID`` and ``PGID`` environment variables. In addition
the name of the Syncthing instance can be optionally defined by using
``--hostname=syncthing`` parameter.

## Example Usage

**Docker cli**
```
$ docker pull syncthing/syncthing
$ docker run -p 8384:8384 -p 22000:22000/tcp -p 22000:22000/udp \
    -v /wherever/st-sync:/var/syncthing \
    --hostname=my-syncthing \
    syncthing/syncthing:latest
```

**Docker compose**
```
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
    volumes:
      - /wherever/st-sync:/var/syncthing
    ports:
      - 8384:8384
      - 22000:22000/tcp
      - 22000:22000/udp
    restart: unless-stopped
```

## Discovery

Note that local device discovery will not work with the above command,
resulting in poor local transfer rates if local device addresses are not
manually configured.

To allow local discovery, the docker host network can be used instead:

```
$ docker pull syncthing/syncthing
$ docker run --network=host \
    -v /wherever/st-sync:/var/syncthing \
    syncthing/syncthing:latest
```

Be aware that syncthing alone is now in control of what interfaces and ports it
listens on. You can edit the syncthing configuration to change the defaults if
there are conflicts.

## GUI Security

By default Syncthing inside the Docker image listens on 0.0.0.0:8384 to
allow GUI connections via the Docker proxy. This is set by the
`STGUIADDRESS` environment variable in the Dockerfile, as it differs from
what Syncthing would otherwise use by default. This means you should set up
authentication in the GUI, like for any other externally reachable Syncthing
instance. If you do not require the GUI, or you use host networking, you can
unset the `STGUIADDRESS` variable to have Syncthing fall back to listening
on 127.0.0.1:

```
$ docker pull syncthing/syncthing
$ docker run -e STGUIADDRESS= \
    -v /wherever/st-sync:/var/syncthing \
    syncthing/syncthing:latest
```

With the environment variable unset Syncthing will follow what is set in the
configuration file / GUI settings dialog.
