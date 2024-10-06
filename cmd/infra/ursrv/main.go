// Copyright (C) 2023 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"log"
	"log/slog"
	"os"

	"github.com/alecthomas/kong"
	"github.com/syncthing/syncthing/cmd/infra/ursrv/serve"
	_ "github.com/syncthing/syncthing/lib/automaxprocs"
)

type CLI struct {
	Serve serve.CLI `cmd:"" default:""`
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	var cli CLI
	ctx := kong.Parse(&cli)
	if err := ctx.Run(); err != nil {
		log.Fatalf("%s: %v", ctx.Command(), err)
	}
}
