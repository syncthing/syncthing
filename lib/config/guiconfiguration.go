// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/syncthing/syncthing/lib/rand"
	"github.com/duo-labs/webauthn/webauthn"
)

func (c GUIConfiguration) IsAuthEnabled() bool {
	return c.AuthMode == AuthModeLDAP || (len(c.User) > 0 && len(c.Password) > 0) || c.WebauthnReady()
}

func (c GUIConfiguration) IsOverridden() bool {
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
	if override := os.Getenv("STGUIADDRESS"); strings.Contains(override, "/") {
		url, err := url.Parse(override)
		if err != nil {
			return "tcp"
		}
		if strings.HasPrefix(url.Scheme, "unix") {
			return "unix"
		}
	}
	if strings.HasPrefix(c.RawAddress, "/") {
		return "unix"
	}
	return "tcp"
}

func (c GUIConfiguration) UseTLS() bool {
	if override := os.Getenv("STGUIADDRESS"); override != "" {
		if strings.HasPrefix(override, "http") {
			return strings.HasPrefix(override, "https:")
		}
		if strings.HasPrefix(override, "unix") {
			return strings.HasPrefix(override, "unixs:")
		}
	}
	return c.RawUseTLS
}

func (c GUIConfiguration) WebauthnReady() bool {
	return c.UseTLS() && len(c.WebauthnCredentials) > 0
}

func (c GUIConfiguration) URL() string {
	if strings.HasPrefix(c.RawAddress, "/") {
		return "unix://" + c.RawAddress
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

// SetHashedPassword hashes the given plaintext password and stores the new hash.
func (c *GUIConfiguration) HashAndSetPassword(password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 0)
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

func (gui GUIConfiguration) WebAuthnID() []byte {
	return []byte{ 0, 1, 2, 3 }
}

func (gui GUIConfiguration) WebAuthnName() string {
	return gui.User
}

func (gui GUIConfiguration) WebAuthnDisplayName() string {
	return gui.User
}

func (gui GUIConfiguration) WebAuthnIcon() string {
	return ""
}

func (gui GUIConfiguration) WebAuthnCredentials() []webauthn.Credential {
	var result []webauthn.Credential
	for _, cred := range gui.WebauthnCredentials {
		id, err := base64.URLEncoding.DecodeString(cred.ID)
		if err != nil {
			l.Warnln(fmt.Sprintf("Failed to base64url-decode ID of WebAuthn credential \"%s\": %s", cred.Nickname, cred.ID), err)
			continue
		}

		pubkey, err := base64.URLEncoding.DecodeString(cred.PublicKeyCose)
		if err != nil {
			l.Warnln(fmt.Sprintf("Failed to base64url-decode public key of WebAuthn credential \"%s\" (%s)", cred.Nickname, cred.ID), err)
			continue
		}

		result = append(result, webauthn.Credential{
			ID: id,
			PublicKey: pubkey,
			Authenticator: webauthn.Authenticator{
				SignCount: cred.SignCount,
			},
		})
	}
	return result
}

func (c *GUIConfiguration) prepare() {
	if c.APIKey == "" {
		c.APIKey = rand.String(32)
	}
}

func (c GUIConfiguration) Copy() GUIConfiguration {
	return c
}
