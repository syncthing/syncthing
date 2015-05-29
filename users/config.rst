###################
Configuration Files
###################

.. warning::
  This page may be outdated and requires review.
  Attributes have been added that are not documented.

Syncthing uses a single directory to store configuration, crypto keys
and index caches. The location defaults to ``~/.config/syncthing``
(Unixes), ``%AppData%/Syncthing`` (Windows XP),
``%localappdata%/Syncthing`` (Windows 7/8) or
``~/Library/Application Support/Syncthing`` (Mac) but is also settable
at runtime using the ``-home`` flag. In this directory you'll generally
see something like the following files:

-  ``cert.pem``: The device's RSA public key, named "cert" for legacy
   reasons.
-  ``key.pem``: The device's RSA private key. This needs to be
   protected.
-  ``config.xml``: The configuration file, in XML format.
-  ``https-cert.pem`` and ``https-key.pem``: The certificate and key for
   HTTPS GUI connections.
-  ``index/``: A directory holding the database with metadata and hashes
   of the files currently on disk and available from peers.
-  ``csrftokens.txt``: A list of recently issued CSRF tokens (for
   protection against browser cross site request forgery).

Config File Format
==================

The following is an example default configuration file:

.. code:: xml

    <configuration version="2">
        <folder id="default" directory="/Users/jb/Sync" ro="false" ignorePerms="false">
            <device id="GXN5ECCWTA2B7EB5FXYL5OWGOADX5EF5VNJAQSIBAY6XHJ24BNOA"></device>
        </folder>
        <device id="GXN5ECCWTA2B7EB5FXYL5OWGOADX5EF5VNJAQSIBAY6XHJ24BNOA" name="jborg-mbp">
            <address>dynamic</address>
        </device>
        <gui enabled="true" tls="true">
            <address>127.0.0.1:54096</address>
            <user>jb</user>
            <password>$2a$10$EKaTIcpz2...</password>
            <apikey>O80CDOJ9LVUVCMHFK2OJDO4T882735</apikey>
        </gui>
        <options>
            <listenAddress>:54097</listenAddress>
            <globalAnnounceServer>announce.syncthing.net:22025</globalAnnounceServer>
            <globalAnnounceEnabled>true</globalAnnounceEnabled>
            <localAnnounceEnabled>true</localAnnounceEnabled>
            <parallelRequests>16</parallelRequests>
            <maxSendKbps>0</maxSendKbps>
            <rescanIntervalS>60</rescanIntervalS>
            <reconnectionIntervalS>60</reconnectionIntervalS>
            <maxChangeKbps>10000</maxChangeKbps>
            <startBrowser>true</startBrowser>
            <upnpEnabled>true</upnpEnabled>
            <urAccepted>0</urAccepted>
        </options>
    </configuration>

The following sections should be present in a well formed config file.

configuration
-------------

The root element.

Attributes
~~~~~~~~~~

-  ``version``: The config version. ``2`` is current.

folder
------

One or more ``folder`` elements must be present in the file. Each
element describes one folder.

Within the ``folder`` element one or more ``device`` element should be
present. These must contain the ``id`` attribute and nothing else.
Mentioned devices are those that will be sharing the folder in question.
Each mentioned device must have a separate ``device`` element later in
the file. It is customary that the local device ID is included in all
repositories. Syncthing will currently add this automatically if it is
not present in the configuration file.

Attributes
~~~~~~~~~~

-  ``id``: The folder ID, must be unique. (mandatory)
-  ``directory``: The directory where the folder is stored on this
   device; not sent to other devices. (mandatory)
-  ``ro``: True if the folder is read only (will not be modified by
   Syncthing) on this device. (optional, defaults to ``false``)
-  ``ignorePerms``: True if the folder should `ignore
   permissions <http://forum.syncthing.net/t/v0-8-10-ignore-permissions/263>`__.

Children
~~~~~~~~

-  One or mode ``device`` elements.

device
------

One or more ``device`` elements must be present in the file. Each
element describes a device participating in the cluster. It is customary
to include a ``device`` element for the local device; Syncthing will
currently add one if it is not present.

Attributes
~~~~~~~~~~

-  ``id``: The device ID. This must be written in canonical form, that
   is without any spaces or dashes. (mandatory)
-  ``name``: A friendly name for the device. (optional)

Children
~~~~~~~~

-  One or mode ``address`` elements.

address
-------

The address section is only valid inside of ``device`` elements. It
contains a single address, on one of the following forms:

-  IPv4 addresses, IPv6 addresses within brackets, or DNS names, all
   optionally followed by a port number.
-  ``dynamic``: The address will be resolved using discovery.

gui
---

There must be exactly one ``gui`` element.

Attributes
~~~~~~~~~~

-  ``enabled``: ``true``/``false``
-  ``tls``: ``true``/``false`` - if true then the GUI will use HTTPS

Children
~~~~~~~~

-  ``address``: One or more address elements must be present, containing
   an ``ip:port`` listen address.
-  ``username`` and ``password``: Set to require authentication.
   ``password`` contains the bcrypt hash of the real password.
-  ``apikey``: If set, this is the API key that enables usage of the REST interface.

options
-------

There must be exactly one ``options`` element. It contains the following
configuration settings as children;

-  ``listenAddress`` : ``host:port`` or ``:port`` string denoting an
   address to listen for BEP (sync protocol) connections. More than one
   ``listenAddress`` may be given. (default ``0.0.0.0:22000``)

-  ``globalAnnounceServer`` : ``host:port`` where a global announce
   server may be reached. (default \`announce.syncthing.net:22025´)

-  ``globalAnnounceEnabled``: ``true``/``false`` (default ``true``)

-  ``localAnnounceEnabled``: ``true``/``false`` (default ``true``)

-  ``parallelRequests``: The maximum number of outstanding block
   requests to have against any given peer. (default ``16``)

-  ``maxSendKbps``: Rate limit

-  ``rescanIntervalS``: The number of seconds to wait between each scan
   for modification of the local repositories. (default ``60``) A value
   of 0 disables the scanner.

-  ``reconnectionIntervalS``: The number of seconds to wait between each
   attempt to connect to currently unconnected devices. (default ``60``)

-  ``maxChangeKbps``: The maximum rate of change allowed for a single
   file. When this rate is exceeded, further changes to the file are not
   announced, until the rate is reduced below the limit. (default
   ``10000``)

-  ``startBrowser``: ``true``/``false`` (default ``true``)

-  ``upnpEnabled``: ``true``/``false`` (default ``true``)

-  ``urAccepted``: Whether the user as accepted to submit anonymous
   usage data. The default, ``0``, mean the user has not made a choice,
   and Syncthing will ask at some point in the future. ``-1`` means no,
   ``1`` means yes.
