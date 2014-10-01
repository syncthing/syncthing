syncthing
=========

[![Latest Build](http://img.shields.io/jenkins/s/http/build.syncthing.net/syncthing.svg?style=flat-square)](http://build.syncthing.net/job/syncthing/lastBuild/)
[![API Documentation](http://img.shields.io/badge/api-Godoc-blue.svg?style=flat-square)](http://godoc.org/github.com/syncthing/syncthing)
[![GPL License](http://img.shields.io/badge/license-GPL-blue.svg?style=flat-square)](http://opensource.org/licenses/GPL-3.0)

This is the `syncthing` project. The following are the project goals:

 1. Define a protocol for synchronization of a folder between a number of
    collaborating devices. The protocol should be well defined, unambiguous,
    easily understood, free to use, efficient, secure and language neutral.
    This is the [Block Exchange
    Protocol](https://github.com/syncthing/syncthing/blob/master/protocol/PROTOCOL.md).

 2. Provide the reference implementation to demonstrate the usability of
    said protocol. This is the `syncthing` utility. It is the hope that
    alternative, compatible implementations of the protocol will come to
    exist.

The two are evolving together; the protocol is not to be considered
stable until syncthing 1.0 is released, at which point it is locked down
for incompatible changes.

Getting Started
---------------

Take a look at the [getting started guide](http://discourse.syncthing.net/t/46).

Building
--------

Building Syncthing from source is easy, and there's a
[guide](http://discourse.syncthing.net/t/44)
that describes it for both Unix and Windows.

Signed Releases
---------------

As of v0.7.0 and onwards, git tags and release binaries are GPG signed with
the key BCE524C7 (http://nym.se/gpg.txt). For release binaries, MD5 and
SHA1 checksums are calculated and signed, available in the
md5sum.txt.asc and sha1sum.txt.asc files.

Documentation
=============

The [syncthing
documentation](http://discourse.syncthing.net/category/documentation) is
on the discourse site.

License
=======

All documentation and protocol specifications are licensed
under the [Creative Commons Attribution 4.0 International
License](http://creativecommons.org/licenses/by/4.0/).

All code is licensed under the
[GPL](https://github.com/syncthing/syncthing/blob/master/LICENSE), v3 or
later.
