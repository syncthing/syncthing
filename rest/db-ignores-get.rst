GET /rest/db/ignores
====================

Takes one parameter, ``folder``, and returns the content of the
``.stignore`` as the ``ignore`` field. A second field, ``expanded``,
provides a list of strings which represent globbing patterns described by gobwas/glob (based on standard wildcards) that match the patterns in ``.stignore`` and all the includes. If appropriate these globs are prepended by the following modifiers: ``!`` to negate the glob, ``(?i)`` to do case insensitive matching and ``(?d)`` to enable removing of ignored files in an otherwise empty directory.

.. code-block:: json

    {
      "ignore": [
        "(?i)/Backups"
      ],
      "expanded": [
        "(?i)Backups",
        "(?i)Backups/**"
      ]
    }
