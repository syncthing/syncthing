.. _relaying:

Relaying
========

.. versionadded:: 0.12.0

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

Running Your Own Relay
----------------------

To run a relay of your own, download the latest release of the `relay server <https://github.com/syncthing/relaysrv/releases>`__
for your operating system and architecture. Unpack the archive and save the
binary to a convenient place such as `/usr/local/bin`.

The relay server takes a number of options, some of which are important for
smooth operation::

    $ relaysrv --help
    Usage of relaysrv:
      -debug
            Enable debug output
      -global-rate int
            Global rate limit, in bytes/s
      -keys string
            Directory where cert.pem and key.pem is stored (default ".")
      -listen string
            Protocol listen address (default ":22067")
      -message-timeout duration
            Maximum amount of time we wait for relevant messages to arrive (default 1m0s)
      -network-timeout duration
            Timeout for network operations between the client and the relay.
            If no data is received between the client and the relay in this
            period of time, the connection is terminated. Furthermore, if no
            data is sent between either clients being relayed within this
            period of time, the session is also terminated. (default 2m0s)
      -per-session-rate int
            Per session rate limit, in bytes/s
      -ping-interval duration
            How often pings are sent (default 1m0s)
      -pools string
            Comma separated list of relay pool addresses to join (default "https://relays.syncthing.net/endpoint")
      -provided-by string
            An optional description about who provides the relay
      -status-srv string
            Listen address for status service (blank to disable) (default ":22070")

Primarily, you need to decide on a directory to store the TLS key and
certificate and a listen port. The default listen port of 22067 works, but for
optimal compatibility a well known port for encrypted traffic such as 443 is
recommended. This may require `additional setup
<https://wiki.apache.org/httpd/NonRootPortBinding>`__ to work without running
as root or a privileged user. In principle something similar to this should
work on a Linux/Unix system::

    $ sudo useradd relaysrv
    $ sudo mkdir /etc/relaysrv
    $ sudo chown relaysrv /etc/relaysrv
    $ sudo -u relaysrv /usr/local/relaysrv -keys /etc/relaysrv

This creates a user ``relaysrv`` and a directory ``/etc/relaysrv`` to store
the keys. The keys are generated on first startup. The relay will join the
global relay pool, unless a ``-pools=""`` argument is given.

To make the relay server start automatically at boot, use the recommended
procedure for your operating system.
