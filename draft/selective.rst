.. note:: This describes an incomplete feature under development.

Selective Sync
==============

This is for when you don't want to synchronize *all* files from the cluster
onto your device, or you want only some directories of yours to be synced
*to* the cluster. There are two mechanisms that support this usage; *Directory
Selection* and *Excluded Files*.


Directory Selection
-------------------

By default, all directories in a given folder are synchronized. Using
directory selection, the synchronization can be limited to a subset of
directories. The selection is done by using a tree browser. The tree
represents the cluster wide contents of a folder. The default is represented
by the top level being checked and directories underneath the top level thus
being implicitly included::

  [X] ~/Sync
      [ ] Documents
          [ ] Processes
          [ ] Standards
      [ ] Pictures
          [ ] Vacation
          [ ] Work
      [ ] Music
          [ ] Classical
          [ ] Rock

To only synchronize documents and vacation pictures, the following selection can be made::

  [/] ~/Sync
      [X] Documents
          [ ] Processes
          [ ] Standards
      [/] Pictures
          [X] Vacation
          [ ] Work
      [ ] Music
          [ ] Classical
          [ ] Rock

The top level shows a partial checkbox to indicate that selections have been
made at a lower level. With this configuration Syncthing will ignore any files
and directories no in ~/Sync/Documents or ~/Sync/Pictures/Vacation -- no
changes will be downloaded from the cluster and local changes will not be
tracked.

.. note:: When displaying the tree we must merge what we actually have on disk
	with what is in the global state, or we will not be able to show new
	directories to the user as we don't know about them...


Excluded Files
--------------

In addition to using directory selection, specific files can be excluded by
adding patterns to match them. Excluded files are:

- Ignored when found on disk (i.e. not hashed and not announced to other
  devices).

- Ignored when announced by other devices (i.e. will not contribute to an "out
  of sync" status or be fetched from the network).

- *Removed* when present in a directory that is marked for deletion by another
  device.

Patterns are in "glob" form, with the following allowed syntax elements:

Asterisk (``*``)
	Matches zero or more characters.

Question mark (``?``)
	Matches exactly one character.

Ranges (``[a-f]``)
	Matches any of the characters in the range exactly once.

Exclamation mark (``!``)
	At the start of a pattern, inverts the pattern (i.e. make matched file *not* be excluded).

Examples:

``*.jpg``
	Matches all files with the ``jpg`` extension.

``[0-9]*``
	Matches all files with names starting with a digit.

``!*.doc``
	Do not exclude ``.doc`` files.

File exclusions apply only to files, not to directories, and apply equally in
all directories in the folder. Patterns are searched in the order given and
the first match wins.


Use Cases
---------

Sync only specific directories
	This is covered perfectly by the "directory selection" part

Exclude common crap files like Thumbs.db
	This is covered perfectly by the "excluded files" part

Sync only a specific file type
	Covered by ``!``-pattern plus exclude everything

Exclude a specific directory only
	Possible by using directory selection and selecting all other directories.

Sync only a specific file in a specific directory; i.e. only one movie out of lots
	Not really possible... Do we need this?

Syncing ignore and exclusion patterns between devices
  Not supported. However not impossible in the future, given that we store the above in the config.

