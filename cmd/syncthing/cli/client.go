// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package cli

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/syncthing/syncthing/lib/config"
)

type APIClient struct {
	http.Client
	cfg    config.GUIConfiguration
	apikey string
}

func getClient(cfg config.GUIConfiguration) *APIClient {
	httpClient := http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial(cfg.Network(), cfg.Address())
			},
		},
	}
	return &APIClient{
		Client: httpClient,
		cfg:    cfg,
		apikey: cfg.APIKey,
	}
}

func (c *APIClient) Endpoint() string {
	if c.cfg.Network() == "unix" {
		return "http://unix/"
	}
	url := c.cfg.URL()
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}
	return url
}

func (c *APIClient) Do(req *http.Request) (*http.Response, error) {
	req.Header.Set("X-API-Key", c.apikey)
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, checkResponse(resp)
}

func (c *APIClient) Request(url, method string, r io.Reader) (*http.Response, error) {
	request, err := http.NewRequest(method, c.Endpoint()+"rest/"+url, r)
	if err != nil {
		return nil, err
	}
	return c.Do(request)
}

func (c *APIClient) RequestString(url, method, data string) (*http.Response, error) {
	return c.Request(url, method, bytes.NewBufferString(data))
}

func (c *APIClient) RequestJSON(url, method string, o interface{}) (*http.Response, error) {
	data, err := json.Marshal(o)
	if err != nil {
		return nil, err
	}
	return c.Request(url, method, bytes.NewBuffer(data))
}

func (c *APIClient) Get(url string) (*http.Response, error) {
	return c.RequestString(url, "GET", "")
}

func (c *APIClient) Post(url, body string) (*http.Response, error) {
	return c.RequestString(url, "POST", body)
}

func (c *APIClient) PutJSON(url string, o interface{}) (*http.Response, error) {
	return c.RequestJSON(url, "PUT", o)
}

func checkResponse(response *http.Response) error {
	if response.StatusCode == 404 {
		return errors.New("invalid endpoint or API call")
	} else if response.StatusCode == 403 {
		return errors.New("invalid API key")
	} else if response.StatusCode != 200 {
		data, err := responseToBArray(response)
		if err != nil {
			return err
		}
		body := strings.TrimSpace(string(data))
		return fmt.Errorf("unexpected HTTP status returned: %s\n%s", response.Status, body)
	}
	return nil
}
