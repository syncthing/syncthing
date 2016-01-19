Syncthing Development
=====================

Controlling Syncthing from External Applications
------------------------------------------------

People all over the world have developed a number of :ref:`useful applications
<contributions>` that build around the Syncthing core, such as tray
notifications and Android support. These are made possible using two APIs:

-  A long polling interface for exposing events from
   the core utility to an external party. This :ref:`event-api` is useful for being
   notified of when changes to files, network connections or sync status occur.

-  A :ref:`rest-api` for controlling the operation of Syncthing and directly
   querying for current status.

If this covers what you need to do, there is no need to delve deeper. However,
if you would like to add functionality to Syncthing itself, or correct a bug
or two in there, please read on.


Contributing to the Syncthing Core
----------------------------------

First of all, follow :ref:`building` to get your workspace set up correctly.
Syncthing is written mainly in `Go <http://golang.org>`__ which has some
fairly specific opinions on the required directory layout. If you're new to
Go, don't fear -- it's a small language and easy to learn. There's a `wealth
of resources <http://dave.cheney.net/resources-for-new-go-programmers>`__ on
the web to help you get up to speed, and many people joining the project have
done so with it being their first contact with Go.

When you are ready to start hacking, take a quick glance at the :ref:`contribution-guidelines`
to know what to expect and to make the process smoother. The main take away is
to keep the code clean, base it on the ``master`` branch, and we'll sort out
the rest once you file a pull request.


Source Code Layout
~~~~~~~~~~~~~~~~~~

In the source repository you'll find a tree of various packages and
directories. There is some Go code at the top level, but it's basically scripts
for the build system. The actual code lives in the ``cmd/syncthing`` and
``lib`` directories. The web GUI lives in ``gui``. The rest is as follows.

Godeps/
   Locally vendored copies of external dependencies.

assets/
   Various graphical assets -- the logo.

cmd/
   Commands either built as end products or used by the build process itself.

   genassets/
      Generates asset files that are compiled into ``syncthing`` as part of the build process (build utility).

   stcompdirs/
      Compares two directories (debugging utility).

   stevents/
      Displays event trace from a remote ``syncthing`` using the API (debugging utility).

   stfileinfo/
      Shows information about a file, in the same manner ``syncthing`` would see it (debugging utility).

   stfinddevice/
      Looks up a device on a global discovery server (debugging utility).

   stindex/
      Prints index (database) contents (debugging utility).

   syncthing/
      Synchronizes files between devices...

   todos/
      Converts line endings from Unix to DOS standard (build utility).

   transifexdl/
      Downloads translations from Transifex (build utility).

   translate/
      Generates translation source for Transifex based on the HTML source (build utility).

etc/
   Startup scripts and integration files. Included as-is in the release packages.

gui/
   The web GUI source. Gets compiled into the ``syncthing`` binary by way of ``genassets`` and the build process.

lib/
   Contains all packages that make up the parts of ``syncthing``.

   auto/
      Auto generated asset data, created by ``genassets`` based on the contents of the ``gui`` directory.

   beacon/
      Multicast and broadcast UDP beacons. Used by the local discovery system.

   config/
      Parses, validates and saves configuration files.

   db/
      Stores and processes file index information in a database on disk.

   discover/
      The local and global device discovery -- maps device IDs to IP and port tuples.

   events/
      The event subsystem, handles emitting of and subscribing to events across the other packages.

   fnmatch/
      Matches strings to glob patterns, used by the ignore package.

   ignore/
      Parses the ``.stignore`` file and matches it against file paths.

   model/
      Ties together many parts of ``syncthing`` and handles the main logic of synchronizing files with other devices.

   osutil/
      Abstracts away certain OS specific quirks.

   rc/
      Remote controls a Syncthing process over the REST API.

   protocol/
      Implementation of the BEP protocol.

   scanner/
      Looks for changes to files and hashes them as appropriate.

   stats/
      Records statistics about devices and folders.

   symlinks/
      Handles symlinks in a platform independent manner.

   sync/
      Provides optional debugging on top of the regular Mutex / RWMutex primitives.

   upgrade/
      Downloads and performs upgrade of the running binary.

   upnp/
      Discovers UPnP devices and sets up port mappings for incoming connections.

   versioner/
      Provides file versioning algorithms; simple, staggered and external.

man/
   Manual pages, generated from the documentation.

pkg/
   Compiled packages, generated by the build process.

protocol/
   Legacy location of the protocol package.

script/
   Various utility scripts for auto generating stuff and so on.

test/
   The integration test suite.


Why are you being so hard on my pull request?
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

A pull request looks a little different depending on whether you're on the
"contributor" or "maintainer" side. The contributor says:

   I implemented a new feature in your project for you!

However, the maintainer hears:

   I wrote some code. I'd like you to test, support, document and
   maintain it for me forever.

The maintainer will want to make sure that the code is something we feel
comfortable taking that responsibility for. That means well tested, clear
implementation, fits into the overall architecture, etc.

But perhaps the existing code doesn't fulfill this to start with; is it then
fair to expect it from a change in a pull request? For example asking for a
test or documentation, where there is none before. Well, the existing code has
some advantage just by being legacy;

-  Perhaps there isn't a test, but we know this code works because it's
   been running in production for a long time without complaints. Then
   it's fair to expect tests from code replacing it.

-  Perhaps there isn't a test, and your code fixes a bug with the code.
   That just highlights that there *should have been* a test to start
   with, and this is the optimal time to add one.

-  Perhaps how the code works (or what exactly it does) isn't clear to the
   reviewer. A test will clarify and lock this down, and also prevent us
   from *inadvertently breaking it later*.

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
not more.
