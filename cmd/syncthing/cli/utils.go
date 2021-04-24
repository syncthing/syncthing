// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"mime"
	"net/http"
	"os"
	"path/filepath"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db/backend"
	"github.com/syncthing/syncthing/lib/locations"
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
		client := c.App.Metadata["client"].(APIClient)
		_, err := client.Post(url, "")
		return err
	}
}

func indexDumpOutput(url string) cli.ActionFunc {
	return func(c *cli.Context) error {
		client := c.App.Metadata["client"].(APIClient)
		response, err := client.Get(url)
		if err != nil {
			return err
		}
		return prettyPrintResponse(c, response)
	}
}

func saveToFile(url string) cli.ActionFunc {
	return func(c *cli.Context) error {
		client := c.App.Metadata["client"].(APIClient)
		response, err := client.Get(url)
		if err != nil {
			return err
		}
		_, params, err := mime.ParseMediaType(response.Header.Get("Content-Disposition"))
		if err != nil {
			return err
		}
		filename := params["filename"]
		if filename == "" {
			return errors.New("Missing filename in response")
		}
		bs, err := responseToBArray(response)
		if err != nil {
			return err
		}
		f, err := os.Create(filename)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = f.Write(bs)
		if err != nil {
			return err
		}
		fmt.Println("Wrote results to", filename)
		return err
	}
}

func getConfig(c APIClient) (config.Configuration, error) {
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

func getDB() (backend.Backend, error) {
	return backend.OpenLevelDBRO(locations.Get(locations.Database))
}

func nulString(bs []byte) string {
	for i := range bs {
		if bs[i] == 0 {
			return string(bs[:i])
		}
	}
	return string(bs)
}

func normalizePath(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}

func getClientFactory(c *cli.Context) *apiClientFactory {
	return c.App.Metadata["clientFactory"].(*apiClientFactory)
}
