Release Schedule
================

Structure
---------

Syncthing follows the `Semantic Versioning <http://semver.org/>`__
scheme of versioning. Each release has a three part version number:
*major*.\ *minor*.\ *patch*.

A new *major* version is released when there are incompatible API or
protocol changes, a new *minor* version is released when there are new
features but compatibility with older releases is retained, and a new
*patch* version is released when there are bug fixes (compatibility with
older releases always retained).

While still in pre-release mode, i.e. versions 0.\ *x*, breaking changes
are made in minor releases rather than major. Version 0.7.3 should be
able to talk to version 0.7.52, but will probably not understand version
0.8.0. This also means that if you don't like 0.7.52, you can safely
downgrade to 0.7.3 again and keep your configuration, index caches, etc.
However 0.8.0 might have a different format for those things so a
downgrade to 0.7.x might be trickier.

Patch Releases
--------------

A new patch release is made each Sunday, if there have been changes
committed since the last release. Serious bugs, such as would crash the
client or corrupt data, cause an immediate (out of schedule) patch
release.

Minor Releases
--------------

Minor releases are made when new functionality is ready for release.
This happen approximately once every few weeks, with the pace slowing as
the 1.0 release nears.

Major Releases
--------------

A new major release is a rare event. At the time of writing this has not
yet happened and is foreseen to happen only once in the foreseeable
future - the 1.0 release.
