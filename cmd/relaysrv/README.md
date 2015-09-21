relaysrv
========

[![Latest Build](http://img.shields.io/jenkins/s/http/build.syncthing.net/relaysrv.svg?style=flat-square)](http://build.syncthing.net/job/relaysrv/lastBuild/)

This is the relay server for the `syncthing` project.

To get it, run `go get github.com/syncthing/relaysrv` or download the
[latest build](http://build.syncthing.net/job/relaysrv/lastSuccessfulBuild/artifact/)
from the build server.

:exclamation:Warnings:exclamation: - Read or regret 
-----

By default, all relay servers will join the default public relay pool, which means that the relay server will be availble for public use, and **will consume your bandwidth** helping others to connect.

If you wish to disable this behaviour, please specify `-pool=""` argument.

Please note that `relaysrv` is only usable by `syncthing` **version v0.12 and onwards**.

To run `relaysrv` you need to have port 22067 available to the internet, which means you might need to allow it through your firewall if you **have a public IP, or setup a port-forwarding** (22067 to 22067) if you are behind a router.

Running for public use
----
Make sure you have a public IP with port 22067 open, or make sure you have port-forwarding (22067 to 22067) if you are behind a router.

Run the `relaysrv` with no arguments (or `-debug` if you want more output), and that should be enough for the server to join the public relay pool.
You should see a message saying:
```
2015/09/21 22:45:46 pool.go:60: Joined https://relays.syncthing.net rejoining in 48m0s
```

See `relaysrv -help` for other options, such as rate limits, timeout intervals, etc.

Running for private use
-----

Once you've started the `relaysrv`, it will generate a key pair and print an URI:
```bash
relay://:22067/?id=EZQOIDM-6DDD4ZI-DJ65NSM-4OQWRAT-EIKSMJO-OZ552BO-WQZEGYY-STS5RQM&pingInterval=1m0s&networkTimeout=2m0s&sessionLimitBps=0&globalLimitBps=0&statusAddr=:22070
```

This URI contains partial address of the relay server, as well as it's options which in the future may be taken into account when choosing the best suitable relay out of multiple available.

Because `-listen` option was not used, the `relaysrv` does not know it's external IP, therefore you should replace the host part of the URI with your public IP address on which the `relaysrv` will be available:

```bash
relay://123.123.123.123:22067/?id=EZQOIDM-6DDD4ZI-DJ65NSM-4OQWRAT-EIKSMJO-OZ552BO-WQZEGYY-STS5RQM&pingInterval=1m0s&networkTimeout=2m0s&sessionLimitBps=0&globalLimitBps=0&statusAddr=:22070
```

If you do not care about certificate pinning (improved security) or do not care about passing verbose settings to the clients, you can shorten the URL to just the host part:

```bash
relay://123.123.123.123:22067
```

This URL can then be used in `syncthing` as one of the relay servers.

See `relaysrv -help` for other options, such as rate limits, timeout intervals, etc.
