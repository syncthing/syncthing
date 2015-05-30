.. _building:

Building Syncthing
==================

.. note::
    You probably only need to go through the build process if you are going
    to do development on Syncthing or if you need to do a special packaging
    of it. For all other purposes we recommend using the the official binary
    releases instead.

.. note::
    If you're on Linux and want the quickest possible start, check out
    :ref:`building-with-docker`. Otherwise follow the guide below to set up the
    development environment on your computer.

Branches and Tags
-----------------

You should base your work on the ``master`` branch when doing your
development. This branch is usually what will be going into the next
release and always what pull requests should be based on.

If you're looking to build and package a release of Syncthing you should
instead use the latest tag (``vX.Y.Z``) as the contents of ``master``
may be unstable and unsuitable for general consumption.

Prerequisites
-------------

-  Go **1.3** or higher
-  Git

If you're not a Go developer since before, the easiest way to get going
is to download the latest version of Go as instructed in
http://golang.org/doc/install and ``export GOPATH=~``.

.. note:: You need to set ``GOPATH`` correctly and the source **must** be checked out into ``$GOPATH/src/github.com/syncthing/syncthing``. The instructions below accomplish this correctly.

Building (Unix)
---------------

-  Install the prerequisites.
-  Open a terminal.

.. code-block:: bash

    # This should output "go version go1.3" or higher.
    $ go version

    # Go is particular about file locations; use this path unless you know very
    # well what you're doing.
    $ mkdir -p ~/src/github.com/syncthing
    $ cd ~/src/github.com/syncthing
    # Note that if you are building from a source code archive, you need to
    # rename the directory from syncthing-XX.YY.ZZ to syncthing
    $ git clone https://github.com/syncthing/syncthing

    # Now we have the source. Time to build!
    $ cd syncthing

    # You should be inside ~/src/github.com/syncthing/syncthing right now.
    $ go run build.go

Unless something goes wrong, you will have a ``syncthing`` binary built
and ready in ``~/src/github.com/syncthing/syncthing/bin``.

Building (Windows)
------------------

-  Install the prerequisites.
-  Open a ``cmd`` Window::

    # This should output "go version go1.3" or higher.
    > go version

    # Go is particular about file locations; use this path unless you know very
    # well what you're doing.
    > mkdir c:\src\github.com\syncthing
    > cd c:\src\github.com\syncthing
    # Note that if you are building from a source code archive, you need to
    # rename the directory from syncthing-XX.YY.ZZ to syncthing
    > git clone https://github.com/syncthing/syncthing

    # Now we have the source. Time to build!
    > cd syncthing
    > go run build.go

Unless something goes wrong, you will have a ``syncthing.exe`` binary
built and ready in ``c:\src\github.com\syncthing\syncthing\bin``.

Subcommands and Options
-----------------------

The following ``build.go`` subcommands and options exist.

-  ``go run build.go install`` -- installs binaries in ``./bin``
   (default command, this is what happens when build.go is run without
   any commands or parameters).

-  ``go run build.go build`` -- forces a rebuild of the binary to the
   current directory; similar to ``install`` but slower.

-  ``go run build.go clean`` -- remove build artefacts, guaranteeing a
   complete rebuild. Use this when switching between normal builds and
   noupgrade builds.

-  ``go run build.go test`` -- run the tests.

-  ``go run build.go tar`` -- create a Syncthing tar.gz dist file in the
   current directory. Assumes a Unixy build.

-  ``go run build.go zip`` -- create a Syncthing zip dist file in the
   current directory. Assumes a Windows build.

-  ``go run build.go  assets`` -- rebuild the compiled-in GUI assets.

-  ``go run build.go  deps`` -- update the in-repo dependencies.

-  ``go run build.go  xdr`` -- regenerate the XDR en/decoders. Only
   necessary when the protocol has changed.

The options ``-no-upgrade``, ``-goos`` and ``-goarch`` can be given to
influence ``install``, ``build``, ``tar`` and ``zip``. Examples:

-  ``go run build.go -goos linux -goarch 386 tar`` -- build a tar.gz
   distribution of Syncthing for linux-386.

-  ``go run build.go -goos windows -no-upgrade zip`` -- build a zip
   distribution of Syncthing for Windows (current architecture) with
   upgrading disabled.

.. note:: Building for a different operating system or architecture than your native one requires Go having been set up for cross compilation. The easiest way to get this right is to use the official Docker image, described below.

Building without Git
--------------------

Syncthing can be built perfectly fine from a source tarball of course.
If the tarball is from our build server it contains a file called
``RELEASE`` that informs the build system of the version being
built. If you're building from a different source package, for example
one automatically generated by Github, you must instead pass the
``-version`` flag to ``build.go``.

If you are building something that will be installed as a package
(Debian, RPM, ...) you almost certainly want to use ``-no-upgrade`` as
well to prevent the built in upgrade system from being activated.

-  ``go run build.go -version v0.10.26 -no-upgrade tar`` -- build a
   tar.gz distribution of syncthing for the current OS/arch, tagged as
   ``v0.10.26``, with upgrades disabled.

.. _building-with-docker:

Building with Docker
--------------------

The Docker based build image exactly replicates the official build
process and is a quick way to get up and running with the full cross
compiled setup. Start by getting the build image. It is fairly large
(about 2 GiB).

::

    $ docker pull syncthing/build:latest

Then check out and build Syncthing.

::

    $ git clone https://github.com/syncthing/syncthing
    $ cd syncthing
    $ ./build.sh docker-all

A full build is done for all supported architectures, and tests are run.
The process should end with a bunch of release files (``.tar.gz`` and
``.zip``) created.
