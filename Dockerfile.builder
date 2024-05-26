ARG GOVERSION=latest
FROM golang:$GOVERSION

LABEL org.opencontainers.image.authors="The Syncthing Project" \
      org.opencontainers.image.url="https://syncthing.net" \
      org.opencontainers.image.documentation="https://docs.syncthing.net" \
      org.opencontainers.image.source="https://github.com/syncthing/syncthing" \
      org.opencontainers.image.vendor="The Syncthing Project" \
      org.opencontainers.image.licenses="MPL-2.0" \
      org.opencontainers.image.title="Syncthing Builder"

# FPM to build Debian packages
RUN apt-get update && apt-get install -y --no-install-recommends \
	locales rubygems ruby-dev build-essential git \
	&& apt-get clean \
	&& rm -rf /var/lib/apt/lists/* \
	&& gem install fpm
