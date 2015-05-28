POST /rest/db/ignores
=====================

Expects a format similar to the output of ``GET`` call, but only
containing the ``ignore`` field (``patterns`` field should be omitted).
It takes one parameter, ``folder``, and either updates the content of
the ``.stignore`` echoing it back as a response, or returns an error.
