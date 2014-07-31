// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
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

	target := flag.String("target", "localhost:8080", "Target Syncthing instance")
	apikey := flag.String("apikey", "", "Syncthing API key")
	flag.Parse()

	if *apikey == "" {
		log.Fatal("Must give -apikey argument")
	}

	since := 0
	for {
		req, err := http.NewRequest("GET", fmt.Sprintf("http://%s/rest/events?since=%d", *target, since), nil)
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
