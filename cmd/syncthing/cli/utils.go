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
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"

	"github.com/syncthing/syncthing/lib/config"
)

func responseToBArray(response *http.Response) ([]byte, error) {
	bytes, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	return bytes, response.Body.Close()
}

func emptyPost(url string, apiClientFactory *apiClientFactory) error {
	client, err := apiClientFactory.getClient()
	if err != nil {
		return err
	}
	_, err = client.Post(url, "")
	return err
}

func indexDumpOutputWrapper(apiClientFactory *apiClientFactory) func(url string) error {
	return func(url string) error {
		return indexDumpOutput(url, apiClientFactory)
	}
}

func indexDumpOutput(url string, apiClientFactory *apiClientFactory) error {
	client, err := apiClientFactory.getClient()
	if err != nil {
		return err
	}
	response, err := client.Get(url)
	if errors.Is(err, errNotFound) {
		return errors.New("not found (folder/file not in database)")
	}
	if err != nil {
		return err
	}
	return prettyPrintResponse(response)
}

func saveToFile(url string, apiClientFactory *apiClientFactory) error {
	client, err := apiClientFactory.getClient()
	if err != nil {
		return err
	}
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
	_, err = f.Write(bs)
	if err != nil {
		_ = f.Close()
		return err
	}
	err = f.Close()
	if err != nil {
		return err
	}
	fmt.Println("Wrote results to", filename)
	return nil
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
	if err != nil {
		return config.Configuration{}, err
	}
	return cfg, nil
}

func prettyPrintJSON(data interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

func prettyPrintResponse(response *http.Response) error {
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
