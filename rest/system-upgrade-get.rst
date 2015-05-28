GET /rest/system/upgrade
========================

Checks for a possible upgrade and returns an object describing the
newest version and upgrade possibility.

.. code:: json

    {
      "latest": "v0.10.27",
      "newer": false,
      "running": "v0.10.27+5-g36c93b7"
    }
