autoNormalize
=============

``autoNormalize`` is an advanced folder setting that affects the handling of
files with incorrect UTF-8 normalization in their name. When set, such files
are renamed to the correctly normalized form during scanning.

.. note:: This option should normally be set to ``true``.

Background
----------

File names can be represented in many different ways. Some systems use an
extended ASCII character set like ISO-8859-1 (Latin), others may use a
different encoding to represent Chinese, Russian and so on. The modern
standard is to use Unicode in UTF-8 encoding, even when the file system itself
doesn't dictate a format, such as is the case on most Unix-like systems.
Syncthing will refuse to synchronize files with names not encoded in UTF-8.

However, there are different ways of encoding the same character even within
UTF-8. These are called normalization forms and differ primarily between Mac
and everything else. Differences in normalization form means that you could on
some systems have three (or more) files all called ``räksmörgås.txt``,
``räksmörgås.txt``,  and ``räksmörgås.txt`` -- those are the same characters,
but expressed in different ways. More commonly an issue arises when files are
copied from a system that uses one normalization form (Mac) to a system using
another normalization form (Windows) without translation; say, via a USB
stick.

To avoid such issues, Syncthing automatically corrects normalization errors
when it runs into them, unless this option is disabled.
