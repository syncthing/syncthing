.. _custom-relaysrv:

Custom Relay Server
===================

Syncthing relies on a network of community-contributed relay servers. Anyone can
run a relay server, and it will automatically join the relay pool and be
available to Syncthing users. The current list of relays can be found at
https://relays.syncthing.net.

To run a relay of your own, you will first need a server. Running a relay from a
home computer is not recommended: the relay ideally needs to be running 24/7,
and home internet connections often have poor upload bandwidth.

The recommended specifications for a relay are:

========= ==============
CPU       At least 1GHz
RAM       At least 256MB
Bandwidth At least 1MBit/s in and out, the higher the better
Traffic   At least 1TiB/month, the more the better
========= ==============

VPS's with these specs are available for about $5-$10/month.

Installing and Running
~~~~~~~~~~~~~~~~~~~~~~

Download the latest release of the `relay server <https://github.com/syncthing/relaysrv/releases>`__
for your operating system and architecture. Unpack the archive and save the
binary to a convenient place such as `/usr/local/bin`.

The relay server takes a number of options, some of which are important for
smooth operation::

    $ relaysrv --help
    Usage of relaysrv:
      -debug
            Enable debug output
      -ext-address string
            An optional address to advertising as being available on.
            Allows listening on an unprivileged port with port forwarding from e.g.
            443, and be connected to on port 443.
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
