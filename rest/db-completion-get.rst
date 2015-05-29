GET /rest/db/completion
=======================

Returns the completion percentage (0 to 100) for a given device and
folder.Takes ``device`` and ``folder`` parameters.

.. code-block:: json

    {
      "completion": 0
    }

.. note::
  This is an expensive call, increasing CPU and RAM usage on the device. Use sparingly.
