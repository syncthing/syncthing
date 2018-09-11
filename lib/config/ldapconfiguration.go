// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

type LDAPConfiguration struct {
	Address            string        `xml:"address,omitempty" json:"addresd"`
	BindDN             string        `xml:"bindDN,omitempty" json:"bindDN"`
	Transport          LDAPTransport `xml:"transport,omitempty" json:"transport"`
	InsecureSkipVerify bool          `xml:"insecureSkipVerify,omitempty" json:"insecureSkipVerify" default:"false"`
}

func (c LDAPConfiguration) Copy() LDAPConfiguration {
	return c
}
