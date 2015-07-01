FolderErrors
------------

The ``FolderErrors`` event is emitted when a folder cannot be successfully
synchronized. The event contains the ID of the affected folder and a list of
errors for files or directories therein. This list of errors is obsolete once
the folder changes state to ``syncing`` - if errors remain after the next
synchronization attempt, a new ``FolderErrors`` event is emitted.

.. code-block:: json

        {
            "id": 132,
            "type": "FolderErrors",
            "time": "2015-06-26T13:39:24.697401384+02:00",
            "data": {
                "errors": [
                    {
                        "error": "open /Users/jb/src/github.com/syncthing/syncthing/test/s2/h2j/.syncthing.aslkjd.tmp: permission denied",
                        "path": "h2j/aslkjd"
                    }
                ],
                "folder": "default"
            }
        }

.. versionadded:: 0.11.12

.. seealso:: The :ref:`statechanged` event.
