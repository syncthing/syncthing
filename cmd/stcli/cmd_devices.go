// Copyright (C) 2014 Audrius Butkevičius

package main

import (
	"fmt"
	"strings"

	"github.com/AudriusButkevicius/cli"
	"github.com/syncthing/syncthing/lib/config"
)

func init() {
	cliCommands = append(cliCommands, cli.Command{
		Name:     "devices",
		HideHelp: true,
		Usage:    "Device command group",
		Subcommands: []cli.Command{
			{
				Name:     "list",
				Usage:    "List registered devices",
				Requires: &cli.Requires{},
				Action:   devicesList,
			},
			{
				Name:     "add",
				Usage:    "Add a new device",
				Requires: &cli.Requires{"device id", "device name?"},
				Action:   devicesAdd,
			},
			{
				Name:     "remove",
				Usage:    "Remove an existing device",
				Requires: &cli.Requires{"device id"},
				Action:   devicesRemove,
			},
			{
				Name:     "get",
				Usage:    "Get a property of a device",
				Requires: &cli.Requires{"device id", "property"},
				Action:   devicesGet,
			},
			{
				Name:     "set",
				Usage:    "Set a property of a device",
				Requires: &cli.Requires{"device id", "property", "value..."},
				Action:   devicesSet,
			},
		},
	})
}

func devicesList(c *cli.Context) {
	cfg := getConfig(c)
	first := true
	writer := newTableWriter()
	for _, device := range cfg.Devices {
		if !first {
			fmt.Fprintln(writer)
		}
		fmt.Fprintln(writer, "ID:\t", device.DeviceID, "\t")
		fmt.Fprintln(writer, "Name:\t", device.Name, "\t(name)")
		fmt.Fprintln(writer, "Address:\t", strings.Join(device.Addresses, " "), "\t(address)")
		fmt.Fprintln(writer, "Compression:\t", device.Compression, "\t(compression)")
		fmt.Fprintln(writer, "Certificate name:\t", device.CertName, "\t(certname)")
		fmt.Fprintln(writer, "Introducer:\t", device.Introducer, "\t(introducer)")
		first = false
	}
	writer.Flush()
}

func devicesAdd(c *cli.Context) {
	nid := c.Args()[0]
	id := parseDeviceID(nid)

	newDevice := config.DeviceConfiguration{
		DeviceID:  id,
		Name:      nid,
		Addresses: []string{"dynamic"},
	}

	if len(c.Args()) > 1 {
		newDevice.Name = c.Args()[1]
	}

	if len(c.Args()) > 2 {
		addresses := c.Args()[2:]
		for _, item := range addresses {
			if item == "dynamic" {
				continue
			}
			validAddress(item)
		}
		newDevice.Addresses = addresses
	}

	cfg := getConfig(c)
	for _, device := range cfg.Devices {
		if device.DeviceID == id {
			die("Device " + nid + " already exists")
		}
	}
	cfg.Devices = append(cfg.Devices, newDevice)
	setConfig(c, cfg)
}

func devicesRemove(c *cli.Context) {
	nid := c.Args()[0]
	id := parseDeviceID(nid)
	if nid == getMyID(c) {
		die("Cannot remove yourself")
	}
	cfg := getConfig(c)
	for i, device := range cfg.Devices {
		if device.DeviceID == id {
			last := len(cfg.Devices) - 1
			cfg.Devices[i] = cfg.Devices[last]
			cfg.Devices = cfg.Devices[:last]
			setConfig(c, cfg)
			return
		}
	}
	die("Device " + nid + " not found")
}

func devicesGet(c *cli.Context) {
	nid := c.Args()[0]
	id := parseDeviceID(nid)
	arg := c.Args()[1]
	cfg := getConfig(c)
	for _, device := range cfg.Devices {
		if device.DeviceID != id {
			continue
		}
		switch strings.ToLower(arg) {
		case "name":
			fmt.Println(device.Name)
		case "address":
			fmt.Println(strings.Join(device.Addresses, "\n"))
		case "compression":
			fmt.Println(device.Compression.String())
		case "certname":
			fmt.Println(device.CertName)
		case "introducer":
			fmt.Println(device.Introducer)
		default:
			die("Invalid property: " + arg + "\nAvailable properties: name, address, compression, certname, introducer")
		}
		return
	}
	die("Device " + nid + " not found")
}

func devicesSet(c *cli.Context) {
	nid := c.Args()[0]
	id := parseDeviceID(nid)
	arg := c.Args()[1]
	config := getConfig(c)
	for i, device := range config.Devices {
		if device.DeviceID != id {
			continue
		}
		switch strings.ToLower(arg) {
		case "name":
			config.Devices[i].Name = strings.Join(c.Args()[2:], " ")
		case "address":
			for _, item := range c.Args()[2:] {
				if item == "dynamic" {
					continue
				}
				validAddress(item)
			}
			config.Devices[i].Addresses = c.Args()[2:]
		case "compression":
			err := config.Devices[i].Compression.UnmarshalText([]byte(c.Args()[2]))
			die(err)
		case "certname":
			config.Devices[i].CertName = strings.Join(c.Args()[2:], " ")
		case "introducer":
			config.Devices[i].Introducer = parseBool(c.Args()[2])
		default:
			die("Invalid property: " + arg + "\nAvailable properties: name, address, compression, certname, introducer")
		}
		setConfig(c, config)
		return
	}
	die("Device " + nid + " not found")
}
