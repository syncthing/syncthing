Syncthing Configuration
=======================

Synopsis
--------

::

    $HOME/.config/syncthing
    $HOME/Library/Application Support/Syncthing
    %AppData%/Syncthing
    %localappdata%/Syncthing

Description
-----------

Syncthing uses a single directory to store configuration, crypto keys
and index caches. The location defaults to ``$HOME/.config/syncthing``
(Unix-like), ``$HOME/Library/Application Support/Syncthing`` (Mac),
``%AppData%/Syncthing`` (Windows XP) or ``%LocalAppData%/Syncthing``
(Windows 7+). It can be changed at runtime using the ``-home`` flag. In this
directory the following files are located:

:file:`config.xml`
    The configuration file, in XML format.

:file:`cert.pem`, :file:`key.pem`
    The device's RSA public and private key. These form the basis for the
    device ID. The key must be kept private.

:file:`https-cert.pem`, :file:`https-key.pem`
    The certificate and key for HTTPS GUI connections. These may be replaced
    with a custom certificate for HTTPS as desired.

:file:`index-{*}.db`
    A directory holding the database with metadata and hashes of the files
    currently on disk and available from peers.

:file:`csrftokens.txt`
    A list of recently issued CSRF tokens (for protection against browser cross
    site request forgery).

Config File Format
------------------

The following is shows the default configuration file:

.. code-block:: xml

    <configuration version="10">
        <folder id="default" path="/Users/jb/Sync" ro="false" rescanIntervalS="60" ignorePerms="false" autoNormalize="false">
            <device id="5SYI2FS-LW6YAXI-JJDYETS-NDBBPIO-256MWBO-XDPXWVG-24QPUM4-PDW4UQU"></device>
            <versioning></versioning>
            <copiers>0</copiers>
            <pullers>0</pullers>
            <hashers>0</hashers>
            <order>random</order>
        </folder>
        <device id="5SYI2FS-LW6YAXI-JJDYETS-NDBBPIO-256MWBO-XDPXWVG-24QPUM4-PDW4UQU" name="syno" compression="metadata" introducer="false">
            <address>dynamic</address>
        </device>
        <gui enabled="true" tls="false">
            <address>127.0.0.1:8384</address>
            <apikey>l7jSbCqPD95JYZ0g8vi4ZLAMg3ulnN1b</apikey>
        </gui>
        <options>
            <listenAddress>0.0.0.0:56847</listenAddress>
            <globalAnnounceServer>udp4://announce.syncthing.net:22026</globalAnnounceServer>
            <globalAnnounceServer>udp6://announce-v6.syncthing.net:22026</globalAnnounceServer>
            <globalAnnounceEnabled>true</globalAnnounceEnabled>
            <localAnnounceEnabled>true</localAnnounceEnabled>
            <localAnnouncePort>21025</localAnnouncePort>
            <localAnnounceMCAddr>[ff32::5222]:21026</localAnnounceMCAddr>
            <maxSendKbps>0</maxSendKbps>
            <maxRecvKbps>0</maxRecvKbps>
            <reconnectionIntervalS>60</reconnectionIntervalS>
            <startBrowser>true</startBrowser>
            <upnpEnabled>true</upnpEnabled>
            <upnpLeaseMinutes>60</upnpLeaseMinutes>
            <upnpRenewalMinutes>30</upnpRenewalMinutes>
            <upnpTimeoutSeconds>10</upnpTimeoutSeconds>
            <urAccepted>0</urAccepted>
            <urUniqueID></urUniqueID>
            <restartOnWakeup>true</restartOnWakeup>
            <autoUpgradeIntervalH>12</autoUpgradeIntervalH>
            <keepTemporariesH>24</keepTemporariesH>
            <cacheIgnoredFiles>true</cacheIgnoredFiles>
            <progressUpdateIntervalS>5</progressUpdateIntervalS>
            <symlinksEnabled>true</symlinksEnabled>
            <limitBandwidthInLan>false</limitBandwidthInLan>
            <databaseBlockCacheMiB>0</databaseBlockCacheMiB>
        </options>
    </configuration>

Configuration Element
---------------------

This is the root element.

version
    The config version. Increments whenever a change is made that requires
    migration from previous formats.

Folder Element
--------------

.. code-block:: xml

    <folder id="default" path="/Users/jb/Sync" ro="false" rescanIntervalS="60" ignorePerms="false" autoNormalize="false">
        <device id="5SYI2FS-LW6YAXI-JJDYETS-NDBBPIO-256MWBO-XDPXWVG-24QPUM4-PDW4UQU"></device>
        <versioning></versioning>
        <copiers>0</copiers>
        <pullers>0</pullers>
        <hashers>0</hashers>
        <order>random</order>
    </folder>

