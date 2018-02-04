FROM golang:1.9 AS builder

WORKDIR /go/src/github.com/syncthing/syncthing
COPY . .

ENV CGO_ENABLED=0
ENV BUILD_HOST=syncthing.net
ENV BUILD_USER=docker
RUN rm -f syncthing && go run build.go build syncthing

FROM alpine

RUN apk add --no-cache ca-certificates

COPY --from=builder /go/src/github.com/syncthing/syncthing/syncthing /bin/syncthing

RUN echo 'syncthing:x:1000:1000::/var/syncthing:/sbin/nologin' >> /etc/passwd \
    && echo 'syncthing:!::0:::::' >> /etc/shadow \
    && mkdir /var/syncthing \
    && chown syncthing /var/syncthing

USER syncthing
ENV STNOUPGRADE=1

HEALTHCHECK --interval=1m --timeout=10s \
  CMD nc -z localhost 8384 || exit 1

ENTRYPOINT ["/bin/syncthing", "-home", "/var/syncthing/config", "-gui-address", "0.0.0.0:8384"]

