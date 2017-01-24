#!/bin/bash

base=https://docs.syncthing.net/man/
pages=(
	syncthing.1
	stdiscosrv.1
	strelaysrv.1
	syncthing-config.5
	syncthing-stignore.5
	syncthing-device-ids.7
	syncthing-event-api.7
	syncthing-faq.7
	syncthing-networking.7
	syncthing-rest-api.7
	syncthing-security.7
	syncthing-versioning.7
	syncthing-bep.7
	syncthing-localdisco.7
	syncthing-globaldisco.7
	syncthing-relay.7
)

for page in "${pages[@]}" ; do
	curl -sLO "$base$page"
done
