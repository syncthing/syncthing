// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
	"net/url"
	"os"
	"strings"

	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/util"
)

type GUIConfiguration struct {
	Enabled                   bool   `xml:"enabled,attr" json:"enabled" default:"true"`
	Address                   string `xml:"address" json:"address" default:"127.0.0.1:8384"`
	User                      string `xml:"user,omitempty" json:"user"`
	Password                  string `xml:"password,omitempty" json:"password"`
	UseTLS                    bool   `xml:"tls,attr" json:"useTLS"`
	APIKey                    string `xml:"apikey,omitempty" json:"apiKey"`
	InsecureAdminAccess       bool   `xml:"insecureAdminAccess,omitempty" json:"insecureAdminAccess"`
	Theme                     string `xml:"theme" json:"theme" default:"default"`
	Debugging                 bool   `xml:"debugging,attr" json:"debugging"`
	InsecureSkipHostCheck     bool   `xml:"insecureSkipHostcheck,omitempty" json:"insecureSkipHostcheck"`
	InsecureAllowFrameLoading bool   `xml:"insecureAllowFrameLoading,omitempty" json:"insecureAllowFrameLoading"`
}

func GUIConfigFromString(envAddr string) (c GUIConfiguration) {
	util.SetDefaults(&c)
	c.Address = envAddr
	if strings.Contains(envAddr, "/") {
		url, err := url.Parse(envAddr)
		if err == nil {
			c.Address = url.Host
		}
	}
	c.UseTLS = strings.HasPrefix(envAddr, "https:")
	c.APIKey = rand.String(32)
	return
}

func (c GUIConfiguration) URL() string {
	u := url.URL{
		Scheme: "http",
		Host:   c.Address,
		Path:   "/",
	}

	if c.UseTLS {
		u.Scheme = "https"
	}

	if strings.HasPrefix(u.Host, ":") {
		// Empty host, i.e. ":port", use IPv4 localhost
		u.Host = "127.0.0.1" + u.Host
	} else if strings.HasPrefix(u.Host, "0.0.0.0:") {
		// IPv4 all zeroes host, convert to IPv4 localhost
		u.Host = "127.0.0.1" + u.Host[7:]
	} else if strings.HasPrefix(u.Host, "[::]:") {
		// IPv6 all zeroes host, convert to IPv6 localhost
		u.Host = "[::1]" + u.Host[4:]
	}

	return u.String()
}

// IsValidAPIKey returns true when the given API key is valid, including both
// the value in config and any overrides
func (c GUIConfiguration) IsValidAPIKey(apiKey string) bool {
	switch apiKey {
	case "":
		return false

	case c.APIKey, os.Getenv("STGUIAPIKEY"):
		return true

	default:
		return false
	}
}
