ItemFinished
------------

Generated when Syncthing ends synchronizing a file to a newer version. A
successful operation:

.. code-block:: json

    {
        "id": 93,
        "type": "ItemFinished",
        "time": "2014-07-13T21:22:03.414609034+02:00",
        "data": {
            "item": "test.txt",
            "folder": "default",
            "error": null,
            "type": "file",
            "action": "update"
        }
    }

An unsuccessful operation:

.. code-block:: json

    {
        "id": 44,
        "type": "ItemFinished",
        "time": "2015-05-27T11:21:05.711133004+02:00",
        "data": {
            "action": "update",
            "error": "open /Users/jb/src/github.com/syncthing/syncthing/test/s2/foo/.syncthing.hej.tmp: permission denied",
            "folder": "default",
            "item": "foo/hej",
            "type": "file"
        }
    }

The ``action`` field is either ``update`` (contents changed), ``metadata`` (file metadata changed but not contents), or ``delete``.

.. versionadded:: 0.11.10
    The ``metadata`` action.
