Syncthing Development
=====================

Here are some notes for getting started with developing for Syncthing.
The first section discusses the options for building an external
application using Syncthing as a component. The second section discusses
the development process of Syncthing itself.

Controlling Syncthing from External Applications
------------------------------------------------

Our community has developed a number of `useful
applications <https://github.com/syncthing/syncthing/wiki/Community-Contributions>`__
that build around the Syncthing core, such as tray notifications or
Android support. These are made possible using two APIs:

-  Syncthing provides a long polling interface for exposing events from
   the core utility towards a GUI. This :ref:`event-api` is useful for being
   notified when changes occur.

-  Syncthing exposes a :ref:`rest-api` over HTTP on the GUI port. This is
   used by the GUI code (Javascript) and can be used by other processes
   wishing to control Syncthing.

Contributing to the Syncthing Core
----------------------------------

Why is it written in Go?
~~~~~~~~~~~~~~~~~~~~~~~~

.. note:: This is an excerpt from a forum post by :user:`calmh`.

Here's a non exhaustive list of the advantages I see in Go. Note that
these are by no means exclusive to Go - there are other languages that
are equally good or better each of these things. However, the
combination is winning.

-  The language is modern, small, simple and quite strict. There's a
   minimalism here that I like - what you see is what you get. Some
   things that wouldn't even merit a warning in other languages (like
   unused variables) are errors in Go - your code won't even compile. I
   like the tidiness this promotes.

-  Awesome concurrency. Go's concept of goroutines and channels is
   simple, beautiful and works well. This is essential for something
   like Syncthing where there's a lot of stuff going on in parallel.

-  Simple deployment. Go compiles to a single statically linked binary
   that you just need to copy to the target system and run. It's trivial
   to cross compile from one os/architecture into all others supported
   by the Go compiler.

-  Modern standard library, "some batteries included". This includes an
   HTTP server, a clean (non-OpenSSL) crypto and TLS implementation,
   JSON and XML serializers, etc.

-  Good enough performance. The Go compiler doesn't generate as fast
   code as the best C or C++ compilers out there, but it's still faster
   than interpreted languages.

-  Tooling and community. Go does things somewhat differently than many
   other languages and this can be a bit of an acquired taste... But for
   example the existence and adoption of "go fmt" means there is no
   discussion about formatting or indenting - there is only one
   standard. "Go get" simplifies fetching and building, plus results in
   a standardized repo layout. Etc.

-  I think it's a really nifty language to work with.

If you came here by asking "Why didn't you write Syncthing in
$my\_favourite\_language", the last point above is really all you need.
Of course you *could* write something like Syncthing in Java, C++ or PHP
for that matter, but I wouldn't want to.

Setting up a Go workspace
~~~~~~~~~~~~~~~~~~~~~~~~~

-  Go is particularly picky about file locations; carefully follow the
   paths used in :ref:`building` unless you know well what you're doing.

-  To customize the web GUI without rebuilding, the ``STGUIASSETS``
   environment parameter can be set to override the default files::

      $ STGUIASSETS=gui ./bin/syncthing

Why are you being so hard on my pull request?
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

.. note:: This is :user:`calmh`'s personal opinion.

This isn't actually a frequently asked question as such, but I've
*thought* it enough times myself when contributing to other projects
that I think it's worth answering.

The things is, a pull request looks a little bit different depending on
whether you're on the "contributor" or "maintainer" side. From a
*contributor* point of view, this is what I might feel when I write and
submit a pull request:

   I fixed a bug (implemented a new feature) in your project for you!

But here's what I see instead from the *maintainer* point of view:

   I wrote some code. I'd like you to test, support, document and
   maintain it for me forever!

With that in mind, the maintainer will want to make sure that the code
is something we feel comfortable taking that responsibility for. That
means well tested, clear implementation, fits into the overall
architecture, etc.

But perhaps the existing code doesn't fulfill this to start with; is it
then fair to expect it from a change in a pull request? For example
asking for a test, where there is no test before. Well, the existing
code has some advantage just by being legacy;

-  Perhaps there isn't a test, but we know this code works because it's
   been running in production for a long time without complaints. Then
   it's fair to expect tests from code replacing it.

-  Perhaps there isn't a test, and your code fixes a bug with the code.
   That just highlights that there *should have been* a test to start
   with, and this is the optimal time to add one.

-  Perhaps how the code works (or what exactly it does) isn't obviously
   clear to the reviewer. A test will clarify and lock this down, and
   also prevent us from *inadvertently breaking the code later*.

Another thing that the maintainer might be hard about is whether the
code actually solves the *entire* problem, or at least enough of it to
stand on it's own. This will be more relevant to new features than
bugfixes and includes questions like;

-  Is the feature general enough to be used by other users? If not, do
   we really need it or can it be implemented as part of something more
   general?

-  Is the feature completely implemented? That is, if a new feature is
   added it should be available in the GUI, emit relevant trace
   information to enable debugging, be correctly saved in the
   configuration, etc. If components of this are missing, that's work
   the maintainer will have to do after accepting the pull request.

All in all, a great pull request creates less work for the maintainer,
not more. If the pull request seems like it would add significantly to
the maintainer workload, there will probably be some resistance to
accepting it. ;)
