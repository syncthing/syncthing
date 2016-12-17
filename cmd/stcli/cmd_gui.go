// Copyright (C) 2014 Audrius Butkevičius

package main

import (
	"fmt"
	"strings"

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
				Requires: &cli.Requires{"setting"},
				Action:   guiGet,
			},
			{
				Name:     "set",
				Usage:    "Set a GUI configuration setting",
				Requires: &cli.Requires{"setting", "value"},
				Action:   guiSet,
			},
			{
				Name:     "unset",
				Usage:    "Unset a GUI configuration setting",
				Requires: &cli.Requires{"setting"},
				Action:   guiUnset,
			},
		},
	})
}

func guiDump(c *cli.Context) {
	cfg := getConfig(c).GUI
	writer := newTableWriter()
	fmt.Fprintln(writer, "Enabled:\t", cfg.Enabled, "\t(enabled)")
	fmt.Fprintln(writer, "Use HTTPS:\t", cfg.UseTLS(), "\t(tls)")
	fmt.Fprintln(writer, "Listen Addresses:\t", cfg.Address(), "\t(address)")
	if cfg.User != "" {
		fmt.Fprintln(writer, "Authentication User:\t", cfg.User, "\t(username)")
		fmt.Fprintln(writer, "Authentication Password:\t", cfg.Password, "\t(password)")
	}
	if cfg.APIKey != "" {
		fmt.Fprintln(writer, "API Key:\t", cfg.APIKey, "\t(apikey)")
	}
	writer.Flush()
}

func guiGet(c *cli.Context) {
	cfg := getConfig(c).GUI
	arg := c.Args()[0]
	switch strings.ToLower(arg) {
	case "enabled":
		fmt.Println(cfg.Enabled)
	case "tls":
		fmt.Println(cfg.UseTLS())
	case "address":
		fmt.Println(cfg.Address())
	case "user":
		if cfg.User != "" {
			fmt.Println(cfg.User)
		}
	case "password":
		if cfg.User != "" {
			fmt.Println(cfg.Password)
		}
	case "apikey":
		if cfg.APIKey != "" {
			fmt.Println(cfg.APIKey)
		}
	default:
		die("Invalid setting: " + arg + "\nAvailable settings: enabled, tls, address, user, password, apikey")
	}
}

func guiSet(c *cli.Context) {
	cfg := getConfig(c)
	arg := c.Args()[0]
	val := c.Args()[1]
	switch strings.ToLower(arg) {
	case "enabled":
		cfg.GUI.Enabled = parseBool(val)
	case "tls":
		cfg.GUI.RawUseTLS = parseBool(val)
	case "address":
		validAddress(val)
		cfg.GUI.RawAddress = val
	case "user":
		cfg.GUI.User = val
	case "password":
		cfg.GUI.Password = val
	case "apikey":
		cfg.GUI.APIKey = val
	default:
		die("Invalid setting: " + arg + "\nAvailable settings: enabled, tls, address, user, password, apikey")
	}
	setConfig(c, cfg)
}

func guiUnset(c *cli.Context) {
	cfg := getConfig(c)
	arg := c.Args()[0]
	switch strings.ToLower(arg) {
	case "user":
		cfg.GUI.User = ""
	case "password":
		cfg.GUI.Password = ""
	case "apikey":
		cfg.GUI.APIKey = ""
	default:
		die("Invalid setting: " + arg + "\nAvailable settings: user, password, apikey")
	}
	setConfig(c, cfg)
}
