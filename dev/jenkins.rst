Interacting with Jenkins
========================

Jenkins will test pull requests from recognized authors. If the pull
request is not from a recognized author, an *admin* needs to tell
Jenkins to perform the tests, after giving the patch a manual look over
to prevent shenanigans. A number of tests are performed, ranging from
verifying correct code formatting and that the author is included in the
AUTHORS file to that the code in fact builds and passes tests. A pull
request should usually only be merged if all tests return green,
although there are exceptions.

To enable testing for this pull request only:

::

    @st-jenkins ok to test

To enable testing for this pull request, and all future pull requests
from the same author:

::

    @st-jenkins add to whitelist

For pull requests where Jenkins has already run it's tests, but should
run them again:

::

    @st-jenkins test this please

This is not necessary when new commits are pushed (tests will be rerun
for the new commits), but is useful if something has changed server side
or to verify that everything is still OK if ``master`` has been updated.
