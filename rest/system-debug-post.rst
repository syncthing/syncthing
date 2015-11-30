POST /rest/system/debug
=======================

.. versionadded:: 0.12.0

Enables or disables debugging for specified facilities. Give one or both of
``enable`` and ``disable`` query parameters, with comma separated facility
names. To disable debugging of the beacon and discovery packages, and enable it
for config and db:

.. code-block:: bash

    $ curl -H X-API-Key:abc123 -X POST 'http://localhost:8384/rest/system/debug?disable=beacon,discovery&enable=config,db'
