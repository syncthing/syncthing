POST /rest/system/reset
=======================

Post with empty body to erase the current index database and restart
Syncthing. With no query parameters, the entire database is erased from disk.
By specifying the ``folder`` parameter with a valid folder ID, only
information for that folder will be erased::

	$ curl -X POST -H "X-API-Key: abc123" http://localhost:8384/rest/system/reset?folder=default
