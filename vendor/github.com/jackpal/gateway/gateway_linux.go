package gateway

import (
	"net"
	"os/exec"
)

func DiscoverGateway() (ip net.IP, err error) {
	ip, err = discoverGatewayUsingRoute()
	if err != nil {
		ip, err = discoverGatewayUsingIp()
	}
	return
}

func discoverGatewayUsingIp() (net.IP, error) {
	routeCmd := exec.Command("ip", "route", "show")
	output, err := routeCmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	return parseLinuxIPRoute(output)
}

func discoverGatewayUsingRoute() (net.IP, error) {
	routeCmd := exec.Command("route", "-n")
	output, err := routeCmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	return parseLinuxRoute(output)
}
