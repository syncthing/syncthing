Block Exchange Protocol v1
==========================

Introduction and Definitions
----------------------------

BEP is used between two or more _nodes_ thus forming a _cluster_. Each
node has one or more _repositories_ of files described by the _local
model_, containing metadata and block hashes. The local model is sent to
the other nodes in the cluster. The union of all files in the local
models, with files selected for highest change version, forms the
_global model_. Each node strives to get it's repositories in sync with
the global model by requesting missing or outdated blocks from the other
nodes in the cluster.

File data is described and transferred in units of _blocks_, each being
128 KiB (131072 bytes) in size.

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL
NOT", "SHOULD", "SHOULD NOT", "RECOMMENDED",  "MAY", and
"OPTIONAL" in this document are to be interpreted as described in
RFC 2119.

Transport and Authentication
----------------------------

BEP is deployed as the highest level in a protocol stack, with the lower
level protocols providing compression, encryption and authentication.

    +-----------------------------|
    |   Block Exchange Protocol   |
    |-----------------------------|
    |   Compression (RFC 1951)    |
    |-----------------------------|
    | Encryption & Auth (TLS 1.2) |
    |-----------------------------|
    |             TCP             |
    |-----------------------------|
    v             ...             v

Compression is started directly after a successfull TLS handshake,
before the first message is sent. The compression is flushed at each
message boundary. Compression SHALL use the DEFLATE format as specified
in RFC 1951.

The encryption and authentication layer SHALL use TLS 1.2 or a higher
revision. A strong cipher suite SHALL be used, with "string cipher
suite" being defined as being without known weaknesses and providing
Perfect Forward Secrecy (PFS). Examples of strong cipher suites are
given at the end of this document. This is not to be taken as an
exhaustive list of allowed cipher suites but represents best practices
at the time of writing.

The exact nature of the authentication is up to the application, however
it SHALL be based on the TLS certificate presented at the start of the
connection. Possibilities include certificates signed by a common
trusted CA, preshared certificates, preshared certificate fingerprints
or certificate pinning combined with some out of band first
verification. The reference implementation uses preshared certificate
fingerprints (SHA-256) referred to as "Node IDs".

There is no required order or synchronization among BEP messages - any
message type may be sent at any time and the sender need not await a
response to one message before sending another. Responses MUST however
be sent in the same order as the requests are received.

The underlying transport protocol MUST be TCP.

Messages
--------

Every message starts with one 32 bit word indicating the message
version, type and ID.

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |  Ver  |  Type |       Message ID      |        Reply To       |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

For BEP v1 the Version field is set to zero. Future versions with
incompatible message formats will increment the Version field.

The Type field indicates the type of data following the message header
and is one of the integers defined below.

The Message ID is set to a unique value for each transmitted message. In
request messages the Reply To is set to zero. In response messages it is
set to the message ID of the corresponding request. The uniqueness
requirement implies that no more than 4096 messages may be outstanding
at any given moment. The ordering requirement implies that a response to
a given message ID also means that all preceding messages have been
received, specifically those which do not otherwise demand a response.
Hence their message ID:s may be reused.

All data following the message header MUST be in XDR (RFC 1014)
encoding. All fields shorter than 32 bits and all variable length data
MUST be padded to a multiple of 32 bits. The actual data types in use by
BEP, in XDR naming convention, are the following:

 - (unsigned) int   -- (unsigned) 32 bit integer
 - (unsigned) hyper -- (unsigned) 64 bit integer
 - opaque<>         -- variable length opaque data
 - string<>         -- variable length string

The transmitted length of string and opaque data is the length of actual
data, excluding any added padding. The encoding of opaque<> and string<>
are identical, the distinction being solely one of interpretation.
Opaque data should not be interpreted but can be compared bytewise to
other opaque data. All strings MUST use the Unicode UTF-8 encoding,
normalization form C.

### Index (Type = 1)

The Index message defines the contents of the senders repository. An
Index message MUST be sent by each node immediately upon connection. A
node with no data to advertise MUST send an empty Index message (a file
list of zero length). If the repository contents change from non-empty
to empty, an empty Index message MUST be sent. There is no response to
the Index message.

