File Versioning
===============

.. todo::
    External versioning requires documenting.

Description
-----------

There are 3 types of File Versioning. When you select each in the web interface,
a short description of each is shown to help you decide.

No File Versioning
------------------

This is the default setting. With no file versioning, files that are replaced or
deleted on one device are deleted on other devices that the directory is shared
with. (Note: If a folder is marked "Master Folder" on a device, that device will
not accept changes to the files in the folder, and therefore will not have files
replaced or deleted.)

Simple File Versioning
----------------------

With "Simple File Versioning" files are moved to the ".stversions" folder
(inside your shared folder) when replaced or deleted on a remote device. This
option also takes a value in an input titled "Keep Versions" which tells
Syncthing how many old versions of the file it should keep. For example, if
you set this value to 5, if a file is replaced 5 times on a remote device, you
will see 5 time-stamped versions on that file in the ".stversions" folder on
the other devices sharing the same folder.

Staggered File Versioning
-------------------------

With "Staggered File Versioning" files are also moved to a different folder
when replaced or deleted on a remote device (just like "Simple File
Versioning"), however, versions are automatically deleted if they are older
than the maximum age or exceed the number of files allowed in an interval.

With this versioning method it's possible to specify where the versions are
stored, with the default being the `.stversions` folder inside the normal
folder path. If you set a custom version path, please ensure that it's on the
same partition or filesystem as the regular folder path, as moving files there
may otherwise fail. You can use an absolute path (this is recommended) or a
relative path. Relative paths are interpreted relative to Syncthing's current
or startup directory.

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
