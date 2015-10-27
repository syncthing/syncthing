GET /rest/system/debug
======================

.. versionadded:: 0.12.0

Returns the set of debug facilities and which of them are currently enabled.

.. code-block:: json

    {
      "enabled": [
        "beacon"
      ],
      "facilities": {
        "beacon": "Multicast and broadcast discovery",
        "config": "Configuration loading and saving",
        "connections": "Connection handling",
        "db": "The database layer",
        "dialer": "Dialing connections",
        "discover": "Remote device discovery",
        "events": "Event generation and logging",
        "http": "REST API",
        "main": "Main package",
        "model": "The root hub",
        "protocol": "The BEP protocol",
        "relay": "Relay connection handling",
        "scanner": "File change detection and hashing",
        "stats": "Persistent device and folder statistics",
        "sync": "Mutexes",
        "upgrade": "Binary upgrades",
        "upnp": "UPnP discovery and port mapping",
        "versioner": "File versioning"
      }
    }
