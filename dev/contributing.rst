.. _contribution-guidelines:

Contribution Guidelines
=======================

Authorship
----------

All code authors are listed in the AUTHORS file. When your first pull request
is accepted, the maintainer will add your details to the AUTHORS file, the
NICKS file and the list of authors in the GUI. Commits must be made with the
same name and email as listed in the AUTHORS file. To accomplish this, ensure
that your git configuration is set correctly prior to making your first
commit::

    $ git config --global user.name "Jane Doe"
    $ git config --global user.email janedoe@example.com

You must be reachable on the given email address. If you do not wish to use
your real name for whatever reason, using a nickname or pseudonym is perfectly
acceptable.

Team Membership
---------------

Contributing useful commits via pull requests will at some point get you
invited to the `Syncthing core team
<https://github.com/orgs/syncthing/teams/core>`__. This team gives you push
access to most repositories, subject to the guidelines below.

The `Syncthing maintainers team
<https://github.com/orgs/syncthing/teams/maintainers>`__ has the same access
permissions, the added responsibility of reviewing major changes, and the
ability to invite members into the core team.

Code Review
-----------

Commits will generally fall into one of the three categories below, with
different requirements on review.

Trivial:
  A small change or refactor that is obviously correct. These may be pushed
  without review by any member of the core team. Examples:
  `removing dead code <https://github.com/syncthing/syncthing/commits/master>`__,
  `updating values <https://github.com/syncthing/syncthing/commit/2aa028facb7ccbe48e85c8c444501cc3fb38ef24>`__.

Minor:
  A new feature, bugfix or refactoring that may need extra eyes on it to weed
  out mistakes, but is architecturally simple or at least uncontroversial.
  Minor changes must go through a pull request and can be merged on approval
  by any other developer on the core or maintainers team. Tests must pass.
  Examples: `adding caching <https://github.com/syncthing/syncthing/pull/2432/files>`__,
  `fixing a small bug <https://github.com/syncthing/syncthing/pull/2406/files>`__.

Major:
  A complex new feature or bugfix, a large refactoring, or a change to the
  underlying architecture of things. A major change must be reviewed by a
  member of the *maintainers* team. Tests must pass.

The categorization is inherently subjective; we recommend erring on the side
of caution - if you are not sure whether a change is *trivial* or merely
*minor*, it's probably minor.

First time contributions from new developers are always major.

Coding Style
------------

General
~~~~~~~

- All text files use Unix line endings. The git settings already present in
  the repository attempts to enforce this.

- When making changes, follow the brace and paranthesis style of the
  surrounding code.

Go Specific
~~~~~~~~~~~

- Follow the conventions laid out in `Effective
  Go <https://golang.org/doc/effective_go.html>`__ as much as makes
  sense. The review guidelines in `Go Code Review Comments
  <https://github.com/golang/go/wiki/CodeReviewComments>`__ should generally
  be followed.

- Each commit should be ``go fmt`` clean.

- Imports are grouped per ``goimports`` standard; that is, standard
  library first, then third party libraries after a blank line.

Commits
-------

- The commit message subject should be a single short sentence
  describing the change, starting with a capital letter but without
  ending punctuation.

- Commits that resolve an existing issue must include the issue number
  as ``(fixes #123)`` at the end of the commit message subject. A correctly
  formatted commit message looks like this::

    Correctly handle nil error in verbose logging (fixes #1921)

- If the commit message subject doesn't say it all, one or more paragraphs of
  describing text should be added to the commit message. This should explain
  why the change is made and what it accomplishes.

- A contribution solving a single issue or introducing a single new
  feature should usually be a single commit based on the current
  ``master`` branch. You may be asked to "rebase" or "squash" your pull
  request to make sure this is the case, especially if there have been
  amendments during review. It's perfectly fine to make review changes
  as new commits and just push them to the same branch -- but a squash
  should be performed when the change is good to be merged.

Tests
-----

Yes please, do add tests when adding features or fixing bugs. Also, when a
pull request is filed a number of automatic tests are run on the code. This
includes:

- That the code actually builds and the test suite passes.

- That the code is correctly formatted (``go fmt``).

- That the commits are based on a reasonably recent ``master``.

- That the author is listed in AUTHORS.

- That the output from ``go lint`` and ``go vet`` is clean. (This checks for a
  number of potential problems the compiler doesn't catch.)

If the pull request is invasive or scary looking, the full integration test
suite can be run as well.

Branches
--------

- ``master`` is the main branch containing good code that will end up
  in the next release. You should base your work on it. It won't ever
  be rebased or force-pushed to.

- ``vx.y`` branches exist to make patch releases on otherwise obsolete
  minor releases. Should only contain fixes cherry picked from master.
  Don't base any work on them.

- Other branches are probably topic branches and may be subject to
  rebasing. Don't base any work on them unless you specifically know
  otherwise.

Tags
----

All releases are tagged semver style as ``vx.y.z``. The maintainer doing the
release signs the tag using their GPG key.

Licensing
---------

All contributions are made under the same MPLv2 license as the rest of the
project, except documentation, user interface text and translation strings
which are licensed under the Creative Commons Attribution 4.0 International
License. You retain the copyright to code you have written.

