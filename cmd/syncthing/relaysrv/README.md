strelaysrv
==========

This is the relay server for the `syncthing` project.

:exclamation:Warnings:exclamation: - Read or regret
-----

By default, all relay servers will join to the default public relay pool, which means that the relay server will be available for public use, and **will consume your bandwidth** helping others to connect.

If you wish to disable this behaviour, please specify the `-pools=""` argument.

Please note that `strelaysrv` is only usable by `syncthing` **version v0.12 and onwards**.

To run `strelaysrv` you need to have port 22067 available to the internet, which means you might need to port forward it and/or allow it through your firewall.

Furthermore, by default `strelaysrv` will also expose a /status HTTP endpoint on port 22070, which is used by the pool servers to read metrics of the `strelaysrv`, such as  the current transfer rates, how many clients are connected, etc. If you wish this information to be available you may need to port forward and allow it through your firewall. This is not mandatory for the `strelaysrv` to function, and is used only to gather metrics and present them in the overview page of the pool server.

At the point of writing the endpoint output looks as follows:

```
{
    "bytesProxied": 0,
    "goArch": "amd64",
    "goMaxProcs": 1,
    "goNumRoutine": 13,
    "goOS": "linux",
    "goVersion": "go1.6",
    "kbps10s1m5m15m30m60m": [
        0,
        0,
        0,
        0,
        0,
        0
    ],
    "numActiveSessions": 0,
    "numConnections": 0,
    "numPendingSessionKeys": 2,
    "numProxies": 0,
    "options": {
        "global-rate": 0,
        "message-timeout": 60,
        "network-timeout": 120,
        "per-session-rate": 0,
        "ping-interval": 60,
        "pools": [
            "https://relays.syncthing.net/endpoint"
        ],
        "provided-by": ""
    },
    "startTime": "2016-03-06T12:53:07.090847749-05:00",
    "uptimeSeconds": 17
}
```

If you wish to disable the /status endpoint, provide `-status-srv=""` as one of the arguments when starting the strelaysrv.

Running for public use
----
Make sure you have a public IP with port 22067 open, or have forwarded port 22067 if you are behind a NAT.

Run the `strelaysrv` with no arguments (or `-debug` if you want more output), and that should be enough for the server to join the public relay pool.
You should see a message saying:
```
2015/09/21 22:45:46 pool.go:60: Joined https://relays.syncthing.net/endpoint rejoining in 48m0s
```

See `strelaysrv -help` for other options, such as rate limits, timeout intervals, etc.

Running for private use
-----

Once you've started the `strelaysrv`, it will generate a key pair and print a URI:
```bash
relay://:22067/?id=EZQOIDM-6DDD4ZI-DJ65NSM-4OQWRAT-EIKSMJO-OZ552BO-WQZEGYY-STS5RQM&pingInterval=1m0s&networkTimeout=2m0s&sessionLimitBps=0&globalLimitBps=0&statusAddr=:22070
```

This URI contains a partial address of the relay server, as well as its options which in the future may be taken into account when choosing the most suitable relay.

Because the `-listen` option was not used `strelaysrv` does not know its external IP, therefore you should replace the host part of the URI with your public IP address on which the `strelaysrv` will be available:

```bash
relay://192.0.2.1:22067/?id=EZQOIDM-6DDD4ZI-DJ65NSM-4OQWRAT-EIKSMJO-OZ552BO-WQZEGYY-STS5RQM&pingInterval=1m0s&networkTimeout=2m0s&sessionLimitBps=0&globalLimitBps=0&statusAddr=:22070
```

If you do not care about certificate pinning (improved security) or do not care about passing verbose settings to the clients, you can shorten the URL to just the host part:

```bash
relay://192.0.2.1:22067
```

This URI can then be used in `syncthing` clients as one of the relay servers by adding the URI to the "Sync Protocol Listen Address" field, under Actions and Settings.

See `strelaysrv -help` for other options, such as rate limits, timeout intervals, etc.

Other items available in this repo
----
##### testutil
A test utility which can be used to test the connectivity of a relay server.
You need to generate two x509 key pairs (key.pem and cert.pem), one for the client and one for the server, in separate directories.
Afterwards, start the client:
```bash
./testutil -relay="relay://192.0.2.1:22067" -keys=certs/client/ -join
```

This prints out the client ID:
```
2015/09/21 23:00:52 main.go:42: ID: BG2C5ZA-W7XPFDO-LH222Z6-65F3HJX-ADFTGRT-3SBFIGM-KV26O2Q-E5RMRQ2
```

In the other terminal run the following:

```bash
 ./testutil -relay="relay://192.0.2.1:22067" -keys=certs/server/ -connect=BG2C5ZA-W7XPFDO-LH222Z6-65F3HJX-ADFTGRT-3SBFIGM-KV26O2Q-E5RMRQ2
```

Which should then give you an interactive prompt, where you can type things in one terminal, and they get relayed to the other terminal.

Relay related libraries used by this repo
----
##### Relay protocol definition.

[Available here](https://github.com/syncthing/syncthing/tree/main/lib/relay/protocol)


##### Relay client

Only used by the testutil.

[Available here](https://github.com/syncthing/syncthing/tree/main/lib/relay/client)
