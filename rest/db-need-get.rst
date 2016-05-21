GET /rest/db/need
=================

Takes one mandatory parameter, ``folder``, and returns lists of files which are
needed by this device in order for it to become in sync.

Furthermore takes an optional ``page`` and ``perpage`` arguments for pagination.
Pagination happens, across the union of all needed files, that is - across all
3 sections of the response.
For example, given the current need state is as follows:

1. ``progress`` has 15 items
2. ``queued`` has 3 items
3. ``rest`` has 12 items

If you issue a query with ``page=1`` and ``perpage=10``, only the ``progress``
section in the response will have 10 items. If you issue a request query with
``page=2`` and ``perpage=10``, ``progress`` section will have the last 5 items,
``queued`` section will have all 3 items, and ``rest`` section will have first
2 items. If you issue a query for ``page=3`` and ``perpage=10``, you will only
have the last 10 items of the ``rest`` section.

In all these calls, ``total`` will be 30 to indicate the total number of
available items.

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
      ],
      "page": 1,
      "perpage": 100,
      "total": 2000
    }

.. note::
  This is an expensive call, increasing CPU and RAM usage on the device. Use sparingly.
