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

android
    Marks an issue as occurring on the Android platform only.

bug
    The issue is a verified bug.

build
    The issue is caused by or requires changes to the build system
    (scripts or Docker image).

docs
    Something requires documenting.

easy
    This could be easily fixed, probably an hour's work or less.
    These issues are good starting points for new contributors.

enhancement
    This is a new feature or an improvement of some kind, as
    opposed to a problem (bug).

protocol
    This requires a change to the protocol.

Milestones
----------

There are milestones for major and sometimes minor versions. An issue being
assigned to a milestone means it is a blocker - the release can't be made
without the issue being closed. Typically this also means that the issue is
being actively worked on, at least for version milestones in the foreseeable
future.

In addition to version specific milestones there are two generic ones:

Planned
    This issue is being worked on, or will soon be worked on, by someone in
    the core team. Expect action on it within the next few days, weeks or
    months.

Unplanned (Contributions Welcome)
    This issue is not being worked on by the core team, and we don't plan on
    doing so in the foreseeable future. We still consider it a valid issue
    and welcome contributions towards resolving it.

Issues lacking a milestone are currently undecided. In practice this is
similar to Unplanned in that probably no-one is working on it, but we are
still considering it and it may end up Planned or closed instead.

Assignee
--------

Users can be assigned to issues. We don't usually do so. Sometimes
someone assigns themself to an issue to indicate "I'm working on this"
to avoid others doing so too. It's not mandatory.

Locking
-------

We don't normally lock issues (prevent further discussion on them).
There are some exceptions though;

-  "Popular" issues that attract lots of "me too" and "+1" comments.
   These are noise and annoy people with useless notifications via mail
   and in the Github interface. Once the issue is clear and it suffers
   from this symptom I may lock it.

-  Contentious bikeshedding discussions. After two sides in a discussion
   have clarified their points, there is no point arguing endlessly
   about it. As above, this may get closed.

-  Duplicates. Once an issue has been identified as a duplicate of
   another issue, it may be locked to prevent further discussion there.
   The intention is to move the discussion to the other (referenced)
   issue, while someone just doing a search and jumping on the first
   match might otherwise resurrect discussion in the duplicate.
