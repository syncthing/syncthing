go-nat-pmp
==========

A Go language client for the NAT-PMP internet protocol for port mapping and discovering the external
IP address of a firewall.

NAT-PMP is supported by Apple brand routers and open source routers like Tomato and DD-WRT.

See http://tools.ietf.org/html/draft-cheshire-nat-pmp-03

Get the package
---------------

    go get -u github.com/jackpal/go-nat-pmp

Usage
-----

    import natpmp "github.com/jackpal/go-nat-pmp"

    client := natpmp.NewClient(gatewayIP)
    response, err := client.GetExternalAddress()
    if err != nil {
        return
    }
    print("External IP address:", response.ExternalIPAddress)

Notes
-----

There doesn't seem to be an easy way to programmatically determine the address of the default gateway.
(Linux and OSX have a "routes" kernel API that can be examined to get this information, but there is
no Go package for getting this information.)

Clients
-------

This library is used in the Taipei Torrent BitTorrent client http://github.com/jackpal/Taipei-Torrent

Complete documentation
----------------------

    http://godoc.org/github.com/jackpal/go-nat-pmp

License
-------

This project is licensed under the Apache License 2.0.
