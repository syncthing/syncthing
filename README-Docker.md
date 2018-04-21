# Docker Container for Syncthing

Use the Dockerfile in this repo, or pull the `syncthing/syncthing` image
from Docker Hub. Use volumes to have the synchronized files available on the
host.

The exposed volumes are by default:

    /var/syncthing/config   - the configuration and index directory into the Container
    /var/syncthing          - the default sync folder into the Container

You can add more folders and map them as you prefer.

Note that Syncthing runs as UID 1000 in the container. This UID must have
permission to read and modify the files in the containers.

Example usage:

```
$ docker pull syncthing/syncthing
$ docker run -p 8384:8384 -p 22000:22000 \
    -v /wherever/st-cfg:/var/syncthing/config \
    -v /wherever/st-sync:/var/syncthing \
    syncthing/syncthing:latest
```

Note that local device discovery will not work with the above command resulting
in poor local transfer rates if local device addresses are not manually
configured.

To allow local discovery, the docker host network can be used instead:

```
$ docker pull syncthing/syncthing
$ docker run --network=host \
    -v /wherever/st-cfg:/var/syncthing/config \
    -v /wherever/st-sync:/var/syncthing \
    syncthing/syncthing:latest
```

Be aware that syncthing alone is now in control of what interfaces and ports it
listens on. You can edit the syncthing configuration to change the defaults if
there are conflicts.
