.. _relay-v1:

Relay Protocol v1
=================

What is a relay?
----------------

Relay is a service which relays data between two *devices* which are not able to
connect to each other directly otherwise. This is usually due to both devices
being behind a NAT and neither side being able to open a port which would
be directly accessible from the internet.

A relay was designed to relay BEP protocol, hence the reliance on device ID's
in the protocol spec, but at the same time it is general enough that could be
reused by other protocols or applications, as the data transferred between two
devices which use a relay is completely obscure and does not affect the
relaying.

Operation modes
---------------

Relay listens on a single TCP socket, but has two different connection modes,
where a connection mode is a predefined set of messages which the relay and
the device are expecting to exchange.

The first mode is the `protocol` mode which allows a client to interact
with the relay, for example join the relay, or request to connect to a device,
given it is available on the relay. Similarly to BEP, protocol mode requires
the device to connect via TLS using a strong suite of ciphers (same as BEP),
which allows the relay to verify and derive the identity (Device ID) of the
device.

The second mode is the `session` mode which after a few initial messages
connects two devices directly to each other via the relay, and is a plain-text
protocol, which for every byte written by one device, sends the same set of
bytes to the other device and vica versa.

Identifying the connection mode
-------------------------------

Because both connection modes operate over the same single socket, a method
of detecting the connection mode is required.

When a new client connects to the relay, the relay checks the first byte
that the client has sent, and if that matches 0x16, that implies to us that
the connection is a protocol mode connection, due to 0x16 being the first byte
in the TLS handshake, and only protocol mode connections use TLS.

If the first byte is not 0x16, then we assume that the connection is a session
mode connection.

Protocol mode
-------------

Protocol mode uses TLS and protocol name defined by the TLS header should be
`bep-relay`.

Protocol mode has two submodes:
1. Permanent protocol submode - Joining the relay, and waiting for messages from
the relay asking to connect to some device which is interested in having a
session with you.
2. Temporary protocol submode - Only used to request a session with a device
which is connected to the relay using the permanent protocol submode.


Permanent protocol submode
^^^^^^^^^^^^^^^^^^^^^^^^^^

A permanent protocol submode begins with the client sending a JoinRelayRequest
message, which the relay responds to with either a ResponseSuccess or
ResponseAlreadyConnected message if a client with the same device ID already
exists.

After the client has joined, no more messages are exchanged apart from
Ping/Pong messages for general connection keep alive checking.

From this point onwards, the client stand-by's and waits for SessionInvitation
messages from the relay, which implies that some other device is trying to
connect with you. SessionInvitation message contains the unique session key
which then can be used to establish a connection in session mode.

If the client fails to send a JoinRelayRequest message within the first ping
interval, the connection is terminated.
If the client fails to send a message (even if its a ping message) every minute
(by default), the connection is terminated.

Temporary protocol submode
^^^^^^^^^^^^^^^^^^^^^^^^^^

A temporary protocol submode begins with ConnectRequest message, to which the
relay responds with either ResponseNotFound if the device the client it is after
is not available, or with a SessionInvitation, which contains the unique session
key which then can be used to establish a connection in session mode.

The connection is terminated immediately after that.

Example Exchange
~~~~~~~~~~~~~~~~

Client A - Permanent protocol submode
Client B - Temporary protocol submode

===  =======================  ====================== =====================
 #         Client (A)                 Relay                Client (B)
===  =======================  ====================== =====================
 1   JoinRelayRequest->
 2                            <-ResponseSuccess
 3   Ping->
 4                            <-Pong
 5                                                    <-ConnectRequest(A)
 6                            SessionInvitation(A)->
 7                            <-SessionInvitation(B)
 8                                                    (Disconnects)
 9   Ping->
 10                           <-Pong
 11  Ping->
 12                           <-Pong
===  =======================  ====================== =====================


Session mode
------------

The first and only message the client sends in the session mode is the
SessionInvitation message which contains the session key identifying which
session you are trying to join. The relay responds with one of the following
Response messages:

1. ResponseNotFound - Session key is invalid
2. ResponseAlreadyConnected - Session is full (both sides already connected)
3. ResponseSuccess - You have successfully joined the session

After the successful response, all the bytes written and received will be
relayed between the two devices in the session directly.

Example Exchange
^^^^^^^^^^^^^^^^

Client A - Permanent protocol mode
Client B - Temporary protocol mode

===  =======================  ====================== =====================
 #         Client (A)                 Relay                Client (B)
===  =======================  ====================== =====================
 1   JoinSessionRequest(A)->
 2                            <-ResponseSuccess
 3   Data->                   (Buffers data)
 4   Data->                   (Buffers data)
 5                                                   <-JoinSessionRequest(B)
 6                            ResponseSuccess->
 7                            Relays data ->
 8                            Relays data ->
 9                            <-Relays data          <-Data
===  =======================  ====================== =====================

Messages
--------

All messages are preceded by a header message. Header message contains the
magic value 0x9E79BC40, message type integer, and message length.

.. warning::

	Some messages have no content, apart from the implied header which allows
	us to identify what type of message it is.


Header structure
^^^^^^^^^^^^^^^^

::

	 0                   1                   2                   3
	 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	|                             Magic                             |
	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	|                         Message Type                          |
	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	|                        Message Length                         |
	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+


	struct Header {
		unsigned int Magic;
		int MessageType;
		int MessageLength;
	}

Ping message (Type = 0)
^^^^^^^^^^^^^^^^^^^^^^^

::

	 0                   1                   2                   3
	 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+


	struct Ping {
	}

