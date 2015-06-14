ItemStarted
-----------

Generated when Syncthing begins synchronizing a file to a newer version.

.. code-block:: json

    {
        "id": 93,
        "type": "ItemStarted",
        "time": "2014-07-13T21:22:03.414609034+02:00",
        "data": {
            "item": "test.txt",
            "folder": "default",
            "type": "file",
            "action": "update"
        }
    }

The ``action`` field is either ``update`` (contents changed), ``metadata`` (file metadata changed but not contents), or ``delete``.

.. versionadded:: 0.11.10
    The ``metadata`` action.
