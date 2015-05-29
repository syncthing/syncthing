ConfigSaved
-----------

Emitted after the config has been saved by the user or by Syncthing
itself.

.. code-block:: json

    {
        "id": 50,
        "type": "ConfigSaved",
        "time": "2014-12-13T00:09:13.5166486Z",
        "data":{
            "Version": 7,
            "Options": { ... },
            "GUI": { ... },
            "Devices": [ ... ],
            "Folders": [ ... ]
        }
    }