Pong message (Type = 1)
^^^^^^^^^^^^^^^^^^^^^^^

::

	 0                   1                   2                   3
	 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+


	struct Pong {
	}

JoinRelayRequest message (Type = 2)
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

::

	 0                   1                   2                   3
	 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+


	struct JoinRelayRequest {
	}

JoinSessionRequest message (Type = 3)
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

::

	 0                   1                   2                   3
	 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	|                         Length of Key                         |
	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	/                                                               /
	\                     Key (variable length)                     \
	/                                                               /
	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+


	struct JoinSessionRequest {
		opaque Key<32>;
	}

: Key
	This is a unique random session key generated by the relay server. It is
	used to identify which session you are trying to connect to.


Response message (Type = 4)
^^^^^^^^^^^^^^^^^^^^^^^^^^^

::

	 0                   1                   2                   3
	 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	|                             Code                              |
	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	|                       Length of Message                       |
	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	/                                                               /
	\                   Message (variable length)                   \
	/                                                               /
	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+


	struct Response {
		int Code;
		string Message<>;
	}

: Code
	An integer representing the status code.
: Message
	Message associated with the code.

.. Protocol defined responses:
	1. ResponseSuccess           = Response{0, "success"}
	2. ResponseNotFound          = Response{1, "not found"}
	3. ResponseAlreadyConnected  = Response{2, "already connected"}
	4. ResponseInternalError     = Response{99, "internal error"}
	5. ResponseUnexpectedMessage = Response{100, "unexpected message"}

ConnectRequest message (Type = 5)
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

::

	 0                   1                   2                   3
	 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	|                         Length of ID                          |
	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	/                                                               /
	\                     ID (variable length)                      \
	/                                                               /
	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+


	struct ConnectRequest {
		opaque ID<32>;
	}

: ID
	Device ID to which the client would like to connect.


SessionInvitation message (Type = 6)
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

::

	 0                   1                   2                   3
	 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	|                        Length of From                         |
	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	/                                                               /
	\                    From (variable length)                     \
	/                                                               /
	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	|                         Length of Key                         |
	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	/                                                               /
	\                     Key (variable length)                     \
	/                                                               /
	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	|                       Length of Address                       |
	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	/                                                               /
	\                   Address (variable length)                   \
	/                                                               /
	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	|            0x0000             |             Port              |
	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	|                  Server Socket (V=0 or 1)                   |V|
	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+


	struct SessionInvitation {
		opaque From<32>;
		opaque Key<32>;
		opaque Address<32>;
		unsigned int Port;
		bool ServerSocket;
	}

: From
	Device ID identifying who you will be connecting with.
: Key
	A unique random session key generated by the relay server. It is used to
	identify which session you are trying to connect to.
: Address
	An optional IP address on which the relay server is expecting you to
	connect, in order to start a connection in session mode.
	Empty/all zero IP should be replaced with the relay's public IP address that
	was used when establishing the protocol mode connection.
: Port
 	An optional port on which the relay server is expecting you to connect,
	in order to start a connection in session mode.
: Server Socket
	Because both sides connecting to the relay use the client side of the socket,
	and some protocols behave differently depending if the connection starts on
	the server side or the client side, this boolean indicates which side of the
	connection this client should assume it's getting. The value is inverted in
	the invitation which is sent to the other device, so that there is always
	one client socket, and one server socket.

How syncthing uses relays, and general security
-----------------------------------------------

In the case of Syncthing and BEP, when two devices connect via relay, they
start their standard TLS connection encapsulated within the relay's plain-text
session connection, effectively upgrading the plain-text connection to a TLS
connection.

Even though the relay could be used for man-in-the-middle attack, using TLS
at the application/BEP level ensures that all the traffic is safely encrypted,
and is completely meaningless to the relay. Furthermore, the secure suite of
ciphers used by BEP provides forward secrecy, meaning that even if the relay
did capture all the traffic, and even if the attacker did get their hands on the
device keys, they would still not be able to recover/decrypt any traffic which
was transported via the relay.

After establishing a relay session, syncthing looks at the SessionInvitation
message, and depending which side it has received, wraps the raw socket in
either a TLS client socket or a TLS server socket depending on the ServerSocket
boolean value in the SessionInvitation, and starts the TLS handshake.

From that point onwards it functions exactly the same way as if syncthing was
establishing a direct connection with the other device over the internet,
performing device ID validation, and full TLS encryption, and provides the same
security properties as it would provide when connecting over the internet.

Examples of Strong Cipher Suites
--------------------------------

======  ===========================  ==================================
ID      Name                         Description
======  ===========================  ==================================
0x009F  DHE-RSA-AES256-GCM-SHA384    TLSv1.2 DH RSA AESGCM(256) AEAD
0x006B  DHE-RSA-AES256-SHA256        TLSv1.2 DH RSA AES(256) SHA256
0xC030  ECDHE-RSA-AES256-GCM-SHA384  TLSv1.2 ECDH RSA AESGCM(256) AEAD
0xC028  ECDHE-RSA-AES256-SHA384      TLSv1.2 ECDH RSA AES(256) SHA384
0x009E  DHE-RSA-AES128-GCM-SHA256    TLSv1.2 DH RSA AESGCM(128) AEAD
0x0067  DHE-RSA-AES128-SHA256        TLSv1.2 DH RSA AES(128) SHA256
0xC02F  ECDHE-RSA-AES128-GCM-SHA256  TLSv1.2 ECDH RSA AESGCM(128) AEAD
0xC027  ECDHE-RSA-AES128-SHA256      TLSv1.2 ECDH RSA AES(128) SHA256
======  ===========================  ==================================

