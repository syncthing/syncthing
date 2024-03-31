// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
	"encoding/base64"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/syncthing/syncthing/lib/rand"
	"golang.org/x/crypto/bcrypt"
)

func (c GUIConfiguration) IsAuthEnabled() bool {
	// This function should match isAuthEnabled() in syncthingController.js
	return c.IsPasswordAuthEnabled() || c.WebauthnReady()
}

func (c GUIConfiguration) IsPasswordAuthEnabled() bool {
	return c.AuthMode == AuthModeLDAP || (len(c.User) > 0 && len(c.Password) > 0)
}

func (GUIConfiguration) IsOverridden() bool {
	return os.Getenv("STGUIADDRESS") != ""
}

func (c GUIConfiguration) Address() string {
	if override := os.Getenv("STGUIADDRESS"); override != "" {
		// This value may be of the form "scheme://address:port" or just
		// "address:port". We need to chop off the scheme. We try to parse it as
		// an URL if it contains a slash. If that fails, return it as is and let
		// some other error handling handle it.

		if strings.Contains(override, "/") {
			url, err := url.Parse(override)
			if err != nil {
				return override
			}
			if strings.HasPrefix(url.Scheme, "unix") {
				return url.Path
			}
			return url.Host
		}

		return override
	}

	return c.RawAddress
}

func (c GUIConfiguration) UnixSocketPermissions() os.FileMode {
	perm, err := strconv.ParseUint(c.RawUnixSocketPermissions, 8, 32)
	if err != nil {
		// ignore incorrectly formatted permissions
		return 0
	}
	return os.FileMode(perm) & os.ModePerm
}

func (c GUIConfiguration) Network() string {
	if override := os.Getenv("STGUIADDRESS"); override != "" {
		url, err := url.Parse(override)
		if err == nil && strings.HasPrefix(url.Scheme, "unix") {
			return "unix"
		}
		return "tcp"
	}
	if strings.HasPrefix(c.RawAddress, "/") {
		return "unix"
	}
	return "tcp"
}

func (c GUIConfiguration) UseTLS() bool {
	if override := os.Getenv("STGUIADDRESS"); override != "" {
		return strings.HasPrefix(override, "https:") || strings.HasPrefix(override, "unixs:")
	}
	return c.RawUseTLS
}

func (c GUIConfiguration) WebauthnReady() bool {
	return c.UseTLS() && len(c.EligibleWebAuthnCredentials()) > 0
}

func (c GUIConfiguration) EligibleWebAuthnCredentials() []WebauthnCredential {
	var result []WebauthnCredential
	rpId := c.WebauthnRpId
	for _, cred := range c.WebauthnCredentials {
		if cred.RpId == rpId {
			result = append(result, cred)
		}
	}
	return result
}

func (c GUIConfiguration) URL() string {
	if c.Network() == "unix" {
		if c.UseTLS() {
			return "unixs://" + c.Address()
		}
		return "unix://" + c.Address()
	}

	u := url.URL{
		Scheme: "http",
		Host:   c.Address(),
		Path:   "/",
	}

	if c.UseTLS() {
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

// matches a bcrypt hash and not too much else
var bcryptExpr = regexp.MustCompile(`^\$2[aby]\$\d+\$.{50,}`)

// SetPassword takes a bcrypt hash or a plaintext password and stores it.
// Plaintext passwords are hashed. Returns an error if the password is not
// valid.
// If the plaintext password is empty, the password is unset instead.
func (c *GUIConfiguration) SetPassword(password string) error {
	if password == "" {
		c.Password = ""
		return nil
	}

	if bcryptExpr.MatchString(password) {
		// Already hashed
		c.Password = password
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	c.Password = string(hash)
	return nil
}

// CompareHashedPassword returns nil when the given plaintext password matches the stored hash.
func (c GUIConfiguration) CompareHashedPassword(password string) error {
	configPasswordBytes := []byte(c.Password)
	passwordBytes := []byte(password)
	return bcrypt.CompareHashAndPassword(configPasswordBytes, passwordBytes)
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

func (c *GUIConfiguration) prepare() error {
	if c.APIKey == "" {
		c.APIKey = rand.String(32)
	}

	if len(c.WebauthnUserId) == 0 {
		newUserId := make([]byte, 64)
		_, err := rand.Read(newUserId)
		if err != nil {
			return err
		}
		c.WebauthnUserId = base64.URLEncoding.EncodeToString(newUserId)
	}

	return nil
}

func (c GUIConfiguration) Copy() GUIConfiguration {
	if c.WebauthnCredentials != nil {
		creds := make([]WebauthnCredential, len(c.WebauthnCredentials))
		for i := range c.WebauthnCredentials {
			creds[i] = c.WebauthnCredentials[i].Copy()
		}
		c.WebauthnCredentials = creds
	}
	return c
}

func (c WebauthnCredential) Copy() WebauthnCredential {
	if c.Transports != nil {
		transports := make([]string, len(c.Transports))
		copy(transports, c.Transports)
		c.Transports = transports
	}
	return c
}

func (c *WebauthnCredential) NicknameOrID() string {
	if c.Nickname != "" {
		return c.Nickname
	} else {
		return c.ID
	}
}
