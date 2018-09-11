// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

type LDAPConfiguration struct {
	LDAPAddress            string        `xml:"address,omitempty" json:"addresd"`
	LDAPBindDn             string        `xml:"bindDn,omitempty" json:"bindDn"`
	LDAPTransport          LDAPTransport `xml:"transport,omitempty" json:"transport"`
	LDAPInsecureSkipVerify bool          `xml:"insecureSkipVerify,omitempty" json:"insecureSkipVerify" default:"false"`
}

func (c LDAPConfiguration) Copy() LDAPConfiguration {
	return c
}
