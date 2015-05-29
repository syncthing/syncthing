GET /rest/stats/device
======================

Returns general statistics about devices. Currently, only contains the
time the device was last seen.

.. code-block:: bash

    $ curl -s http://localhost:8384/rest/stats/device | json
    {
      "P56IOI7-MZJNU2Y-IQGDREY-DM2MGTI-MGL3BXN-PQ6W5BM-TBBZ4TJ-XZWICQ2": {
        "lastSeen" : "2015-04-18T11:21:31.3256277+01:00"
      }
    }
