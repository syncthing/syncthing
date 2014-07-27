package main

import (
	"fmt"
	"github.com/AudriusButkevicius/cli"
	"strings"
)

var guiCommand = cli.Command{
	Name:     "gui",
	HideHelp: true,
	Usage:    "GUI command group",
	Subcommands: []cli.Command{
		{
			Name:     "dump",
			Usage:    "Show all GUI configuration settings",
			Requires: &cli.Requires{},
			Action: func(c *cli.Context) {
				cfg := getConfig(c).GUI
				writer := newTableWriter()
				fmt.Fprintf(writer, "Enabled:\t%t\t(enabled)\n", cfg.Enabled)
				fmt.Fprintf(writer, "Use HTTPS:\t%t\t(tls)\n", cfg.UseTLS)
				fmt.Fprintf(writer, "Listen Addresses:\t%s\t(address)\n", cfg.Address)
				if cfg.User != "" {
					fmt.Fprintf(writer, "Authentication User:\t%s\t(username)\n", cfg.User)
					fmt.Fprintf(writer, "Authentication Password:\t%s\t(password)\n", cfg.Password)
				}
				if cfg.APIKey != "" {
					fmt.Fprintf(writer, "API Key:\t%s\t(apikey)\n", cfg.APIKey)
				}
				writer.Flush()
			},
		},
		{
			Name:     "get",
			Usage:    "Get a GUI configuration setting",
			Requires: &cli.Requires{"setting"},
			Action: func(c *cli.Context) {
				cfg := getConfig(c).GUI
				arg := c.Args()[0]
				switch strings.ToLower(arg) {
				case "enabled":
					fmt.Println(cfg.Enabled)
				case "tls":
					fmt.Println(cfg.UseTLS)
				case "address":
					fmt.Println(cfg.Address)
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
			},
		},
		{
			Name:     "set",
			Usage:    "Set a GUI configuration setting",
			Requires: &cli.Requires{"setting", "value"},
			Action: func(c *cli.Context) {
				cfg := getConfig(c)
				arg := c.Args()[0]
				val := c.Args()[1]
				switch strings.ToLower(arg) {
				case "enabled":
					cfg.GUI.Enabled = parseBool(val)
				case "tls":
					cfg.GUI.UseTLS = parseBool(val)
				case "address":
					validAddress(val)
					cfg.GUI.Address = val
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
			},
		},
		{
			Name:     "unset",
			Usage:    "Unset a GUI configuration setting",
			Requires: &cli.Requires{"setting"},
			Action: func(c *cli.Context) {
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
			},
		},
	},
}
