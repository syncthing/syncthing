.. _release-signing:

Release Signing
===============

Syncthing releases are *signed* in various ways to enable users and automatic
systems to determine that it is in fact a genuine release.

Checksum Files
--------------

Two checksum files are created during the release process. These are
``sha1sum.txt`` and ``sha256sum.txt``. They contain the SHA1 and SHA256 checksums
of the release archives, respectively. To protect against tampering the
checksum files are signed by the Syncthing Release Management GPG key and thus
gain a ``.asc`` extension. To verify that a download is geniuine, first verify
the signature on the checksum file is correct, then that the checksum matches
the release archive.

Binary Signing
--------------

.. versionadded:: 0.12.0

In a genuine release archive you expect to find the ``syncthing`` binary
(``syncthing.exe`` on Windows) and an accompanying signature ``syncthing.sig``
(``syncthing.exe.sig`` on Windows). The signature file contains the `ECDSA
signature
<https://en.wikipedia.org/wiki/Elliptic_Curve_Digital_Signature_Algorithm>`__
of the binary, computed at the time the release was made and signed by the
Syncthing Release Management private key. The keys and signature are PEM
encoded for ease of transmission - the details of the signature and encoding
handling are in `the signature package
<http://godoc.org/github.com/syncthing/syncthing/lib/signature>`__ The public
key is included in the source code and compiled into Syncthing.

When Syncthing performs an automatic upgrade, it verifies the included
signature using the actual binary and the public key. If these match, we know
that the binary has not been tampered with and the release is genuine - the
upgrade proceeds. If there is a mismatch, Syncthing deletes any temporary
files and aborts the upgrade.

Creating and Verifying Binary Signatures Manually
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

As a developer, you may need to verify and create signatures manually.
Syncthing provides a tool to perform these operations in the same manner as
the built in verification: ``stsigtool``. To get this tool, ensure that you
have Go installed and run::

	$ go install github.com/syncthing/syncthing/cmd/stsigtool

After installation you can test it on an arbitrary release (from v0.12.0 and
onwards)::

	$ stsigtool verify syncthing.sig syncthing
	correct signature
	$ echo >> syncthing  # append a newline to the binary
	$ stsigtool verify syncthing.sig syncthing
	incorrect signature

To create signatures of your own, you need a private key. The Syncthing
private key is a closely guarded secret, but you can generate your own using
``stsigtool gen``. The ``gen`` command generates and outputs a new private and
public key pair to stdout; you'll need to paste them into a PEM file each for
storage. You can then sign binaries with the private key using ``stsigtool
sign``, verify them with the public key using ``stsigtool verify``, and have
Syncthing accept these signatures by replacing the compiled in public key.
This may be useful in an enterprise setting, for example.
