// Copyright (C) 2022 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"errors"
	"os/user"
	"strings"

	"github.com/syncthing/syncthing/lib/protocol"
)

func (f *sendReceiveFolder) syncOwnership(file *protocol.FileInfo, path string) error {
	if file.Platform.Windows == nil || file.Platform.Windows.OwnerName == "" {
		// No owner data, nothing to do
		return nil
	}

	l.Debugln("Owner name for %s is %s (group=%v)", path, file.Platform.Windows.OwnerName, file.Platform.Windows.OwnerIsGroup)
	usid, gsid, err := lookupUserAndGroup(file.Platform.Windows.OwnerName, file.Platform.Windows.OwnerIsGroup)
	if err != nil {
		return err
	}

	l.Debugln("Owner for %s resolved to uid=%q gid=%q", path, usid, gsid)
	return f.mtimefs.Lchown(path, usid, gsid)
}

func lookupUserAndGroup(name string, group bool) (string, string, error) {
	// Look up either the the user or the group, returning the other kind as
	// blank. This might seem an odd maneuver, but it matches what Chown
	// wants as input and hides the ugly nested if:s down here.

	if group {
		gr, err := lookupWithoutDomain(name, func(name string) (string, error) {
			gr, err := user.LookupGroup(name)
			if err == nil {
				return gr.Gid, nil
			}
			return "", err
		})
		if err != nil {
			return "", "", err
		}
		return "", gr, nil
	}

	us, err := lookupWithoutDomain(name, func(name string) (string, error) {
		us, err := user.Lookup(name)
		if err == nil {
			return us.Uid, nil
		}
		return "", err
	})
	if err != nil {
		return "", "", err
	}
	return us, "", nil
}

func lookupWithoutDomain(name string, lookup func(s string) (string, error)) (string, error) {
	// Try to look up the user by name. The username will be either a plain
	// username or a qualified DOMAIN\user. We'll first try to look up
	// whatever we got, if that fails, we'll try again with just the user
	// part without domain.

	v, err := lookup(name)
	if err == nil {
		return v, nil
	}
	parts := strings.Split(name, `\`)
	if len(parts) == 2 {
		if v, err := lookup(parts[1]); err == nil {
			return v, nil
		}
	}
	return "", errors.New("lookup failed")
}
