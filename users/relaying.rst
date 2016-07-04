.. _relaying:

Relaying
========

.. versionadded:: 0.12.0

Syncthing can bounce traffic via a *relay* when it's not possible to establish
a direct connection between two devices. There are a number of public relays
available for this purpose. The advantage is that it makes a connection
possible where it would otherwise not be; the downside is that the transfer rate
is much lower than a direct connection would allow.

Relaying is enabled by default but will only be used if two devices are unable
to communicate directly with each other. When connected via a relay, Syncthing
will periodically retry a direct connection and, if one is established, stop
communicating via the relay.

Security
--------

The connection between two devices is still end to end encrypted, the relay
only retransmits the encrypted data much like a router. However, a device must
register with a relay in order to be reachable over that relay, so the relay
knows your IP and device ID. In that respect it is similar to a discovery
server. The relay operator can see the amount of traffic flowing between
devices.

Running Your Own Relay
----------------------

See :ref:`strelaysrv`.
