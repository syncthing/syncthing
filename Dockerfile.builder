# It's difficult to install snapcraft (the packaging tool) on a non-snap
# distribution such as Debian without running snapd (which doesn't work under
# Docker). We cheat by taking their Docker image and snarfing the resulting
# binaries.
FROM snapcore/snapcraft AS snapcraft

# Otherwise we base on the Go image, which is Debian based.
FROM golang

# The snap stuff, as mentioned, with variables set to let the snapcraft snap play ball
COPY --from=snapcraft /snap /snap
ENV LANG="en_US.UTF-8"
ENV LANGUAGE="en_US:en"
ENV LC_ALL="en_US.UTF-8"
ENV SNAP="/snap/snapcraft/current"
ENV SNAP_NAME="snapcraft"
ENV SNAP_ARCH="amd64"
ENV PATH="$PATH:/snap/bin"

# FPM to build Debian packages
RUN apt-get update && apt-get install -y --no-install-recommends \
	locales rubygems ruby-dev \
	&& apt-get clean \
	&& rm -rf /var/lib/apt/lists/* \
	&& gem install --no-ri --no-rdoc fpm \
	&& locale-gen en_US.UTF-8
