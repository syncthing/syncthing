.. _syncing:

Understanding Synchronization
=============================

Blocks
------

Files are divided into *blocks*. The blocks are currently fixed size, 128 KiB,
except the last one in the file which may be smaller. Each file is sliced into
a number of these blocks, and the SHA256 hash of each block is computed. This
results in a *block list* containing the offset, size and hash of all blocks
in the file.

To update a file, Syncthing compares the block list of the current version of
the file to the block list of the desired version of the file. It then tries
to find a source for each block that differs. This might be locally, if
another file already has a block with the same hash, or it may be from another
device in the cluster. In the first case the block is simply copied on disk,
in the second case it is requested over the network from the other device.

When a block is copied or recieved from another device, its SHA256 hash is
computed and compared with the expected value. If it matches the block is
written to a temporary copy of the file, otherwise it is discarded and
Syncthing tries to find another source for the block.

Scanning
--------

Syncthing detects changes to files by scanning. By default this happens every
60 seconds, but this can be changed per folder. Increasing the scan interval
uses less resources and is useful for example on large folders that changes
infrequently. Syncthing-inotify can also be used, which tells Syncthing to
scan changed files when changes are detected, thus reducing the need for
periodic rescans.

During a rescan the existing files are checked for changes to their
modification time or size. The file is "rehashed" if a change is detected
based on those attributes, that is a new block list is calculated for the
file. It is not possible to know which parts of a file changed without reading
the file and computing new SHA256 hashes for each block.

Changes that were detected and hashed are transmitted to the other devices
after a rescan.

Temporary Files
---------------

Syncthing never writes directly to a file. Instead all changes are made to a
temporary copy which is then moved in place over the old version. If an error
occurrs during the copying or syncing, such as a necessary block not being
available, the temporary file is kept around for up to a day. This is to avoid
needlessly requesting data over the network.
