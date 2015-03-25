discosrv
========

[![Latest Build](http://img.shields.io/jenkins/s/http/build.syncthing.net/discosrv.svg?style=flat-square)](http://build.syncthing.net/job/discosrv/lastBuild/)

This is the global discovery server for the `syncthing` project.

To get it, run `go get github.com/syncthing/discosrv` or download the
[latest build](http://build.syncthing.net/job/discosrv/lastSuccessfulBuild/artifact/)
from the build server.

Usage
-----

The discovery server requires a postgresql backend server. You will need
to create a database and a user with permissions to create tables in it.
Set the database URL in the environment variable `DISCOSRV_DB` before
starting discosrv.

```bash
$ export DISCOSRV_DB="postgres://user:password@localhost/databasename"
$ discosrv
```

The appropriate tables and indexes will be created at first startup. If
it doesn't exit with an error, you're fine.

See `discosrv -help` for other options.
