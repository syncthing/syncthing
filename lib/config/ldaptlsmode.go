// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

type LDAPTLSMode int

const (
	LDAPTLSModeNoTLS LDAPTLSMode = iota // default is notls
	LDAPTLSModeTLS
	LDAPTLSModeStartTLS
)

func (t LDAPTLSMode) String() string {
	switch t {
	case LDAPTLSModeNoTLS:
		return "notls"
	case LDAPTLSModeTLS:
		return "tls"
	case LDAPTLSModeStartTLS:
		return "starttls"
	default:
		return "unknown"
	}
}

func (t LDAPTLSMode) MarshalText() ([]byte, error) {
	return []byte(t.String()), nil
}

func (t *LDAPTLSMode) UnmarshalText(bs []byte) error {
	switch string(bs) {
	case "notls":
		*t = LDAPTLSModeNoTLS
	case "tls":
		*t = LDAPTLSModeTLS
	case "starttls":
		*t = LDAPTLSModeStartTLS
	default:
		*t = LDAPTLSModeNoTLS
	}
	return nil
}
