// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package cli

import (
	"github.com/alecthomas/kong"
)

type indexCommand struct {
	Dump     struct{} `cmd:"" help:"Print the entire db"`
	DumpSize struct{} `cmd:"" help:"Print the db size of different categories of information"`
	Check    struct{} `cmd:"" help:"Check the database for inconsistencies"`
	Account  struct{} `cmd:"" help:"Print key and value size statistics per key type"`
}

func (i *indexCommand) Run(kongCtx *kong.Context) error {
	switch kongCtx.Selected().Name {
	case "dump":
		return indexDump()
	case "dump-size":
		return indexDumpSize()
	case "check":
		return indexCheck()
	case "account":
		return indexAccount()
	}
	return nil
}
