GET /rest/system/connections
============================

Returns the list of current connections and some metadata associated
with the connection/peer.

.. code-block:: json

    {
      "connections": {
        "SMAHWLH-AP74FAB-QWLDYGV-Q65ASPL-GAAR2TB-KEF5FLB-DRLZCPN-DJBFZAG": {
          "address": "172.21.20.78:22000",
          "at": "2015-03-16T21:51:38.672758819+01:00",
          "clientVersion": "v0.10.27",
          "inBytesTotal": 415980,
          "outBytesTotal": 396300
        }
      },
      "total": {
        "address": "",
        "at": "2015-03-16T21:51:38.672868814+01:00",
        "clientVersion": "",
        "inBytesTotal": 415980,
        "outBytesTotal": 396300
      }
    }
