Custom Discovery Server
=======================

Description
-----------

This guide assumes that you have already set up Syncthing. If you
haven't yet, head over to :ref:`getting-started` first.

Installing
----------

Go to `releases <https://github.com/syncthing/discosrv/releases>`__ and
download the file appropriate for your operating system. Unpacking it will
yield a binary called ``discosrv`` (or ``discosrv.exe`` on Windows). Start
this in whatever way you are most comfortable with; double clicking should
work in any graphical environment. At first start, discosrv will generate the
directory ``/var/discosrv`` (``X:\var\discosrv`` on Windows, where X is the
partition ``discosrv.exe`` is executed from) with configuration. If the user
running ``discosrv`` doesn't have permission to do so, create the directory
and set the owner appropriately or use the command line switches (see below)
to select a different location.

Configuring
-----------

.. note::
   If you are running an instance of syncthing on the discovery server,
   you must either add that instance to other nodes using a static
   address or bind the discovery server and syncthing instances to
   different IP addresses.

Running discosrv with non-default settings requires passing the
respective parameters to discosrv on every start. ``discosrv -help``
gives you all the tweakables with their defaults:

::

  Usage of discosrv:
    -cert string
        Certificate file (default "cert.pem")
    -db-backend string
        Database backend to use (default "ql")
    -db-dsn string
        Database DSN (default "memory://discosrv")
    -debug
        Debug
    -http
        Listen on HTTP (behind an HTTPS proxy)
    -key string
        Key file (default "key.pem")
    -limit-avg int
        Allowed average package rate, per 10 s (default 5)
    -limit-burst int
        Allowed burst size, packets (default 20)
    -limit-cache int
        Limiter cache entries (default 10240)
    -listen string
        Listen address (default ":8443")
    -stats-file string
        File to write periodic operation stats to

Certificates
^^^^^^^^^^^^

The discovery server provides service over HTTPS. To ensure secure connections
from clients there are three options:

- Use a CA-signed certificate pair for the domain name you will use for the
  discovery server. This is like any other HTTPS website; clients will
  authenticate the server based on it's certificate and domain name.

- Use any certificate pair and let clients authenticate the server based on
  it's "device ID" (similar to Syncthing-to-Syncthing authentication). In
  this case, using ``syncthing -generate`` is a good option to create a
  certificate pair.
  
- Pass the ``-http`` flag if the discovery server is behind an SSL-secured 
  reverse proxy. See below for configuration.

For the first two options, the discovery server must be given the paths to
the certificate and key at startup. This isn't necessary with the ``http`` flag::

  $ discosrv -cert /path/to/cert.pem -key /path/to/key.pem
  Server device ID is 7DDRT7J-UICR4PM-PBIZYL3-MZOJ7X7-EX56JP6-IK6HHMW-S7EK32W-G3EUPQA

The discovery server prints it's device ID at startup. In the case where you
are using a non CA signed certificate, this device ID (fingerprint) must be
given to the clients in the discovery server URL::

  https://disco.example.com:8443/v2/?id=7DDRT7J-UICR4PM-PBIZYL3-MZOJ7X7-EX56JP6-IK6HHMW-S7EK32W-G3EUPQA

Otherwise, the URL (note the trailing slash after the ``v2``) will be::

  https://disco.example.com:8443/v2/
  
Pointing Syncthing at Your Discovery Server
-------------------------------------------

By default, Syncthing uses a number of global discovery servers, signified by
the entry ``default`` in the list of discovery servers. To make Syncthing use
your own instance of discosrv, open up Syncthing's web GUI. Go to settings,
Global Discovery Server and add discosrv's host address to the comma-separated
list, e.g. ``https://disco.example.com:8443/v2/``. Note that discosrv uses port
8443 by default. For discosrv to be available over the internet with a dynamic
IP address, you will need a dynamic DNS service.

If you wish to use *only* your own discovery server, remove the ``default``
entry from the list.

Reverse Proxy Setup
-------------------

The discovery server can be run behind an SSL-secured reverse proxy. This
allows:

