GET /rest/db/ignores
====================

Takes one parameter, ``folder``, and returns the content of the
``.stignore`` as the ``ignore`` field. A second field, ``patterns``,
provides a compiled version of all included ignore patterns in the form
of regular expressions. Excluded items in the ``patterns`` field have a
nonstandard ``(?exclude)`` marker in front of the regular expression.

.. code-block:: json

    {
      "ignore": [
        "/Backups"
      ],
      "patterns": [
        "(?i)^Backups$",
        "(?i)^Backups/.*$"
      ]
    }
