stdiscosrv
==========

This is the global discovery server for the `syncthing` project.

Usage
-----

The discovery server supports `ql` and `postgres` backends.
Specify the backend via `-db-backend` and the database DSN via `-db-dsn`.

By default it will use in-memory `ql` backend. If you wish to persist the
information on disk between restarts in `ql`, specify a file DSN:

```bash
$ stdiscosrv -db-dsn="file:///var/run/stdiscosrv.db"
```

For `postgres`, you will need to create a database and a user with permissions
to create tables in it, then start the stdiscosrv as follows:

```bash
$ export STDISCOSRV_DB_DSN="postgres://user:password@localhost/databasename"
$ stdiscosrv -db-backend="postgres"
```

You can pass the DSN as command line option, but the value what you pass in will
be visible in most process managers, potentially exposing the database password
to other users.

In all cases, the appropriate tables and indexes will be created at first
startup. If it doesn't exit with an error, you're fine.

See `stdiscosrv -help` for other options.

##### Third-party attribution

[cznic/lldb](https://github.com/cznic/lldb), Copyright (C) 2014 The lldb Authors.
