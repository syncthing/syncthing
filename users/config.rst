.. _config:

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

The following shows the default configuration file:

.. code-block:: xml

    <configuration version="14">
        <folder id="zj2AA-q55a7" label="Default Folder (zj2AA-q55a7)" path="/Users/jb/Sync/" type="readwrite" rescanIntervalS="60" ignorePerms="false" autoNormalize="true">
            <device id="3LT2GA5-CQI4XJM-WTZ264P-MLOGMHL-MCRLDNT-MZV4RD3-KA745CL-OGAERQZ"></device>
            <minDiskFreePct>1</minDiskFreePct>
            <versioning></versioning>
            <copiers>0</copiers>
            <pullers>0</pullers>
            <hashers>0</hashers>
            <order>random</order>
            <ignoreDelete>false</ignoreDelete>
            <scanProgressIntervalS>0</scanProgressIntervalS>
            <pullerSleepS>0</pullerSleepS>
            <pullerPauseS>0</pullerPauseS>
            <maxConflicts>-1</maxConflicts>
            <disableSparseFiles>false</disableSparseFiles>
            <disableTempIndexes>false</disableTempIndexes>
        </folder>
        <device id="3LT2GA5-CQI4XJM-WTZ264P-MLOGMHL-MCRLDNT-MZV4RD3-KA745CL-OGAERQZ" name="syno" compression="metadata" introducer="false">
            <address>dynamic</address>
        </device>
        <gui enabled="true" tls="false">
            <address>127.0.0.1:8384</address>
            <apikey>k1dnz1Dd0rzTBjjFFh7CXPnrF12C49B1</apikey>
            <theme>default</theme>
        </gui>
        <options>
            <listenAddress>default</listenAddress>
            <globalAnnounceServer>default</globalAnnounceServer>
            <globalAnnounceEnabled>true</globalAnnounceEnabled>
            <localAnnounceEnabled>true</localAnnounceEnabled>
            <localAnnouncePort>21027</localAnnouncePort>
            <localAnnounceMCAddr>[ff12::8384]:21027</localAnnounceMCAddr>
            <maxSendKbps>0</maxSendKbps>
            <maxRecvKbps>0</maxRecvKbps>
            <reconnectionIntervalS>60</reconnectionIntervalS>
            <relaysEnabled>true</relaysEnabled>
            <relayReconnectIntervalM>10</relayReconnectIntervalM>
            <startBrowser>true</startBrowser>
            <natEnabled>true</natEnabled>
            <natLeaseMinutes>60</natLeaseMinutes>
            <natRenewalMinutes>30</natRenewalMinutes>
            <natTimeoutSeconds>10</natTimeoutSeconds>
            <urAccepted>0</urAccepted>
            <urUniqueID></urUniqueID>
            <urURL>https://data.syncthing.net/newdata</urURL>
            <urPostInsecurely>false</urPostInsecurely>
            <urInitialDelayS>1800</urInitialDelayS>
            <restartOnWakeup>true</restartOnWakeup>
            <autoUpgradeIntervalH>12</autoUpgradeIntervalH>
            <keepTemporariesH>24</keepTemporariesH>
            <cacheIgnoredFiles>false</cacheIgnoredFiles>
            <progressUpdateIntervalS>5</progressUpdateIntervalS>
            <symlinksEnabled>true</symlinksEnabled>
            <limitBandwidthInLan>false</limitBandwidthInLan>
            <minHomeDiskFreePct>1</minHomeDiskFreePct>
            <releasesURL>https://api.github.com/repos/syncthing/syncthing/releases?per_page=30</releasesURL>
            <overwriteRemoteDeviceNamesOnConnect>false</overwriteRemoteDeviceNamesOnConnect>
            <tempIndexMinBlocks>10</tempIndexMinBlocks>
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

    <folder id="zj2AA-q55a7" label="Default Folder (zj2AA-q55a7)" path="/Users/jb/Sync/" type="readwrite" rescanIntervalS="60" ignorePerms="false" autoNormalize="true" ro="false">
        <device id="3LT2GA5-CQI4XJM-WTZ264P-MLOGMHL-MCRLDNT-MZV4RD3-KA745CL-OGAERQZ"></device>
        <minDiskFreePct>1</minDiskFreePct>
        <versioning></versioning>
        <copiers>0</copiers>
        <pullers>0</pullers>
        <hashers>0</hashers>
        <order>random</order>
        <ignoreDelete>false</ignoreDelete>
        <scanProgressIntervalS>0</scanProgressIntervalS>
        <pullerSleepS>0</pullerSleepS>
        <pullerPauseS>0</pullerPauseS>
        <maxConflicts>-1</maxConflicts>
        <disableSparseFiles>false</disableSparseFiles>
        <disableTempIndexes>false</disableTempIndexes>
    </folder>