#### Graphical Representation

    IndexMessage Structure:

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                     Length of Repository                      |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    /                                                               /
    \                 Repository (variable length)                  \
    /                                                               /
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                        Number of Files                        |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    /                                                               /
    \               Zero or more FileInfo Structures                \
    /                                                               /
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+


    FileInfo Structure:

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                        Length of Name                         |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    /                                                               /
    \                    Name (variable length)                     \
    /                                                               /
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                             Flags                             |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                                                               |
    +                      Modified (64 bits)                       +
    |                                                               |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                                                               |
    +                       Version (64 bits)                       +
    |                                                               |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                       Number of Blocks                        |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    /                                                               /
    \               Zero or more BlockInfo Structures               \
    /                                                               /
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+


    BlockInfo Structure:

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                             Size                              |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                        Length of Hash                         |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    /                                                               /
    \                    Hash (variable length)                     \
    /                                                               /
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

#### Fields

The Repository field identifies the repository that the index message
pertains to. For single repository implementations the node MAY send an
empty repository ID or use the string "default".

The Name is the file name path relative to the repository root. Like all
strings in BEP, the Name is always in UTF-8 NFC regardless of operating
system or file system specific conventions. The Name field uses the
slash character ("/") as path separator, regardless of the
implementation's operating system conventions. The combination of
Repository and Name uniquely identifies each file in a cluster.

The Version field is the value of a cluster wide Lamport clock
indicating when the change was detected. The clock ticks on every
detected and received change. The combination of Repository, Name and
Version uniquely identifies the contents of a file at a given point in
time.

The Flags field is made up of the following single bit flags:

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |              Reserved             |I|D|   Unix Perm. & Mode   |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

 - The lower 12 bits hold the common Unix permission and mode bits. An
   implemention MAY ignore or interpret these as is suitable on the host
   operating system.

 - Bit 19 ("D") is set when the file has been deleted. The block list
   SHALL be of length zero and the modification time indicates the time
   of deletion or, if the time of deletion is not reliably determinable,
   the last known modification time.

 - Bit 18 ("I") is set when the file is invalid and unavailable for
   synchronization. A peer MAY set this bit to indicate that it can
   temporarily not serve data for the file.

 - Bit 0 through 17 are reserved for future use and SHALL be set to
   zero.

The hash algorithm is implied by the Hash length. Currently, the hash
MUST be 32 bytes long and computed by SHA256.

The Modified time is expressed as the number of seconds since the Unix
Epoch (1970-01-01 00:00:00 UTC).

In the rare occasion that a file is simultaneously and independently
modified by two nodes in the same cluster and thus end up on the same
Version number after modification, the Modified field is used as a tie
breaker (higher being better), followed by the hash values of the file
blocks (lower being better).

The Blocks list contains the size and hash for each block in the file.
Each block represents a 128 KiB slice of the file, except for the last
block which may represent a smaller amount of data.

#### XDR

    struct IndexMessage {
        string Repository<>;
        FileInfo Files<>;
    }

    struct FileInfo {
        string Name<>;
        unsigned int Flags;
        hyper Modified;
        unsigned hyper Version;
        BlockInfo Blocks<>;
    }

    struct BlockInfo {
        unsigned int Size;
        opaque Hash<>;
    }

### Request (Type = 2)

The Request message expresses the desire to receive a data block
corresponding to a part of a certain file in the peer's repository.

#### Graphical Representation

    RequestMessage Structure:

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                     Length of Repository                      |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    /                                                               /
    \                 Repository (variable length)                  \
    /                                                               /
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                        Length of Name                         |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    /                                                               /
    \                    Name (variable length)                     \
    /                                                               /
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                                                               |
    +                       Offset (64 bits)                        +
    |                                                               |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                             Size                              |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

#### Fields

The Repository and Name fields are as documented for the Index message.
The Offset and Size fields specify the region of the file to be
transferred. This SHOULD equate to exactly one block as seen in an Index
message.

#### XDR

    struct RequestMessage {
        string Repository<>;
        string Name<>;
        unsigned hyper Offset;
        unsigned int Size;
    }

### Response (Type = 3)

The Response message is sent in response to a Request message.

#### Graphical Representation

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                        Length of Data                         |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    /                                                               /
    \                    Data (variable length)                     \
    /                                                               /
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

#### Fields

