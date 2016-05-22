package gateway

import (
	"bytes"
	"errors"
	"net"
)

var errNoGateway = errors.New("no gateway found")

func parseRoutePrint(output []byte) (net.IP, error) {
	// Windows route output format is always like this:
	// ===========================================================================
	// Active Routes:
	// Network Destination        Netmask          Gateway       Interface  Metric
	//           0.0.0.0          0.0.0.0      192.168.1.1    192.168.1.100     20
	// ===========================================================================
	// I'm trying to pick the active route,
	// then jump 2 lines and pick the third IP
	// Not using regex because output is quite standard from Windows XP to 8 (NEEDS TESTING)
	outputLines := bytes.Split(output, []byte("\n"))
	for idx, line := range outputLines {
		if bytes.Contains(line, []byte("Active Routes:")) {
			if len(outputLines) <= idx+2 {
				return nil, errNoGateway
			}

			ipFields := bytes.Fields(outputLines[idx+2])
			if len(ipFields) < 3 {
				return nil, errNoGateway
			}

			ip := net.ParseIP(string(ipFields[2]))
			return ip, nil
		}
	}
	return nil, errNoGateway
}
