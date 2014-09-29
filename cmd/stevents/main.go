// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

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
