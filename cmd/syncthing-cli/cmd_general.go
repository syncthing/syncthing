package main

import (
	"encoding/json"
	"fmt"
	"github.com/AudriusButkevicius/cli"
)

func init() {
	cliCommands = append(cliCommands, []cli.Command{
		{
			Name:     "id",
			Usage:    "Get ID of the Syncthing client",
			Requires: &cli.Requires{},
			Action:   generalID,
		},
		{
			Name:     "status",
			Usage:    "Configuration status, whether or not a restart is required for changes to take effect",
			Requires: &cli.Requires{},
			Action:   generalStatus,
		},
		{
			Name:     "restart",
			Usage:    "Restart syncthing",
			Requires: &cli.Requires{},
			Action:   wrappedHttpPost("restart"),
		},
		{
			Name:     "shutdown",
			Usage:    "Shutdown syncthing",
			Requires: &cli.Requires{},
			Action:   wrappedHttpPost("shutdown"),
		},
		{
			Name:     "reset",
			Usage:    "Reset syncthing deleting all repositories and nodes",
			Requires: &cli.Requires{},
			Action:   wrappedHttpPost("reset"),
		},
		{
			Name:     "upgrade",
			Usage:    "Upgrade syncthing (if a newer version is available)",
			Requires: &cli.Requires{},
			Action:   wrappedHttpPost("upgrade"),
		},
		{
			Name:     "version",
			Usage:    "Syncthing client version",
			Requires: &cli.Requires{},
			Action:   generalVersion,
		},
	}...)
}

func generalID(c *cli.Context) {
	fmt.Println(getMyID(c))
}

func generalStatus(c *cli.Context) {
	response := httpGet(c, "config/sync")
	status := make(map[string]interface{})
	json.Unmarshal(responseToBArray(response), &status)
	if status["configInSync"] != true {
		die("Config out of sync")
	}
	fmt.Println("Config in sync")
}

func generalVersion(c *cli.Context) {
	response := httpGet(c, "version")
	output := string(responseToBArray(response))
	fmt.Println(output)
}
