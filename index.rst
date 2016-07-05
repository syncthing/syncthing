.. We register the :strike: role for strikethrough text. Use it like
.. :strike:`this is struck out`.
.. role:: strike

Welcome to Syncthing's documentation!
=====================================

As a new user, the :ref:`getting started guide <getting-started>` is a good
place to start, then perhaps moving on to :ref:`the FAQ <faq>`. If you run
into trouble getting devices to connect to each other, the page about
:ref:`firewall setup <firewall-setup>` explains the networking necessary to
get it to work.

As a developer looking to get started with a contribution, see :ref:`how to
build <building>`, :ref:`how to debug <debugging>` and the `contribution
guidelines`_. This documentation site can be edited on Github_.

Contact
-------

* If you're looking for specific people to talk to, check out the
  :ref:`project-presentation`.

* To report bugs or request features, please use the `issue tracker`_ Before you
  do so, make sure you are running the `latest version`_, and please do a
  quick search to see if the issue has already been reported.

  .. image:: issues.png

* To report security issues, please follow the instructions on the
  `Security page`_.

* To get help and support, to discuss scenarios, or just connect with other
  users and developers you can head over to the `friendly forum`_.

* For a more real time experience, there's also an IRC channel ``#syncthing`` on
  `Freenode <https://freenode.net/>`_.

* For other concerns you may reach out to members of the core team, currently
  :user:`calmh`, :user:`AudriusButkevicius` and :user:`Zillode`.

The main documentation for the site is organized into a couple of sections. You
can use the headings in the left sidebar to navigate the site.

.. toctree::
   :caption: Introduction
   :maxdepth: 1
   :glob:

   intro/getting-started
   intro/gui
   intro/project-presentation

.. toctree::
   :caption: For Users
   :maxdepth: 1
   :glob:

   Command Line Operation <users/syncthing>
   users/faq

   Configuration <users/config>
   users/advanced
   users/foldermaster

   users/syncing

   users/firewall
   users/relaying
   users/proxying

   users/ignoring
   users/versioning

   users/stdiscosrv
   users/strelaysrv
   users/custom-upgrades

   users/*

.. toctree::
   :caption: For Developers
   :maxdepth: 1
   :glob:

   Introduction <dev/intro>
   dev/*

.. toctree::
   :caption: Specifications
   :maxdepth: 1
   :glob:

   specs/index.rst
   specs/*

.. _`contribution guidelines`: https://github.com/syncthing/syncthing/blob/master/CONTRIBUTING.md
.. _Github: https://github.com/syncthing/docs
.. _`issue tracker`: https://github.com/syncthing/syncthing/issues
.. _`latest version`: https://github.com/syncthing/syncthing/releases/latest
.. _`Security page`: https://syncthing.net/security.html
.. _`friendly forum`: https://forum.syncthing.net
