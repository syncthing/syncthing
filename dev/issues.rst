Issue Management
================

Bugs, feature requests and other things we need to do are tracked as
Github issues. Issues can be of various types and in various states, and
also belong to milestones or not. This page is an attempt to document
the current practice.

Labels
------

Issues without labels are undecided - that is, we don't yet know if it's
a bug, a configuration issue, a feature request or what. Issues that are
invalid for whatever reason are closed with a short explanation of why.
Examples include "Duplicate of #123", "Discovered to be configuration
error", "Rendered moot by #123" and so on. We don't use the "invalid" or
"wontfix" labels.

-  **android** - Marks an issue as occurring on the Android platform
   only.

-  **bug** - The issue is a verified bug.

-  **build** - The issue is caused by or requires changes to the build
   system (scripts or Docker image).

-  **docs** - Something requires documenting.

-  **easy** - This could be easily fixed, probably an hours work or
   less.

-  **enhancement** - This is a new feature or an improvement of some
   kind, as opposed to a problem (bug).

-  **help-wanted** - The core team can't or won't do this, but someone
   else is welcome to. This does not mean that help is not wanted on the
   *other* issues. You can see this as a soft ``wontfix``.

-  **pr-bugfix** - This pull request *fixes* a bug. This is different
   from the ``bug`` label, as there may also be pull requests with for
   example tests that *prove* a bug which would then be labeled ``bug``.

-  **pr-refactor** - This pull request is a refactoring, i.e. not
   supposed to change behavior.

-  **pr-wait-or-pending** - This pull request is not ready for merging,
   even if the tests pass and it looks good. It is incomplete or
   requires more discussion.

-  **protocol** - This requires a change to the protocol.
