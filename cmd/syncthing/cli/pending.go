// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package cli

import (
	"net/url"

	"github.com/alecthomas/kong"
)

type pendingCommand struct {
	Devices struct{} `cmd:"" help:"Show pending devices"`
	Folders struct {
		Device string `help:"Show pending folders offered by given device"`
	} `cmd:"" help:"Show pending folders"`
}

func (p *pendingCommand) Run(ctx Context, kongCtx *kong.Context) error {
	indexDumpOutput := indexDumpOutputWrapper(ctx.clientFactory)

	switch kongCtx.Path[len(kongCtx.Path)-1].Command.Name {
	case "devices":
		return indexDumpOutput("cluster/pending/devices")
	case "folders":
		if p.Folders.Device != "" {
			query := make(url.Values)
			query.Set("device", p.Folders.Device)
			return indexDumpOutput("cluster/pending/folders?" + query.Encode())
		}
		return indexDumpOutput("cluster/pending/folders")
	}

	return nil
}
