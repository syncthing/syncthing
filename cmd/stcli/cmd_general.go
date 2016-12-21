// Copyright (C) 2014 Audrius Butkevičius

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
			Action:   wrappedHTTPPost("system/restart"),
		},
		{
			Name:     "shutdown",
			Usage:    "Shutdown syncthing",
			Requires: &cli.Requires{},
			Action:   wrappedHTTPPost("system/shutdown"),
		},
		{
			Name:     "reset",
			Usage:    "Reset syncthing deleting all folders and devices",
			Requires: &cli.Requires{},
			Action:   wrappedHTTPPost("system/reset"),
		},
		{
			Name:     "upgrade",
			Usage:    "Upgrade syncthing (if a newer version is available)",
			Requires: &cli.Requires{},
			Action:   wrappedHTTPPost("system/upgrade"),
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
	response := httpGet(c, "system/config/insync")
	var status struct{ ConfigInSync bool }
	json.Unmarshal(responseToBArray(response), &status)
	if !status.ConfigInSync {
		die("Config out of sync")
	}
	fmt.Println("Config in sync")
}

func generalVersion(c *cli.Context) {
	response := httpGet(c, "system/version")
	version := make(map[string]interface{})
	json.Unmarshal(responseToBArray(response), &version)
	prettyPrintJSON(version)
}
