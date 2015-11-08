Running a Discovery Server
==========================

.. note:: This describes the procedure for a v0.12 discovery server.

Description
-----------

This guide assumes that you have already set up Syncthing. If you
haven't yet, head over to :ref:`getting-started` first.

Installing
----------

Go to `releases <https://github.com/syncthing/discosrv/releases>`__ and
download the file appropriate for your operating system. Unpacking it will
yield a binary called ``discosrv`` (or ``discosrv.exe`` on Windows). Start
this in whatever way you are most comfortable with; double clicking should
work in any graphical environment. At first start, discosrv will generate the
directory ``/var/discosrv`` (``X:\var\discosrv`` on Windows, where X is the
partition ``discosrv.exe`` is executed from) with configuration. If the user
running ``discosrv`` doesn't have permission to do so, create the directory
and set the owner appropriately or use the command line switches (see below)
to select a different location.

Configuring
-----------

Running discosrv with non-default settings requires passing the
respective parameters to discosrv on every start. ``discosrv -help``
gives you all the tweakables with their defaults:

::

  Usage of discosrv:
    -cert string
        Certificate file (default "cert.pem")
    -db-backend string
        Database backend to use (default "ql")
    -db-dsn string
        Database DSN (default "memory://discosrv")
    -debug
        Debug
    -key string
        Key file (default "key.pem")
    -limit-avg int
        Allowed average package rate, per 10 s (default 5)
    -limit-burst int
        Allowed burst size, packets (default 20)
    -limit-cache int
        Limiter cache entries (default 10240)
    -listen string
        Listen address (default ":8443")
    -stats-file string
        File to write periodic operation stats to

Certificates
^^^^^^^^^^^^

The discovery server provides service over HTTPS. To ensure secure connections
from clients there are two options:

- Use a CA-signed certificate pair for the domain name you will use for the
  discovery server. This is like any other HTTPS website; clients will
  authenticate the server based on it's certificate and domain name.

- Use any certificate pair and let clients authenticate the server based on
  it's "device ID" (similar to Syncthing-to-Syncthing authentication). In
  this case, using `syncthing -generate` is a good option to create a
  certificate pair.

Whichever option you choose, the discovery server must be given the paths to
the certificate and key at startup::

  $ discosrv -cert /path/to/cert.pem -key /path/to/key.pem
  Server device ID is 7DDRT7J-UICR4PM-PBIZYL3-MZOJ7X7-EX56JP6-IK6HHMW-S7EK32W-G3EUPQA

The discovery server prints it's device ID at startup. In the case where you
are using a non CA signed certificate, this device ID (fingerprint) must be
given to the clients in the discovery server URL::

  https://disco.example.com:8443/?id=7DDRT7J-UICR4PM-PBIZYL3-MZOJ7X7-EX56JP6-IK6HHMW-S7EK32W-G3EUPQA

Pointing Syncthing at Your Discovery Server
-------------------------------------------

By default, Syncthing uses a number of global discovery servers, signified by
the entry ``default`` in the list of discovery servers. To make Syncthing use
your own instance of discosrv, open up Syncthing's web GUI. Go to settings,
Global Discovery Server and add discosrv's host address to the comma-separated
list, e.g. ``https://disco.example.com:8443/``. Note that discosrv uses port
8443 by default. For discosrv to be available over the internet with a dynamic
IP address, you will need a dynamic DNS service.

If you wish to use *only* your own discovery server, remove the ``default``
entry from the list.
