GET /rest/stats/folder
======================

Returns general statistics about folders. Currently contains the
last scan time and the last synced file.

.. code-block:: bash

    $ curl -s http://localhost:8384/rest/stats/folder | json
    {
      "folderid" : {
        "lastScan": "2016-06-02T13:28:01.288181412-04:00",
        "lastFile" : {
          "filename" : "file/name",
            "at" : "2015-04-16T22:04:18.3066971+01:00"
          }
      }
    }
