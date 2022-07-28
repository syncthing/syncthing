// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package cli

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/AudriusButkevicius/recli"
	"github.com/pkg/errors"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/urfave/cli"
)

type configHandler struct {
	original, cfg config.Configuration
	client        APIClient
	err           error
}

func getConfigCommand(f *apiClientFactory) (cli.Command, error) {
	h := new(configHandler)
	h.client, h.err = f.getClient()
	if h.err == nil {
		h.cfg, h.err = getConfig(h.client)
	}
	h.original = h.cfg.Copy()

	// Copy the config and set the default flags
	recliCfg := recli.DefaultConfig
	recliCfg.IDTag.Name = "xml"
	recliCfg.SkipTag.Name = "json"

	commands, err := recli.New(recliCfg).Construct(&h.cfg)
	if err != nil {
		return cli.Command{}, fmt.Errorf("config reflect: %w", err)
	}

	return cli.Command{
		Name:        "config",
		HideHelp:    true,
		Usage:       "Configuration modification command group",
		Subcommands: commands,
		Before:      h.configBefore,
		After:       h.configAfter,
	}, nil
}

func (h *configHandler) configBefore(c *cli.Context) error {
	for _, arg := range c.Args() {
		if arg == "--help" || arg == "-h" {
			return nil
		}
	}
	return h.err
}

func (h *configHandler) configAfter(_ *cli.Context) error {
	if h.err != nil {
		// Error was already returned in configBefore
		return nil
	}
	if reflect.DeepEqual(h.cfg, h.original) {
		return nil
	}
	body, err := json.MarshalIndent(h.cfg, "", "  ")
	if err != nil {
		return err
	}
	resp, err := h.client.Post("system/config", string(body))
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		body, err := responseToBArray(resp)
		if err != nil {
			return err
		}
		return errors.New(string(body))
	}
	return nil
}
