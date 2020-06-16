// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
	"encoding/json"
	"encoding/xml"

	"github.com/syncthing/syncthing/lib/util"
)

type LDAPConfiguration struct {
	Address            string        `xml:"address,omitempty" json:"address"`
	BindDN             string        `xml:"bindDN,omitempty" json:"bindDN"`
	Transport          LDAPTransport `xml:"transport,omitempty" json:"transport"`
	InsecureSkipVerify bool          `xml:"insecureSkipVerify,omitempty" json:"insecureSkipVerify" default:"false"`
	SearchBaseDN       string        `xml:"searchBaseDN,omitempty" json:"searchBaseDN"`
	SearchFilter       string        `xml:"searchFilter,omitempty" json:"searchFilter"`
}

func (c LDAPConfiguration) Copy() LDAPConfiguration {
	return c
}

func (c *LDAPConfiguration) UnmarshalJSON(data []byte) error {
	util.SetDefaults(c)
	type noCustomUnmarshal LDAPConfiguration
	ptr := (*noCustomUnmarshal)(c)
	return json.Unmarshal(data, ptr)
}

func (c *LDAPConfiguration) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	util.SetDefaults(c)
	type noCustomUnmarshal LDAPConfiguration
	ptr := (*noCustomUnmarshal)(c)
	return d.DecodeElement(ptr, &start)
}
