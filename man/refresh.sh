#!/bin/sh

base=http://docs.syncthing.net/man/
pages=(syncthing-config.5 syncthing-device-ids.7 syncthing-event-api.7 syncthing-faq.7 syncthing-networking.7 syncthing-rest-api.7 syncthing-security.7 syncthing-stignore.5 syncthing-versioning.7 syncthing.1)

for page in "${pages[@]}" ; do
	curl -sLO "$base$page"
done