The Data field contains either a full 128 KiB block, a shorter block in
the case of the last block in a file, or is empty (zero length) if the
requested block is not available.

#### XDR

    struct ResponseMessage {
        opaque Data<>
    }

### Ping (Type = 4)

The Ping message is used to determine that a connection is alive, and to
keep connections alive through state tracking network elements such as
firewalls and NAT gateways. The Ping message has no contents.

### Pong (Type = 5)

The Pong message is sent in response to a Ping. The Pong message has no
contents, but copies the Message ID from the Ping.

### Index Update (Type = 6)

This message has exactly the same structure as the Index message.
However instead of replacing the contents of the repository in the
model, the Index Update merely amends it with new or updated file
information. Any files not mentioned in an Index Update are left
unchanged.

### Options (Type = 7)

This informational message provides information about the client
configuration, version, etc. It is sent at connection initiation and,
optionally, when any of the sent parameters have changed. The message is
in the form of a list of (key, value) pairs, both of string type.

Key ID:s apart from the well known ones are implementation specific. An
implementation is expected to ignore unknown keys. An implementation may
impose limits on key and value size.

Well known keys:

  - "clientId" -- The name of the implementation. Example: "syncthing".

  - "clientVersion" -- The version of the client. Example: "v1.0.33-47".
    The Following the SemVer 2.0 specification for version strings is
    encouraged but not enforced.

#### Graphical Representation

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                       Number of Options                       |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    /                                                               /
    \               Zero or more KeyValue Structures                \
    /                                                               /
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

    KeyValue Structure:

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                         Length of Key                         |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    /                                                               /
    \                     Key (variable length)                     \
    /                                                               /
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                        Length of Value                        |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    /                                                               /
    \                    Value (variable length)                    \
    /                                                               /
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

#### XDR

    struct OptionsMessage {
        KeyValue Options<>;
    }

    struct KeyValue {
        string Key<>;
        string Value<>;
    }

Message Limits
--------------

An implementation MAY impose reasonable limits on the length of message
fields to aid robustness in the face of corruption or broken
implementations. These limits, if imposed, SHOULD not be more
restrictive than the following:

### Index and Index Update Messages

 - Repository: 64 bytes
 - Number of Files: 100.000
 - Name: 1024 bytes
 - Number of Blocks: 100.000
 - Hash: 64 bytes

### Request Messages

 - Repository: 64 bytes
 - Name: 1024 bytes

### Response Messages

 - Data: 256 KiB

 ### Options Message

 - Number of Options: 64
 - Key: 64 bytes
 - Value: 1024 bytes

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
    10. Index Update->
        ...
    11. Ping->
    12.            <-Pong

The connection is established and at 1. both peers send Index records.
The Index records are received and both peers recompute their knowledge
of the data in the cluster. In this example, peer A has four missing or
outdated blocks. At 2 through 5 peer A sends requests for these blocks.
The requests are received by peer B, who retrieves the data from the
repository and transmits Response records (6 through 9). Node A updates
their repository contents and transmits an Index Update message (10).
Both peers enter idle state after 10. At some later time 11, peer A
determines that it has not seen data from B for some time and sends a
Ping request. A response is sent at 12.

Examples of Strong Cipher Suites
--------------------------------

0x009F DHE-RSA-AES256-GCM-SHA384 (TLSv1.2 DH RSA AESGCM(256) AEAD)
0x006B DHE-RSA-AES256-SHA256 (TLSv1.2 DH RSA AES(256) SHA256)
0xC030 ECDHE-RSA-AES256-GCM-SHA384 (TLSv1.2 ECDH RSA AESGCM(256) AEAD)
0xC028 ECDHE-RSA-AES256-SHA384 (TLSv1.2 ECDH RSA AES(256) SHA384)
0x009E DHE-RSA-AES128-GCM-SHA256 (TLSv1.2 DH RSA AESGCM(128) AEAD)
0x0067 DHE-RSA-AES128-SHA256 (TLSv1.2 DH RSA AES(128) SHA256)
0xC02F ECDHE-RSA-AES128-GCM-SHA256 (TLSv1.2 ECDH RSA AESGCM(128) AEAD)
0xC027 ECDHE-RSA-AES128-SHA256 (TLSv1.2 ECDH RSA AES(128) SHA256)
