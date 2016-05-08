ignoreDelete
============

``ignoreDelete`` is an advanced folder setting that affects the handling of
incoming index updates. When set, incoming updates with the delete flag set
are ignored.

.. note:: This option should normally be set to ``false``.

Example Scenario
----------------

Assume two devices, "Alice" and "Bob", are sharing a folder. Bob has set
``ignoreDelete``.

New and updated files are synchronized as usual between Alice and Bob. When
Bob deletes a file, it is deleted for Alice as well. When Alice deletes a
file, Bob ignores that update and does not delete the file.

In this state, Bob is fully up to date from his own point of view, as is Alice
from her own point of view.

However from the point of view of Alice, who deleted a file and propagated the
update, Bob is now out of date because he is yet to remove the file that was
deleted.

From the point of view of Bob, who ignored the delete entry from Alice, Alice
is now out of date because she is missing the file that was deleted.

.. note:: 
	 Bob may continue to see Alice as up to date until the next time
	 Syncthing is restarted on his device.

Should Bob modify any of the files that Alice has deleted, the update will be
sent to Alice and Alice will download the now updated file.

.. note::
	 Ignoring deletes in both directions between two devices can be a
	 confusing configuration.
