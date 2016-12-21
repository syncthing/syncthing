// Copyright (C) 2014 Audrius Butkevičius

package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AudriusButkevicius/cli"
)

func init() {
	cliCommands = append(cliCommands, cli.Command{
		Name:     "errors",
		HideHelp: true,
		Usage:    "Error command group",
		Subcommands: []cli.Command{
			{
				Name:     "show",
				Usage:    "Show pending errors",
				Requires: &cli.Requires{},
				Action:   errorsShow,
			},
			{
				Name:     "push",
				Usage:    "Push an error to active clients",
				Requires: &cli.Requires{"error message..."},
				Action:   errorsPush,
			},
			{
				Name:     "clear",
				Usage:    "Clear pending errors",
				Requires: &cli.Requires{},
				Action:   wrappedHTTPPost("system/error/clear"),
			},
		},
	})
}

func errorsShow(c *cli.Context) {
	response := httpGet(c, "system/error")
	var data map[string][]map[string]interface{}
	json.Unmarshal(responseToBArray(response), &data)
	writer := newTableWriter()
	for _, item := range data["errors"] {
		time := item["time"].(string)[:19]
		time = strings.Replace(time, "T", " ", 1)
		err := item["error"].(string)
		err = strings.TrimSpace(err)
		fmt.Fprintln(writer, time+":\t"+err)
	}
	writer.Flush()
}

func errorsPush(c *cli.Context) {
	err := strings.Join(c.Args(), " ")
	response := httpPost(c, "system/error", strings.TrimSpace(err))
	if response.StatusCode != 200 {
		err = fmt.Sprint("Failed to push error\nStatus code: ", response.StatusCode)
		body := string(responseToBArray(response))
		if body != "" {
			err += "\nBody: " + body
		}
		die(err)
	}
}