- Use of a subdomain name without requiring a port number added to the URL
- Sharing an SSL certificate with multiple services on the same server

Requirements
^^^^^^^^^^^^

- Run the discovery server using the -http flag  :code:`discosrv -http`.
- SSL certificate/key configured for the reverse proxy
- The "X-Forwarded-For" http header must be passed through with the client's
  real IP address
- The "X-SSL-Cert" must be passed through with the PEM-encoded client SSL
  certificate
- The proxy must request the client SSL certificate but not require it to be
  signed by a trusted CA.

Nginx
^^^^^

These three lines in the configuration take care of the last three requirements
listed above:

.. code-block:: nginx

    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-SSL-Cert $ssl_client_cert;
    ssl_verify_client optional_no_ca;

The following is a complete example Nginx configuration file. With this setup,
clients can use https://discovery.mydomain.com as the discovery server URL in
the Syncthing settings.

.. code-block:: nginx

    # HTTP 1.1 support
    proxy_http_version 1.1;
    proxy_buffering off;
    proxy_set_header Host $http_host;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection $proxy_connection;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $proxy_x_forwarded_proto;
    proxy_set_header X-SSL-Cert $ssl_client_cert;
    upstream discovery.mydomain.com {
        # Local IP address:port for discovery server
        server 172.17.0.6:8443;
    }
    server {
            server_name discovery.mydomain.com;
            listen 80;
            access_log /var/log/nginx/access.log vhost;
            return 301 https://$host$request_uri;
    }
    server {
            server_name discovery.mydomain.com;
            listen 443 ssl http2;
            access_log /var/log/nginx/access.log vhost;
            ssl_protocols TLSv1 TLSv1.1 TLSv1.2;
            ssl_ciphers ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES256-GCM-SHA384:ECDHE-ECDSA-AES256-GCM-SHA384: DHE-RSA-AES128-GCM-SHA256:DHE-DSS-AES128-GCM-SHA256:kEDH+AESGCM:ECDHE-RSA-AES128-SHA256:ECDHE-ECDSA-AES128-SHA256:ECDHE-RSA-AES128-SHA:E CDHE-ECDSA-AES128-SHA:ECDHE-RSA-AES256-SHA384:ECDHE-ECDSA-AES256-SHA384:ECDHE-RSA-AES256-SHA:ECDHE-ECDSA-AES256-SHA:DHE-RSA-AES128-SHA25 6:DHE-RSA-AES128-SHA:DHE-DSS-AES128-SHA256:DHE-RSA-AES256-SHA256:DHE-DSS-AES256-SHA:DHE-RSA-AES256-SHA:AES128-GCM-SHA256:AES256-GCM-SHA3 84:AES128-SHA256:AES256-SHA256:AES128-SHA:AES256-SHA:AES:CAMELLIA:DES-CBC3-SHA:!aNULL:!eNULL:!EXPORT:!DES:!RC4:!MD5:!PSK:!aECDH:!EDH-DSS -DES-CBC3-SHA:!EDH-RSA-DES-CBC3-SHA:!KRB5-DES-CBC3-SHA;
            ssl_prefer_server_ciphers on;
            ssl_session_timeout 5m;
            ssl_session_cache shared:SSL:50m;
            ssl_certificate /etc/nginx/certs/discovery.mydomain.com.crt;
            ssl_certificate_key /etc/nginx/certs/discovery.mydomain.com.key;
            ssl_dhparam /etc/nginx/certs/discovery.mydomain.com.dhparam.pem;
            add_header Strict-Transport-Security "max-age=31536000";
            ssl_verify_client optional_no_ca;
            location / {
                    proxy_pass http://discovery.mydomain.com;
            }
    }

An example of automating the SSL certificates and reverse-proxying the Discovery
Server and Syncthing using Nginx, `Let's Encrypt`_ and Docker can be found here_.

.. _Let's Encrypt: https://letsencrypt.org/
.. _here: https://forum.syncthing.net/t/docker-syncthing-and-syncthing-discovery-behind-nginx-reverse-proxy-with-lets-encrypt/6880
