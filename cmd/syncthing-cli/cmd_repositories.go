package main

import (
	"fmt"
	"github.com/AudriusButkevicius/cli"
	"github.com/calmh/syncthing/config"
	"strings"
)

var repositoryCommand = cli.Command{
	Name:     "repositories",
	HideHelp: true,
	Usage:    "Repository command group",
	Subcommands: []cli.Command{
		{
			Name:     "list",
			Usage:    "List available repositories",
			Requires: &cli.Requires{},
			Action: func(c *cli.Context) {
				cfg := getConfig(c)
				first := true
				writer := newTableWriter()
				for _, repo := range cfg.Repositories {
					if !first {
						fmt.Fprintln(writer)
					}
					fmt.Fprintln(writer, "ID:\t", repo.ID, "\t")
					fmt.Fprintln(writer, "Directory:\t", repo.Directory, "\t(directory)")
					fmt.Fprintln(writer, "Repository master:\t", repo.ReadOnly, "\t(master)")
					fmt.Fprintln(writer, "Ignore permissions:\t", repo.IgnorePerms, "\t(permissions)")

					if repo.Versioning.Type != "" {
						fmt.Fprintln(writer, "Versioning:\t", repo.Versioning.Type, "\t(versioning)")
						for key, value := range repo.Versioning.Params {
							fmt.Fprintf(writer, "Versioning %s:\t %s \t(versioning-%s)\n", key, value, key)
						}
					}
					if repo.Invalid != "" {
						fmt.Fprintln(writer, "Invalid:\t", repo.Invalid, "\t")
					}
					first = false
				}
				writer.Flush()
			},
		},
		{
			Name:     "add",
			Usage:    "Add a new repository",
			Requires: &cli.Requires{"repository id", "directory"},
			Action: func(c *cli.Context) {
				cfg := getConfig(c)
				repo := config.RepositoryConfiguration{
					ID:        c.Args()[0],
					Directory: c.Args()[1],
				}
				cfg.Repositories = append(cfg.Repositories, repo)
				setConfig(c, cfg)
			},
		},
		{
			Name:     "remove",
			Usage:    "Remove an existing repository",
			Requires: &cli.Requires{"repository id"},
			Action: func(c *cli.Context) {
				cfg := getConfig(c)
				rid := c.Args()[0]
				for i, repo := range cfg.Repositories {
					if repo.ID == rid {
						last := len(cfg.Repositories) - 1
						cfg.Repositories[i] = cfg.Repositories[last]
						cfg.Repositories = cfg.Repositories[:last]
						setConfig(c, cfg)
						return
					}
				}
				die("Repository " + rid + " not found")
			},
		},

		{
			Name:     "get",
			Usage:    "Get a property of a repository",
			Requires: &cli.Requires{"repository id", "property"},
			Action: func(c *cli.Context) {
				cfg := getConfig(c)
				rid := c.Args()[0]
				arg := strings.ToLower(c.Args()[1])
				for _, repo := range cfg.Repositories {
					if repo.ID != rid {
						continue
					}
					if strings.HasPrefix(arg, "versioning-") {
						arg = arg[11:]
						value, ok := repo.Versioning.Params[arg]
						if ok {
							fmt.Println(value)
							return
						}
						die("Versioning property " + c.Args()[1][11:] + " not found")
					}
					switch arg {
					case "directory":
						fmt.Println(repo.Directory)
					case "master":
						fmt.Println(repo.ReadOnly)
					case "permissions":
						fmt.Println(repo.IgnorePerms)
					case "versioning":
						if repo.Versioning.Type != "" {
							fmt.Println(repo.Versioning.Type)
						}
					default:
						die("Invalid property: " + c.Args()[1] + "\nAvailable properties: directory, master, permissions, versioning, versioning-<key>")
					}
					return
				}
				die("Repository " + rid + " not found")
			},
		},
		{
			Name:     "set",
			Usage:    "Set a property of a repository",
			Requires: &cli.Requires{"repository id", "property", "value..."},
			Action: func(c *cli.Context) {
				rid := c.Args()[0]
				arg := strings.ToLower(c.Args()[1])
				val := strings.Join(c.Args()[2:], " ")
				cfg := getConfig(c)
				for i, repo := range cfg.Repositories {
					if repo.ID != rid {
						continue
					}
					if strings.HasPrefix(arg, "versioning-") {
						cfg.Repositories[i].Versioning.Params[arg[11:]] = val
						setConfig(c, cfg)
						return
					}
					switch arg {
					case "directory":
						cfg.Repositories[i].Directory = val
					case "master":
						cfg.Repositories[i].ReadOnly = parseBool(val)
					case "permissions":
						cfg.Repositories[i].IgnorePerms = parseBool(val)
					case "versioning":
						cfg.Repositories[i].Versioning.Type = val
					default:
						die("Invalid property: " + c.Args()[1] + "\nAvailable properties: directory, master, permissions, versioning, versioning-<key>")
					}
					setConfig(c, cfg)
					return
				}
				die("Repository " + rid + " not found")
			},
		},
		{
			Name:     "unset",
			Usage:    "Unset a property of a repository",
			Requires: &cli.Requires{"repository id", "property"},
			Action: func(c *cli.Context) {
				rid := c.Args()[0]
				arg := strings.ToLower(c.Args()[1])
				cfg := getConfig(c)
				for i, repo := range cfg.Repositories {
					if repo.ID != rid {
						continue
					}
					if strings.HasPrefix(arg, "versioning-") {
						arg = arg[11:]
						if _, ok := repo.Versioning.Params[arg]; ok {
							delete(cfg.Repositories[i].Versioning.Params, arg)
							setConfig(c, cfg)
							return
						}
						die("Versioning property " + c.Args()[1][11:] + " not found")
					}
					switch arg {
					case "versioning":
						cfg.Repositories[i].Versioning.Type = ""
						cfg.Repositories[i].Versioning.Params = make(map[string]string)
					default:
						die("Invalid property: " + c.Args()[1] + "\nAvailable properties: versioning, versioning-<key>")
					}
					setConfig(c, cfg)
					return
				}
				die("Repository " + rid + " not found")
			},
		},
		{
			Name:     "nodes",
			Usage:    "Repository nodes command group",
			HideHelp: true,
			Subcommands: []cli.Command{
				{
					Name:     "list",
					Requires: &cli.Requires{"repository id"},
					Action: func(c *cli.Context) {
						rid := c.Args()[0]
						cfg := getConfig(c)
						for _, repo := range cfg.Repositories {
							if repo.ID != rid {
								continue
							}
							for _, node := range repo.Nodes {
								fmt.Println(node.NodeID)
							}
							return
						}
						die("Repository " + rid + " not found")
					},
				},
				{
					Name:     "add",
					Usage:    "Share a repository with a node",
					Requires: &cli.Requires{"repository id", "node id"},
					Action: func(c *cli.Context) {
						rid := c.Args()[0]
						nid := parseNodeID(c.Args()[1])
						cfg := getConfig(c)
						for i, repo := range cfg.Repositories {
							if repo.ID != rid {
								continue
							}
							for _, node := range repo.Nodes {
								if node.NodeID == nid {
									die("Node " + c.Args()[1] + " is already part of this repository")
								}
							}
							for _, node := range cfg.Nodes {
								if node.NodeID == nid {
									cfg.Repositories[i].Nodes = append(repo.Nodes, node)
									setConfig(c, cfg)
									return
								}
							}
							die("Node " + c.Args()[1] + " not found in node list")
						}
						die("Repository " + rid + " not found")
					},
				},
				{
					Name:     "remove",
					Usage:    "Unshare a repository with a node",
					Requires: &cli.Requires{"repository id", "node id"},
					Action: func(c *cli.Context) {
						rid := c.Args()[0]
						nid := parseNodeID(c.Args()[1])
						cfg := getConfig(c)
						for ri, repo := range cfg.Repositories {
							if repo.ID != rid {
								continue
							}
							for ni, node := range repo.Nodes {
								if node.NodeID == nid {
									last := len(repo.Nodes) - 1
									cfg.Repositories[ri].Nodes[ni] = repo.Nodes[last]
									cfg.Repositories[ri].Nodes = cfg.Repositories[ri].Nodes[:last]
									setConfig(c, cfg)
									return
								}
							}
							die("Node " + c.Args()[1] + " not found")
						}
						die("Repository " + rid + " not found")
					},
				},
				{
					Name:     "clear",
					Usage:    "Unshare a repository with all nodes",
					Requires: &cli.Requires{"repository id"},
					Action: func(c *cli.Context) {
						rid := c.Args()[0]
						cfg := getConfig(c)
						for i, repo := range cfg.Repositories {
							if repo.ID != rid {
								continue
							}
							cfg.Repositories[i].Nodes = []config.NodeConfiguration{}
							setConfig(c, cfg)
							return
						}
						die("Repository " + rid + " not found")
					},
				},
			},
		},
	},
}
