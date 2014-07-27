package main

import (
	"encoding/json"
	"fmt"
	"github.com/AudriusButkevicius/cli"
)

var reportCommand = cli.Command{
	Name:     "report",
	HideHelp: true,
	Usage:    "Reporting command group",
	Subcommands: []cli.Command{
		{
			Name:     "system",
			Usage:    "Report system state",
			Requires: &cli.Requires{},
			Action: func(c *cli.Context) {
				response := httpGet(c, "system")
				data := make(map[string]interface{})
				json.Unmarshal(responseToBArray(response), &data)
				prettyPrintJson(data)
			},
		},
		{
			Name:     "connections",
			Usage:    "Report about connections to other nodes",
			Requires: &cli.Requires{},
			Action: func(c *cli.Context) {
				response := httpGet(c, "connections")
				data := make(map[string]map[string]interface{})
				json.Unmarshal(responseToBArray(response), &data)
				var overall map[string]interface{}
				for key, value := range data {
					if key == "total" {
						overall = value
						continue
					}
					value["Node ID"] = key
					prettyPrintJson(value)
					fmt.Println()
				}
				if overall != nil {
					fmt.Println("=== Overall statistics ===")
					prettyPrintJson(overall)
				}
			},
		},
		{
			Name:     "usage",
			Usage:    "Usage report",
			Requires: &cli.Requires{},
			Action: func(c *cli.Context) {
				response := httpGet(c, "report")
				report := make(map[string]interface{})
				json.Unmarshal(responseToBArray(response), &report)
				prettyPrintJson(report)
			},
		},
	},
}
