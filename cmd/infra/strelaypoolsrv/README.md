# relaypoolsrv

This is the relay pool server for the `syncthing` project, which allows
community hosted [relaysrv](https://github.com/syncthing/relaysrv)'s to join
the public pool.

Servers that join the pool are then advertised to users of `syncthing` as
potential connection points for those who are unable to connect directly due
to NAT or firewall issues.

There is very little reason why you'd want to run this yourself, as
`relaypoolsrv` is just used for announcement and lookup of public relay
servers. If you are looking to set up a private or a public relay, please
check the documentation for
[relaysrv](https://github.com/syncthing/relaysrv), which also explains how
to join the default public pool.

See `relaypoolsrv -help` for configuration options.

##### Third-party attributions

[oschwald/geoip2-golang](https://github.com/oschwald/geoip2-golang), [oschwald/maxminddb-golang](https://github.com/oschwald/maxminddb-golang), Copyright (C) 2015 [Gregory J. Oschwald](mailto:oschwald@gmail.com).

