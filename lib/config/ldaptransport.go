// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

func (t LDAPTransport) String() string {
	switch t {
	case LDAPTransportPlain:
		return "plain"
	case LDAPTransportTLS:
		return "tls"
	case LDAPTransportStartTLS:
		return "starttls"
	default:
		return "unknown"
	}
}

func (t LDAPTransport) MarshalText() ([]byte, error) {
	return []byte(t.String()), nil
}

func (t *LDAPTransport) UnmarshalText(bs []byte) error {
	switch string(bs) {
	case "plain":
		*t = LDAPTransportPlain
	case "tls":
		*t = LDAPTransportTLS
	case "starttls":
		*t = LDAPTransportStartTLS
	default:
		*t = LDAPTransportPlain
	}
	return nil
}
