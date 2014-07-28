package main

import (
	"encoding/json"
	"fmt"
	"github.com/AudriusButkevicius/cli"
	"strings"
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
				Action:   wrappedHttpPost("error/clear"),
			},
		},
	})
}

func errorsShow(c *cli.Context) {
	response := httpGet(c, "errors")
	var data []map[string]interface{}
	json.Unmarshal(responseToBArray(response), &data)
	writer := newTableWriter()
	for _, item := range data {
		time := item["Time"].(string)[:19]
		time = strings.Replace(time, "T", " ", 1)
		err := item["Error"].(string)
		err = strings.TrimSpace(err)
		fmt.Fprintln(writer, time+":\t"+err)
	}
	writer.Flush()
}

func errorsPush(c *cli.Context) {
	err := strings.Join(c.Args(), " ")
	response := httpPost(c, "error", strings.TrimSpace(err))
	if response.StatusCode != 200 {
		err = fmt.Sprint("Failed to push error\nStatus code: ", response.StatusCode)
		body := string(responseToBArray(response))
		if body != "" {
			err += "\nBody: " + body
		}
		die(err)
	}
}
