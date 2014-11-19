FROM debian:jessie

# SCMs for "go get", gcc for cgo
RUN apt-get update && apt-get install -y \
        ca-certificates curl gcc libc6-dev make \
        bzr git mercurial unzip \
        --no-install-recommends \
        && rm -rf /var/lib/apt/lists/*

ENV GOLANG_VERSION 1.3.3

# Get the binary dist of Go to be able to bootstrap gonative.

RUN curl -sSL https://golang.org/dl/go$GOLANG_VERSION.linux-amd64.tar.gz \
        | tar -v -C /usr/local -xz

ENV PATH /usr/local/go/bin:$PATH
RUN mkdir /go
ENV GOPATH /go
ENV PATH /go/bin:$PATH
WORKDIR /go

# Use gonative to install native Go for most arch/OS combos

RUN go get github.com/calmh/gonative
RUN cd /usr/local \
        && rm -rf go \
        && gonative -version $GOLANG_VERSION

# Random build users needs to be able to create stuff in /go/pkg

RUN chmod -R 777 /go/pkg

# Rebuild the special and missing versions

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
        '

# And rebuild native without the flags above

RUN cd /usr/local/go/src \
        && ./make.bash --no-clean

# Install packages needed for test coverage

RUN go get github.com/tools/godep && go get code.google.com/p/go.tools/cmd/cover && go get github.com/axw/gocov/gocov && go get github.com/AlekSi/gocov-xml

