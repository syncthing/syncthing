ARG GOVERSION=latest
FROM golang:$GOVERSION

# FPM to build Debian packages
RUN apt-get update && apt-get install -y --no-install-recommends \
	locales rubygems ruby-dev build-essential git \
	&& apt-get clean \
	&& rm -rf /var/lib/apt/lists/* \
	&& gem install fpm
