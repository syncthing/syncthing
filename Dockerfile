FROM golang:1.12 AS builder

WORKDIR /src
COPY . .

ENV CGO_ENABLED=0
ENV BUILD_HOST=syncthing.net
ENV BUILD_USER=docker
RUN rm -f syncthing && go run build.go -no-upgrade build syncthing

FROM alpine

EXPOSE 8384 22000 21027/udp

VOLUME ["/var/syncthing"]

RUN apk add --no-cache ca-certificates su-exec

COPY --from=builder /src/syncthing /bin/syncthing
COPY --from=builder /src/script/docker-entrypoint.sh /bin/entrypoint.sh

ENV PUID=1000 PGID=1000

HEALTHCHECK --interval=1m --timeout=10s \
  CMD nc -z localhost 8384 || exit 1

ENTRYPOINT ["/bin/entrypoint.sh", "-home", "/var/syncthing/config", "-gui-address", "0.0.0.0:8384"]
