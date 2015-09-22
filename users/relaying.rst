.. _relaying:

Relaying
========

Syncthing can bounce traffic via a *relay* when it's not possible to establish
a direct connection between two devices. There are a number of public relays
available for this purpose. The advantage is that it makes a connection
possible where it would otherwise not be; the downside is that the transfer
rate is much lower than a direct connection would allow. When connected via a
relay, Syncthing will periodically retry a direct connection.

Relaying is enabled by default.

Security
--------

The connection between two devices is still end to end encrypted, the relay
only retransmits the encrypted data much like a router. However, a device must
register with a relay in order to be reachable over that relay, so the relay
knows your IP and device ID. In that respect it is similar to a discovery
server. The relay operator can see the amount of traffic flowing between
devices.

Configuration
-------------

Running Your Own Relay
----------------------

.. versionadded:: 0.12.0
