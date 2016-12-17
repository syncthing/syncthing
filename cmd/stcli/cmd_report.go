// Copyright (C) 2014 Audrius Butkevičius

package main

import (
	"encoding/json"
	"fmt"

	"github.com/AudriusButkevicius/cli"
)

func init() {
	cliCommands = append(cliCommands, cli.Command{
		Name:     "report",
		HideHelp: true,
		Usage:    "Reporting command group",
		Subcommands: []cli.Command{
			{
				Name:     "system",
				Usage:    "Report system state",
				Requires: &cli.Requires{},
				Action:   reportSystem,
			},
			{
				Name:     "connections",
				Usage:    "Report about connections to other devices",
				Requires: &cli.Requires{},
				Action:   reportConnections,
			},
			{
				Name:     "usage",
				Usage:    "Usage report",
				Requires: &cli.Requires{},
				Action:   reportUsage,
			},
		},
	})
}

func reportSystem(c *cli.Context) {
	response := httpGet(c, "system/status")
	data := make(map[string]interface{})
	json.Unmarshal(responseToBArray(response), &data)
	prettyPrintJSON(data)
}

func reportConnections(c *cli.Context) {
	response := httpGet(c, "system/connections")
	data := make(map[string]map[string]interface{})
	json.Unmarshal(responseToBArray(response), &data)
	var overall map[string]interface{}
	for key, value := range data {
		if key == "total" {
			overall = value
			continue
		}
		value["Device ID"] = key
		prettyPrintJSON(value)
		fmt.Println()
	}
	if overall != nil {
		fmt.Println("=== Overall statistics ===")
		prettyPrintJSON(overall)
	}
}

func reportUsage(c *cli.Context) {
	response := httpGet(c, "svc/report")
	report := make(map[string]interface{})
	json.Unmarshal(responseToBArray(response), &report)
	prettyPrintJSON(report)
}
