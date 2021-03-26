[![Syncthing][14]][15]

---

[![Latest Linux & Cross Build](https://img.shields.io/teamcity/https/build.syncthing.net/s/Syncthing_BuildLinuxCross.svg?style=flat-square&label=linux+%26+cross+build)](https://build.syncthing.net/viewType.html?buildTypeId=Syncthing_BuildLinuxCross&guest=1)
[![Latest Windows Build](https://img.shields.io/teamcity/https/build.syncthing.net/s/Syncthing_BuildWindows.svg?style=flat-square&label=windows+build)](https://build.syncthing.net/viewType.html?buildTypeId=Syncthing_BuildWindows&guest=1)
[![Latest Mac Build](https://img.shields.io/teamcity/https/build.syncthing.net/s/Syncthing_BuildMac.svg?style=flat-square&label=mac+build)](https://build.syncthing.net/viewType.html?buildTypeId=Syncthing_BuildMac&guest=1)
[![MPLv2 License](https://img.shields.io/badge/license-MPLv2-blue.svg?style=flat-square)](https://www.mozilla.org/MPL/2.0/)
[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/88/badge)](https://bestpractices.coreinfrastructure.org/projects/88)
[![Go Report Card](https://goreportcard.com/badge/github.com/syncthing/syncthing)](https://goreportcard.com/report/github.com/syncthing/syncthing)

## Goals

Syncthing is a **continuous file synchronization program**. It synchronizes
files between two or more computers. We strive to fulfill the goals below.
The goals are listed in order of importance, the most important one being
the first. This is the summary version of the goal list - for more
commentary, see the full [Goals document][13].

Syncthing should be:

1. **Safe From Data Loss**

   Protecting the user's data is paramount. We take every reasonable
   precaution to avoid corrupting the user's files.

2. **Secure Against Attackers**

   Again, protecting the user's data is paramount. Regardless of our other
   goals we must never allow the user's data to be susceptible to
   eavesdropping or modification by unauthorized parties.

3. **Easy to Use**

   Syncthing should be approachable, understandable and inclusive.

4. **Automatic**

   User interaction should be required only when absolutely necessary.

5. **Universally Available**

   Syncthing should run on every common computer. We are mindful that the
   latest technology is not always available to any given individual.

6. **For Individuals**

   Syncthing is primarily about empowering the individual user with safe,
   secure and easy to use file synchronization.

7. **Everything Else**

   There are many things we care about that don't make it on to the list. It
   is fine to optimize for these values, as long as they are not in conflict
   with the stated goals above.

## Getting Started

Take a look at the [getting started guide][2].

There are a few examples for keeping Syncthing running in the background
on your system in [the etc directory][3]. There are also several [GUI
implementations][11] for Windows, Mac and Linux.

## Docker

To run Syncthing in Docker, see [the Docker README][16].

## Vote on features/bugs

We'd like to encourage you to [vote][12] on issues that matter to you.
This helps the team understand what are the biggest pain points for our users, and could potentially influence what is being worked on next.

## Getting in Touch

The first and best point of contact is the [Forum][8]. There is also an IRC
channel, `#syncthing` on [freenode][4] (with a [web client][9]), for talking
directly to developers and users. If you've found something that is clearly a
bug, feel free to report it in the [GitHub issue tracker][10].

## Building

Building Syncthing from source is easy, and there's [a guide][5]
that describes it for both Unix and Windows systems.

## Signed Releases

As of v0.10.15 and onwards release binaries are GPG signed with the key
D26E6ED000654A3E, available from https://syncthing.net/security.html and
most key servers.

There is also a built in automatic upgrade mechanism (disabled in some
distribution channels) which uses a compiled in ECDSA signature. macOS
binaries are also properly code signed.

## Documentation

Please see the [Syncthing documentation site][6].

All code is licensed under the [MPLv2 License][7].

[1]: https://docs.syncthing.net/specs/bep-v1.html
[2]: https://docs.syncthing.net/intro/getting-started.html
[3]: https://github.com/syncthing/syncthing/blob/main/etc
[4]: https://www.freenode.net/
[5]: https://docs.syncthing.net/dev/building.html
[6]: https://docs.syncthing.net/
[7]: https://github.com/syncthing/syncthing/blob/main/LICENSE
[8]: https://forum.syncthing.net/
[9]: https://kiwiirc.com/client/irc.freenode.net/#syncthing
[10]: https://github.com/syncthing/syncthing/issues
[11]: https://docs.syncthing.net/users/contrib.html#gui-wrappers
[12]: https://www.bountysource.com/teams/syncthing/issues
[13]: https://github.com/syncthing/syncthing/blob/main/GOALS.md
[14]: assets/logo-text-128.png
[15]: https://syncthing.net/
[16]: https://github.com/syncthing/syncthing/blob/main/README-Docker.md

