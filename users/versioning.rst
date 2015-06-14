File Versioning
===============

.. todo::
    External versioning needs example for Windows.

Description
-----------

Syncthing supports archiving the old version of a file when it is deleted or
replaced with a newer version from the cluster. This is called "file
versioning" and uses one of the available *versioning strategies* described
below. File versioning is configured per folder and defaults to "no file
versioning", i.e. no old copies of files are kept.

Trash Can File Versioning
-------------------------

This versioning strategy emulates the common "trash can" approach. When a file
is deleted or replaced due to a change on a remote device, it is a moved to
the trash can in the ``.stversions`` folder. If a file with the same name was
already in the trash can it is replaced.

A configuration option is available to clean the trash can from files older
than a specified number of days. If this is set to a positive number of days,
files will be removed when they have been in the trash can that long. Setting
this to zero prevents any files from being removed from the trash can
automatically.

Simple File Versioning
----------------------

With "Simple File Versioning" files are moved to the ``.stversions`` folder
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
stored, with the default being the ``.stversions`` folder inside the normal
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

External File Versioning
------------------------

This versioning method delegates the decision on what to do to an external
command (program or script). The only configuration option is the name of the
command. This should be an absolute path name. Just prior to a file being
replaced, the command will be run with two parameters: the path to the folder,
and the path to the file within the folder.

Example for Unixes
~~~~~~~~~~~~~~~~~~

Lets say I want to keep the latest version of each file as they are replaced
or removed; essentially I want a "trash can"-like behavior. For this, I create
the following script and store it as ``/Users/jb/bin/onlylatest.sh`` (i.e. the
``bin`` directory in my home directory):

.. code-block:: bash

    #!/bin/sh
    set -eu

    # Where I want my versions stored
    versionspath=~/.trashcan

    # The parameters we get from Syncthing
    folderpath="$1"
    filepath="$2"

    # First ensure the dir where we need to store the file exists
    mkdir -p `dirname "$versionspath/$filepath"`
    # Then move the file there
    mv -f "$folderpath/$filepath" "$versionspath/$filepath"

I must ensure that the script has execute permissions (``chmod 755
onlylatest.sh``), then configure Syncthing with the above path as the command
name.

Lets assume I have a folder "default" in ~/Sync, and that within that folder
there is a file ``docs/letter.txt`` that is being replaced or deleted. The
script will be called as if I ran this from the command line::

    $ /Users/jb/bin/onlylatest.sh /Users/jb/Sync docs/letter.txt

The script will then move the file in question to
``~/.trashcan/docs/letter.txt``, replacing any previous version of that letter
that may already have been there.
