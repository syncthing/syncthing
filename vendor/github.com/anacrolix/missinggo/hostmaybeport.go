package missinggo

import (
	"net"
	"strconv"
	"strings"
)

// Represents a split host port.
type HostMaybePort struct {
	Host   string // Just the host, with no port.
	Port   int    // The port if NoPort is false.
	NoPort bool   // Whether a port is specified.
	Err    error  // The error returned from net.SplitHostPort.
}

func (me *HostMaybePort) String() string {
	if me.NoPort {
		return me.Host
	}
	return net.JoinHostPort(me.Host, strconv.FormatInt(int64(me.Port), 10))
}

// Parse a "hostport" string, a concept that floats around the stdlib a lot
// and is painful to work with. If no port is present, what's usually present
// is just the host.
func SplitHostMaybePort(hostport string) HostMaybePort {
	host, portStr, err := net.SplitHostPort(hostport)
	if err != nil {
		if strings.Contains(err.Error(), "missing port") {
			return HostMaybePort{
				Host:   hostport,
				NoPort: true,
			}
		}
		return HostMaybePort{
			Err: err,
		}
	}
	portI64, err := strconv.ParseInt(portStr, 0, 0)
	if err != nil {
		return HostMaybePort{
			Host: host,
			Port: -1,
			Err:  err,
		}
	}
	return HostMaybePort{
		Host: host,
		Port: int(portI64),
	}
}
