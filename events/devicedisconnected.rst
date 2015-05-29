DeviceDisconnected
------------------

Generated each time a connection to a device has been terminated.

.. code-block:: json

    {
        "id": 48,
        "type": "DeviceDisconnected",
        "time": "2014-07-13T21:18:52.859929215+02:00",
        "data": {
            "error": "unexpected EOF",
            "id": "NFGKEKE-7Z6RTH7-I3PRZXS-DEJF3UJ-FRWJBFO-VBBTDND-4SGNGVZ-QUQHJAG"
        }
    }


.. note::
    The error key contains the cause for disconnection, which might not
    necessarily be an error as such. Specifically, "EOF" and "unexpected
    EOF" both signify TCP connection termination, either due to the other
    device restarting or going offline or due to a network change.
