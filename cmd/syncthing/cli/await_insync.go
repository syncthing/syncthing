// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/urfave/cli"
)

var awaitInsyncCommand = cli.Command{
	Name:     "await-insync",
	HideHelp: true,
	Usage:    "Block until the current dir finishes syncing",
	Action:   expects(0, insyncAction),
}

func insyncAction(c *cli.Context) error {
	// v2:
	// - read devices from the config
	// - trigger Rescan on each device
	// - allow passing the dir as a param
	dir, err := os.Getwd()
	if err != nil {
		return errors.New("couldn't not get the current dir")
	}
	client, err := getClientFactory(c).getClient()
	if err != nil {
		return err
	}
	cfg, err := getConfig(client)
	if err != nil {
		return err
	}

	// get the folder's ID
	folderID := ""
	for _, folder := range cfg.Folders {
		if folder.Path == dir {
			folderID = folder.ID
			break
		}
	}
	if folderID == "" {
		return errors.New("current folder not in the config")
	}

	waitingDisplayed := false
	// block until finished
	for {
		// request folder's status
		resp, err := client.Get("db/status?folder=" + folderID)
		if errors.Is(err, errNotFound) {
			return errors.New("not found (folder/file not in database)")
		}
		if err != nil {
			return err
		}

		bs, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		resp.Body.Close()

		// verify the sync state
		var status map[string]interface{}
		if err := json.Unmarshal(bs, &status); err != nil {
			return err
		}
		needTotalItems := int(status["needTotalItems"].(float64))
		if needTotalItems == 0 {
			if waitingDisplayed {
				fmt.Println("Done!")
			} else {
				fmt.Println("Already synced")
			}
			// synced
			break
		}
		if !waitingDisplayed {
			fmt.Println("Waiting for syncing to finish...")
			waitingDisplayed = true
		}
		// not synced
		time.Sleep(250 * time.Millisecond)
	}
	return nil
}
