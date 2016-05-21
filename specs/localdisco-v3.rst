.. _localdisco-v3:

Local Discovery Protocol v3
===========================

Mode of Operation
-----------------

Each participating device periodically sends an Announcement packet. It also
keeps a table of the announcements it has seen. There is no way to solicit a
reply; the only message type is Announcement.

On multihomed hosts the announcement packets should be sent on each interface
on which Syncthing will accept connections.

For IPv4, the Announcement packet is broadcast either to the link-specific
broadcast address, or to the generic link-local broadcast address
``255.255.255.255``, with destination port 21027.

For IPv6, the Announcement packet is multicast to the transient link-local
multicast address ``[ff12::8384]``, with destination port 21027.

It is recommended that local discovery Announcement packets be sent on a 30 to
60 second interval, possibly with immediate transmissions when a previously
unknown device is discovered.

Device ID
---------

The device ID is the SHA-256 (32 bytes) of the device X.509 certificate. See
:ref:`device-ids` in the Syncthing documentation.

Announcement packet
-------------------

The Announcement packet has the following structure::

    Announce Structure:

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                             Magic                             |
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
    |                         Length of ID                          |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    /                                                               /
    \                     ID (variable length)                      \
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
    |                         Length of URL                         |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    /                                                               /
    \                     URL (variable length)                     \
    /                                                               /
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

The corresponding XDR representation is as follows (see
`RFC4506 <http://tools.ietf.org/html/rfc4506>`__ for the XDR format):

::

    struct Announcement {
        unsigned int Magic;
        Device This;
        Device Extra<>;
    }

    struct Device {
        opaque ID<32>;
        Address Addresses<16>;
    }

    struct Address {
        string URL<2083>;
    }


In the ``Announce``  structure  field ``Magic`` is used to ensure
a correct datagram was received and MUST be equal to ``0x7D79BC40``.

The first Device structure contains information about the sending
device. The following zero or more Extra devices contain information
about other devices known to the sending device.

In the ``Device`` structure, field ``DeviceID`` is the SHA-256 (32
bytes) of the device X.509 certificate, as explained in section *Device
ID*.

For each ``Address`` the ``URL`` field contains the actual target address.
Direct connections will typically have the ``tcp://`` scheme. Relay connections
will typically use the ``relay://`` scheme.
