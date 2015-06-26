.. _statechanged:

StateChanged
------------

Emitted when a folder changes state. Possible states are ``idle``,
``scanning``, ``cleaning`` and ``syncing``. The field ``duration`` is
the number of seconds the folder spent in state ``from``. In the example
below, the folder ``default`` was in state ``scanning`` for 0.198
seconds and is now in state ``idle``.

.. code-block:: json

    {
        "id": 8,
        "type": "StateChanged",
        "time": "2014-07-17T13:14:28.697493016+02:00",
        "data": {
            "folder": "default",
            "from": "scanning",
            "duration": 0.19782869900000002,
            "to": "idle"
        }
    }