One or more ``folder`` elements must be present in the file. Each element
describes one folder. The following attributes may be set on the ``folder``
element:

id
    The folder ID, must be unique. (mandatory)

label
    The label of a folder is a human readable and descriptive local name. May
    be different on each device, empty, and/or identical to other folder
    labels. (optional)

path
    The path to the directory where the folder is stored on this
    device; not sent to other devices. (mandatory)

type
    Controls how the folder is handled by Syncthing. Possible values are:

    readwrite
        The folder is in default mode. Sending local and accepting remote changes.

    readonly
        The folder is in "master" mode -- it will not be modified by
        syncthing on this device.

rescanIntervalS
    The rescan interval, in seconds. Can be set to zero to disable when external
    plugins are used to trigger rescans.

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

minDiskFreePct
    The percentage of space that should be available on the disk this folder
    resides. The folder will be stopped when the percentage of free space goes
    below the threshold. Set to zero to disable.

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

ignoreDelete
    When set to true, this device will pretend not to see instructions to
    delete files from other devices.

scanProgressIntervalS
    The interval with which scan progress information is sent to the GUI. Zero
    means the default value (two seconds).

pullerSleepS, pullerPauseS
    Tweaks for rate limiting the puller. Don't change these unless you know
    what you're doing.

maxConflicts
    The maximum number of conflict copies to keep around for any given file.
    The default, -1, means an unlimited number. Setting this to zero disables
    conflict copies altogether.

disableSparseFiles
    By default, blocks containing all zeroes are not written, causing files
    to be sparse on filesystems that support the concept. When set to true,
    sparse files will not be created.

disableTempIndexes
    By default, devices exchange information about blocks available in
    transfers that are still in progress. When set to true, such information
    is not exchanged for this folder.


Device Element
--------------

.. code-block:: xml

    <device id="5SYI2FS-LW6YAXI-JJDYETS-NDBBPIO-256MWBO-XDPXWVG-24QPUM4-PDW4UQU" name="syno" compression="metadata" introducer="false">
        <address>dynamic</address>
    </device>
    <device id="2CYF2WQ-AKZO2QZ-JAKWLYD-AGHMQUM-BGXUOIS-GYILW34-HJG3DUK-LRRYQAR" name="syno local" compression="metadata" introducer="false">
        <address>tcp://192.0.2.1:22001</address>
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
contains an address or host name to use when attempting to connect to this device and will
be tried in order. Entries other than ``dynamic`` must be prefixed with ``tcp://`` (dual-stack), ``tcp4://`` (IPv4 only) or ``tcp6://` (IPv6 only). Note that IP addresses need not use tcp4/tcp6; these are optional. Accepted formats are:

