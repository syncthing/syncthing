ARG SERVE_U=amd64
ARG SERVE_R=alpine

### Build ###
FROM golang:1.11-alpine AS builder

ARG BUILD_GOOS=linux
ARG BUILD_GOARCH=amd64
ENV CGO_ENABLED=0
ENV BUILD_HOST=syncthing.net
ENV BUILD_USER=docker

RUN apk add --no-cache git

WORKDIR /go/src/github.com/syncthing/syncthing
COPY . .
RUN go run build.go -no-upgrade -goos=$BUILD_GOOS -goarch=$BUILD_GOARCH build

### Serve ###
FROM $SERVE_U/$SERVE_R

ARG QEMUARCH=amd64
ENV PUID=1000 PGID=1000
EXPOSE 8384 22000 21027/udp
VOLUME ["/var/syncthing"]

__MULTIARCH_COPY qemu-${QEMUARCH}-static /usr/bin/
RUN apk add --update --no-cache ca-certificates su-exec
__MULTIARCH_RUN rm /usr/bin/qemu-${QEMUARCH}-static
COPY --from=builder /go/src/github.com/syncthing/syncthing/syncthing /bin/syncthing

HEALTHCHECK --interval=1m --timeout=10s \
  CMD nc -z localhost 8384 || exit 1
ENTRYPOINT \
  chown "${PUID}:${PGID}" /var/syncthing \
  && su-exec "${PUID}:${PGID}" \
     env HOME=/var/syncthing \
     /bin/syncthing \
       -home /var/syncthing/config \
       -gui-address 0.0.0.0:8384
