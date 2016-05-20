package gateway

import (
	"bytes"
	"io/ioutil"
	"net"
	"os/exec"
)

func DiscoverGateway() (ip net.IP, err error) {
	routeCmd := exec.Command("route", "print", "0.0.0.0")
	stdOut, err := routeCmd.StdoutPipe()
	if err != nil {
		return
	}
	if err = routeCmd.Start(); err != nil {
		return
	}
	output, err := ioutil.ReadAll(stdOut)
	if err != nil {
		return
	}

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
			ipFields := bytes.Fields(outputLines[idx+2])
			ip = net.ParseIP(string(ipFields[2]))
			break
		}
	}
	err = routeCmd.Wait()
	return
}
