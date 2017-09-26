// Copyright (C) 2014 Audrius Butkevičius

package main

import (
	"fmt"
	"strings"

	"github.com/syncthing/syncthing/lib/config"

	"github.com/AudriusButkevicius/cli"
)

func init() {
	cliCommands = append(cliCommands, cli.Command{
		Name:     "gui",
		HideHelp: true,
		Usage:    "GUI command group",
		Subcommands: []cli.Command{
			{
				Name:     "dump",
				Usage:    "Show all GUI configuration settings",
				Requires: &cli.Requires{},
				Action:   guiDump,
			},
			{
				Name:     "get",
				Usage:    "Get a GUI configuration setting",
				Requires: &cli.Requires{"gui-index", "setting"},
				Action:   guiGet,
			},
			{
				Name:     "set",
				Usage:    "Set a GUI configuration setting",
				Requires: &cli.Requires{"gui-index", "setting", "value"},
				Action:   guiSet,
			},
			{
				Name:     "unset",
				Usage:    "Unset a GUI configuration setting",
				Requires: &cli.Requires{"gui-index", "setting"},
				Action:   guiUnset,
			},
		},
	})
}

func guiDumpOne(guiCfg *config.GUIConfiguration) {
	writer := newTableWriter()
	fmt.Fprintln(writer, "	Enabled:\t", guiCfg.Enabled, "\t(enabled)")
	fmt.Fprintf(writer, "	Listen Address:\t", guiCfg.Address, "\t(address)")
	fmt.Fprintf(writer, "	Use HTTPS\t", guiCfg.UseTLS, "\t(tls)")
	if guiCfg.User != "" {
		fmt.Fprintln(writer, "	Authentication User:\t", guiCfg.User, "\t(username)")
		fmt.Fprintln(writer, "	Authentication Password:\t", guiCfg.Password, "\t(password)")
	}
	if guiCfg.APIKey != "" {
		fmt.Fprintln(writer, "	API Key:\t", guiCfg.APIKey, "\t(apikey)")
	}

	writer.Flush()
}

func guiDump(c *cli.Context) {
	guiCfgs := getConfig(c).GUIs()
	for _, guiCfg := range guiCfgs {
		guiDumpOne(&guiCfg)
	}
}

func guiGet(c *cli.Context) {
	guiCfgs := getConfig(c).GUIs()
	idx := c.Args()[0]
	arg := c.Args()[1]
	guiCfg := guiCfgs[parseUint(idx)]
	switch strings.ToLower(arg) {
	case "enabled":
		fmt.Println(guiCfg.Enabled)
	case "tls":
		fmt.Println(guiCfg.UseTLS)
	case "address":
		fmt.Println(guiCfg.Address, ",")
	case "user":
		if guiCfg.User != "" {
			fmt.Println(guiCfg.User)
		}
	case "password":
		if guiCfg.User != "" {
			fmt.Println(guiCfg.Password)
		}
	case "apikey":
		if guiCfg.APIKey != "" {
			fmt.Println(guiCfg.APIKey)
		}
	default:
		die("Invalid setting: " + arg + "\nAvailable settings: enabled, tls, address, user, password, apikey")
	}
}

func guiSet(c *cli.Context) {
	cfg := getConfig(c)
	idx := c.Args()[0]
	arg := c.Args()[1]
	val := c.Args()[2]
	guiCfg := &cfg.GUIs()[parseUint(idx)]
	switch strings.ToLower(arg) {
	case "enabled":
		guiCfg.Enabled = parseBool(val)
	case "tls":
		guiCfg.UseTLS = parseBool(val)
	case "address":
		guiCfg.Address = val
	case "user":
		guiCfg.User = val
	case "password":
		guiCfg.Password = val
	case "apikey":
		guiCfg.APIKey = val
	default:
		die("Invalid setting: " + arg + "\nAvailable settings: enabled, tls, address, user, password, apikey")
	}
	setConfig(c, cfg)
}

func guiUnset(c *cli.Context) {
	cfg := getConfig(c)
	idx := c.Args()[0]
	arg := c.Args()[1]
	guiCfg := &cfg.GUIs()[parseUint(idx)]
	switch strings.ToLower(arg) {
	case "user":
		guiCfg.User = ""
	case "password":
		guiCfg.Password = ""
	case "apikey":
		guiCfg.APIKey = ""
	default:
		die("Invalid setting: " + arg + "\nAvailable settings: user, password, apikey")
	}
	setConfig(c, cfg)
}
