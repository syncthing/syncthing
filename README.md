# Syncthing

[![Latest Build (Official)](https://img.shields.io/jenkins/s/http/build.syncthing.net/syncthing.svg?style=flat-square&label=unix%20build)](http://build.syncthing.net/job/syncthing/lastBuild/)
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
on your system in [the etc directory][3]. There are also several [GUI
implementations][11] for Windows, Mac and Linux.

## Vote on features/bugs

We'd like to encourage you to [vote][12] on issues that matter to you.
This helps the team understand what are the biggest pain points for our users, and could potentially influence what is being worked on next.

## Getting in Touch

The first and best point of contact is the [Forum][8]. There is also an IRC
channel, `#syncthing` on [freenode][4] (with a [web client][9]), for talking
directly to developers and users. If you've found something that is clearly a
bug, feel free to report it in the [GitHub issue tracker][10].

## Building

Building Syncthing from source is easy, and there's a [guide][5]
that describes it for both Unix and Windows systems.

## Signed Releases

As of v0.10.15 and onwards release binaries are GPG signed with the key
D26E6ED000654A3E, available from https://syncthing.net/security.html and
most key servers.

There is also a built in automatic upgrade mechanism (disabled in some
distribution channels) which uses a compiled in ECDSA signature. Mac OS
X binaries are also properly code signed.

## Documentation

Please see the [Syncthing documentation site][6].

All code is licensed under the [MPLv2 License][7].

[1]: http://docs.syncthing.net/specs/bep-v1.html
[2]: http://docs.syncthing.net/intro/getting-started.html
[3]: https://github.com/syncthing/syncthing/blob/master/etc
[4]: http://www.freenode.net/irc_servers.shtml
[5]: http://docs.syncthing.net/dev/building.html
[6]: http://docs.syncthing.net/
[7]: https://github.com/syncthing/syncthing/blob/master/LICENSE
[8]: https://forum.syncthing.net/
[9]: https://kiwiirc.com/client/irc.freenode.net/#syncthing
[10]: https://github.com/syncthing/syncthing/issues
[11]: http://docs.syncthing.net/users/contrib.html#gui-wrappers
[12]: https://www.bountysource.com/teams/syncthing/issues
