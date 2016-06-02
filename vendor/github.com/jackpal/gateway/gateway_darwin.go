package gateway

import (
	"net"
	"os/exec"
)

func DiscoverGateway() (net.IP, error) {
	routeCmd := exec.Command("/sbin/route", "-n", "get", "0.0.0.0")
	output, err := routeCmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	return parseDarwinRouteGet(output)
}
