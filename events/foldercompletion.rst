FolderCompletion
----------------

The ``FolderCompletion`` event is emitted when the local or remote
contents for a folder changes. It contains the completion percentage for
a given remote device and is emitted once per currently connected remote
device.

.. code-block:: json

    {
        "id": 84,
        "type": "FolderCompletion",
        "time": "2015-04-17T14:14:27.043576583+09:00",
        "data": {
            "completion": 100,
            "device": "I6KAH76-66SLLLB-5PFXSOA-UFJCDZC-YAOMLEK-CP2GB32-BV5RQST-3PSROAU",
            "folder": "default"
        }
    }
