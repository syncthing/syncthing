// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// Package rc provides remote control of a Syncthing process via the REST API.
package rc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/syncthing/syncthing/lib/dialer"
)

type API struct {
	addr   string
	apiKey string
}

func NewAPI(addr, apiKey string) *API {
	p := &API{
		addr:   addr,
		apiKey: apiKey,
	}
	return p
}

func (p *API) Get(ctx context.Context, path string, dst any) error {
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			DialContext:       dialer.DialContext,
			Proxy:             http.ProxyFromEnvironment,
			DisableKeepAlives: true,
		},
	}

	url := fmt.Sprintf("http://%s%s", p.addr, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	req.Header.Add("Authorization", "Bearer "+p.apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	return p.readResponse(resp, dst)
}

func (p *API) Post(path string, src, dst any) error {
	client := &http.Client{
		Timeout: 600 * time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}
	url := fmt.Sprintf("http://%s%s", p.addr, path)
	var postBody io.Reader
	if src != nil {
		data, err := json.Marshal(src)
		if err != nil {
			return err
		}
		postBody = bytes.NewReader(data)
	}
	req, err := http.NewRequest(http.MethodPost, url, postBody)
	if err != nil {
		return err
	}

	req.Header.Add("Authorization", "Bearer "+p.apiKey)
	req.Header.Add("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	return p.readResponse(resp, dst)
}

type Event struct {
	ID   int
	Time time.Time
	Type string
	Data any
}

func (p *API) Events(ctx context.Context, since int) ([]Event, error) {
	var evs []Event
	if err := p.Get(ctx, fmt.Sprintf("/rest/events?since=%d&timeout=10", since), &evs); err != nil {
		return nil, err
	}
	return evs, nil
}

func (*API) readResponse(resp *http.Response, dst any) error {
	bs, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		return errors.New(resp.Status)
	}
	if dst == nil {
		return nil
	}
	return json.Unmarshal(bs, dst)
}
