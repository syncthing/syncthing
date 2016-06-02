package missinggo

import (
	"net"
	"strconv"
)

// Extracts the port as an integer from an address string.
func AddrPort(addr net.Addr) int {
	switch raw := addr.(type) {
	case *net.UDPAddr:
		return raw.Port
	default:
		_, port, err := net.SplitHostPort(addr.String())
		if err != nil {
			panic(err)
		}
		i64, err := strconv.ParseInt(port, 0, 0)
		if err != nil {
			panic(err)
		}
		return int(i64)
	}
}

func AddrIP(addr net.Addr) net.IP {
	switch raw := addr.(type) {
	case *net.UDPAddr:
		return raw.IP
	case *net.TCPAddr:
		return raw.IP
	default:
		host, _, err := net.SplitHostPort(addr.String())
		if err != nil {
			panic(err)
		}
		return net.ParseIP(host)
	}
}
