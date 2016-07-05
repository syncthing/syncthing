.. _localdisco-v4:

Local Discovery Protocol v4
===========================

Mode of Operation
-----------------

Each participating device periodically sends an Announcement packet. It also
keeps a table of the announcements it has seen. There is no way to solicit a
reply; the only message type is Announcement.

On multihomed hosts the announcement packets should be sent on each interface
on which Syncthing will accept connections.

The announcement packet is sent over UDP.

For IPv4, the Announcement packet is broadcast either to the link-specific
broadcast address, or to the generic link-local broadcast address
``255.255.255.255``, with destination port 21027.

For IPv6, the Announcement packet is multicast to the transient link-local
multicast address ``ff12::8384``, with destination port 21027.

It is recommended that local discovery Announcement packets be sent on a 30
to 60 second interval, possibly with immediate transmissions when a
previously unknown device is discovered or a device has restarted (see the
``instance_id`` field).

Device ID
---------

The device ID is the SHA-256 (32 bytes) of the device X.509 certificate. See
:ref:`device-ids` in the Syncthing documentation.

Announcement packet
-------------------

The Announcement packet has the following structure:

.. code-block:: none

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                             Magic                             |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    /                                                               /
    \                       Announce Message                        \
    /                                                               /
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

There is no explicit length field as the length is given by the length of
the discovery announcement packet itself.

The Magic field is a 32 bit word representing 0x2EA7D90B in network (big
endian) byte order. It identifies the packet as being a Syncthing discovery
protocol packet.

The Announce Message contents are in protocol buffer format using the
following schema:

.. code-block:: proto

    message Announce {
        bytes           id          = 1;
        repeated string addresses   = 2;
        int64           instance_id = 3;
    }

The ``id`` field contains the Device ID of the sending device.

The ``addresses`` field contains a list of addresses where the device can be
contacted. Direct connections will typically have the ``tcp://`` scheme.
Relay connections will typically use the ``relay://`` scheme.

When interpreting addresses with an unspecified address, e.g.,
``tcp://0.0.0.0:22000`` or ``tcp://:42424``, the source address of the
discovery announcement is to be used.

The ``instance_id`` field is set to a randomly generated ID at client
startup. Other devices on the network can detect a change in instance ID
between two announces and conclude that the announcing device has restarted.
