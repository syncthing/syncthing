// Copyright (C) 2014 Audrius Butkevičius

package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/AudriusButkevicius/cli"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/fs"
)

func init() {
	cliCommands = append(cliCommands, cli.Command{
		Name:     "folders",
		HideHelp: true,
		Usage:    "Folder command group",
		Subcommands: []cli.Command{
			{
				Name:     "list",
				Usage:    "List available folders",
				Requires: &cli.Requires{},
				Action:   foldersList,
			},
			{
				Name:     "add",
				Usage:    "Add a new folder",
				Requires: &cli.Requires{"folder id", "directory"},
				Action:   foldersAdd,
			},
			{
				Name:     "remove",
				Usage:    "Remove an existing folder",
				Requires: &cli.Requires{"folder id"},
				Action:   foldersRemove,
			},
			{
				Name:     "override",
				Usage:    "Override changes from other nodes for a master folder",
				Requires: &cli.Requires{"folder id"},
				Action:   foldersOverride,
			},
			{
				Name:     "get",
				Usage:    "Get a property of a folder",
				Requires: &cli.Requires{"folder id", "property"},
				Action:   foldersGet,
			},
			{
				Name:     "set",
				Usage:    "Set a property of a folder",
				Requires: &cli.Requires{"folder id", "property", "value..."},
				Action:   foldersSet,
			},
			{
				Name:     "unset",
				Usage:    "Unset a property of a folder",
				Requires: &cli.Requires{"folder id", "property"},
				Action:   foldersUnset,
			},
			{
				Name:     "devices",
				Usage:    "Folder devices command group",
				HideHelp: true,
				Subcommands: []cli.Command{
					{
						Name:     "list",
						Usage:    "List of devices which the folder is shared with",
						Requires: &cli.Requires{"folder id"},
						Action:   foldersDevicesList,
					},
					{
						Name:     "add",
						Usage:    "Share a folder with a device",
						Requires: &cli.Requires{"folder id", "device id"},
						Action:   foldersDevicesAdd,
					},
					{
						Name:     "remove",
						Usage:    "Unshare a folder with a device",
						Requires: &cli.Requires{"folder id", "device id"},
						Action:   foldersDevicesRemove,
					},
					{
						Name:     "clear",
						Usage:    "Unshare a folder with all devices",
						Requires: &cli.Requires{"folder id"},
						Action:   foldersDevicesClear,
					},
				},
			},
		},
	})
}

func foldersList(c *cli.Context) {
	cfg := getConfig(c)
	first := true
	writer := newTableWriter()
	for _, folder := range cfg.Folders {
		if !first {
			fmt.Fprintln(writer)
		}
		fs := folder.Filesystem()
		fmt.Fprintln(writer, "ID:\t", folder.ID, "\t")
		fmt.Fprintln(writer, "Path:\t", fs.URI(), "\t(directory)")
		fmt.Fprintln(writer, "Path type:\t", fs.Type(), "\t(directory-type)")
		fmt.Fprintln(writer, "Folder type:\t", folder.Type, "\t(type)")
		fmt.Fprintln(writer, "Ignore permissions:\t", folder.IgnorePerms, "\t(permissions)")
		fmt.Fprintln(writer, "Rescan interval in seconds:\t", folder.RescanIntervalS, "\t(rescan)")

		if folder.Versioning.Type != "" {
			fmt.Fprintln(writer, "Versioning:\t", folder.Versioning.Type, "\t(versioning)")
			for key, value := range folder.Versioning.Params {
				fmt.Fprintf(writer, "Versioning %s:\t %s \t(versioning-%s)\n", key, value, key)
			}
		}
		first = false
	}
	writer.Flush()
}

func foldersAdd(c *cli.Context) {
	cfg := getConfig(c)
	abs, err := filepath.Abs(c.Args()[1])
	die(err)
	folder := config.FolderConfiguration{
		ID:             c.Args()[0],
		Path:           filepath.Clean(abs),
		FilesystemType: fs.FilesystemTypeBasic,
	}
	cfg.Folders = append(cfg.Folders, folder)
	setConfig(c, cfg)
}

func foldersRemove(c *cli.Context) {
	cfg := getConfig(c)
	rid := c.Args()[0]
	for i, folder := range cfg.Folders {
		if folder.ID == rid {
			last := len(cfg.Folders) - 1
			cfg.Folders[i] = cfg.Folders[last]
			cfg.Folders = cfg.Folders[:last]
			setConfig(c, cfg)
			return
		}
	}
	die("Folder " + rid + " not found")
}

func foldersOverride(c *cli.Context) {
	cfg := getConfig(c)
	rid := c.Args()[0]
	for _, folder := range cfg.Folders {
		if folder.ID == rid && folder.Type == config.FolderTypeSendOnly {
			response := httpPost(c, "db/override", "")
			if response.StatusCode != 200 {
				err := fmt.Sprint("Failed to override changes\nStatus code: ", response.StatusCode)
				body := string(responseToBArray(response))
				if body != "" {
					err += "\nBody: " + body
				}
				die(err)
			}
			return
		}
	}
	die("Folder " + rid + " not found or folder not master")
}

