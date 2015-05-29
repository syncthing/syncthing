POST /rest/system/reset
=======================

Post with empty body to immediately *reset* Syncthing. This means
renaming all folder directories to temporary, unique names, wiping all
indexes and restarting. This should probably not be used during normal
operations...
