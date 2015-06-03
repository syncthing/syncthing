.. _event-api:

Event API
=========

Description
-----------

Syncthing provides a simple long polling interface for exposing events from the
core utility towards a GUI.

To receive events, perform a HTTP GET of ``/rest/events?since=<lastSeenID>``,
where ``<lastSeenID>`` is the ID of the last event you've already seen or zero.
Syncthing returns a JSON encoded array of event objects, starting at the event
just after the one with the last seen ID. There is a limit to the number of
events buffered, so if the rate of events is high or the time between polling
calls is long some events might be missed. This can be detected by noting a
discontinuity in the event IDs.

If no new events are produced since ``<lastSeenID>``, the HTTP call blocks and
waits for new events to happen before returning, or if no new events are
produced within 60 seconds, times out.

To receive only a limited number of events, add the ``limit=n`` parameter with a
suitable value for ``n`` and only the *last* ``n`` events will be returned. This
can be used to catch up with the latest event ID after a disconnection for
example: ``/rest/events?since=0&limit=1``.

Event Structure
---------------

Each event is represented by an object similar to the following::

    {
        "id": 2,
        "type": "DeviceConnected",
        "time": "2014-07-13T21:04:33.687836696+02:00",
        "data": {
            "addr": "172.16.32.25:22000",
            "id": "NFGKEKE-7Z6RTH7-I3PRZXS-DEJF3UJ-FRWJBFO-VBBTDND-4SGNGVZ-QUQHJAG"
        }
    }

The top level keys ``id``, ``time``, ``type`` and ``data`` are always present,
though ``data`` may be ``null``.

id
    A monotonically increasing integer. The first event generated has id ``1``,
    the next has id ``2`` etc.
time
    The time the event was generated.
type
    Indicates the type of (i.e. reason for) the event and is one of the event
    types below.
data
    An object containing optional extra information; the exact structure is
    determined by the event type.

Events
------

.. toctree::
    :maxdepth: 2
    :glob:

    ../events/*