func foldersGet(c *cli.Context) {
	cfg := getConfig(c)
	rid := c.Args()[0]
	arg := strings.ToLower(c.Args()[1])
	for _, folder := range cfg.Folders {
		if folder.ID != rid {
			continue
		}
		if strings.HasPrefix(arg, "versioning-") {
			arg = arg[11:]
			value, ok := folder.Versioning.Params[arg]
			if ok {
				fmt.Println(value)
				return
			}
			die("Versioning property " + c.Args()[1][11:] + " not found")
		}
		switch arg {
		case "directory":
			fmt.Println(folder.Filesystem().URI())
		case "directory-type":
			fmt.Println(folder.Filesystem().Type())
		case "type":
			fmt.Println(folder.Type)
		case "permissions":
			fmt.Println(folder.IgnorePerms)
		case "rescan":
			fmt.Println(folder.RescanIntervalS)
		case "versioning":
			if folder.Versioning.Type != "" {
				fmt.Println(folder.Versioning.Type)
			}
		default:
			die("Invalid property: " + c.Args()[1] + "\nAvailable properties: directory, directory-type, type, permissions, versioning, versioning-<key>")
		}
		return
	}
	die("Folder " + rid + " not found")
}

func foldersSet(c *cli.Context) {
	rid := c.Args()[0]
	arg := strings.ToLower(c.Args()[1])
	val := strings.Join(c.Args()[2:], " ")
	cfg := getConfig(c)
	for i, folder := range cfg.Folders {
		if folder.ID != rid {
			continue
		}
		if strings.HasPrefix(arg, "versioning-") {
			cfg.Folders[i].Versioning.Params[arg[11:]] = val
			setConfig(c, cfg)
			return
		}
		switch arg {
		case "directory":
			cfg.Folders[i].Path = val
		case "directory-type":
			var fsType fs.FilesystemType
			fsType.UnmarshalText([]byte(val))
			cfg.Folders[i].FilesystemType = fsType
		case "type":
			var t config.FolderType
			if err := t.UnmarshalText([]byte(val)); err != nil {
				die("Invalid folder type: " + err.Error())
			}
			cfg.Folders[i].Type = t
		case "permissions":
			cfg.Folders[i].IgnorePerms = parseBool(val)
		case "rescan":
			cfg.Folders[i].RescanIntervalS = parseInt(val)
		case "versioning":
			cfg.Folders[i].Versioning.Type = val
		default:
			die("Invalid property: " + c.Args()[1] + "\nAvailable properties: directory, master, permissions, versioning, versioning-<key>")
		}
		setConfig(c, cfg)
		return
	}
	die("Folder " + rid + " not found")
}

func foldersUnset(c *cli.Context) {
	rid := c.Args()[0]
	arg := strings.ToLower(c.Args()[1])
	cfg := getConfig(c)
	for i, folder := range cfg.Folders {
		if folder.ID != rid {
			continue
		}
		if strings.HasPrefix(arg, "versioning-") {
			arg = arg[11:]
			if _, ok := folder.Versioning.Params[arg]; ok {
				delete(cfg.Folders[i].Versioning.Params, arg)
				setConfig(c, cfg)
				return
			}
			die("Versioning property " + c.Args()[1][11:] + " not found")
		}
		switch arg {
		case "versioning":
			cfg.Folders[i].Versioning.Type = ""
			cfg.Folders[i].Versioning.Params = make(map[string]string)
		default:
			die("Invalid property: " + c.Args()[1] + "\nAvailable properties: versioning, versioning-<key>")
		}
		setConfig(c, cfg)
		return
	}
	die("Folder " + rid + " not found")
}

func foldersDevicesList(c *cli.Context) {
	rid := c.Args()[0]
	cfg := getConfig(c)
	for _, folder := range cfg.Folders {
		if folder.ID != rid {
			continue
		}
		for _, device := range folder.Devices {
			fmt.Println(device.DeviceID)
		}
		return
	}
	die("Folder " + rid + " not found")
}

func foldersDevicesAdd(c *cli.Context) {
	rid := c.Args()[0]
	nid := parseDeviceID(c.Args()[1])
	cfg := getConfig(c)
	for i, folder := range cfg.Folders {
		if folder.ID != rid {
			continue
		}
		for _, device := range folder.Devices {
			if device.DeviceID == nid {
				die("Device " + c.Args()[1] + " is already part of this folder")
			}
		}
		for _, device := range cfg.Devices {
			if device.DeviceID == nid {
				cfg.Folders[i].Devices = append(folder.Devices, config.FolderDeviceConfiguration{
					DeviceID: device.DeviceID,
				})
				setConfig(c, cfg)
				return
			}
		}
		die("Device " + c.Args()[1] + " not found in device list")
	}
	die("Folder " + rid + " not found")
}

func foldersDevicesRemove(c *cli.Context) {
	rid := c.Args()[0]
	nid := parseDeviceID(c.Args()[1])
	cfg := getConfig(c)
	for ri, folder := range cfg.Folders {
		if folder.ID != rid {
			continue
		}
		for ni, device := range folder.Devices {
			if device.DeviceID == nid {
				last := len(folder.Devices) - 1
				cfg.Folders[ri].Devices[ni] = folder.Devices[last]
				cfg.Folders[ri].Devices = cfg.Folders[ri].Devices[:last]
				setConfig(c, cfg)
				return
			}
		}
		die("Device " + c.Args()[1] + " not found")
	}
	die("Folder " + rid + " not found")
}

func foldersDevicesClear(c *cli.Context) {
	rid := c.Args()[0]
	cfg := getConfig(c)
	for i, folder := range cfg.Folders {
		if folder.ID != rid {
			continue
		}
		cfg.Folders[i].Devices = []config.FolderDeviceConfiguration{}
		setConfig(c, cfg)
		return
	}
	die("Folder " + rid + " not found")
}
