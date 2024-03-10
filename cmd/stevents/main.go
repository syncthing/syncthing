// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/syncthing/syncthing/lib/automaxprocs"
)

type event struct {
	ID   int                    `json:"id"`
	Type string                 `json:"type"`
	Time time.Time              `json:"time"`
	Data map[string]interface{} `json:"data"`
}

func main() {
	log.SetOutput(os.Stdout)
	log.SetFlags(0)

	target := flag.String("target", "localhost:8384", "Target Syncthing instance")
	types := flag.String("types", "", "Filter for specific event types (comma-separated)")
	apikey := flag.String("apikey", "", "Syncthing API key")
	flag.Parse()

	if *apikey == "" {
		log.Fatal("Must give -apikey argument")
	}
	var eventsArg string
	if len(*types) > 0 {
		eventsArg = "&events=" + *types
	}

	since := 0
	for {
		req, err := http.NewRequest("GET", fmt.Sprintf("http://%s/rest/events?since=%d%s", *target, since, eventsArg), nil)
		if err != nil {
			log.Fatal(err)
		}
		req.Header.Set("X-API-Key", *apikey)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Fatal(err)
		}

		var events []event
		err = json.NewDecoder(res.Body).Decode(&events)
		if err != nil {
			log.Fatal(err)
		}
		res.Body.Close()

		for _, event := range events {
			bs, _ := json.MarshalIndent(event, "", "    ")
			log.Printf("%s", bs)
			since = event.ID
		}
	}
}
