.. _rest-api:

REST API
========

Description
-----------

Syncthing exposes a REST interface over HTTP on the GUI port. This is used by
the GUI code (JavaScript) and can be used by other processes wishing to control
Syncthing. In most cases both the input and output data is in JSON format. The
interface is subject to change.

API Key
-------

To use the POST methods, or *any* method when authentication is enabled, an API
key must be set and used. The API key can be generated in the GUI, or set in the
``configuration/gui/apikey`` element in the configuration file. To use an API
key, set the request header ``X-API-Key`` to the API key value.

System Endpoints
----------------

.. toctree::
   :maxdepth: 1
   :glob:

   ../rest/system-*

Database Endpoints
------------------

.. toctree::
   :maxdepth: 1
   :glob:

   ../rest/db-*

Statistics Endpoints
--------------------

.. toctree::
   :maxdepth: 1
   :glob:

   ../rest/stats-*

Misc Services Endpoints
-----------------------

.. toctree::
   :maxdepth: 1
   :glob:

   ../rest/svc-*
