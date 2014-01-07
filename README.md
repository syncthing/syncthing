syncthing
=========

This is `syncthing`, an open BitTorrent Sync alternative. It is
currently far from ready for mass consumption, but it is a usable proof
of concept and tech demo. The following are the project goals:

 1. Define an open, secure, language neutral protocol usable for
    efficient synchronization of a file repository between an arbitrary
    number of nodes. This is the [Block Exchange
    Protocol](https://github.com/calmh/syncthing/blob/master/protocol/PROTOCOL.md)
    (BEP).

 2. Provide the reference implementation to demonstrate the usability of
    said protocol. This is the `syncthing` utility.

The two are evolving together; the protocol is not to be considered
stable until syncthing 1.0 is released, at which point it is locked down
for incompatible changes.

Syncthing does not use the BitTorrent protocol. The reasons for this are
1) we don't know if BitTorrent Sync does either, so there's nothing to
be compatible with, 2) BitTorrent includes a lot of functionality for
making sure large swarms of selfish agents behave and somehow work
towards a common goal. Here we have a much smaller swarm of cooperative
agents and a simpler approach will suffice.

Documentation
=============

The syncthing documentation is kept on the
[GitHub Wiki](https://github.com/calmh/syncthing/wiki).

License
=======

MIT

