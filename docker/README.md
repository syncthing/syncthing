Docker Build
============

Official builds are produced using a Docker image specified by the
Dockerfile in this directory. The following commands exactly reproduce
the official build process.

Create an image called `syncthing/build` with the build environment.

```
./build.sh docker-init
```

> This is a Debian based image containing the latest stable version of
> Go set up for cross compilation. The cross compilation uses the
> dynamically linked standard libraries and SSE instructions for amd64
> builds, but static linking and minimal instruction set for the 386 and
> arm builds. The command should be run in the main repo directory, as a
> user with permission to perform Docker operations.

Build the full set of supported binaries.

```
./build.sh docker
```

> This uses a temporary container with the image from above and a volume
> mapped to the directory containing the source. Tests are run and
> binary packages created.
