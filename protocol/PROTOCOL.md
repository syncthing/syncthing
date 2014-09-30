Block Exchange Protocol v1
==========================

Introduction and Definitions
----------------------------

BEP is used between two or more _devices_ thus forming a _cluster_. Each
device has one or more _folders_ of files described by the _local
model_, containing metadata and block hashes. The local model is sent to
the other devices in the cluster. The union of all files in the local
models, with files selected for highest change version, forms the
_global model_. Each device strives to get its folders in sync with
the global model by requesting missing or outdated blocks from the other
devices in the cluster.

File data is described and transferred in units of _blocks_, each being
128 KiB (131072 bytes) in size.

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL
NOT", "SHOULD", "SHOULD NOT", "RECOMMENDED",  "MAY", and
"OPTIONAL" in this document are to be interpreted as described in
RFC 2119.

Transport and Authentication
----------------------------

BEP is deployed as the highest level in a protocol stack, with the lower
level protocols providing encryption and authentication.

    +-----------------------------|
    |   Block Exchange Protocol   |
    |-----------------------------|
    | Encryption & Auth (TLS 1.2) |
    |-----------------------------|
    |             TCP             |
    |-----------------------------|
    v             ...             v

The encryption and authentication layer SHALL use TLS 1.2 or a higher
revision. A strong cipher suite SHALL be used, with "strong cipher
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
fingerprints (SHA-256) referred to as "Device IDs".

There is no required order or synchronization among BEP messages except
as noted per message type - any message type may be sent at any time and
the sender need not await a response to one message before sending
another. Responses MUST however be sent in the same order as the
requests are received.

The underlying transport protocol MUST be TCP.

Messages
--------

Every message starts with one 32 bit word indicating the message
version, type and ID, followed by the length of the message. The header
is in network byte order, i.e. big endian.

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |  Ver  |       Message ID      |      Type     |   Reserved  |C|
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                            Length                             |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

For BEP v1 the Version field is set to zero. Future versions with
incompatible message formats will increment the Version field. A message
with an unknown version is a protocol error and MUST result in the
connection being terminated. A client supporting multiple versions MAY
retry with a different protocol version upon disconnection.

The Message ID is set to a unique value for each transmitted request
message. In response messages it is set to the Message ID of the
corresponding request message. The uniqueness requirement implies that
no more than 4096 messages may be outstanding at any given moment. The
ordering requirement implies that a response to a given message ID also
means that all preceding messages have been received, specifically those
which do not otherwise demand a response. Hence their message ID:s may
be reused.

The Type field indicates the type of data following the message header
and is one of the integers defined below. A message of an unknown type
is a protocol error and MUST result in the connection being terminated.

The Compression bit "C" indicates the compression used for the message.

For C=1:

  * The Length field contains the length, in bytes, of the
    compressed message data plus a four byte uncompressed length field.

  * The compressed message data is preceeded by a 32 bit field denoting
    the length of the uncompressed message.

  * The message data is compressed using the LZ4 format and algorithm
    described in https://code.google.com/p/lz4/.

For C=0:

  * The Length field contains the length, in bytes, of the
    uncompressed message data.

  * The message is not compressed.

All data within the message (post decompression, if compression is
in use) MUST be in XDR (RFC 1014) encoding. All fields shorter than 32
bits and all variable length data MUST be padded to a multiple of 32
bits. The actual data types in use by BEP, in XDR naming convention, are
the following:

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

### Cluster Config (Type = 0)

This informational message provides information about the cluster
configuration as it pertains to the current connection. A Cluster Config
message MUST be the first message sent on a BEP connection. Additional
Cluster Config messages MUST NOT be sent after the initial exchange.

#### Graphical Representation

    ClusterConfigMessage Structure:

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                     Length of ClientName                      |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    /                                                               /
    \                 ClientName (variable length)                  \
    /                                                               /
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                    Length of ClientVersion                    |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    /                                                               /
    \                ClientVersion (variable length)                \
    /                                                               /
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                       Number of Folders                       |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    /                                                               /
    \                Zero or more Folder Structures                 \
    /                                                               /
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                       Number of Options                       |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    /                                                               /
    \                Zero or more Option Structures                 \
    /                                                               /
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+


    Folder Structure:

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                         Length of ID                          |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    /                                                               /
    \                     ID (variable length)                      \
    /                                                               /
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                       Number of Devices                       |
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
    |                             Flags                             |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                                                               |
    +                  Max Local Version (64 bits)                  +
    |                                                               |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+


    Option Structure:

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

#### Fields

