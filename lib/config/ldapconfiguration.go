// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

/*
LDAPConfiguration supports two modes of authentication. The two are
distinguished by whether BindPassword is set or not. When not set,
authenticate is performed by bind using the user supplied credentials
directly. The pattern "%s" in the BindDN is replaced by the user entered
username. Example configuration:

  {
      Addresses: []string{"ldap.example.com:389"},
      BindDN: "CN=%s,DC=example,DC=com",
  }

The other mode uses an LDAP search for the entered username using the
SearchBaseDN and SearchPattern, again replacing "%s" with the user entered
username. To perform the search a bind is done using BindDN and
BindPassword. If exactly one entry is found by the search, a bind is
attempted as that entry with the user supplied password. Example
configuration:

  {
      Addresses: []string{"dc.example.com:389"},
      BindDN: "syncthing@ad.example.com",
      BindPassword: "s00pers3cret",
      SearchPattern: "(|(sAMAccountName=%s)(mail=%s))",
      SearchBaseDN: "CN=Users,DC=example,DC=com",
  }

The SearchPattern can be used to enforce group membership by adding a
suitable memberOf term.

  SearchPattern:
  "(&(|(sAMAccountName=%s)(mail=%s))(memberOf=CN=Syncthing,CN=Users,DC=example,DC=com)",

*/
type LDAPConfiguration struct {
	Addresses          []string        `xml:"address,omitempty" json:"addresses"`
	BindDN             string          `xml:"bindDN,omitempty" json:"bindDN"`
	BindPassword       encryptedString `xml:"bindPassword,omitempty" json:"bindPassword"`
	Transport          LDAPTransport   `xml:"transport,omitempty" json:"transport"`
	InsecureSkipVerify bool            `xml:"insecureSkipVerify,omitempty" json:"insecureSkipVerify"`
	SearchPattern      string          `xml:"searchPattern,omitempty" json:"searchPattern"`
	SearchBaseDN       string          `xml:"searchBaseDN,omitempty" json:"searchBaseDN"`
}

func (c LDAPConfiguration) Copy() LDAPConfiguration {
	return c
}
