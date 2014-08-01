syncthing
=========

[![Build Status](https://img.shields.io/travis/syncthing/syncthing.svg?style=flat)](https://travis-ci.org/syncthing/syncthing)
[![Coverage Status](https://img.shields.io/coveralls/syncthing/syncthing.svg?style=flat)](https://coveralls.io/r/syncthing/syncthing?branch=master)
[![API Documentation](http://img.shields.io/badge/api-Godoc-blue.svg?style=flat)](http://godoc.org/github.com/syncthing/syncthing)
[![MIT License](http://img.shields.io/badge/license-MIT-blue.svg?style=flat)](http://opensource.org/licenses/MIT)

This is the `syncthing` project. The following are the project goals:

 1. Define a protocol for synchronization of a file repository between a
    number of collaborating nodes. The protocol should be well defined,
    unambiguous, easily understood, free to use, efficient, secure and
    language neutral. This is the [Block Exchange
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

Take a look at the [getting started guide](http://discourse.syncthing.net/t/getting-started/46).

Signed Releases
---------------

As of v0.7.0 and onwards, git tags and release binaries are GPG signed with
the key BCE524C7 (http://nym.se/gpg.txt). The signature is included in the
normal release bundle as `syncthing.asc` or `syncthing.exe.asc`.

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

All code is licensed under the [MIT
License](https://github.com/syncthing/syncthing/blob/master/LICENSE).
