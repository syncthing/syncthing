// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package cli

import (
	"github.com/urfave/cli"
)

var indexCommand = cli.Command{
	Name:  "index",
	Usage: "Show information about the index (database)",
	Subcommands: []cli.Command{
		{
			Name:   "dump",
			Usage:  "Print the entire db",
			Action: expects(0, indexDump),
		},
		{
			Name:   "dump-size",
			Usage:  "Print the db size of different categories of information",
			Action: expects(0, indexDumpSize),
		},
		{
			Name:   "check",
			Usage:  "Check the database for inconsistencies",
			Action: expects(0, indexCheck),
		},
		{
			Name:   "account",
			Usage:  "Print key and value size statistics per key type",
			Action: expects(0, indexAccount),
		},
	},
}
