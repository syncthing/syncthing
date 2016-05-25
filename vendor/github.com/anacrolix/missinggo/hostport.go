package missinggo

import (
	"net"
	"strconv"
)

func ParseHostPort(hostport string) (host string, port int, err error) {
	host, portStr, err := net.SplitHostPort(hostport)
	if err != nil {
		return
	}
	port64, err := strconv.ParseInt(portStr, 0, 0)
	if err != nil {
		return
	}
	port = int(port64)
	return
}
