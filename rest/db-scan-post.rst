POST /rest/db/scan
==================

Request immediate rescan of a folder, or a specific path within a folder.
Takes the mandatory parameter `folder` (folder ID), an optional parameter
``sub`` (path relative to the folder root) and an optional parameter ``next``. If
``sub`` is omitted or empty, the entire folder is scanned for changes, otherwise
only the given path (and children, in case it's a directory) is scanned. The
``next`` argument delays Syncthing's automated rescan interval for a given
amount of seconds.

Requesting scan of a path that no longer exists, but previously did, is
valid and will result in Syncthing noticing the deletion of the path in
question.

Returns status 200 and no content upon success, or status 500 and a
plain text error if an error occurred during scanning.

.. code-block:: bash

    curl -X POST http://127.0.0.1:8384/rest/db/scan?folder=default&sub=foo/bar
