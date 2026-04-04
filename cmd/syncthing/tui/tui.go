// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package tui

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/locations"
)

// CLI is the kong command struct for `syncthing tui`.
type CLI struct {
	GUIAddress string `name:"gui-address" env:"STGUIADDRESS" help:"Override GUI address (e.g. http://127.0.0.1:8384)"`
	GUIAPIKey  string `name:"gui-apikey" env:"STGUIAPIKEY" help:"Override GUI API key"`
}

func (c *CLI) Run() error {
	var client *Client
	if c.GUIAddress != "" || c.GUIAPIKey != "" {
		if c.GUIAddress == "" || c.GUIAPIKey == "" {
			return fmt.Errorf("both --gui-address and --gui-apikey must be specified")
		}
		guiCfg := config.GUIConfiguration{
			RawAddress: c.GUIAddress,
			APIKey:     c.GUIAPIKey,
		}
		client = newClientFromGUI(guiCfg)
	} else {
		var err error
		client, err = NewClientFromConfig()
		if err != nil {
			return fmt.Errorf("connecting to Syncthing: %w\n\nMake sure Syncthing is running, or specify --gui-address and --gui-apikey", err)
		}
	}

	// Quick health check
	if err := client.Ping(); err != nil {
		return fmt.Errorf("cannot reach Syncthing at %s: %w\n\nMake sure Syncthing is running, e.g.:\n  systemctl start syncthing@$USER\n  syncthing serve", client.baseURL, err)
	}

	// On Unix-like systems, check if the GUI is listening on TCP and offer
	// to switch to a unix socket for better multi-user security.
	if client, err := offerSocketSwitch(client); err != nil {
		return err
	} else {
		app := newApp(client)
		p := tea.NewProgram(app)
		_, err := p.Run()
		return err
	}
}

// noSocketPromptFile is a marker file that suppresses the unix socket prompt.
const noSocketPromptFile = "no-socket-prompt"

func noSocketPromptPath() string {
	return filepath.Join(locations.GetBaseDir(locations.ConfigBaseDir), noSocketPromptFile)
}

// offerSocketSwitch checks whether the daemon is listening on TCP and, on
// Unix-like systems, offers to switch to a unix socket for security. Returns
// the (possibly new) client to use.
func offerSocketSwitch(client *Client) (*Client, error) {
	if build.IsWindows {
		return client, nil
	}

	// User previously chose "don't ask again"
	if _, err := os.Stat(noSocketPromptPath()); err == nil {
		return client, nil
	}

	guiCfg, err := client.GUIConfigGet()
	if err != nil || guiCfg.Network() == "unix" {
		return client, nil // already on a socket, or can't check
	}

	socketPath := defaultSocketPath()
	if socketPath == "" {
		return client, nil // can't determine a safe path
	}

	fmt.Println("WARNING: The Syncthing GUI is listening on a TCP port (" + guiCfg.Address() + ").")
	fmt.Println("On multi-user systems, any local user can attempt to connect to this port.")
	fmt.Println()
	fmt.Println("For better security, the TUI can switch the daemon to a unix socket:")
	fmt.Println("  " + socketPath + " (permissions 0700, owner-only access)")
	fmt.Println("This disables browser access to the web UI. The TUI and CLI will")
	fmt.Println("continue to work normally.")
	fmt.Println()
	fmt.Println("  y = switch to unix socket (daemon will restart)")
	fmt.Println("  n = keep current setup")
	fmt.Println("  d = keep current setup and don't ask again")
	fmt.Print("\nSwitch to unix socket? [y/n/d] ")

	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))

	switch answer {
	case "d":
		if err := os.WriteFile(noSocketPromptPath(), []byte("User chose not to switch to unix socket.\n"), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not save preference: %v\n", err)
		}
		return client, nil

	case "y":
		if err := switchToSocket(client, socketPath); err != nil {
			return nil, err
		}
		fmt.Println("\nGUI switched to unix socket: " + socketPath)
		fmt.Println("The daemon will restart. Run `syncthing tui` again to connect.")
		os.Exit(0)
		return nil, nil // unreachable

	default:
		return client, nil
	}
}

// defaultSocketPath returns the preferred unix socket path for the current
// user, or "" if one cannot be determined.
func defaultSocketPath() string {
	// Prefer XDG_RUNTIME_DIR (typically /run/user/<UID>), which is
	// per-user, tmpfs, and already mode 0700.
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "syncthing", "gui.sock")
	}
	// Fallback: put the socket next to the config file.
	if base := locations.GetBaseDir(locations.ConfigBaseDir); base != "" {
		return filepath.Join(base, "gui.sock")
	}
	return ""
}

func switchToSocket(client *Client, socketPath string) error {
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o700); err != nil {
		return fmt.Errorf("creating socket directory: %w", err)
	}

	patch := config.GUIConfiguration{
		RawAddress:               socketPath,
		RawUnixSocketPermissions: "0700",
	}
	if err := client.GUIConfigPatch(patch); err != nil {
		return fmt.Errorf("updating GUI config: %w", err)
	}

	// Restart the daemon so it picks up the new address.
	// The connection may drop before we get a response — that's fine.
	_ = client.Restart()
	return nil
}
