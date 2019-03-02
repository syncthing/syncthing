### Build ###
FROM golang:1.11-alpine AS builder

ENV CGO_ENABLED=0
ENV BUILD_HOST=syncthing.net
ENV BUILD_USER=docker

RUN apk add --no-cache git

WORKDIR /src
COPY . .
RUN go run build.go -no-upgrade build

### Serve ###
FROM alpine

ENV PUID=1000 PGID=1000
EXPOSE 8384 22000 21027/udp
VOLUME ["/var/syncthing"]

RUN apk add --update --no-cache ca-certificates su-exec
COPY --from=builder /src/syncthing /bin/syncthing

HEALTHCHECK --interval=1m --timeout=10s \
  CMD nc -z localhost 8384 || exit 1
ENTRYPOINT \
  chown "${PUID}:${PGID}" /var/syncthing \
  && su-exec "${PUID}:${PGID}" \
     env HOME=/var/syncthing \
     /bin/syncthing \
       -home /var/syncthing/config \
       -gui-address 0.0.0.0:8384
