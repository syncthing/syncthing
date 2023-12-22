// Copyright (C) 2023 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"log"
	"os"

	"github.com/alecthomas/kong"
	"github.com/syncthing/syncthing/cmd/ursrv/aggregate"
	"github.com/syncthing/syncthing/cmd/ursrv/blob"
	"github.com/syncthing/syncthing/cmd/ursrv/serve"
)

type CLI struct {
	Serve       serve.CLI     `cmd:"" default:""`
	Aggregate   aggregate.CLI `cmd:""`
	S3Bucket    string        `env:"UR_S3_BUCKET"`
	S3Endpoint  string        `env:"UR_S3_ENDPOINT"`
	S3Region    string        `env:"UR_S3_REGION" default:"eu-north-1"`
	S3AccessKey string        `env:"UR_S3_ACCESS_KEY"`
	S3SecretKey string        `env:"UR_S3_SECRET_KEY"`
}

func main() {
	log.SetFlags(log.Ltime | log.Ldate | log.Lshortfile)
	log.SetOutput(os.Stdout)

	var cli CLI
	ctx := kong.Parse(&cli)

	s3Config := blob.S3Config{
		Bucket:    cli.S3Bucket,
		Endpoint:  cli.S3Endpoint,
		Region:    cli.S3Region,
		AccessKey: cli.S3AccessKey,
		SecretKey: cli.S3SecretKey,
	}

	if err := ctx.Run(s3Config); err != nil {
		log.Fatalf("%s: %v", ctx.Command(), err)
	}
}
