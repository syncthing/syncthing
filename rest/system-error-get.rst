GET /rest/system/error
======================

.. note:: Return format changed in 0.12.0.

Returns the list of recent errors.

.. code-block:: json

    {
      "errors": [
        {
          "when": "2014-09-18T12:59:26.549953186+02:00",
          "message": "This is an error string"
        }
      ]
    }