One or more ``folder`` elements must be present in the file. Each element
describes one folder. The following attributes may be set on the ``folder``
element:

id
    The folder ID, must be unique. (mandatory)

path
    The oath to the directory where the folder is stored on this
    device; not sent to other devices. (mandatory)

ro
    True if the folder is read only (Master mode; will not be modified by
    Syncthing) on this device.

rescanIntervalS
    The rescan interval, in seconds.

ignorePerms
    True if the folder should ignore permissions.

autoNormalize
    Automatically correct UTF-8 normalization errors found in file names.

The following child elements may exist:

device
    These must have the ``id`` attribute and nothing else. Mentioned devices
    are those that will be sharing the folder in question. Each mentioned
    device must have a separate ``device`` element later in the file. It is
    customary that the local device ID is included in all repositories.
    Syncthing will currently add this automatically if it is not present in
    the configuration file.

versioning
    Specifies a versioning configuration.

.. seealso::
    :ref:`versioning`

copiers, pullers, hashers
    The number of copier, puller and hasher routines to use, or zero for the
    system determined optimum. These are low level performance options for
    advanced users only; do not change unless requested to or you've actually
    read and understood the code yourself. :)

order
    The order in which needed files should be pulled from the cluster.
    The possibles values are:

    random
        Pull files in random order. This optimizes for balancing resources among
        the devices in a cluster.

    alphabetic
        Pull files ordered by file name alphabetically.

    smallestFirst, largestFirst
        Pull files ordered by file size; smallest and largest first respectively.

    oldestFirst, newestFirst
        Pull files ordered by modification time; oldest and newest first
        respectively.

Device Element
--------------

.. code-block:: xml

    <device id="5SYI2FS-LW6YAXI-JJDYETS-NDBBPIO-256MWBO-XDPXWVG-24QPUM4-PDW4UQU" name="syno" compression="metadata" introducer="false">
        <address>dynamic</address>
    </device>

One or more ``device`` elements must be present in the file. Each element
describes a device participating in the cluster. It is customary to include a
``device`` element for the local device; Syncthing will currently add one if
it is not present. The following attributes may be set on the ``device``
element:

id
    The device ID. This must be written in canonical form, that is without any
    spaces or dashes. (mandatory)

name
    A friendly name for the device. (optional)

compression
    Whether to use protocol compression when sending messages to this device.
    The possible values are:

    metadata
        Compress metadata packets, such as index information. Metadata is
        usually very compression friendly so this is a good default.

    always
        Compress all packets, including file data. This is recommended if the
        folders contents are mainly compressible data such as documents or
        text files.

    never
        Disable all compression.

introducer
    Set to true if this device should be trusted as an introducer, i.e. we
    should copy their list of devices per folder when connecting.

In addition, one or more ``address`` child elements must be present. Each
contains an address to use when attempting to connect to this device and will
be tried in order. Accepted formats are:

IPv4 address (``192.0.2.42``)
    The default port (22000) is used.

IPv4 address and port (``192.0.2.42:12345``)
    The address and port is used as given.

IPv6 address (``2001:db8::23:42``)
    The default port (22000) is used.

IPv6 address and port (``[2001:db8::23:42]:12345``)
    The address and port is used as given. The address must be enclosed in
    square brackets.

``dynamic``
    The word ``dynamic`` means to use local and global discovery to find the
    device.

GUI Element
-----------

.. code-block:: xml

    <gui enabled="true" tls="false">
        <address>127.0.0.1:8384</address>
        <apikey>l7jSbCqPD95JYZ0g8vi4ZLAMg3ulnN1b</apikey>
    </gui>


There must be exactly one ``gui`` element. The GUI configuration is also used
by the :ref:`rest-api` and the :ref:`event-api`. The following attributes may
be set on the ``gui`` element:

enabled
    If not ``true``, the GUI and API will not be started.

tls
    If set to ``true``, TLS (HTTPS) will be enforced. Non-HTTPS requests will
    be redirected to HTTPS. When this is set to ``false``, TLS connections are
    still possible but it is not mandatory.

The following child elements may be present:

address
    Set the listen addresses. One or more address elements must be present.
    Allowed address formats are:

    IPv4 address and port (``127.0.0.1:8384``)
        The address and port is used as given.

    IPv6 address and port (``[::1]:8384``)
        The address and port is used as given. The address must be enclosed in
        square brackets.

    Wildcard and port (``0.0.0.0:12345``, ``[::]:12345``, ``:12345``)
        These are equivalent and will result in Syncthing listening on all
        interfaces and both IPv4 and IPv6.

username
    Set to require authentication.

password
    Contains the bcrypt hash of the real password.

apikey
    If set, this is the API key that enables usage of the REST interface.

Options Element
---------------

