# relaypoolsrv

[![Latest Build](http://img.shields.io/jenkins/s/http/build.syncthing.net/relaypoolsrv.svg?style=flat-square)](http://build.syncthing.net/job/relaypoolsrv/lastBuild/)

This is the relay pool server for the `syncthing` project, which allows community hosted [relaysrv](https://github.com/syncthing/relaysrv)'s to join the public pool.

Servers that join the pool are then advertised to users of `syncthing` as potential connection points for those who are unable to connect directly due to NAT or firewall issues.

There is very little reason why you'd want to run this yourself, as `relaypoolsrv` is just used for announcement and lookup of public relay servers. If you are looking to setup a private or a public relay, please check the documentation for [relaysrv](https://github.com/syncthing/relaysrv), which also explains how to join the default public pool.

If you still want to run it, you can run `go get github.com/syncthing/relaypoolsrv` download it or download the
[latest build](http://build.syncthing.net/job/relaypoolsrv/lastSuccessfulBuild/artifact/)
from the build server.

See `relaypoolsrv -help` for configuration options.
