POST /rest/system/discovery/hint
================================

Post with the query parameters ``device`` and ``addr`` to add entries to
the discovery cache.

.. code-block:: bash

    curl -X POST http://127.0.0.1:8384/rest/system/discovery/hint?device=LGFPDIT7SKNNJVJZA4FC7QNCRKCE753K72BW5QD2FOZ7FRFEP57Q\&addr=192.162.129.11:22000
    # Or with the X-API-Key header:
    curl -X POST --header "X-API-Key: TcE28kVPdtJ8COws1JdM0b2nodj77WeQ" http://127.0.0.1:8384/rest/system/discovery/hint?device=LGFPDIT7SKNNJVJZA4FC7QNCRKCE753K72BW5QD2FOZ7FRFEP57Q\&addr=192.162.129.11:22000
