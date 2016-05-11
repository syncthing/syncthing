Syncthing Docs
==============

This repo is the source behind http://docs.syncthing.net/.

Editing
-------

To edit the documentation you need a GitHub account. Once you have created one
and logged in, you can edit any page by navigating to the corresponding file and
clicking the edit (pen) icon. This will create a so called "fork" and a "pull
request", which will be approved by one of the existing documentation team
members. Once you have made a contribution or two, you can be added to the
documentation team and perform edits without requiring approval.

In the long run, learning to use Git_ and running Sphinx_ on your computer is
beneficial.

First steps to run is locally
 ::
 
  git clone https://github.com/syncthing/docs.git
  pip install -U Sphinx
  cd docs 
  make html
  # open _build/html/index.html

Structure
---------

The documentation is divided into an index page (``index.rst``) and various
subsections. The sections are:

- Introductory information in ``intro``.
- Information for users in ``users``.
- Information for developers in ``dev``.

The documentation uses the `rst format`_. For a starting point check out the
`reStructuredText Primer`_

.. _Git: http://www.git-scm.com/
.. _Sphinx: http://sphinx-doc.org/
.. _`rst format`: http://docutils.sourceforge.net/docs/ref/rst/restructuredtext.html
.. _`reStructuredText Primer`: http://sphinx-doc.org/rest.html

License
=======

All documentation and protocol specifications are licensed under the `Creative
Commons Attribution 4.0 International License
<http://creativecommons.org/licenses/by/4.0/>`__.
