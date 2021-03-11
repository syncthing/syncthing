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

func (c *APIClient) Request(req *http.Request) (*http.Response, error) {
	req.Header.Set("X-API-Key", c.apikey)
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, checkResponse(resp)
}

func (c *APIClient) Do(url, method, body string) (*http.Response, error) {
	var r io.Reader
	if body != "" {
		r = bytes.NewBufferString(body)
	}
	request, err := http.NewRequest(method, c.Endpoint()+"rest/"+url, r)
	if err != nil {
		return nil, err
	}
	return c.Request(request)
}

func (c *APIClient) Get(url string) (*http.Response, error) {
	return c.Do(url, "GET", "")
}

func (c *APIClient) Post(url, body string) (*http.Response, error) {
	return c.Do(url, "POST", body)
}

func checkResponse(response *http.Response) error {
	switch response.StatusCode {
	case 403:
		return errors.New("invalid API key")
	case 404:
		body, err := responseToString(response)
		if err != nil {
			return err
		}
		if strings.HasPrefix(body, "404") {
			return unexpectedError(response.Status, body)
		}
		// This is 404 because some Syncthing object (folder, path) does not exist
		return errors.New(body)
	case 405:
		return errors.New(response.Status)
	case 200:
	default:
		body, err := responseToString(response)
		if err != nil {
			return err
		}
		return unexpectedError(response.Status, body)
	}
	return nil
}

func unexpectedError(status, body string) error {
	msg := fmt.Sprintf("unexpected HTTP status returned: %s", status)
	if !strings.HasSuffix(msg, body) {
		msg = fmt.Sprintf("%s\n%s", msg, body)
	}
	return errors.New(msg)
}
