GET /rest/db/need
=================

Takes one parameter, ``folder``, and returns lists of files which are
needed by this device in order for it to become in sync.

.. code-block:: bash

    {
      # Files currently being downloaded
      "progress": [
        {
          "flags": "0755",
          "localVersion": 6,
          "modified": "2015-04-20T23:06:12+09:00",
          "name": "ls",
          "size": 34640,
          "version": [
            "5157751870738175669:1"
          ]
        }
      ],
      # Files queued to be downloaded next (as per array order)
      "queued": [
          ...
      ],
      # Files to be downloaded after all queued files will be downloaded.
      # This happens when we start downloading files, and new files get added while we are downloading.
      "rest": [
          ...
      ]
    }
