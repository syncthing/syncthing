Suture
======

[![Build Status](https://travis-ci.org/thejerf/suture.png?branch=master)](https://travis-ci.org/thejerf/suture)

Suture provides Erlang-ish supervisor trees for Go. "Supervisor trees" ->
"sutree" -> "suture" -> holds your code together when it's trying to die.

This library has hit maturity, and isn't expected to be changed
radically. This can also be imported via gopkg.in/thejerf/suture.v1 .

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
with the ["jerf" keybase account](https://keybase.io/jerf).

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

1. 1.0.0
  * Initial release.
2. 1.0.1
  * Fixed data race on the .state variable.