IPv4 address (``tcp://192.0.2.42``)
    The default port (22000) is used.

IPv4 address and port (``tcp://192.0.2.42:12345``)
    The address and port is used as given.

IPv6 address (``tcp://[2001:db8::23:42]``)
    The default port (22000) is used. The address must be enclosed in
    square brackets.

IPv6 address and port (``tcp://[2001:db8::23:42]:12345``)
    The address and port is used as given. The address must be enclosed in
    square brackets.

Host name (``tcp6://fileserver``)
    The host name will be used on the default port (22000) and connections will be attempted only via IPv6.

Host name and port (``tcp://fileserver:12345``)
    The host name will be used on the given port and connections will be attempted via both IPv4 and IPv6, depending on name resolution.

``dynamic``
    The word ``dynamic`` (without ``tcp://`` prefix) means to use local and global discovery to find the
    device.

IgnoredDevice Element
---------------------

.. code-block:: xml

    <ignoredDevice>5SYI2FS-LW6YAXI-JJDYETS-NDBBPIO-256MWBO-XDPXWVG-24QPUM4-PDW4UQU</ignoredDevice>

This optional element lists device IDs that have been specifically ignored. One element must be present for each device ID. Connection attempts from these devices are logged to the console but never displayed in the web GUI.

GUI Element
-----------

.. code-block:: xml

    <gui enabled="true" tls="false">
        <address>127.0.0.1:8384</address>
        <apikey>l7jSbCqPD95JYZ0g8vi4ZLAMg3ulnN1b</apikey>
        <theme>default</theme>
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

theme
    The name of the theme to use.

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
        interfaces via both IPv4 and IPv6.

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
        <listenAddress>default</listenAddress>
        <globalAnnounceServer>default</globalAnnounceServer>
        <globalAnnounceEnabled>true</globalAnnounceEnabled>
        <localAnnounceEnabled>true</localAnnounceEnabled>
        <localAnnouncePort>21027</localAnnouncePort>
        <localAnnounceMCAddr>[ff12::8384]:21027</localAnnounceMCAddr>
        <maxSendKbps>0</maxSendKbps>
        <maxRecvKbps>0</maxRecvKbps>
        <reconnectionIntervalS>60</reconnectionIntervalS>
        <relaysEnabled>true</relaysEnabled>
        <relayReconnectIntervalM>10</relayReconnectIntervalM>
        <startBrowser>true</startBrowser>
        <natEnabled>true</natEnabled>
        <natLeaseMinutes>60</natLeaseMinutes>
        <natRenewalMinutes>30</natRenewalMinutes>
        <natTimeoutSeconds>10</natTimeoutSeconds>
        <urAccepted>0</urAccepted>
        <urUniqueID></urUniqueID>
        <urURL>https://data.syncthing.net/newdata</urURL>
        <urPostInsecurely>false</urPostInsecurely>
        <urInitialDelayS>1800</urInitialDelayS>
        <restartOnWakeup>true</restartOnWakeup>
        <autoUpgradeIntervalH>12</autoUpgradeIntervalH>
        <keepTemporariesH>24</keepTemporariesH>
        <cacheIgnoredFiles>false</cacheIgnoredFiles>
        <progressUpdateIntervalS>5</progressUpdateIntervalS>
        <symlinksEnabled>true</symlinksEnabled>
        <limitBandwidthInLan>false</limitBandwidthInLan>
        <minHomeDiskFreePct>1</minHomeDiskFreePct>
        <releasesURL>https://api.github.com/repos/syncthing/syncthing/releases?per_page=30</releasesURL>
        <overwriteRemoteDeviceNamesOnConnect>false</overwriteRemoteDeviceNamesOnConnect>
        <tempIndexMinBlocks>10</tempIndexMinBlocks>
    </options>

The ``options`` element contains all other global configuration options.

listenAddress
    The listen address for incoming sync connections. See
    `Listen Addresses`_ for allowed syntax.

globalAnnounceServer
    A URI to a global announce (discovery) server, or the word ``default`` to
    include the default servers. Any number of globalAnnounceServer elements
    may be present. The syntax for non-default entries is that of a HTTP or
    HTTPS URL. A number of options may be added as query options to the URL:
    ``insecure`` to prevent certificate validation (required for HTTP URLs)
    and ``id=<device ID>`` to perform certificate pinning. The device ID to
    use is printed by the discovery server on startup.

globalAnnounceEnabled
    Whether to announce this device to the global announce (discovery) server,
    and also use it to look up other devices.

localAnnounceEnabled
    Whether to send announcements to the local LAN, also use such
    announcements to find other devices.

localAnnouncePort
    The port on which to listen and send IPv4 broadcast announcements to.

localAnnounceMCAddr
    The group address and port to join and send IPv6 multicast announcements on.

relayServer
    Lists one or more relay servers, on the format ``relay://hostname:port``.
    Alternatively, a relay list can be loaded over https by using an URL like
    ``dynamic+https://somehost/path``. The default loads the list of relays
    from the relay pool server, ``relays.syncthing.net``.

maxSendKbps
    Outgoing data rate limit, in kibibytes per second.

maxRecvKbps
    Incoming data rate limits, in kibibytes per second.

reconnectionIntervalS
    The number of seconds to wait between each attempt to connect to currently
    unconnected devices.

relaysEnabled
    When true, relays will be connected to and potentially used for device to device connections.

relayReconnectIntervalM
    Sets the interval, in minutes, between relay reconnect attempts.

startBrowser
    Whether to attempt to start a browser to show the GUI when Syncthing starts.

natEnabled
    Whether to attempt to perform an UPnP and NAT-PMP port mapping for
    incoming sync connections.

natLeaseMinutes
    Request a lease for this many minutes; zero to request a permanent lease.

natRenewalMinutes
    Attempt to renew the lease after this many minutes.

natTimeoutSeconds
    When scanning for UPnP devices, wait this long for responses.

urAccepted
    Whether the user has accepted to submit anonymous usage data. The default,
    ``0``, mean the user has not made a choice, and Syncthing will ask at some
    point in the future. ``-1`` means no, a number above zero means that that
    version of usage reporting has been accepted.

urUniqueID
    The unique ID sent together with the usage report. Generated when usage
    reporting is enabled.

urURL
    The URL to post usage report data to, when enabled.

urPostInsecurely
    When true, the UR URL can be http instead of https, or have a self-signed
    certificate. The default is ``false``.

urInitialDelayS
    The time to wait from startup to the first usage report being sent. Allows
    the system to stabilize before reporting statistics.

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
    Whether to cache the results of ignore pattern evaluation. Performance
    at the price of memory. Defaults to ``false`` as the cost for evaluating
    ignores is usually not significant.

progressUpdateIntervalS
    How often in seconds the progress of ongoing downloads is made available to
    the GUI.

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
    slow response time (slow connection/cpu) and large index exchanges.

pingIdleTimeS
    Ping interval in seconds. Don't change it unless you feel it's necessary.

minHomeDiskFreePct
    The percentage of space that should be available on the partition holding
    the configuration and index.

releasesURL
    The URL from which release information is loaded, for automatic upgrades.

overwriteRemoteDeviceNamesOnConnect
    If set, device names will always be overwritten with the name given by
    remote on each connection. By default, the name that the remote device
    announces will only be adopted when a name has not already been set.

tempIndexMinBlocks
    When exchanging index information for incomplete transfers, only take
    into account files that have at least this many blocks.

Listen Addresses
^^^^^^^^^^^^^^^^

The following address types are accepted in sync protocol listen addresses:

TCP wildcard and port (``tcp://0.0.0.0:22000``, ``tcp://:22000``)
    These are equivalent and will result in Syncthing listening on all
    interfaces, IPv4 and IPv6, on the specified port.

TCP IPv4 wildcard and port (``tcp4://0.0.0.0:22000``, ``tcp4://:22000``)
    These are equivalent and will result in Syncthing listening on all
    interfaces via IPv4 only.

TCP IPv4 address and port (``tcp4://192.0.2.1:22000``)
    These are equivalent and will result in Syncthing listening on the
    specified address and port only.

TCP IPv6 wildcard and port (``tcp6://[::]:22000``, ``tcp6://:22000``)
    These are equivalent and will result in Syncthing listening on all
    interfaces via IPv6 only.

TCP IPv6 address and port (``tcp6://[2001:db8::42]:22000``)
    These are equivalent and will result in Syncthing listening on the
    specified address and port only.

Static relay address (``relay://192.0.2.42:22067?id=abcd123...``)
    Syncthing will connect to and listen for incoming connections via the
    specified relay address.

    .. todo:: Document available URL parameters.

Dynamic relay pool (``dynamic+https://192.0.2.42/relays``)
    Syncthing will fetch the specified HTTPS URL, parse it for a JSON payload
    describing relays, select a relay from the available ones and listen via
    that as if specified as a static relay above.

    .. todo:: Document available URL parameters.


Syncing Configuration files
---------------------------

Syncing configuration files between devices (such that multiple devices are
using the same configuration files) can cause issues. This is easy to do
accidentally if you sync your home folder between devices. A common symptom
of syncing configuration files is two devices ending up with the same Device ID.

If you want to use Syncthing to backup your configuration files, it is recommended
that the files you are backing up are in a :ref:`folder-master` to prevent other
devices from overwriting the per device configuration. The folder on the remote
device(s) should not be used as configuration for the remote devices.

If you'd like to sync your home folder in non-master mode, you may add the
folder that stores the configuration files to the :ref:`ignore list <ignoring-files>`.
If you'd also like to backup your configuration files, add another folder in
master mode for just the configuration folder.

