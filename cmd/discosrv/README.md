discosrv
========

[![Latest Build](http://img.shields.io/jenkins/s/http/build.syncthing.net/discosrv.svg?style=flat-square)](http://build.syncthing.net/job/discosrv/lastBuild/)

This is the global discovery server for the `syncthing` project.

To get it, run `go get github.com/syncthing/discosrv` or download the
[latest build](http://build.syncthing.net/job/discosrv/lastSuccessfulBuild/artifact/)
from the build server.

Usage
-----

The discovery server supports `ql` and `postgres` backends.
Specify the backend via `-db-backend` and the database DSN via `-db-dsn`.

By default it will use in-memory `ql` backend. If you wish to persist the
information on disk between restarts in `ql`, specify a file DSN:

```bash
$ discosrv -db-dsn="file:///var/run/discosrv.db"
```

For `postgres`, you will need to create a database and a user with permissions
to create tables in it, then start the discosrv as follows:

```bash
$ export DISCOSRV_DB_DSN="postgres://user:password@localhost/databasename"
$ discosrv -db-backend="postgres"
```

You can pass the DSN as command line option, but the value what you pass in will
be visible in most process managers, potentially exposing the database password
to other users.

In all cases, the appropriate tables and indexes will be created at first
startup. If it doesn't exit with an error, you're fine.

See `discosrv -help` for other options.
