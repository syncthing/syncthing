Device Discovery Protocol v2
============================

Mode of Operation
-----------------

There are two distinct modes: **local discovery**, performed on a LAN
segment (broadcast domain) and **global discovery**, performed over the
Internet with the support of a well-known server. Both modes are over UDP.

Local discovery
---------------

Each participating device sends periodically an Announcement packet and keeps
a table of the announcements it has seen. There is no way to solicit a reply;
the only message type is Announcement.

On multihomed hosts the announcement packets should be sent on each
interface on which syncthing will accept connections.

For IPv4, the Announcement packet is broadcasted either to the link-specific
broadcast address, or to the generic link-local broadcast address
`255.255.255.255`, with source and destination port 21025.

For IPv6, the Announcement packet is multicasted to the transient link-local
multicast address `[ff32::5222]`, with source and destination port 21026.

It is recommended that local discovery Announcement packets be sent on
a 30 to 60 second interval, possibly with immediate transmissions when a
previously unknown device is discovered.

Global discovery
----------------

Global discovery is performed in two steps: announcement and discovery.

In the announcement step, a device periodically unicasts an Announcement
packet to the global server `announce.syncthing.net`, port 22026.

In the discovery step, a device sends a Query packet for a given device ID to
the server. If the server knows the ID, it replies with an Announcement packet.
If the server doesn't know the ID, it will not reply. The device must interpret
a timeout as lookup failure.

There is no message to unregister from the server; instead the server forgets
about an Announcement after 60 minutes.

It is recommended to send Announcement packets to the global server on a 30
minute interval.

Device ID
---------

The device ID is the SHA-256 (32 bytes) of the device X.509 certificate.
See [How device IDs work] in the Syncthing documentation.

Announcement packet
-------------------

The Announcement packet has the following structure:

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                      Magic (0x9D79BC39)                       |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    /                                                               /
    \                       Device Structure                        \
    /                                                               /
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                    Number of Extra Devices                    |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    /                                                               /
    \                Zero or more Device Structures                 \
    /                                                               /
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

Device Structure:

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                         Length of Device ID                   |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    /                                                               /
    \                          Device ID                            \
    /                                                               /
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                      Number of Addresses                      |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    /                                                               /
    \                Zero or more Address Structures                \
    /                                                               /
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

Address Structure:

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                         Length of IP                          |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    /                                                               /
    \                     IP (variable length)                      \
    /                                                               /
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                              Port                             |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

The corresponding XDR representation is as follows (see [RFC4506] for the XDR
format):

    struct Announcement {
        unsigned int Magic;
        Device This;
        Device Extra<>;
    }

    struct Device {
        opaque DeviceID<32>;
        Address Addresses<>;
    }

    struct Address {
        opaque IP<>;
        unsigned int Port;
    }

The first Device structure contains information about the sending device.
The following zero or more Extra devices contain information about other
devices known to the sending device.

In the `Device` structure, field `DeviceID` is the SHA-256 (32 bytes) of the
device X.509 certificate, as explained in section _Device ID_.

In the `Address` structure, the IP field can be of three different kinds:

 - A zero length indicates that the IP address should be taken from the
   source address of the announcement packet, be it IPv4 or IPv6. The
   source address must be a valid unicast address. This is only valid
   in the first device structure, not in the list of extras. In case of
   global discovery, the discovery server will reply to a Query with an
   announcement packet containing the expanded address of the queried
   device ID as seen from the server, allowing to traverse the majority of
   NAT devices.

 - A four byte length indicates that the address is an IPv4 unicast
   address.

 - A sixteen byte length indicates that the address is an IPv6 unicast
   address.

Although the `port` field is 32-bit, the value should fit in a 16-bit unsigned
integer, since that is the size of a UDP port.

Query packet
------------

The Query packet has the following structure:

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                   Magic Number (0x2CA856F5)                   |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                      Length of Device ID                      |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    /                                                               /
    \                           Device ID                           \
    /                                                               /
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

This is the XDR encoding of:

    struct Query {
        unsigned int MagicNumber;
        opaque DeviceID<32>;
    }

Design rationale
----------------

At the beginning, also IPv4 was using multicast. It has been changed to
broadcast after some bugs, especially on Android.

[How device IDs work]: https://discourse.syncthing.net/t/how-device-ids-work/365
[RFC4506]: http://tools.ietf.org/html/rfc4506

