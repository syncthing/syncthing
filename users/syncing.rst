.. _syncing:

Understanding Synchronization
=============================

This article describes the mechanisms Syncthing uses to bring files in sync
on a high level.

Blocks
------

Files are divided into *blocks*. The blocks are currently fixed size, 128
KiB, except the last one in the file which may be smaller. Each file is
sliced into a number of these blocks, and the SHA256 hash of each block is
computed. This results in a *block list* containing the offset, size and
hash of all blocks in the file.

To update a file, Syncthing compares the block list of the current version
of the file to the block list of the desired version of the file. It then
tries to find a source for each block that differs. This might be locally,
if another file already has a block with the same hash, or it may be from
another device in the cluster. In the first case the block is simply copied
on disk, in the second case it is requested over the network from the other
device.

When a block is copied or received from another device, its SHA256 hash is
computed and compared with the expected value. If it matches the block is
written to a temporary copy of the file, otherwise it is discarded and
Syncthing tries to find another source for the block.

Scanning
--------

Syncthing detects changes to files by scanning. By default this happens
every 60 seconds, but this can be changed per folder. Increasing the scan
interval uses less resources and is useful for example on large folders that
changes infrequently. ``syncthing-inotify`` can also be used, which tells
Syncthing to scan changed files when changes are detected, thus reducing the
need for periodic scans.

During a rescan the existing files are checked for changes to their
modification time, size or permission bits. The file is "rehashed" if a
change is detected based on those attributes, that is a new block list is
calculated for the file. It is not possible to know which parts of a file
have changed without reading the file and computing new SHA256 hashes for
each block.

Changes that were detected and hashed are transmitted to the other devices
after each rescan.

Syncing
-------

Syncthing keeps track of several version of each file - the version that it
currently has on disk, called the *local* version, the versions announced by
all other connected devices, and the "best" (usually the most recent)
version of the file. This version is called the *global* version and is the
one that each device strives to be up to date with.

This information is kept in the *index database*, which is stored in the
configuration directory and called ``index-vx.y.z.db`` (for some version
x.y.z which may not be exactly the version of Syncthing you're running).

When new index data is received from other devices Syncthing recalculates
which version for each file should be the global version, and compares this
to the current local version. When the two differ, Syncthing needs to
synchronize the file. The block lists are compared to build a list of needed
blocks, which are then requested from the network or copied locally, as
described above.

Temporary Files
---------------

Syncthing never writes directly to a destination file. Instead all changes
are made to a temporary copy which is then moved in place over the old
version. If an error occurs during the copying or syncing, such as a
necessary block not being available, the temporary file is kept around for
up to a day. This is to avoid needlessly requesting data over the network.

The temporary files are named ``.syncthing.original-filename.ext.tmp`` or,
on Windows, ``~syncthing~original-filename.ext.tmp`` where
``original-filename.ext`` is the destination filename. The temporary file is
normally hidden. If the temporary file name would be too long due to the
addition of the prefix and extra extension, a hash of the original file name
is used instead of the actual original file name.

