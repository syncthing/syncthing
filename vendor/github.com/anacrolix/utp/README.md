# utp
[![GoDoc](https://godoc.org/github.com/anacrolix/utp?status.svg)](https://godoc.org/github.com/anacrolix/utp)
[![Build Status](https://drone.io/github.com/anacrolix/utp/status.png)](https://drone.io/github.com/anacrolix/utp/latest)

Package utp implements uTP, the micro transport protocol as used with Bittorrent. It opts for simplicity and reliability over strict adherence to the (poor) spec.

## Supported

 * Multiple uTP connections switched on a single PacketConn, including those initiated locally.
 * Raw access to the PacketConn for non-uTP purposes, like sharing the PacketConn with a DHT implementation.

## Implementation characteristics

 * Receive window size is used to limit out of order packets received.
 * There is no MTU path discovery. The minimum size is always used.
 * A fixed 64 slot selective ack window is used in both sending and receiving.
 * All received non-ACK packets are ACKed in response.

Patches welcomed.
