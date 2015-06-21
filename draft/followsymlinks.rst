.. note:: This describes an incomplete feature under development.

.. warning::

  This is an advanced feature. Be sure to read and fully understand this
  guide, and have a backup of your data. Incorrect configuration may result in
  the deletion of your files. Currently it's probably best to only use
  ``FollowSymlinks`` on a folder master.

Symbolic Link Following
=======================

It is possible to synchronize directory trees not present directly under a
sync folder by using symbolic links ("symlinks") and enabling "following" of
them. This feature is currently experimental and cannot be enabled by using
the graphical interface.

Operation
---------

When a folder is configured to follow symlinks, any such links that are
encountered during scanning will be resolved to their destination and scanned.
Symlinks can point to either files or directories. When symlink following is
enabled, the behavior is changed from the default (copy symlinks verbatim) to
the following:

#. Symlinks pointing to a nonexistent destination are ignored.

#. Symlinks pointing to a file are interpreted as being that file.

#. Editing such a file on another device results in the *symlink* being
   replaced with the new version of the file.

#. Deleting such a file on another device results in the *symlink* being
   deleted.

#. Symlinks pointing to directories are interpreted as being that directory.

#. Symlinks pointing to a directory that is a child of another already scanned
   directory are ignored. This is to avoid infinite recursion in symlink
   following.

Enabling
--------

.. code-block:: xml

  <configuration version="10">
    <folder id="default" path="/Users/jb/Sync">
        ...
        <followSymlinks>true</followSymlinks>
    </folder>
    ...
  </configuration>

Disabling
---------

.. warning::

  Disabling ``FollowSymlinks``, once enabled, is not fully supported. Doing so
  by the same mechanism used to enable it is likely to destroy your files.

Disabling ``FollowSymlinks`` is inherently unsafe as it generates delete
records for all files that were previously accessible via the symlink. Under
some conditions, these files may be deleted from the reconfigured device
having the symlink, in addition to the other devices.

The safest course of action is to remove the symlinks themselves, then disable
``FollowSymlinks``. The files previously reachable via the symlink will be
deleted from other devices, but (given the symlink was removed) are not
reachable and hence are preserved on the source device.
