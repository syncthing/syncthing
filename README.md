# Syncthing

[![Latest Build (Official)](https://img.shields.io/jenkins/s/http/build.syncthing.net/syncthing.svg?style=flat-square&label=unix%20build)](http://build.syncthing.net/job/syncthing/lastBuild/)
[![AppVeyor Build](https://img.shields.io/appveyor/ci/calmh/syncthing/master.svg?style=flat-square&label=windows%20build)](https://ci.appveyor.com/project/calmh/syncthing)
[![API Documentation](https://img.shields.io/badge/api-Godoc-blue.svg?style=flat-square)](http://godoc.org/github.com/syncthing/syncthing)
[![MPLv2 License](https://img.shields.io/badge/license-MPLv2-blue.svg?style=flat-square)](https://www.mozilla.org/MPL/2.0/)

This is the Syncthing project which pursues the following goals:

 1. Define a protocol for synchronization of a folder between a number of
    collaborating devices. This protocol should be well defined, unambiguous,
    easily understood, free to use, efficient, secure and language neutral.
    This is called the [Block Exchange Protocol][1].

 2. Provide the reference implementation to demonstrate the usability of
    said protocol. This is the `syncthing` utility. We hope that
    alternative, compatible implementations of the protocol will arise.

The two are evolving together; the protocol is not to be considered
stable until Syncthing 1.0 is released, at which point it is locked down
for incompatible changes.

## Getting Started

Take a look at the [getting started guide][2].

There are a few examples for keeping Syncthing running in the background
on your system in [the etc directory][3].

There is an IRC channel, `#syncthing` on [Freenode][4], for talking directly
to developers and users.

## Building

Building Syncthing from source is easy, and there's a [guide][5].
that describes it for both Unix and Windows systems.

## Signed Releases

As of v0.10.15 and onwards, git tags and release binaries are GPG signed
with the key D26E6ED000654A3E (see https://syncthing.net/security.html).
For release binaries, MD5 and SHA1 checksums are calculated and signed,
available in the md5sum.txt.asc and sha1sum.txt.asc files.

## Documentation

Please see the [Syncthing documentation site][6].

All code is licensed under the [MPLv2 License][7].

[1]: http://docs.syncthing.net/specs/bep-v1.html
[2]: http://docs.syncthing.net/intro/getting-started.html
[3]: https://github.com/syncthing/syncthing/blob/master/etc
[4]: https://webchat.freenode.net/
[5]: http://docs.syncthing.net/dev/building.html
[6]: http://docs.syncthing.net/
[7]: https://github.com/syncthing/syncthing/blob/master/LICENSE
