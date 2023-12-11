// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/alecthomas/kong"
)

type errorsCommand struct {
	Show  struct{}          `cmd:"" help:"Show pending errors"`
	Push  errorsPushCommand `cmd:"" help:"Push an error to active clients"`
	Clear struct{}          `cmd:"" help:"Clear pending errors"`
}

type errorsPushCommand struct {
	ErrorMessage string `arg:""`
}

func (e *errorsPushCommand) Run(ctx Context) error {
	client, err := ctx.clientFactory.getClient()
	if err != nil {
		return err
	}
	errStr := e.ErrorMessage
	response, err := client.Post("system/error", strings.TrimSpace(errStr))
	if err != nil {
		return err
	}
	if response.StatusCode != 200 {
		errStr = fmt.Sprint("Failed to push error\nStatus code: ", response.StatusCode)
		bytes, err := responseToBArray(response)
		if err != nil {
			return err
		}
		body := string(bytes)
		if body != "" {
			errStr += "\nBody: " + body
		}
		return errors.New(errStr)
	}
	return nil
}

func (*errorsCommand) Run(ctx Context, kongCtx *kong.Context) error {
	switch kongCtx.Selected().Name {
	case "show":
		return indexDumpOutput("system/error", ctx.clientFactory)
	case "clear":
		return emptyPost("system/error/clear", ctx.clientFactory)
	}
	return nil
}
