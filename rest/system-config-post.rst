POST /rest/system/config
========================

Post the full contents of the configuration, in the same format as returned by
the corresponding GET request. The configuration will be saved to disk and the
``configInSync`` flag set to false. Restart Syncthing to activate.
