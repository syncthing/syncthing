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
	"net"
	"net/http"
	"strings"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/locations"
	"github.com/syncthing/syncthing/lib/protocol"
)

type APIClient interface {
	Get(url string) (*http.Response, error)
	Post(url, body string) (*http.Response, error)
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
		f.cfg, err = loadGUIConfig()
		if err != nil {
			return nil, err
		}
	} else if f.cfg.Address() == "" || f.cfg.APIKey == "" {
		return nil, errors.New("Both --gui-address and --gui-apikey should be specified")
	}

	httpClient := http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial(f.cfg.Network(), f.cfg.Address())
			},
		},
	}
	return &apiClient{
		Client: httpClient,
		cfg:    f.cfg,
		apikey: f.cfg.APIKey,
	}, nil
}

func loadGUIConfig() (config.GUIConfiguration, error) {
	// Load the certs and get the ID
	cert, err := tls.LoadX509KeyPair(
		locations.Get(locations.CertFile),
		locations.Get(locations.KeyFile),
	)
	if err != nil {
		return config.GUIConfiguration{}, fmt.Errorf("reading device ID: %w", err)
	}

	myID := protocol.NewDeviceID(cert.Certificate[0])

	// Load the config
	cfg, _, err := config.Load(locations.Get(locations.ConfigFile), myID, events.NoopLogger)
	if err != nil {
		return config.GUIConfiguration{}, fmt.Errorf("loading config: %w", err)
	}

	guiCfg := cfg.GUI()

	if guiCfg.Address() == "" {
		return config.GUIConfiguration{}, errors.New("Could not find GUI Address")
	}

	if guiCfg.APIKey == "" {
		return config.GUIConfiguration{}, errors.New("Could not find GUI API key")
	}

	return guiCfg, nil
}

func (c *apiClient) Endpoint() string {
	if c.cfg.Network() == "unix" {
		return "http://unix/"
	}
	url := c.cfg.URL()
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}
	return url
}

func (c *apiClient) Do(req *http.Request) (*http.Response, error) {
	req.Header.Set("X-API-Key", c.apikey)
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, checkResponse(resp)
}

func (c *apiClient) Get(url string) (*http.Response, error) {
	request, err := http.NewRequest("GET", c.Endpoint()+"rest/"+url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(request)
}

func (c *apiClient) Post(url, body string) (*http.Response, error) {
	request, err := http.NewRequest("POST", c.Endpoint()+"rest/"+url, bytes.NewBufferString(body))
	if err != nil {
		return nil, err
	}
	return c.Do(request)
}

func checkResponse(response *http.Response) error {
	if response.StatusCode == http.StatusNotFound {
		return errors.New("invalid endpoint or API call")
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
