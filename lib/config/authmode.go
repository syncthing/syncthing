// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

func (t AuthMode) String() string {
	switch t {
	case AuthModeStatic:
		return "static"
	case AuthModeLDAP:
		return "ldap"
	default:
		return "unknown"
	}
}

func (t AuthMode) MarshalText() ([]byte, error) {
	return []byte(t.String()), nil
}

func (t *AuthMode) UnmarshalText(bs []byte) error {
	switch string(bs) {
	case "ldap":
		*t = AuthModeLDAP
	case "static":
		*t = AuthModeStatic
	default:
		*t = AuthModeStatic
	}
	return nil
}
