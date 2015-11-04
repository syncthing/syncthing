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

Global Discovery Servers
------------------------

Runs the ``discosrv`` instances for v0.11 and v0.12.

- discovery-v4-1.syncthing.net (Ubuntu 14.04, 512 MB, hosted by :user:`calmh`)
- discovery-v6-1.syncthing.net (alias for above)
- discovery-v4-2.syncthing.net (Ubuntu 14.04, 512 MB, hosted at DigitalOcean)
- discovery-v6-2.syncthing.net (alias for above)
- discovery-v4-3.syncthing.net (Ubuntu 14.04, 512 MB, hosted at DigitalOcean)
- discovery-v6-3.syncthing.net (alias for above)

Relay Pool Server
-----------------

Runs the ``relaypoolsrv`` to handle dynamic registration and announcement of relays.

- `relays.syncthing.net <http://relays.syncthing.net>`__ (SmartOS container, 256 MB)

Relay Servers
-------------

Hosted by friendly people on the internet.

Usage Reporting Server
----------------------

Runs the ``ursrv`` instance, PostgreSQL and Nginx.

- `data.syncthing.net <http://data.syncthing.net/>`__ (Ubuntu Linux, 512 MB)

Build Servers, Core and Android
-------------------------------

Runs Jenkins and does the core and Android builds, Ubuntu Linux.

- `build.syncthing.net <http://build.syncthing.net/>`__ (Jenkins frontend, SmartOS container, 2048 MB)
- build2.syncthing.net (build runner, SmartOS container, 8192 MB)
- `android.syncthing.net <http://android.syncthing.net/>`__ (Ubuntu Linux, 3072 MB)

OSX and Windows Build Slaves
----------------------------

Runs Jenkins build slaves and runs builds and tests on the Mac and
Windows operating systems. Hosted by :user:`Zillode`.

APT Server
----------

Serves the APT repository for Debian/Ubuntu users. Runs Nginx.

- `apt.syncthing.net <http://apt.syncthing.net>`__ (SmartOS container, 256 MB)

Signing Server
--------------

Signs and uploads the release bundles to Github.

- secure.syncthing.net (SmartOS container, 2048 MB)
