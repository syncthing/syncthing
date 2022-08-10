// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

/*
Package discover implements the local and global device discovery protocols.

Global Discovery
================

Announcements
-------------

A device should announce itself at startup. It does this by an HTTPS POST to
the announce server URL (with the path usually being "/", but this is of
course up to the discovery server). The POST has a JSON payload listing direct
connection addresses (if any) and relay addresses (if any).

	{
		direct: ["tcp://192.0.2.45:22000", "tcp://:22202"],
		relays: [{"url": "relay://192.0.2.99:22028", "latency": 142}]
	}

It's OK for either of the "direct" or "relays" fields to be either the empty
list ([]), null, or missing entirely. An announcement with both fields missing
or empty is however not useful...

Any empty or unspecified IP addresses (i.e. addresses like tcp://:22000,
tcp://0.0.0.0:22000, tcp://[::]:22000) are interpreted as referring to the
source IP address of the announcement.

The device ID of the announcing device is not part of the announcement.
Instead, the server requires that the client perform certificate
authentication. The device ID is deduced from the presented certificate.

The server response is empty, with code 200 (OK) on success. If no certificate
was presented, status 403 (Forbidden) is returned. If the posted data doesn't
conform to the expected format, 400 (Bad Request) is returned.

In successful responses, the server may return a "Reannounce-After" header
containing the number of seconds after which the client should perform a new
announcement.

In error responses, the server may return a "Retry-After" header containing
the number of seconds after which the client should retry.

Performing announcements significantly more often than indicated by the
Reannounce-After or Retry-After headers may result in the client being
throttled. In such cases the server may respond with status code 429 (Too Many
Requests).

Queries
=======

Queries are performed as HTTPS GET requests to the announce server URL. The
requested device ID is passed as the query parameter "device", in canonical
string form, i.e. https://announce.syncthing.net/?device=ABC12345-....

Successful responses will have status code 200 (OK) and carry a JSON payload
of the same format as the announcement above. The response will not contain
empty or unspecified addresses.

If the "device" query parameter is missing or malformed, the status code 400
(Bad Request) is returned.

If the device ID is of a valid format but not found in the registry, 404 (Not
Found) is returned.

If the client has exceeded a rate limit, the server may respond with 429 (Too
Many Requests).
*/
package discover
