package gateway

import (
	"bytes"
	"io/ioutil"
	"net"
	"os/exec"
)

func DiscoverGateway() (ip net.IP, err error) {
	routeCmd := exec.Command("/sbin/route", "-n", "get", "0.0.0.0")
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

	// Darwin route out format is always like this:
	//    route to: default
	// destination: default
	//        mask: default
	//     gateway: 192.168.1.1
	outputLines := bytes.Split(output, []byte("\n"))
	for _, line := range outputLines {
		if bytes.Contains(line, []byte("gateway:")) {
			gatewayFields := bytes.Fields(line)
			ip = net.ParseIP(string(gatewayFields[1]))
			break
		}
	}

	err = routeCmd.Wait()
	return
}
