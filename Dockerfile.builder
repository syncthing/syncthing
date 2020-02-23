# We will grab the Go compiler from the latest Go image.
FROM golang as go

# Otherwise we base on the snapcraft container as that is by far the
# most complex and tricky thing to get installed and working...
FROM snapcore/snapcraft

# Go
COPY --from=go /usr/local/go /usr/local/go
ENV PATH="/usr/local/go/bin:$PATH"

# FPM to build Debian packages
RUN apt-get update && apt-get install -y --no-install-recommends \
	locales rubygems ruby-dev build-essential git \
	&& apt-get clean \
	&& rm -rf /var/lib/apt/lists/* \
	&& gem install --no-ri --no-rdoc fpm
