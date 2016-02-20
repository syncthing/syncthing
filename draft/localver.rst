.. _localver:

Understanding the Local Version Counter
=======================================

Description
-----------

Changes to files are tracked by so called *version vectors*, which can be used
to understand a file's provenance in terms of who changed it after receiving a
copy from whom, etc. However, for other purposes we just need to know which
files have changed since a certain point in time. For example this is used to
determine which files to send information about when connecting to a device
that we have already exchanged some index information with at an earlier time.
For this purpose we have the *local version counter*.

Conceptually it is an ever increasing integer kept for each folder by each
device. Each time a change is made to the index information of an item (file
or directory), that item is tagged with the current value of the local version
counter, and the counter is increased.

Principal of Operation
----------------------

Assume that the starting value of the counter is ``0``. The next change that
happens will be assigned local version ``1``. Local scanning reveals the
presence of three new files, ``foo``, ``bar`` and ``baz``. When ``foo`` is
discovered, it's index entry gets assigned local version ``1`` and the counter
is increased. Likewise ``bar`` gets ``2`` and ``baz`` gets ``3``.

====  =============
File  Local Version
====  =============
foo   1
bar   2
baz   3
====  =============

The next change to happen will be assigned the value ``4``. If the file
``bar`` is updated, the index will look like:

====  =============
File  Local Version
====  =============
foo   1
bar   4
baz   3
====  =============

If we had already given the first index above to another device, and they tell
us that they see a maximum local version value of ``3``, then we know that we
only need to send them information about entries with local version number
``4`` or higher -- in this case the entry for ``bar``.

Implementation Details
----------------------

In actual operation, the local version is assigned from the current timestamp
in Unix epoch nanoseconds, potentially incremented in case of multiple changes
happening during the same clock nanosecond. This guarantees that local version
numbers are monotonically increasing even in the case of losing the local
index database, and allows for detecting that case.

During the initial protocol handshake (the "Cluster Configuration" message)
each device sends the maximum known local version number (per folder) to the
other device. That device can then send index information based on the
difference between the received maximum local version number and the current
local version counter.

If the received maximum local version is lower than the *minimum* local
version number in our index, the other device is fully out of date with our
index information or a database reset has occurred. We send a *full index*
("Index message" followed by one or more "Index Update" messages)

If the received maximum local version number is lower than the *maximum* local
version number in our index, the other device is missing some of our index
information. We send an *incremental index* ("Index Update" messages only)
containing all index entries with higher local version numbers than the
received maximum.

If the received maximum local version number is equal to the maximum local
version number in our index, the other device is fully up to date with our
index information.
