// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

type LDAPConfiguration struct {
	Address            string        `json:"address" xml:"address,omitempty"`
	BindDN             string        `json:"bindDN" xml:"bindDN,omitempty"`
	Transport          LDAPTransport `json:"transport" xml:"transport,omitempty"`
	InsecureSkipVerify bool          `json:"insecureSkipVerify" xml:"insecureSkipVerify,omitempty" default:"false"`
	SearchBaseDN       string        `json:"searchBaseDN" xml:"searchBaseDN,omitempty"`
	SearchFilter       string        `json:"searchFilter" xml:"searchFilter,omitempty"`
}

func (c LDAPConfiguration) Copy() LDAPConfiguration {
	return c
}
