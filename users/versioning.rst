File Versioning
===============

.. warning::
    This page may be out of date and requires review.

.. todo::
    External versioning requires documenting.

Description
-----------

There are 3 types of File Versioning. When you select each in the web interface,
a short description of each is shown to help you decide.

.. todo::
    More detail needed here: Can this be a relative path, or must it be
    an absolute path?

With "Staggered File Versioning" method (only), you would like to specify where
removed and deleted files are stored as part of the Versioning feature, you can
specify the path in the "Versions Path" input after this method is selected.

No File Versioning
------------------

This is the default setting. With no file versioning, files that are replaced or
deleted on one device are deleted on other devices that the directory is shared
with. (Note: If a folder is marked "Master Folder" on a device, that device will
not accept changes to the files in the folder, and therefore will not have files
replaced or deleted.)

Simple File Versioning
----------------------

With "Simple File Versioning" files are moved to the ".stversions"
folder (inside your shared folder) when replaced or deleted on a remote
device. This option also takes a value in an input titled "Keep
Versions" which tells Syncthing how many old versions of the file it
should keep. For example, if you set this value to 5, if a file is
replaced 5 times on a remote device, you will see 5 time-stamped
versions on that file in the ".stversions" folder on the other devices
sharing the same folder.
With "Simple File Versioning" files are moved to the ".stversions" folder
(inside your shared folder) when replaced or deleted on a remote device. This
option also takes a value in an input titled "Keep Versions" which tells
Syncthing how many old versions of the file it should keep. For example, if you
set this value to 5, if a file is replaced 5 times on a remote device, you will
see 5 time-stamped versions on that file in the ".stversions" folder on the
other devices sharing the same folder.

Staggered File Versioning
-------------------------

With "Staggered File Versioning" files are also moved to the ".stversions"
folder (inside your shared folder) when replaced or deleted on a remote device
(just like "Simple File Versioning"), however, Version are automatically deleted
if they are older than the maximum age or exceed the number of files allowed in
an interval.

The following intervals are used and they each have a maximum number of files
that will be kept for each.

1 Hour
    For the first hour, the most recent version is kept every 30 seconds.
1 Day
    For the first day, the most recent version is kept every hour.
30 Days
    For the first 30 days, the most recent version is kept every day.
Until Maximum Age
    The maximum time to keep a version in days. For example, to keep replaced or
    deleted files in the ".stversions" folder for an entire year, use 365. If
    only or 10 days, use 10. **Note: Set to 0 to keep versions forever.**
