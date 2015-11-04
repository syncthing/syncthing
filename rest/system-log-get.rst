GET /rest/system/log
====================

.. versionadded:: 0.12.0

Returns the list of recent log entries.

.. code-block:: json

    {
      "messages": [
        {
          "when": "2014-09-18T12:59:26.549953186+02:00",
          "message": "This is a log entry"
        }
      ]
    }
