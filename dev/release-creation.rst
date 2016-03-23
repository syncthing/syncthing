Creating a Release
==================

Prerequisites
-------------

-  Push access to the ``syncthing`` repo, for pushing a new tag.

-  SSH account on the signing server.

Process
-------

Make sure the build seems sane. I.e. the build is clean on the build
server, the integration tests pass without complaints.

Update the documentation and translations, and commit the result.

.. code-block:: bash

    $ ./build.sh prerelease
    $ git commit -m "Translation and docs update"
    $ git push

Create a new, signed tag on master, with the version as comment, and
push it:

.. code-block:: bash

    $ git tag -a -s -m v0.10.15 v0.10.15
    $ git push --tags

(The tag is signed with your personal key. The binary releases will be
signed by the Syncthing Release key later. I use `git autotag
<https://github.com/calmh/git-autotag>`__ to do this correcly with minimal
pain.)

Trigger the ``syncthing-release`` job for the newly created tag and wait for
it to complete successfully before moving on. This builds the
``syncthing-release-windows``, ``syncthing-release-mac`` and
``syncthing-release-debian`` jobs as well, and these must succeed.

Run ``go run script/changelog.go`` (in the repo) to create the changelog
comparison from the previous release. Copy to clipboard.

On the Github releases page, select the newly pushed tag and hit "Edit
Tag". Set the "Release title" to the same version as the tag, paste in
the changelog from above, and publish the release.

On the signing server, logged in via ssh, run ``sign-upload-release``. This
will download the build artefacts from Jenkins, sign all the binaries,
create the sha1sum and sha256sum files, sign them with the release GPG key
and upload the whole shebang to Github.

Verify it looks sane on the releases page.

Then run ``sign-upload-debian`` which does the same for the Debian packages
-- downloads them, signs the package archive, and pushes the new archive to
the web server.
