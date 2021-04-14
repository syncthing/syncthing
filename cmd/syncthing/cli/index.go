// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package cli

import (
	"github.com/urfave/cli"

	"github.com/syncthing/syncthing/lib/db/backend"
	"github.com/syncthing/syncthing/lib/locations"
)

var indexCommand = cli.Command{
	Name:  "index",
	Usage: "Show information about the index (database)",
	Subcommands: []cli.Command{
		{
			Name:   "dump",
			Usage:  "Print the entire db",
			Action: expects(0, dump),
		},
		{
			Name:   "dumpsize",
			Usage:  "Print the db size of different categories of information",
			Action: expects(0, dumpsize),
		},
		{
			Name:   "idxck",
			Usage:  "Check the database for inconsistencies",
			Action: expects(0, idxck),
		},
		{
			Name:   "account",
			Usage:  "Print key and value size statistics per key type",
			Action: expects(0, account),
		},
	},
}

func getDB() (backend.Backend, error) {
	return backend.OpenLevelDBRO(locations.Get(locations.Database))
}

func nulString(bs []byte) string {
	for i := range bs {
		if bs[i] == 0 {
			return string(bs[:i])
		}
	}
	return string(bs)
}
