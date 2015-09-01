Syncthing Infrastructure
========================

This is a list of the infrastructure that powers the Syncthing project.
Unless otherwise noted, the default is that it's a VM hosted by :user:`calmh`.

GitHub
------

All repos, issue trackers and binary releases are hosted at `GitHub <https://github.com/syncthing>`__.

Main & Documenatation Websites
------------------------------

Static HTML, served by Nginx.

- `syncthing.net <https://syncthing.net/>`__ (SmartOS container, 1024 MB)
- `docs.syncthing.net <http://docs.syncthing.net/>`__ (Sphinx for site generation)

Forum Website
-------------

Powered by Discourse.

- `forum.syncthing.net <https://forum.syncthing.net/>`__ (Ubuntu Linux, 3072 MB)

Global Discovery Server
-----------------------

Runs the ``discosrv`` instances for v0.11 and v0.12 and PostgreSQL.

- `announce.syncthing.net <http://announce.syncthing.net/>`__ (Ubuntu Linux, 512 MB)
- announce-v6.syncthing.net (just an alias)

Usage Reporting Server
----------------------

Runs the ``ursrv`` instance, PostgreSQL and Nginx.

- `data.syncthing.net <http://data.syncthing.net/>`__ (Ubuntu Linux, 512 MB)

Build Servers, Core and Android
-------------------------------

Runs Jenkins and does the core and Android builds, Ubuntu Linux.

- `build.syncthing.net <http://build.syncthing.net/>`__ (Ubuntu Linux, 4096 MB)
- `android.syncthing.net <http://android.syncthing.net/>`__ (Ubuntu Linux, 3072 MB)

OSX and Windows Build Slaves
----------------------------

Runs Jenkins build slaves and run builds and tests on the Mac and
Windows operating systems. Hosted by :user:`Zillode`.

APT Server
----------

Serves the APT repository for Debian/Ubuntu users. Runs Nginx.

- `apt.syncthing.net <http://apt.syncthing.net>`__ (SmartOS container, 256 MB)

Relay Servers
-------------

Runs the ``relaysrv`` that allows communication between otherwise
blocked v0.12 clients.

-  `a.relays.syncthing.net <http://a.relays.syncthing.net:22070/status>`__ hosted by :user:`Zillode`
-  `b.relays.syncthing.net <http://b.relays.syncthing.net:22070/status>`__ hosted by :user:`calmh` (SmartOS container, 256 MB)

Signing Server
--------------

Signs and uploads the release bundles to Github.

- secure.syncthing.net (SmartOS container, 2048 MB)
