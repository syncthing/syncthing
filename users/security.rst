Security Principles
===================

Security is one of the primary project goals. This means that it should not be
possible for an attacker to join a cluster uninvited, and it should not be
possible to extract private information from intercepted traffic. Currently this
is implemented as follows.

All traffic is protected by TLS. To prevent uninvited nodes from joining a
cluster, the certificate fingerprint of each node is compared to a preset list
of acceptable nodes at connection establishment. The fingerprint is computed as
the SHA-256 hash of the certificate and displayed in BASE32 encoding to form a
reasonably compact and convenient string.

Incoming requests for file data are verified to the extent that the requested
file name must exist in the local index and the global model.

For information about ensuring you are running the code you think you are and
for reporting security vulnerabilities, please see the official `security page
<http://syncthing.net/security.html>`__.

Information Leakage
-------------------

Global Discovery
~~~~~~~~~~~~~~~~

When global discovery is enabled, Syncthing sends an announcement packet every
30 minutes to the global discovery server, so that it can keep a mapping between
your device ID and external IP. Also, when connecting to other devices that have
not been seen on the local network, a query is sent to the global discovery
server containing the device ID of the requested device. The discovery server is
currently hosted by :user:`calmh`. Global discovery defaults to **on**.

When turned off, devices with dynamic addresses not on the local network cannot
be found and connected to.

If a different global discovery server is configured, no data is sent to the
default global discovery server.

Local Discovery
~~~~~~~~~~~~~~~

When local discovery is enabled, Syncthing sends broadcast (IPv4) and multicast
(IPv6) packets to the local network every 30 seconds. The packets contain the
device ID and listening port. Local discovery defaults to **on**.

An eavesdropper on the local network can deduce which machines are running
Syncthing with local discovery enabled, and what their device IDs are.

When turned off, devices with dynamic addresses on the local network cannot be
found and connected to.

Upgrade Checks
~~~~~~~~~~~~~~

When automatic upgrades are enabled, Syncthing checks for a new version at
startup and then once every twelve hours. This is by an HTTPS request to the
download site for releases, currently **hosted at GitHub**. Automatic upgrades
default to **on** (unless Syncthing was compiled with upgrades disabled).

Even when automatic upgrades are disabled in the configuration, an upgrade check
as above is done when the GUI is loaded, in order to show the "Upgrade to ..."
button when necessary. This can be disabled only by compiling syncthing with
upgrades disabled.

In effect this exposes the majority of the Syncthing population to tracking by
the operator of the download site (currently GitHub). That data is not available
to outside parties (including :user:`calmh` etc), except that download counts
per release binary are available in the GitHub API. The upgrade check (or
download) requests *do not* contain any identifiable information about the user,
device, Syncthing version, etc.

Usage Reporting
~~~~~~~~~~~~~~~

When usage reporting is enabled, Syncthing reports usage data at startup and
then every 24 hours. The report is sent as an HTTPS POST to the usage reporting
server, currently hosted by :user:`calmh`. The contents of the usage report can
be seen behind the "Preview" link in settings. Usage reporting defaults to
**off** but the GUI will ask once about enabling it, shortly after the first
install.

The reported data is protected from eavesdroppers, but the connection to the
usage reporting server itself may expose the client as running Syncthing.

Sync Connections (BEP)
~~~~~~~~~~~~~~~~~~~~~~

Sync connections are attempted to all configured devices, when the address is
possible to resolve. The sync connection is based on TLS 1.2. The TLS
certificates are sent in clear text (as in HTTPS etc), meaning that the
certificate Common Name (by default ``syncthing``) is visible.

An eavesdropper can deduce that this is a Syncthing connection and calculate the
device ID:s involved based on the hashes of the sent certificates.

Likewise, if the sync port (default 22000) is accessible from the internet, a
port scanner may discover it, attempt a TLS negotiation and thus obtain the
device certificate. This provides the same information as in the eavesdropper
case.

Web GUI
~~~~~~~

If the web GUI is accessible, it exposes the device as running Syncthing. The
web GUI defaults to being reachable from the **local host only**.

In Short
--------

Parties doing surveillance on your network (whether that be corporate IT, the
NSA or someone else) will be able to see that you use Syncthing, and your device
ID's `are OK to share anyway
<http://docs.syncthing.net/users/faq.html#should-i-keep-my-device-ids-secret>`__,
but the actual transmitted data is protected as well as we can. Knowing your
device ID can expose your IP address, using global discovery.
