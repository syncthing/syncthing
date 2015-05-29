.. _firewall-setup:

Firewall Setup
==============

Port Forwards
-------------

If you have a NAT router which supports UPnP, the easiest way to get a working
port forward is to make sure UPnP setting is enabled on both Syncthing and the
router – Syncthing will try to handle the rest. If it succeeds you will see a
message in the console saying::

    Created UPnP port mapping for external port XXXXX on UPnP device YYYYY.

If this is not possible or desirable you should set up a port forward for port
**22000/TCP**, or the port set in the *Sync Protocol Listen Address* setting.
The external forwarded port and the internal destination port has to be the same
(i.e. 22000/TCP).

Communication in Syncthing works both ways. Therefore if you set up port
forwards for one device, other devices will be able to connect to it even when
they are behind a NAT network or firewall.

Local Firewall
--------------

If your PC has a local firewall, you will need to open the following ports for
incoming traffic:

-  Port **22000/TCP** (or the actual listening port if you have changed
   the *Sync Protocol Listen Address* setting.)
-  Port **21025/UDP** (for discovery broadcasts)

Remote Web GUI
--------------

To be able to access the web GUI from other computers, you need to change the
*GUI Listen Address* setting from the default ``127.0.0.1:8384`` to
``0.0.0.0:8384``. You also need to open the port in your local firewall if you
have one.

Tunneling via SSH
~~~~~~~~~~~~~~~~~

If you have SSH access to the machine running Syncthing but would rather not
open the web GUI port to the outside world, you can access it through a SSH
tunnel instead. You can start a tunnel with a command like the following::

    ssh -L 9999:localhost:8384 machine

This will bind to your local port 9999 and forward all connections from there to
port 8384 on the target machine. This still works even if Syncthing is bound to
listen on localhost only.

You can forward multiple ports corresponding to many machines this way, but
because Syncthing uses session cookies for the entire domain (i.e. your local
machine), you will need to connect to each control panel in a separate browser
instance or explicitly issue a browser reload when switching between them.
