.. _reverse-proxy-setup:

Reverse Proxy Setup
===================

A reverse proxy allows you to "pass" requests through your web server to another
site or program. The reverse proxy will make it look like Syncthing's GUI is a
page within your existing site.

This is especially useful if:

- You need to access the GUI on port 80 or 443 but you already host a website
  on the same device.
- You want to share SSL certificates with an existing site.
- You want to share authentication with an existing setup.

Server Configuration
--------------------

If you have access to your web server's configuration use the following
examples to pass the location ``/syncthing`` on your web server to Syncthing's
GUI hosted on ``localhost:8384``.

Apache
~~~~~~

.. code-block:: apache

    ProxyPass /syncthing/ http://localhost:8384/
    <Location /syncthing/>
        ProxyPassReverse http://localhost:8384/
        Require all granted
    </Location>

Nginx
~~~~~

.. code-block:: nginx

    location /syncthing/ {
      proxy_set_header        Host $host;
      proxy_set_header        X-Real-IP $remote_addr;
      proxy_set_header        X-Forwarded-For $proxy_add_x_forwarded_for;
      proxy_set_header        X-Forwarded-Proto $scheme;

      proxy_pass              http://localhost:8384/;
    }

Folder Configuration
--------------------

If you don't have access to your web server configuration files you might try
the following technique.

Apache
~~~~~~

Add the configuration below to a ``.htaccess`` file in the folder of your
webroot which should redirect to the WebUI, ``/syncthing`` to produce the same
behaviour as above

.. code-block:: apache

    RewriteEngine On
    RewriteCond %{HTTPS} !=on
    RewriteCond %{ENV:HTTPS} !=on
    RewriteRule .* https://%{SERVER_NAME}%{REQUEST_URI} [R=301,L]
    RewriteRule ^(.*) http://localhost:8384/$1 [P]


This method also redirects to HTTPS to prevent opening the GUI unencrypted.
