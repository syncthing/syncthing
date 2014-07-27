package main

import (
	"fmt"
	"github.com/AudriusButkevicius/cli"
	"github.com/calmh/syncthing/config"
	"strings"
)

var nodeCommand = cli.Command{
	Name:     "nodes",
	HideHelp: true,
	Usage:    "Node command group",
	Subcommands: []cli.Command{
		{
			Name:     "list",
			Usage:    "List registered nodes",
			Requires: &cli.Requires{},
			Action: func(c *cli.Context) {
				cfg := getConfig(c)
				first := true
				writer := newTableWriter()
				for _, node := range cfg.Nodes {
					if !first {
						fmt.Fprintln(writer)
					}
					fmt.Fprintln(writer, "ID:\t", node.NodeID, "\t")
					fmt.Fprintln(writer, "Name:\t", node.Name, "\t(name)")
					fmt.Fprintln(writer, "Address:\t", strings.Join(node.Addresses, " "), "\t(address)")
					first = false
				}
				writer.Flush()
			},
		},
		{
			Name:     "add",
			Usage:    "Add a new node",
			Requires: &cli.Requires{"node id", "node name?"},
			Action: func(c *cli.Context) {
				nid := c.Args()[0]
				id := parseNodeID(nid)

				newNode := config.NodeConfiguration{
					NodeID:    id,
					Name:      nid,
					Addresses: []string{"dynamic"},
				}

				if len(c.Args()) > 1 {
					newNode.Name = c.Args()[1]
				}

				if len(c.Args()) > 2 {
					addresses := c.Args()[2:]
					for _, item := range addresses {
						if item == "dynamic" {
							continue
						}
						validAddress(item)
					}
					newNode.Addresses = addresses
				}

				cfg := getConfig(c)
				for _, node := range cfg.Nodes {
					if node.NodeID == id {
						die("Node " + nid + " already exists")
					}
				}
				cfg.Nodes = append(cfg.Nodes, newNode)
				setConfig(c, cfg)
			},
		},
		{
			Name:     "remove",
			Usage:    "Remove an existing node",
			Requires: &cli.Requires{"node id"},
			Action: func(c *cli.Context) {
				nid := c.Args()[0]
				id := parseNodeID(nid)
				if nid == getMyID(c) {
					die("Cannot remove yourself")
				}
				cfg := getConfig(c)
				for i, node := range cfg.Nodes {
					if node.NodeID == id {
						last := len(cfg.Nodes) - 1
						cfg.Nodes[i] = cfg.Nodes[last]
						cfg.Nodes = cfg.Nodes[:last]
						setConfig(c, cfg)
						return
					}
				}
				die("Node " + nid + " not found")
			},
		},
		{
			Name:     "get",
			Usage:    "Get a property of a node",
			Requires: &cli.Requires{"node id", "property"},
			Action: func(c *cli.Context) {
				nid := c.Args()[0]
				id := parseNodeID(nid)
				arg := c.Args()[1]
				cfg := getConfig(c)
				for _, node := range cfg.Nodes {
					if node.NodeID != id {
						continue
					}
					switch strings.ToLower(arg) {
					case "name":
						fmt.Println(node.Name)
					case "address":
						fmt.Println(strings.Join(node.Addresses, "\n"))
					default:
						die("Invalid property: " + arg + "\nAvailable properties: name, address")
					}
					return
				}
				die("Node " + nid + " not found")
			},
		},
		{
			Name:     "set",
			Usage:    "Set a property of a node",
			Requires: &cli.Requires{"node id", "property", "value..."},
			Action: func(c *cli.Context) {
				nid := c.Args()[0]
				id := parseNodeID(nid)
				arg := c.Args()[1]
				config := getConfig(c)
				for i, node := range config.Nodes {
					if node.NodeID != id {
						continue
					}
					switch strings.ToLower(arg) {
					case "name":
						config.Nodes[i].Name = strings.Join(c.Args()[2:], " ")
					case "address":
						for _, item := range c.Args()[2:] {
							if item == "dynamic" {
								continue
							}
							validAddress(item)
						}
						config.Nodes[i].Addresses = c.Args()[2:]
					default:
						die("Invalid property: " + arg + "\nAvailable properties: name, address")
					}
					setConfig(c, config)
					return
				}
				die("Node " + nid + " not found")
			},
		},
	},
}
