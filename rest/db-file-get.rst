GET /rest/db/file
=================

Returns most data available about a given file, including version and
availability.

.. code-block:: json

    {
      "availability": [
        "I6KAH76-66SLLLB-5PFXSOA-UFJCDZC-YAOMLEK-CP2GB32-BV5RQST-3PSROAU"
      ],
      "global": {
        "flags": "0644",
        "localVersion": 3,
        "modified": "2015-04-20T22:20:45+09:00",
        "name": "util.go",
        "numBlocks": 1,
        "size": 9642,
        "version": [
          "5407294127585413568:1"
        ]
      },
      "local": {
        "flags": "0644",
        "localVersion": 4,
        "modified": "2015-04-20T22:20:45+09:00",
        "name": "util.go",
        "numBlocks": 1,
        "size": 9642,
        "version": [
          "5407294127585413568:1"
        ]
      }
    }
