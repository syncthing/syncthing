Suture
======

[![Build Status](https://travis-ci.org/thejerf/suture.png?branch=master)](https://travis-ci.org/thejerf/suture)

Suture provides Erlang-ish supervisor trees for Go. "Supervisor trees" ->
"sutree" -> "suture" -> holds your code together when it's trying to die.

This library has hit maturity, and isn't expected to be changed
radically. This can also be imported via gopkg.in/thejerf/suture.v2 .

It is intended to deal gracefully with the real failure cases that can
occur with supervision trees (such as burning all your CPU time endlessly
restarting dead services), while also making no unnecessary demands on the
"service" code, and providing hooks to perform adequate logging with in a
production environment.

[A blog post describing the design decisions](http://www.jerf.org/iri/post/2930)
is available.

This module is fully covered with [godoc](http://godoc.org/github.com/thejerf/suture),
including an example, usage, and everything else you might expect from a
README.md on GitHub. (DRY.)

Code Signing
------------

Starting with the commit after ac7cf8591b, I will be signing this repository
with the ["jerf" keybase account](https://keybase.io/jerf). If you are viewing
this repository through GitHub, you should see the commits as showing as
"verified" in the commit view.

(Bear in mind that due to the nature of how git commit signing works, there
may be runs of unverified commits; what matters is that the top one is signed.)

Aspiration
----------

One of the big wins the Erlang community has with their pervasive OTP
support is that it makes it easy for them to distribute libraries that
easily fit into the OTP paradigm. It ought to someday be considered a good
idea to distribute libraries that provide some sort of supervisor tree
functionality out of the box. It is possible to provide this functionality
without explicitly depending on the Suture library.

Changelog
---------

suture uses semantic versioning.

* 2.0.3
  * Accepted PR #23, making the logging functions in the supervisor public.
  * Added a new Supervisor method RemoveAndWait, allowing you to make a
    best effort way to wait for a service to terminate.
  * Accepted PR #24, adding an optional IsCompletable interface that
    Services can implement that indicates they do not need to be restarted
    upon a normal return.
* 2.0.2
  * Fixed issue #21. gccgo doesn't like `case (<-c)`, with the parentheses.
    Of course the parens aren't doing anything useful anyhow. No behavior
    changes.
* 2.0.1
  * __Test code change only__. Addresses the possibility that one of the
    tests can spuriously fail if they run in a certain order.
* 2.0.0
  * Major version due to change to the signature of the logging methods:

    A race condition could occur when the Supervisor rendered the service
    name via fmt.Sprintf("%#v"), because fmt examines the entire object
    regardless of locks through reflection. 2.0.0 changes the supervisors
    to snapshot the Service's name once, when it is added, and to pass it
    to the logging methods.
  * Removal of use of sync/atomic due to possible brokenness in the Debian
    architecture.
* 1.1.2
  * TravisCI showed that the fix for 1.1.1 induced a deadlock in Go 1.4 and
    before.
  * If the supervisor is terminated before a service, the service goroutine
    could be orphaned trying the shutdown notification to the supervisor.
    This should no longer occur.
* 1.1.1
  * Per #14, the fix in 1.1.0 did not actually wait for the Supervisor
    to stop.
* 1.1.0
  * Per #12, Supervisor.stop now tries to wait for its children before
    returning. A careful reading of the original .Stop() contract
    says this is the correct behavior.
* 1.0.1
  * Fixed data race on the .state variable.
* 1.0.0
  * Initial release.
