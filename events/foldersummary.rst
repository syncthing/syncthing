FolderSummary
-------------

The FolderSummary event is emitted when folder contents have changed
locally. This can be used to calculate the current local completion
state.

.. code-block:: json

    {
        "id": 16,
        "type": "FolderSummary",
        "time": "2015-04-17T14:12:20.460121585+09:00",
        "data": {
            "folder": "default",
            "summary": {
                "globalBytes": 0,
                "globalDeleted": 0,
                "globalFiles": 0,
                "ignorePatterns": false,
                "inSyncBytes": 0,
                "inSyncFiles": 0,
                "invalid": "",
                "localBytes": 0,
                "localDeleted": 0,
                "localFiles": 0,
                "needBytes": 0,
                "needFiles": 0,
                "state": "idle",
                "stateChanged": "2015-04-17T14:12:12.455224687+09:00",
                "version": 0
            }
        }
    }
