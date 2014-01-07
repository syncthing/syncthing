Block Exchange Protocol v1.0
============================

Introduction and Definitions
----------------------------

The BEP is used between two or more _nodes_ thus forming a _cluster_.
Each node has a _repository_ of files described by the _local model_,
containing modifications times and block hashes. The local model is sent
to the other nodes in the cluster. The union of all files in the local
models, with files selected for most recent modification time, forms the
_global model_. Each node strives to get it's repository in sync with
the global model by requesting missing blocks from the other nodes.

Transport and Authentication
----------------------------

The BEP itself does not provide retransmissions, compression, encryption
nor authentication. It is expected that this is performed at lower
layers of the networking stack. A typical deployment stack should be
similar to the following:

    |-----------------------------|
    |   Block Exchange Protocol   |
    |-----------------------------|
    |   Compression (RFC 1951)    |
    |-----------------------------|
    | Encryption & Auth (TLS 1.0) |
    |-----------------------------|
    |             TCP             |
    |-----------------------------|
    v                             v

The exact nature of the authentication is up to the application.
Possibilities include certificates signed by a common trusted CA,
preshared certificates, preshared certificate fingerprints or
certificate pinning combined with some out of band first verification.

There is no required order or synchronization among BEP messages - any
message type may be sent at any time and the sender need not await a
response to one message before sending another. Responses must however
be sent in the same order as the requests are received.

Compression is started directly after a successfull TLS handshake,
before the first message is sent. The compression is flushed at each
message boundary.

Messages
--------

Every message starts with one 32 bit word indicating the message version
and type. For BEP v1.0 the Version field is set to zero. Future versions
with incompatible message formats will increment the Version field. The
reserved bits must be set to zero.

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    | Ver=0 |       Message ID      |     Type      |    Reserved   |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

All data following the message header is in XDR (RFC 1014) encoding.
The actual data types in use by BEP, in XDR naming convention, are:

 - unsigned int   -- unsigned 32 bit integer
 - hyper          -- signed 64 bit integer
 - unsigned hyper -- signed 64 bit integer
 - opaque<>       -- variable length opaque data
 - string<>       -- variable length string

The encoding of opaque<> and string<> are identical, the distinction is
solely in interpretation. Opaque data should not be interpreted as such,
but can be compared bytewise to other opaque data. All strings use the
UTF-8 encoding.

### Index (Type = 1)

The Index message defines the contents of the senders repository. A Index
message is sent by each peer immediately upon connection and whenever the
local repository contents changes. However, if a peer has no data to
advertise (the repository is empty, or it is set to only import data) it
is allowed but not required to send an empty Index message (a file list of
zero length). If the repository contents change from non-empty to empty,
an empty Index message must be sent. There is no response to the Index
message.

    struct IndexMessage {
        FileInfo Files<>;
    }

    struct FileInfo {
        string Name<>;
        unsigned int Flags;
        hyper Modified;
        BlockInfo Blocks<>;
    }

    struct BlockInfo {
        unsigned int Length;
        opaque Hash<>
    }

The file name is the part relative to the repository root. The
modification time is expressed as the number of seconds since the Unix
Epoch. The hash algorithm is implied by the hash length. Currently, the
hash must be 32 bytes long and computed by SHA256.

The flags field is made up of the following single bit flags:

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |              Reserved               |D|   Unix Perm. & Mode   |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

 - The lower 12 bits hold the common Unix permission and mode bits.

 - Bit 19 ("D") is set when the file has been deleted. The block list
   shall contain zero blocks and the modification time indicates the
   time of deletion or, if deletion time is not reliably determinable,
   one second past the last know modification time.

 - Bit 0 through 18 are reserved for future use and shall be set to
   zero.

### Request (Type = 2)

The Request message expresses the desire to receive a data block
corresponding to a part of a certain file in the peer's repository.

The requested block must correspond exactly to one block seen in the
peer's Index message. The hash field must be set to the expected value by
the sender. The receiver may validate that this is actually the case
before transmitting data. Each Request message must be met with a Response
message.

    struct RequestMessage {
        string Name<>;
        unsigned hyper Offset;
        unsigned int Length;
        opaque Hash<>;
    }

The hash algorithm is implied by the hash length. Currently, the hash
must be 32 bytes long and computed by SHA256.

The Message ID in the header must set to a unique value to be able to
correlate the request with the response message.

### Response (Type = 3)

The Response message is sent in response to a Request message. In case the
requested data was not available (an outdated block was requested, or
the file has been deleted), the Data field is empty.

    struct ResponseMessage {
        opaque Data<>
    }

The Message ID in the header is used to correlate requests and
responses.

### Ping (Type = 4)

The Ping message is used to determine that a connection is alive, and to
keep connections alive through state tracking network elements such as
firewalls and NAT gateways. The Ping message has no contents.

    struct PingMessage {
    }

### Pong (Type = 5)

The Pong message is sent in response to a Ping. The Pong message has no
contents, but copies the Message ID from the Ping.

    struct PongMessage {
    }

### IndexUpdate (Type = 6)

This message has exactly the same structure as the Index message.
However instead of replacing the contents of the repository in the
model, the Index Update merely amends it with new or updated file
information. Any files not mentioned in an Index Update are left
unchanged.

Example Exchange
----------------

          A            B
     1. Index->      <-Index
     2. Request->
     3. Request->
     4. Request->
     5. Request->
     6.         <-Response
     7.         <-Response
     8.         <-Response
     9.         <-Response
    10. Index->
        ...
    11. Ping->
    12.            <-Pong

The connection is established and at 1. both peers send Index records.
The Index records are received and both peers recompute their knowledge
of the data in the cluster. In this example, peer A has four missing or
outdated blocks. At 2 through 5 peer A sends requests for these blocks.
The requests are received by peer B, who retrieves the data from the
repository and transmits Response records (6 through 9). Node A updates
their repository contents and transmits an updated Index message (10).
Both peers enter idle state after 10. At some later time 11, peer A
determines that it has not seen data from B for some time and sends a
Ping request. A response is sent at 12.

