.. _strelaysrv:

Syncthing Relay Server
======================

Synopsis
--------

::

    strelaysrv [-debug] [-ext-address=<address>] [-global-rate=<bytes/s>] [-keys=<dir>] [-listen=<listen addr>]
               [-message-timeout=<duration>] [-network-timeout=<duration>] [-per-session-rate=<bytes/s>]
               [-ping-interval=<duration>] [-pools=<pool addresses>] [-provided-by=<string>] [-status-srv=<listen addr>]

Description
-----------

Syncthing relies on a network of community-contributed relay servers. Anyone
can run a relay server, and it will automatically join the relay pool and be
available to Syncthing users. The current list of relays can be found at
https://relays.syncthing.net.

Options
-------

.. cmdoption:: -debug

    Enable debug output.

.. cmdoption:: -ext-address=<address>

    An optional address to advertising as being available on. Allows listening
    on an unprivileged port with port forwarding from e.g. 443, and be
    connected to on port 443.

.. cmdoption:: -global-rate=<bytes/s>

    Global rate limit, in bytes/s.

.. cmdoption:: -keys=<dir>

    Directory where cert.pem and key.pem is stored (default ".").

.. cmdoption:: -listen=<listen addr>

    Protocol listen address (default ":22067").

.. cmdoption:: -message-timeout=<duration>

    Maximum amount of time we wait for relevant messages to arrive (default 1m0s).

.. cmdoption:: -network-timeout=<duration>

    Timeout for network operations between the client and the relay. If no data
    is received between the client and the relay in this period of time, the
    connection is terminated. Furthermore, if no data is sent between either
    clients being relayed within this period of time, the session is also
    terminated. (default 2m0s)

.. cmdoption:: -per-session-rate=<bytes/s>

    Per session rate limit, in bytes/s.

.. cmdoption:: -ping-interval=<duration>

    How often pings are sent (default 1m0s).

.. cmdoption:: -pools=<pool addresses>

    Comma separated list of relay pool addresses to join (default
    "https://relays.syncthing.net/endpoint"). Blank to disable announcement to
    a pool, thereby remaining a private relay.

.. cmdoption:: -provided-by=<string>

    An optional description about who provides the relay.

.. cmdoption:: -status-srv=<listen addr>

    Listen address for status service (blank to disable) (default ":22070").


Setting Up
----------

Primarily, you need to decide on a directory to store the TLS key and
certificate and a listen port. The default listen port of 22067 works, but for
optimal compatibility a well known port for encrypted traffic such as 443 is
recommended. This may require additional setup to work without running
as root or a privileged user, see `Running on port 443 as an unprivileged user`_
below. In principle something similar to this should work on a Linux/Unix
system::

    $ sudo useradd relaysrv
    $ sudo mkdir /etc/relaysrv
    $ sudo chown relaysrv /etc/relaysrv
    $ sudo -u relaysrv /usr/local/bin/relaysrv -keys /etc/relaysrv

This creates a user ``relaysrv`` and a directory ``/etc/relaysrv`` to store
the keys. The keys are generated on first startup. The relay will join the
global relay pool, unless a ``-pools=""`` argument is given.

To make the relay server start automatically at boot, use the recommended
procedure for your operating system.

Running on port 443 as an unprivileged user
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

It is recommended that you run the relay on port 443 (or another port which is
commonly allowed through corporate firewalls), in order to maximise the chances
that people are able to connect. However, binding to ports below 1024 requires
root privileges, and running a relay as root is not recommended. Thankfully
there are a couple of approaches available to you.

One option is to run the relay on port 22067, and use an ``iptables`` rule
to forward traffic from port 443 to port 22067, for example::

    iptables -t nat -A PREROUTING -i eth0 -p tcp --dport 443 -j REDIRECT --to-port 22067

Or, if you're using ``ufw``, add the following to ``/etc/ufw/before.rules``::

    *nat
    :PREROUTING ACCEPT [0:0]
    :POSTROUTING ACCEPT [0:0]

    -A PREROUTING -i eth0 -p tcp --dport 443 -j REDIRECT --to-port 22067

    COMMIT

You will need to start ``relaysrv`` with ``-ext-address ":443"``. This tells
``relaysrv`` that it can be contacted on port 443, even though it is listening
on port 22067. You will also need to let both port 443 and 22067 through your
firewall.

Another option is `described here <https://wiki.apache.org/httpd/NonRootPortBinding>`__,
although your milage may vary.

See Also
--------

:manpage:`syncthing-relay(7)`, :manpage:`syncthing-faq(7)`,
:manpage:`syncthing-networking(7)`
