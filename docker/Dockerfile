FROM debian:squeeze
MAINTAINER Jakob Borg <jakob@nym.se>

ENV GOLANG_VERSION 1.4

# SCMs for "go get", gcc for cgo
RUN apt-get update && apt-get install -y \
        ca-certificates curl gcc libc6-dev make \
        bzr git mercurial unzip patch \
        --no-install-recommends \
	&& apt-get clean \
        && rm -rf /var/lib/apt/lists/*

# Get the binary dist of Go to be able to bootstrap gonative.

RUN curl -sSL https://golang.org/dl/go${GOLANG_VERSION}.linux-amd64.tar.gz \
        | tar -v -C /usr/local -xz

ENV PATH /usr/local/go/bin:$PATH
RUN mkdir /go
ENV GOPATH /go
ENV PATH /go/bin:$PATH
WORKDIR /go

# Use gonative to install native Go for most arch/OS combos

RUN go get github.com/calmh/gonative \
	&& cd /usr/local \
        && rm -rf go \
        && gonative -version $GOLANG_VERSION

# Rebuild the special and missing versions, using patches as appropriate

RUN mkdir /tmp/patches
ADD *.patch /tmp/patches/
RUN bash -xec '\
	cd /usr/local/go ; \
	for patch in /tmp/patches/*.patch ; do \
		patch -p0 < "$patch" ; \
	done \
	'

RUN bash -xec '\
                cd /usr/local/go/src; \
                for platform in linux/386 freebsd/386 windows/386 linux/arm openbsd/amd64 openbsd/386; do \
                        GOOS=${platform%/*} \
                        GOARCH=${platform##*/} \
                        GOARM=5 \
                        GO386=387 \
                        CGO_ENABLED=0 \
                        ./make.bash --no-clean 2>&1; \
                done \
		&& ./make.bash --no-clean \
        '

# Install packages needed for test coverage

RUN go get github.com/tools/godep \
	&& go get golang.org/x/tools/cmd/cover \
	&& go get github.com/axw/gocov/gocov \
	&& go get github.com/AlekSi/gocov-xml

# Install tools "go vet" and "golint"

RUN go get golang.org/x/tools/cmd/vet \
        && go get github.com/golang/lint/golint

# Build standard library for race

RUN go install -race std

# Random build users needs to be able to create stuff in /go

RUN chmod -R 777 /go/bin /go/pkg /go/src
