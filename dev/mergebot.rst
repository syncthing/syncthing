Interacting with the Merge Bot
==============================

There is a bot that hangs around on the Syncthing Github projects and
assists with doing correct merges of pull requests. Using the merge bot to
accept pull requests is the recommended way in all cases as it enforces some
extra checks that the "standard" Github merge button does not. The merge bot
is currently called ``st-review``.

Merging a PR
------------

To merge a pull request, simply tell the bot to do so, making sure that the
first word of the command is ``merge``::

    @st-review merge

.. image:: merge-1.png

It's also possible to override the resulting commit subject and message when
doing this. Just add a blank line, the commit subject, another blank line,
and then the commit body (which can be empty). Don't worry about the text
formatting - the commit body will be reflowed appropriately by the bot::

    @st-review merge

    lib/dialer: Add env var to disable proxy fallback (fixes #3006)

.. image:: merge-2.png

Handling Check Results
----------------------

The merge bot will wait for status checks to resolve, but will refuse to
merge pull requests with unclean statuses:

.. image:: merge-3.png

It is possible to override this in cases where it's necessary, by adding a
``Skip-check`` command to the commit message body. Note that this must be in
the commit message *body*, which means that you need to supply both a commit
message subject and body. Don't overuse this -- it's better to ask Jenkins
to retest if something spurious happened. It can be used to allow merge of
commits from unregistered authors that only touches comments, for example.

The tag must be exactly ``Skip-check:`` followed by a *space separated* list
of check "contexts" as seen in the list on Github. I.e., to skip these two
checks:

.. image:: merge-4-0.png

Use the following syntax::

    @st-review merge this please

    all: Correct spelling in comments

    Skip-check: authors pr-build-mac

.. image:: merge-4-1.png

Please note that the exact string ``Skip-check: authors`` is magic in that
it also allows the build to pass, when it would otherwise stop with commits
from unknown authors.
