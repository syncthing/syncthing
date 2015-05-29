GET /rest/system/status
=======================

Returns information about current system status and resource usage.

.. code-block:: json

    {
      "alloc": 30618136,
      "cpuPercent": 0.006944836512046966,
      "extAnnounceOK": {
        "udp4://announce.syncthing.net:22026": true,
        "udp6://announce-v6.syncthing.net:22026": true
      },
      "goroutines": 49,
      "myID": "P56IOI7-MZJNU2Y-IQGDREY-DM2MGTI-MGL3BXN-PQ6W5BM-TBBZ4TJ-XZWICQ2",
      "pathSeparator": "/",
      "sys": 42092792,
      "tilde": "/Users/jb"
    }
