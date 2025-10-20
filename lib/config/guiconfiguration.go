// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
	"encoding/hex"
	"net"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/sliceutil"
	"github.com/syncthing/syncthing/lib/structutil"
)

type GUIConfiguration struct {
	Enabled                   bool                 `json:"enabled" xml:"enabled,attr" default:"true"`
	RawAddress                string               `json:"address" xml:"address" default:"127.0.0.1:8384"`
	RawUnixSocketPermissions  string               `json:"unixSocketPermissions" xml:"unixSocketPermissions,omitempty"`
	User                      string               `json:"user" xml:"user,omitempty"`
	Password                  string               `json:"password" xml:"password,omitempty"`
	AuthMode                  AuthMode             `json:"authMode" xml:"authMode,omitempty"`
	MetricsWithoutAuth        bool                 `json:"metricsWithoutAuth" xml:"metricsWithoutAuth" default:"false"`
	RawUseTLS                 bool                 `json:"useTLS" xml:"tls,attr"`
	APIKey                    string               `json:"apiKey" xml:"apikey,omitempty"`
	InsecureAdminAccess       bool                 `json:"insecureAdminAccess" xml:"insecureAdminAccess,omitempty"`
	Theme                     string               `json:"theme" xml:"theme" default:"default"`
	InsecureSkipHostCheck     bool                 `json:"insecureSkipHostcheck" xml:"insecureSkipHostcheck,omitempty"`
	InsecureAllowFrameLoading bool                 `json:"insecureAllowFrameLoading" xml:"insecureAllowFrameLoading,omitempty"`
	SendBasicAuthPrompt       bool                 `json:"sendBasicAuthPrompt" xml:"sendBasicAuthPrompt,attr"`
	WebauthnUserId            []byte               `json:"webauthnUserId" xml:"webauthnUserId"`
	WebauthnRpId              string               `json:"webauthnRpId" xml:"webauthnRpId" default:"localhost"`
	WebauthnOrigins           []string             `json:"webauthnOrigins" xml:"webauthnOrigin"`
	WebauthnCredentials       []WebauthnCredential `json:"webauthnCredentials" xml:"webauthnCredential"`
}

type WebauthnCredential struct {
	ID            string    `json:"id" xml:"id"`
	RpId          string    `json:"rpId" xml:"rpId"`
	Nickname      string    `json:"nickname" xml:"nickname"`
	PublicKeyCose string    `json:"publicKeyCose" xml:"publicKeyCose"`
	Transports    []string  `json:"transports" xml:"transports"`
	RequireUv     bool      `json:"requireUv" xml:"requireUv"`
	CreateTime    time.Time `json:"createTime" xml:"createTime"`
}

func (c GUIConfiguration) IsPasswordAuthEnabled() bool {
	return c.AuthMode == AuthModeLDAP || (len(c.User) > 0 && len(c.Password) > 0)
}

func (c GUIConfiguration) IsWebauthnAuthEnabled() bool {
	return len(c.WebauthnCredentials) > 0
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

func (c *GUIConfiguration) defaultWebauthnRpId() string {
	defaultGuiCfg := structutil.WithDefaults(GUIConfiguration{})
	host, _, err := net.SplitHostPort(c.Address())
	if err != nil {
		defaultHost, _, err := net.SplitHostPort(defaultGuiCfg.Address())
		if err != nil {
			return defaultGuiCfg.WebauthnRpId
		}
		host = defaultHost
	}
	if net.ParseIP(host) != nil {
		return defaultGuiCfg.WebauthnRpId
	}
	return host
}

func (c *GUIConfiguration) defaultWebauthnOrigins() ([]string, error) {
	_, port, err := net.SplitHostPort(c.Address())
	if err != nil {
		defaultGuiCfg := structutil.WithDefaults(GUIConfiguration{})
		_, defaultPort, err := net.SplitHostPort(defaultGuiCfg.Address())
		if err != nil {
			return nil, err
		}
		port = defaultPort
	}
	secure_origin := "https://" + c.WebauthnRpId
	if port != "443" {
		secure_origin += ":" + port
	}
	return []string{secure_origin}, nil
}

func (c *GUIConfiguration) prepare() error {
	if c.APIKey == "" {
		c.APIKey = rand.String(32)
	}

	if len(c.WebauthnUserId) == 0 {
		// Spec recommends 64 random bytes; 32 is enough and fits hex-encoded in the max of 64 bytes
		newUserId := make([]byte, 32)
		_, err := rand.Read(newUserId)
		if err != nil {
			return err
		}
		// Hex-encode the random bytes so that the ID is printable ASCII, for config.xml etc.
		c.WebauthnUserId = []byte(hex.EncodeToString(newUserId))
	}

	defaultGuiCfg := structutil.WithDefaults(GUIConfiguration{})
	if c.WebauthnRpId == "" {
		c.WebauthnRpId = defaultGuiCfg.WebauthnRpId
	}
	if len(c.WebauthnOrigins) == 0 {
		origins, err := c.defaultWebauthnOrigins()
		if err != nil {
			return err
		}
		c.WebauthnOrigins = origins
	}

	return nil
}

func (c GUIConfiguration) EligibleWebAuthnCredentials(guiCfg GUIConfiguration) []WebauthnCredential {
	return sliceutil.Filter(c.WebauthnCredentials, func(cred *WebauthnCredential) bool {
		return cred.RpId == guiCfg.WebauthnRpId
	})
}

func (orig *GUIConfiguration) Copy() GUIConfiguration {
	c := *orig
	c.WebauthnCredentials = make([]WebauthnCredential, len(orig.WebauthnCredentials))
	for i := range orig.WebauthnCredentials {
		c.WebauthnCredentials[i] = orig.WebauthnCredentials[i].Copy()
	}
	return c
}

func (orig *WebauthnCredential) Copy() WebauthnCredential {
	c := *orig
	if c.Transports != nil {
		c.Transports = make([]string, len(c.Transports))
		copy(c.Transports, orig.Transports)
	}
	return c
}

func (c *WebauthnCredential) NicknameOrID() string {
	if c.Nickname != "" {
		return c.Nickname
	}
	return c.ID
}

func SanitizeWebauthnStateChanges(from *GUIConfiguration, to *GUIConfiguration, pendingRegistrations []WebauthnCredential) {
	// Don't allow adding new WebAuthn credentials without passing a registration challenge,
	// and only allow updating the Nickname and RequireUv fields
	existingCredentials := make(map[string]WebauthnCredential)
	for _, cred := range from.WebauthnCredentials {
		existingCredentials[cred.ID] = cred
	}
	for _, cred := range pendingRegistrations {
		existingCredentials[cred.ID] = cred
	}

	var updatedCredentials []WebauthnCredential
	for _, newCred := range to.WebauthnCredentials {
		if exCred, ok := existingCredentials[newCred.ID]; ok {
			exCred.Nickname = newCred.Nickname
			exCred.RequireUv = newCred.RequireUv
			updatedCredentials = append(updatedCredentials, exCred)
		}
	}
	to.WebauthnCredentials = updatedCredentials
}
