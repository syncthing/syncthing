// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package cli

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/urfave/cli"
)

func responseToBArray(response *http.Response) ([]byte, error) {
	bytes, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	return bytes, response.Body.Close()
}

func emptyPost(url string) cli.ActionFunc {
	return func(c *cli.Context) error {
		client := c.App.Metadata["client"].(*APIClient)
		_, err := client.Post(url, "")
		return err
	}
}

func dumpOutput(url string) cli.ActionFunc {
	return func(c *cli.Context) error {
		client := c.App.Metadata["client"].(*APIClient)
		response, err := client.Get(url)
		if err != nil {
			return err
		}
		return prettyPrintResponse(c, response)
	}
}

func getConfig(c *APIClient) (config.Configuration, error) {
	cfg := config.Configuration{}
	response, err := c.Get("system/config")
	if err != nil {
		return cfg, err
	}
	bytes, err := responseToBArray(response)
	if err != nil {
		return cfg, err
	}
	err = json.Unmarshal(bytes, &cfg)
	if err == nil {
		return cfg, err
	}
	return cfg, nil
}

func expects(n int, actionFunc cli.ActionFunc) cli.ActionFunc {
	return func(ctx *cli.Context) error {
		if ctx.NArg() != n {
			plural := ""
			if n != 1 {
				plural = "s"
			}
			return fmt.Errorf("expected %d argument%s, got %d", n, plural, ctx.NArg())
		}
		return actionFunc(ctx)
	}
}

func prettyPrintJSON(data interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

func prettyPrintResponse(c *cli.Context, response *http.Response) error {
	bytes, err := responseToBArray(response)
	if err != nil {
		return err
	}
	var data interface{}
	if err := json.Unmarshal(bytes, &data); err != nil {
		return err
	}
	// TODO: Check flag for pretty print format
	return prettyPrintJSON(data)
}
