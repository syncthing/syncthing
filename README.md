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

Features
--------

The following features are _currently implemented and working_:

 * The formation of a cluster of nodes, certificate authenticated and
   communicating over TLS over TCP.

 * Synchronization of a single directory among the cluster nodes.

 * Change detection by periodic scanning of the local repository.

 * Static configuration of cluster nodes.

 * Automatic discovery of cluster nodes. See [discover.go][discover.go]
   for the protocol specification. Discovery on the LAN is performed by
   broadcasts, Internet wide discovery is performed with the assistance
   of a global server.

 * Handling of deleted files. Deletes can be propagated or ignored per
   client.

 * Synchronizing multiple unrelated directory trees by following
   symlinks directly below the repository level.

The following features are _not yet implemented but planned_:

 * Change detection by listening to file system notifications instead of
   periodic scanning.

 * HTTP GUI.

The following features are _not implemented but may be implemented_ in
the future:

 * Syncing multiple directories from the same syncthing instance.

 * Automatic NAT handling via UPNP.

 * Conflict resolution. Currently whichever file has the newest
   modification time "wins". The correct behavior in the face of
   conflicts is open for discussion.

[discover.go]: (https://github.com/calmh/syncthing/blob/master/discover/discover.go

Security
--------

Security is one of the primary project goals. This means that it should
not be possible for an attacker to join a cluster uninvited, and it
should not be possible to extract private information from intercepted
traffic. Currently this is implemented as follows.

All traffic is protected by TLS. To prevent uninvited nodes from joining
a cluster, the certificate fingerprint of each node is compared to a
preset list of acceptable nodes at connection establishment. The
fingerprint is computed as the SHA-1 hash of the certificate and
displayed in BASE32 encoding to form a compact yet convenient string.
Currently SHA-1 is deemed secure against preimage attacks.

Installing
==========

Download the appropriate precompiled binary from the
[releases](https://github.com/calmh/syncthing/releases) page. Untar and
put the `syncthing` binary somewhere convenient in your `$PATH`.

If you are a developer and have Go 1.2 installed you can also install
the latest version from source:

`go get github.com/calmh/syncthing`

Usage
=====

Check out the options:

```
$ syncthing --help
Usage:
  syncthing [options]

...
```

Run syncthing to let it create it's config directory and certificate:

```
$ syncthing
11:34:13 main.go:85: INFO: Version v0.1-40-gbb0fd87
11:34:13 tls.go:61: OK: Created TLS certificate file
11:34:13 tls.go:67: OK: Created TLS key file
11:34:13 main.go:66: INFO: My ID: NCTBZAAHXR6ZZP3D7SL3DLYFFQERMW4Q
11:34:13 main.go:90: FATAL: No config file
```

Take note of the "My ID: ..." line. Perform the same operation on
another computer to create another node. Take note of that ID as well,
and create a config file `~/.syncthing/syncthing.ini` looking something
like this:

```
[repository]
dir = /Users/jb/Synced

[nodes]
NCTBZAAHXR6ZZP3D7SL3DLYFFQERMW4Q = 172.16.32.1:22000 192.23.34.56:22000
CUGAE43Y5N64CRJU26YFH6MTWPSBLSUL = dynamic
```

This assumes that the first node is reachable on either of the two
addresses listed (perhaps one internal and one port-forwarded external)
and that the other node is not normally reachable from the outside. Save
this config file, identically, to both nodes.

If the nodes are running on the same network, or reachable on port 22000
from the outside world, you can set all addresses to "dynamic" and they
will find each other using automatic discovery. (This discovery,
including port numbers, can be tweaked or disabled using command line
options.)

Start syncthing on both nodes. For the cautious, one side can be set to
be read only.

```
$ syncthing --ro
13:30:55 main.go:85: INFO: Version v0.1-40-gbb0fd87
13:30:55 main.go:102: INFO: My ID: NCTBZAAHXR6ZZP3D7SL3DLYFFQERMW4Q
13:30:55 main.go:149: INFO: Initial repository scan in progress
13:30:59 main.go:153: INFO: Listening for incoming connections
13:30:59 main.go:157: INFO: Attempting to connect to other nodes
13:30:59 main.go:247: INFO: Starting local discovery
13:30:59 main.go:165: OK: Ready to synchronize
13:31:04 discover.go:113: INFO: Discovered node CUGAE43Y5N64CRJU26YFH6MTWPSBLSUL at 172.16.32.24:22000
13:31:14 main.go:296: INFO: Connected to node CUGAE43Y5N64CRJU26YFH6MTWPSBLSUL
13:31:19 main.go:345: INFO: Transferred 139 KiB in (14 KiB/s), 139 KiB out (14 KiB/s)
13:32:20 model.go:94: INFO: CUGAE43Y5N64CRJU26YFH6MTWPSBLSUL: 263.4 KB/s in, 69.1 KB/s out
13:32:20 model.go:104: INFO:  18289 files,  24.24 GB in cluster
13:32:20 model.go:111: INFO:  17132 files,  22.39 GB in local repo
13:32:20 model.go:117: INFO:   1157 files,   1.84 GB to synchronize
...
```
You should see the synchronization start and then finish a short while
later. Add nodes to taste.

License
=======

MIT

