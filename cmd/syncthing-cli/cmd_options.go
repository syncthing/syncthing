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

				fmt.Fprint(writer, "Sync protocol listen addresses:\t")
				for _, address := range cfg.ListenAddress {
					fmt.Fprintf(writer, "%s ", address)
				}
				fmt.Fprintf(writer, "\t(address)\n")

				fmt.Fprintf(writer, "Global discovery enabled:\t%t\t(globalannenabled)\n", cfg.GlobalAnnEnabled)
				fmt.Fprintf(writer, "Global discovery server:\t%s\t(globalannserver)\n", cfg.GlobalAnnServer)

				fmt.Fprintf(writer, "Local discovery enabled:\t%t\t(localannenabled)\n", cfg.LocalAnnEnabled)
				fmt.Fprintf(writer, "Local discovery port:\t%d\t(localannport)\n", cfg.LocalAnnPort)

				fmt.Fprintf(writer, "Maximum outstanding requests:\t%d\t(requests)\n", cfg.ParallelRequests)
				fmt.Fprintf(writer, "Maximum file change rate in KiB/s:\t%d\t(maxchange)\n", cfg.MaxChangeKbps)
				fmt.Fprintf(writer, "Outgoing rate limit in KiB/s:\t%d\t(maxsend)\n", cfg.MaxSendKbps)
				fmt.Fprintf(writer, "Rescan interval in seconds:\t%d\t(rescan)\n", cfg.RescanIntervalS)
				fmt.Fprintf(writer, "Reconnect interval in seconds:\t%d\t(reconnect)\n", cfg.ReconnectIntervalS)
				fmt.Fprintf(writer, "Start browser:\t%t\t(browser)\n", cfg.StartBrowser)
				fmt.Fprintf(writer, "Enable UPnP:\t%t\t(upnp)\n", cfg.UPnPEnabled)
				fmt.Fprint(writer, "Anonymous usage reporting:\t")
				switch cfg.URAccepted {
				case -1:
					fmt.Fprint(writer, "false")
				case 0:
					fmt.Fprint(writer, "undecided/false")
				case 1:
					fmt.Fprint(writer, "true")
				default:
					fmt.Fprint(writer, "unrecognized value")
				}
				fmt.Fprint(writer, "\t(reporting)\n")
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
					for _, item := range cfg.ListenAddress {
						fmt.Println(item)
					}
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
