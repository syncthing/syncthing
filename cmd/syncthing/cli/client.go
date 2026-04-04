// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/syncthing/syncthing/cmd/syncthing/internal/guiclient"
	"github.com/syncthing/syncthing/lib/config"
)

type APIClient interface {
	Get(url string) (*http.Response, error)
	Post(url, body string) (*http.Response, error)
	PutJSON(url string, o interface{}) (*http.Response, error)
}

type apiClient struct {
	http.Client

	cfg    config.GUIConfiguration
	apikey string
}

type apiClientFactory struct {
	cfg config.GUIConfiguration
}

func (f *apiClientFactory) getClient() (APIClient, error) {
	// Now if the API key and address is not provided (we are not connecting to a remote instance),
	// try to rip it out of the config.
	if f.cfg.RawAddress == "" && f.cfg.APIKey == "" {
		var err error
		f.cfg, err = guiclient.LoadGUIConfig()
		if err != nil {
			return nil, err
		}
	} else if f.cfg.Address() == "" || f.cfg.APIKey == "" {
		return nil, errors.New("Both --gui-address and --gui-apikey should be specified")
	}

	httpClient := *guiclient.NewHTTPClient(f.cfg)
	return &apiClient{
		Client: httpClient,
		cfg:    f.cfg,
		apikey: f.cfg.APIKey,
	}, nil
}

func (c *apiClient) Endpoint() string {
	return guiclient.BaseURL(c.cfg)
}

func (c *apiClient) Do(req *http.Request) (*http.Response, error) {
	req.Header.Set("X-Api-Key", c.apikey)
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, checkResponse(resp)
}

func (c *apiClient) Request(url, method string, r io.Reader) (*http.Response, error) {
	request, err := http.NewRequest(method, c.Endpoint()+"rest/"+url, r)
	if err != nil {
		return nil, err
	}
	return c.Do(request)
}

func (c *apiClient) RequestString(url, method, data string) (*http.Response, error) {
	return c.Request(url, method, bytes.NewBufferString(data))
}

func (c *apiClient) RequestJSON(url, method string, o interface{}) (*http.Response, error) {
	data, err := json.Marshal(o)
	if err != nil {
		return nil, err
	}
	return c.Request(url, method, bytes.NewBuffer(data))
}

func (c *apiClient) Get(url string) (*http.Response, error) {
	return c.RequestString(url, "GET", "")
}

func (c *apiClient) Post(url, body string) (*http.Response, error) {
	return c.RequestString(url, "POST", body)
}

func (c *apiClient) PutJSON(url string, o interface{}) (*http.Response, error) {
	return c.RequestJSON(url, "PUT", o)
}

var errNotFound = errors.New("invalid endpoint or API call")

func checkResponse(response *http.Response) error {
	if response.StatusCode == http.StatusNotFound {
		return errNotFound
	} else if response.StatusCode == http.StatusUnauthorized {
		return errors.New("invalid API key")
	} else if response.StatusCode != http.StatusOK {
		data, err := responseToBArray(response)
		if err != nil {
			return err
		}
		body := strings.TrimSpace(string(data))
		return fmt.Errorf("unexpected HTTP status returned: %s\n%s", response.Status, body)
	}
	return nil
}
