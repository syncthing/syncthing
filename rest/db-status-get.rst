GET /rest/db/status
===================

Returns information about the current status of a folder.

Parameters: ``folder``, the ID of a folder.

.. code-block:: bash

    {
      # latest version according to cluster:
      "globalBytes": 13173473780,
      "globalDeleted": 1847,
      "globalFiles": 42106,
      # what we have locally:
      "localBytes": 13173473780,
      "localDeleted": 1847,
      "localFiles": 42106,
      # which part of what we have locally is the latest cluster version:
      "inSyncBytes": 13173473780,
      "inSyncFiles": 42106,
      # which part of what we have locally should be fetched from the cluster:
      "needBytes": 0,
      "needFiles": 0,
      # various other metadata
      "ignorePatterns": true,
      "invalid": "",
      "state": "idle",
      "stateChanged": "2015-03-16T21:47:28.750853241+01:00",
      "version": 71989
    }

.. note::
  This is an expensive call, increasing CPU and RAM usage on the device. Use sparingly.
