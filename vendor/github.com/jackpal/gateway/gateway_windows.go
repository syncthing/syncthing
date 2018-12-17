package gateway

import (
	"net"
	"os/exec"
)

func DiscoverGateway() (ip net.IP, err error) {
	routeCmd := exec.Command("route", "print", "0.0.0.0")
	output, err := routeCmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	return parseWindowsRoutePrint(output)
}
