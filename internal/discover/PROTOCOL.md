Node Discovery Protocol v2
==========================

Mode of Operation
-----------------

There are two distinct modes: "local discovery", performed on a LAN
segment (broadcast domain) and "global discovery" performed over the
Internet in general with the support of a well known server.

Local discovery does not use Query packets. Instead Announcement packets
are sent periodically and each participating node keeps a table of the
announcements it has seen. On multihomed hosts the announcement packets
should be sent on each interface that syncthing will accept connections.

It is recommended that local discovery Announcement packets are sent on
a 30 to 60 second interval, possibly with forced transmissions when a
previously unknown node is discovered.

Global discovery is made possible by periodically updating a global server
using Announcement packets indentical to those transmitted for local
discovery. The node performing discovery will transmit a Query packet to
the global server and expect an Announcement packet in response. In case
the global server has no knowledge of the queried node ID, there will be
no response. A timeout is to be used to determine lookup failure.

There is no message to unregister from the global server; instead
registrations are forgotten after 60 minutes. It is recommended to
send Announcement packets to the global server on a 30 minute interval.

Packet Formats
--------------

The Announcement packet has the following structure:

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                      Magic (0x9D79BC39)                       |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    /                                                               /
    \                        Node Structure                         \
    /                                                               /
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                     Number of Extra Nodes                     |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    /                                                               /
    \                 Zero or more Node Structures                  \
    /                                                               /
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

    Node Structure:

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
    |                         Length of IP                          |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    /                                                               /
    \                     IP (variable length)                      \
    /                                                               /
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |             Port              |            0x0000             |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

This is the XDR encoding of:

    struct Announcement {
        unsigned int Magic;
        Node This;
        Node Extra<>;
    }

    struct Node {
        string ID<>;
        Address Addresses<>;
    }

    struct Address {
        opaque IP<>;
        unsigned short Port;
    }

The first Node structure contains information about the sending node.
The following zero or more Extra nodes contain information about other
nodes known to the sending node.

In the Address structure, the IP field can be of three differnt kinds;

 - A zero length indicates that the IP address should be taken from the
   source address of the announcement packet, be it IPv4 or IPv6. The
   source address must be a valid unicast address. This is only valid
   in the first node structure, not in the list of extras.

 - A four byte length indicates that the address is an IPv4 unicast
   address.

 - A sixteen byte length indicates that the address is an IPv6 unicast
   address.

The Query packet has the following structure:

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                   Magic Number (0x2CA856F5)                   |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                       Length of Node ID                       |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    /                                                               /
    \                   Node ID (variable length)                   \
    /                                                               /
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

This is the XDR encoding of:

    struct Announcement {
        unsigned int MagicNumber;
        string NodeID<>;
    }
