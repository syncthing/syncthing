GET /rest/svc/deviceid
======================

Verifies and formats a device ID. Accepts all currently valid formats
(52 or 56 characters with or without separators, upper or lower case,
with trivial substitutions). Takes one parameter, ``id``, and returns
either a valid device ID in modern format, or an error.

.. code-block:: bash

    $ curl -s http://localhost:8384/rest/svc/deviceid?id=1234 | json
    {
      "error": "device ID invalid: incorrect length"
    }

    $ curl -s http://localhost:8384/rest/svc/deviceid?id=p56ioi7m--zjnu2iq-gdr-eydm-2mgtmgl3bxnpq6w5btbbz4tjxzwicq | json
    {
      "id": "P56IOI7-MZJNU2Y-IQGDREY-DM2MGTI-MGL3BXN-PQ6W5BM-TBBZ4TJ-XZWICQ2"
    }