.. code-block:: xml

    <options>
        <listenAddress>0.0.0.0:56847</listenAddress>
        <globalAnnounceServer>udp4://announce.syncthing.net:22026</globalAnnounceServer>
        <globalAnnounceServer>udp6://announce-v6.syncthing.net:22026</globalAnnounceServer>
        <globalAnnounceEnabled>true</globalAnnounceEnabled>
        <localAnnounceEnabled>true</localAnnounceEnabled>
        <localAnnouncePort>21025</localAnnouncePort>
        <localAnnounceMCAddr>[ff32::5222]:21026</localAnnounceMCAddr>
        <maxSendKbps>0</maxSendKbps>
        <maxRecvKbps>0</maxRecvKbps>
        <reconnectionIntervalS>60</reconnectionIntervalS>
        <startBrowser>true</startBrowser>
        <upnpEnabled>true</upnpEnabled>
        <upnpLeaseMinutes>60</upnpLeaseMinutes>
        <upnpRenewalMinutes>30</upnpRenewalMinutes>
        <upnpTimeoutSeconds>10</upnpTimeoutSeconds>
        <urAccepted>0</urAccepted>
        <urUniqueID></urUniqueID>
        <restartOnWakeup>true</restartOnWakeup>
        <autoUpgradeIntervalH>12</autoUpgradeIntervalH>
        <keepTemporariesH>24</keepTemporariesH>
        <cacheIgnoredFiles>true</cacheIgnoredFiles>
        <progressUpdateIntervalS>5</progressUpdateIntervalS>
        <symlinksEnabled>true</symlinksEnabled>
        <limitBandwidthInLan>false</limitBandwidthInLan>
        <databaseBlockCacheMiB>0</databaseBlockCacheMiB>
        <pingTimeoutS>60</pingTimeoutS>
        <pingIdleTimeS>120</pingIdleTimeS>
    </options>

The ``options`` element contains all other global configuration options.

listenAddress
    The listen address for incoming sync connections. See the ``address``
    element under the `GUI Element`_ for allowed syntax.

globalAnnounceServer
    A URI to a global announce (discovery) server. Allowed protocol prefixes
    are ``udp4://`` (UDP over IPv4), ``udp6://`` (UDP over IPv6) and
    ``udp://`` (UDP over any available protocol).

globalAnnounceEnabled
    Whether to announce this device to the global announce (discovery) server,
    and also use it to look up other device.

localAnnounceEnabled
    Whether to send announcements to the local LAN, also use such
    announcements to find other devices.

localAnnouncePort
    The port on which to listen and send IPv4 broadcast announcements to.

localAnnounceMCAddr
    The group address and port to join and send IPv6 multicast announcements on.

maxSendKbps
    Outgoing data rate limit, in kibibits per second.

maxRecvKbps
    Incoming data rate limits, in kibibits per second.

reconnectionIntervalS
    The number of seconds to wait between each attempt to connect to currently
    unconnected devices.

startBrowser
    Whether to attempt to start a browser to show the GUI when Syncthing starts.

upnpEnabled
    Whether to attempt to perform an UPnP port mapping for incoming sync
    connections.

upnpLeaseMinutes
    Request a lease for this many minutes; zero to request a permanent lease.

upnpRenewalMinutes
    Attempt to renew the lease after this many minutes.

upnpTimeoutSeconds
    When scanning for UPnP devices, wait this long for responses.

urAccepted
    Whether the user as accepted to submit anonymous usage data. The default,
    ``0``, mean the user has not made a choice, and Syncthing will ask at some
    point in the future. ``-1`` means no, a number above zero means that that
    version of usage reporting has been accepted.

urUniqueID
    The unique ID sent together with the usage report. Generated when usage
    reporting is enabled.

restartOnWakeup
    Whether to perform a restart of Syncthing when it is detected that we are
    waking from sleep mode (i.e. a folded up laptop).

autoUpgradeIntervalH
    Check for a newer version after this many hours. Set to zero to disable
    automatic upgrades.

keepTemporariesH
    Keep temporary failed transfers for this many hours. While the temporaries
    are kept, the data they contain need not be transferred again.

cacheIgnoredFiles
    Whether to cache the results of ignore pattern evaluation. Performance at
    the price of memory.

progressUpdateIntervalS
    .. note:: Requires explanation.

symlinksEnabled
    Whether to sync symlinks, if supported by the system.

limitBandwidthInLan
    Whether to apply bandwidth limits to devices in the same broadcast domain
    as the local device.

databaseBlockCacheMiB
    Override the automatically calculated database block cache size. Don't,
    unless you're very short on memory, in which case you want to set this to
    ``8``.

pingTimeoutS
    Ping-timeout in seconds. Don't change it unless you are having issues due to
    slow response time (slow connection/cpu) and large index exchanges

pingIdleTimeS
    ping interval in seconds. Don't change it unless you feel it's necessary.
