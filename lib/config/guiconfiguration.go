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
)

type GUIListener struct {
	Address                   string `xml:"address" json:"address"`
	UseTLS                    bool   `xml:"tls,attr" json:"useTLS"`
	InsecureAdminAccess       bool   `xml:"insecureAdminAccess,omitempty" json:"insecureAdminAccess"`
	InsecureSkipHostCheck     bool   `xml:"insecureSkipHostcheck,omitempty" json:"insecureSkipHostcheck"`
	InsecureAllowFrameLoading bool   `xml:"insecureAllowFrameLoading,omitempty" json:"insecureAllowFrameLoading"`
}

type GUIConfiguration struct {
	Listeners []GUIListener `xml:"listener" json:"listeners"`
	Enabled   bool          `xml:"enabled,attr" json:"enabled" default:"true"`
	User      string        `xml:"user,omitempty" json:"user"`
	Password  string        `xml:"password,omitempty" json:"password"`
	APIKey    string        `xml:"apikey,omitempty" json:"apiKey"`
	Theme     string        `xml:"theme" json:"theme" default:"default"`
	Debugging bool          `xml:"debugging,attr" json:"debugging"`

	// Deprecated. Old listener configuration style.
	Deprecated_RawAddress                string `xml:"address,omitempty" json:"-"`
	Deprecated_RawUseTLS                 bool   `xml:"tls,attr,omitempty" json:"-"`
	Deprecated_InsecureAdminAccess       bool   `xml:"insecureAdminAccess,omitempty" json:"-"`
	Deprecated_InsecureSkipHostCheck     bool   `xml:"insecureSkipHostcheck,omitempty" json:"-"`
	Deprecated_InsecureAllowFrameLoading bool   `xml:"insecureAllowFrameLoading,omitempty" json:"-"`
}

func GUIListenerFromString(envAddr string) (l GUIListener) {
	l.Address = envAddr
	if strings.Contains(envAddr, "/") {
		url, err := url.Parse(envAddr)
		if err == nil {
			l.Address = url.Host
		}
	}
	l.UseTLS = strings.HasPrefix(envAddr, "https:")
	return
}

func (c GUIConfiguration) GUIListeners() []GUIListener {
	if override := os.Getenv("STGUIADDRESSES"); override != "" {
		// This value may be a comma separated list of urls.
		//
		// Each url may be of the form "scheme://address:port" or just
		// "address:port". We need to chop off the scheme. We try to
		// parse it as an URL if it contains a slash. If that fails,
		// return it as is and let some other error handling handle
		// it.
		var overrideListeners []GUIListener
		for _, overrideEntry := range strings.Split(override, ",") {
			overrideListeners = append(overrideListeners, GUIListenerFromString(overrideEntry))
		}
		return overrideListeners
	} else if override := os.Getenv("STGUIADDRESS"); override != "" {
		// Legacy overriding form which only supports one address.
		return []GUIListener{GUIListenerFromString(override)}
	}

	return c.Listeners
}

func (l GUIListener) URL() string {
	u := url.URL{
		Scheme: "http",
		Host:   l.Address,
		Path:   "/",
	}

	if l.UseTLS {
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

func (c GUIConfiguration) URLs() []string {
	guiListeners := c.GUIListeners()
	urls := make([]string, 0, len(guiListeners))
	for _, guiListener := range guiListeners {
		urls = append(urls, guiListener.URL())
	}
	return urls
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
