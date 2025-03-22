// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package cli

import (
	"fmt"
	"net/url"
)

type fileCommand struct {
	FolderID string `arg:""`
	Path     string `arg:""`
}

func (f *fileCommand) Run(ctx Context) error {
	indexDumpOutput := indexDumpOutputWrapper(ctx.clientFactory)

	query := make(url.Values)
	query.Set("folder", f.FolderID)
	query.Set("file", normalizePath(f.Path))
	return indexDumpOutput("debug/file?" + query.Encode())
}

type profileCommand struct {
	Type string `arg:"" help:"cpu | heap"`
}

func (p *profileCommand) Run(ctx Context) error {
	switch t := p.Type; t {
	case "cpu", "heap":
		return saveToFile(fmt.Sprintf("debug/%vprof", p.Type), ctx.clientFactory)
	default:
		return fmt.Errorf("expected cpu or heap as argument, got %v", t)
	}
}

type debugCommand struct {
	File    fileCommand    `cmd:"" help:"Show information about a file (or directory/symlink)"`
	Profile profileCommand `cmd:"" help:"Save a profile to help figuring out what Syncthing does"`
}
