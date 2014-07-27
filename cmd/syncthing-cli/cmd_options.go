package main

import (
	"fmt"
	"github.com/AudriusButkevicius/cli"
	"strings"
)

var optionsCommand = cli.Command{
	Name:     "options",
	HideHelp: true,
	Usage:    "Options command group",
	Subcommands: []cli.Command{
		{
			Name:     "dump",
			Usage:    "Show all Syncthing option settings",
			Requires: &cli.Requires{},
			Action: func(c *cli.Context) {
				cfg := getConfig(c).Options
				writer := newTableWriter()

				fmt.Fprintln(writer, "Sync protocol listen addresses:\t", strings.Join(cfg.ListenAddress, " "), "\t(address)")
				fmt.Fprintln(writer, "Global discovery enabled:\t", cfg.GlobalAnnEnabled, "\t(globalannenabled)")
				fmt.Fprintln(writer, "Global discovery server:\t", cfg.GlobalAnnServer, "\t(globalannserver)")

				fmt.Fprintln(writer, "Local discovery enabled:\t", cfg.LocalAnnEnabled, "\t(localannenabled)")
				fmt.Fprintln(writer, "Local discovery port:\t", cfg.LocalAnnPort, "\t(localannport)")

				fmt.Fprintln(writer, "Maximum outstanding requests:\t", cfg.ParallelRequests, "\t(requests)")
				fmt.Fprintln(writer, "Maximum file change rate in KiB/s:\t", cfg.MaxChangeKbps, "\t(maxchange)")
				fmt.Fprintln(writer, "Outgoing rate limit in KiB/s:\t", cfg.MaxSendKbps, "\t(maxsend)")
				fmt.Fprintln(writer, "Rescan interval in seconds:\t", cfg.RescanIntervalS, "\t(rescan)")
				fmt.Fprintln(writer, "Reconnect interval in seconds:\t", cfg.ReconnectIntervalS, "\t(reconnect)")
				fmt.Fprintln(writer, "Start browser:\t", cfg.StartBrowser, "\t(browser)")
				fmt.Fprintln(writer, "Enable UPnP:\t", cfg.UPnPEnabled, "\t(upnp)")

				reporting := "unrecognized value"
				switch cfg.URAccepted {
				case -1:
					reporting = "false"
				case 0:
					reporting = "undecided/false"
				case 1:
					reporting = "true"
				}
				fmt.Fprintln(writer, "Anonymous usage reporting:\t", reporting, "\t(reporting)")

				writer.Flush()
			},
		},
		{
			Name:     "get",
			Usage:    "Get a Syncthing option setting",
			Requires: &cli.Requires{"setting"},
			Action: func(c *cli.Context) {
				cfg := getConfig(c).Options
				arg := c.Args()[0]
				switch strings.ToLower(arg) {
				case "address":
					fmt.Println(strings.Join(cfg.ListenAddress, "\n"))
				case "globalannenabled":
					fmt.Println(cfg.GlobalAnnEnabled)
				case "globalannserver":
					fmt.Println(cfg.GlobalAnnServer)
				case "localannenabled":
					fmt.Println(cfg.LocalAnnEnabled)
				case "localannport":
					fmt.Println(cfg.LocalAnnPort)
				case "requests":
					fmt.Println(cfg.ParallelRequests)
				case "maxsend":
					fmt.Println(cfg.MaxSendKbps)
				case "maxchange":
					fmt.Println(cfg.MaxChangeKbps)
				case "rescan":
					fmt.Println(cfg.RescanIntervalS)
				case "reconnect":
					fmt.Println(cfg.ReconnectIntervalS)
				case "browser":
					fmt.Println(cfg.StartBrowser)
				case "upnp":
					fmt.Println(cfg.UPnPEnabled)
				case "reporting":
					switch cfg.URAccepted {
					case -1:
						fmt.Println("false")
					case 0:
						fmt.Println("undecided/false")
					case 1:
						fmt.Println("true")
					default:
						fmt.Println("unknown")
					}
				default:
					die("Invalid setting: " + arg + "\nAvailable settings: address, globalannenabled, globalannserver, localannenabled, localannport, requests, maxsend, maxchange, rescan, reconnect, browser, upnp, reporting")
				}
			},
		},
		{
			Name:     "set",
			Usage:    "Set a Syncthing option setting",
			Requires: &cli.Requires{"setting", "value..."},
			Action: func(c *cli.Context) {
				config := getConfig(c)
				arg := c.Args()[0]
				val := c.Args()[1]
				switch strings.ToLower(arg) {
				case "address":
					for _, item := range c.Args().Tail() {
						validAddress(item)
					}
					config.Options.ListenAddress = c.Args().Tail()
				case "globalannenabled":
					config.Options.GlobalAnnEnabled = parseBool(val)
				case "globalannserver":
					validAddress(val)
					config.Options.GlobalAnnServer = val
				case "localannenabled":
					config.Options.LocalAnnEnabled = parseBool(val)
				case "localannport":
					config.Options.LocalAnnPort = parsePort(val)
				case "requests":
					config.Options.ParallelRequests = parseUint(val)
				case "maxsend":
					config.Options.MaxSendKbps = parseUint(val)
				case "maxchange":
					config.Options.MaxChangeKbps = parseUint(val)
				case "rescan":
					config.Options.RescanIntervalS = parseUint(val)
				case "reconnect":
					config.Options.ReconnectIntervalS = parseUint(val)
				case "browser":
					config.Options.StartBrowser = parseBool(val)
				case "upnp":
					config.Options.UPnPEnabled = parseBool(val)
				case "reporting":
					switch strings.ToLower(val) {
					case "u", "undecided", "unset":
						config.Options.URAccepted = 0
					default:
						boolvalue := parseBool(val)
						if boolvalue {
							config.Options.URAccepted = 1
						} else {
							config.Options.URAccepted = -1
						}
					}
				default:
					die("Invalid setting: " + arg + "\nAvailable settings: address, globalannenabled, globalannserver, localannenabled, localannport, requests, maxsend, maxchange, rescan, reconnect, browser, upnp, reporting")
				}
				setConfig(c, config)
			},
		},
	},
}