The ClientName and ClientVersion fields identify the implementation. The
values SHOULD be simple strings identifying the implementation name, as
a user would expect to see it, and the version string in the same
manner. An example ClientName is "syncthing" and an example
ClientVersion is "v0.7.2". The ClientVersion field SHOULD follow the
patterns laid out in the [Semantic Versioning](http://semver.org/)
standard.

The Folders field lists all folders that will be synchronized
over the current connection. Each folder has a list of participating
Devices. Each device has an associated Flags field to indicate the sharing
mode of that device for the folder in question. See the discussion on
Sharing Modes.

The Device Flags field contains the following single bit flags:

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |          Reserved         |Pri|          Reserved       |I|R|T|
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

 - Bit 31 ("T", Trusted) is set for devices that participate in trusted
   mode.

 - Bit 30 ("R", Read Only) is set for devices that participate in read
   only mode.

 - Bit 29 ("I", Introducer) is set for devices that are trusted as cluster
   introducers.

 - Bits 16 through 28 are reserved and MUST be set to zero.

 - Bits 14-15 ("Pri) indicate the device's upload priority for this
   folder. Possible values are:

    - 00: The default. Normal priority.

    - 01: High priority. Other devices SHOULD favour requesting files from
      this device over devices with normal or low priority.

    - 10: Low priority. Other devices SHOULD avoid requesting files from
      this device when they are available from other devices.

    - 11: Sharing disabled. Other devices SHOULD NOT request files from
      this device.

 - Bits 0 through 14 are reserved and MUST be set to zero.

Exactly one of the T and R bits MUST be set.

The per device Max Local Version field contains the highest local file
version number of the files already known to be in the index sent by
this device. If nothing is known about the index of a given device, this
field MUST be set to zero. When receiving a Cluster Config message with
a non-zero Max Local Version for the local device ID, a device MAY elect to
send an Index Update message containing only files with higher local
version numbers in place of the initial Index message.

The Options field contain option values to be used in an implementation
specific manner. The options list is conceptually a map of Key => Value
items, although it is transmitted in the form of a list of (Key, Value)
pairs, both of string type. Key ID:s are implementation specific. An
implementation MUST ignore unknown keys. An implementation MAY impose
limits on the length keys and values. The options list may be used to
inform devices of relevant local configuration options such as rate
limiting or make recommendations about request parallelism, device
priorities, etc. An empty options list is valid for devices not having any
such information to share. Devices MAY NOT make any assumptions about
peers acting in a specific manner as a result of sent options.

#### XDR

    struct ClusterConfigMessage {
        string ClientName<>;
        string ClientVersion<>;
        Folder Folders<>;
        Option Options<>;
    }

    struct Folder {
        string ID<>;
        Device Devices<>;
    }

    struct Device {
        string ID<>;
        unsigned int Flags;
        unsigned hyper MaxLocalVersion;
    }

    struct Option {
        string Key<>;
        string Value<>;
    }

### Index (Type = 1) and Index Update (Type = 6)

The Index and Index Update messages define the contents of the senders
folder. An Index message represents the full contents of the
folder and thus supersedes any previous index. An Index Update
amends an existing index with new information, not affecting any entries
not included in the message. An Index Update MAY NOT be sent unless
preceded by an Index, unless a non-zero Max Local Version has been
announced for the given folder by the peer device.

An Index or Index Update message MUST be sent for each folder
included in the Cluster Config message, and MUST be sent before any
other message referring to that folder. A device with no data to
advertise MUST send an empty Index message (a file list of zero length).
If the folder contents change from non-empty to empty, an empty
Index message MUST be sent. There is no response to the Index message.

#### Graphical Representation

    IndexMessage Structure:

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                       Length of Folder                        |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    /                                                               /
    \                   Folder (variable length)                    \
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
    |                                                               |
    +                    Local Version (64 bits)                    +
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

The Folder field identifies the folder that the index message
pertains to. For single folder implementations the device MAY send an
empty folder ID or use the string "default".

The Name is the file name path relative to the folder root. Like all
strings in BEP, the Name is always in UTF-8 NFC regardless of operating
system or file system specific conventions. The Name field uses the
slash character ("/") as path separator, regardless of the
implementation's operating system conventions. The combination of
Folder and Name uniquely identifies each file in a cluster.

The Version field is the value of a cluster wide Lamport clock
indicating when the change was detected. The clock ticks on every
detected and received change. The combination of Folder, Name and
Version uniquely identifies the contents of a file at a given point in
time.

The Local Version field is the value of a device local monotonic clock at
the time of last local database update to a file. The clock ticks on
every local database update.

The Flags field is made up of the following single bit flags:

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |              Reserved           |P|I|D|   Unix Perm. & Mode   |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

 - The lower 12 bits hold the common Unix permission and mode bits. An
   implementation MAY ignore or interpret these as is suitable on the host
   operating system.

 - Bit 19 ("D") is set when the file has been deleted. The block list
   SHALL be of length zero and the modification time indicates the time
   of deletion or, if the time of deletion is not reliably determinable,
   the last known modification time.

 - Bit 18 ("I") is set when the file is invalid and unavailable for
   synchronization. A peer MAY set this bit to indicate that it can
   temporarily not serve data for the file.

 - Bit 17 ("P") is set when there is no permission information for the
   file. This is the case when it originates on a non-permission-
   supporting file system. Changes to only permission bits SHOULD be
   disregarded on files with this bit set. The permissions bits MUST be
   set to the octal value 0666.

 - Bit 0 through 16 are reserved for future use and SHALL be set to
   zero.

The hash algorithm is implied by the Hash length. Currently, the hash
MUST be 32 bytes long and computed by SHA256.

The Modified time is expressed as the number of seconds since the Unix
Epoch (1970-01-01 00:00:00 UTC).

In the rare occasion that a file is simultaneously and independently
modified by two devices in the same cluster and thus end up on the same
Version number after modification, the Modified field is used as a tie
breaker (higher being better), followed by the hash values of the file
blocks (lower being better).

The Blocks list contains the size and hash for each block in the file.
Each block represents a 128 KiB slice of the file, except for the last
block which may represent a smaller amount of data.

#### XDR

    struct IndexMessage {
        string Folder<>;
        FileInfo Files<>;
    }

    struct FileInfo {
        string Name<>;
        unsigned int Flags;
        hyper Modified;
        unsigned hyper Version;
        unsigned hyper LocalVer;
        BlockInfo Blocks<>;
    }

    struct BlockInfo {
        unsigned int Size;
        opaque Hash<>;
    }

### Request (Type = 2)

The Request message expresses the desire to receive a data block
corresponding to a part of a certain file in the peer's folder.

#### Graphical Representation

    RequestMessage Structure:

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                       Length of Folder                        |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    /                                                               /
    \                   Folder (variable length)                    \
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

The Folder and Name fields are as documented for the Index message.
The Offset and Size fields specify the region of the file to be
transferred. This SHOULD equate to exactly one block as seen in an Index
message.

#### XDR

    struct RequestMessage {
        string Folder<>;
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

### Close (Type = 7)

The Close message MAY be sent to indicate that the connection will be
torn down due to an error condition. A Close message MUST NOT be
followed by further messages.

#### Graphical Representation

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                       Length of Reason                        |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    /                                                               /
    \                   Reason (variable length)                    \
    /                                                               /
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

#### Fields

The Reason field contains a human description of the error condition,
suitable for consumption by a human.

    struct CloseMessage {
        string Reason<1024>;
    }

Sharing Modes
-------------

### Trusted

Trusted mode is the default sharing mode. Updates are exchanged in both
directions.

    +------------+     Updates      /---------\
    |            |  ----------->   /           \
    |   Device   |                 |  Cluster  |
    |            |  <-----------   \           /
    +------------+     Updates      \---------/

### Read Only

In read only mode, a device does not synchronize the local folder to
the cluster, but publishes changes to its local folder contents as
usual. The local folder can be seen as a "master copy" that is never
affected by the actions of other cluster devices.

    +------------+     Updates      /---------\
    |            |  ----------->   /           \
    |   Device   |                 |  Cluster  |
    |            |                 \           /
    +------------+                  \---------/

Message Limits
--------------

An implementation MAY impose reasonable limits on the length of message
fields to aid robustness in the face of corruption or broken
implementations. These limits, if imposed, SHOULD NOT be more
restrictive than the following:

### Index and Index Update Messages

 - Folder: 64 bytes
 - Number of Files: 10.000.000
 - Name: 1024 bytes
 - Number of Blocks: 1.000.000
 - Hash: 64 bytes

### Request Messages

 - Folder: 64 bytes
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
folder and transmits Response records (6 through 9). Device A updates
their folder contents and transmits an Index Update message (10).
Both peers enter idle state after 10. At some later time 11, peer A
determines that it has not seen data from B for some time and sends a
Ping request. A response is sent at 12.

Examples of Strong Cipher Suites
--------------------------------

* 0x009F DHE-RSA-AES256-GCM-SHA384 (TLSv1.2 DH RSA AESGCM(256) AEAD)
* 0x006B DHE-RSA-AES256-SHA256 (TLSv1.2 DH RSA AES(256) SHA256)
* 0xC030 ECDHE-RSA-AES256-GCM-SHA384 (TLSv1.2 ECDH RSA AESGCM(256) AEAD)
* 0xC028 ECDHE-RSA-AES256-SHA384 (TLSv1.2 ECDH RSA AES(256) SHA384)
* 0x009E DHE-RSA-AES128-GCM-SHA256 (TLSv1.2 DH RSA AESGCM(128) AEAD)
* 0x0067 DHE-RSA-AES128-SHA256 (TLSv1.2 DH RSA AES(128) SHA256)
* 0xC02F ECDHE-RSA-AES128-GCM-SHA256 (TLSv1.2 ECDH RSA AESGCM(128) AEAD)
* 0xC027 ECDHE-RSA-AES128-SHA256 (TLSv1.2 ECDH RSA AES(128) SHA256)
