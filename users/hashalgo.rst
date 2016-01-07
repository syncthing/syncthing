.. _hashalgo:

Hash Algorithms
===============

.. versionadded:: 0.13.0

Description
-----------

A folder can use one of several available hash algorithms when determining
changes to files. The algorithms have different properties -- see the table
below where the differences between the algorithms are highlighted.

=========  ========  =============  =================  ================
Algorithm  Speed     Delta Changes  Efficient Renames  Efficient Copies
=========  ========  =============  =================  ================
SHA256     Slow      Yes            Yes                **Yes**
Murmur3    **Fast**  Yes            Yes                No
=========  ========  =============  =================  ================

Speed:
	This affects the speed with which content can be hashed. A "slow"
	algorithm requires a lot of CPU power, while a "fast" algorithm does not.
	The actual scanning speed may however be limited by the disk's transfer
	rate.

Delta Changes:
	Indicates whether changes to the contents of a file are transferred
	efficiently. "Yes" means that only the changed blocks are transferred,
	"no" means the whole file must be resent.

Efficient Renames:
	Indicates whether renaming a file on one device results in a renaming
	operation on another device ("yes"), or whether the renamed file must be
	transferred over the network ("no").

Efficient Copies:
	Indicates whether a copy of a file on one device results in a local copy
	operation on another device ("yes"), or whether the copy must be
	transferred over the network ("no").

Changing the Folder Hash Algorithm
----------------------------------

The hash algorithm can only be set when a folder is added to the
configuration, it can not be changed afterwards. To change the hash algorithm
of a folder it must be removed and then added again with the new hash
algorithm. This necessarily results in a full (slow) rescan of the folder
contents.

The hash algorithm for a given folder must be the same for all devices sharing
that folder.
