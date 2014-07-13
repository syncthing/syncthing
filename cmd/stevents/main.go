package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"
)

type event struct {
	ID   int
	Type string
	Time time.Time
	Data map[string]interface{}
}

func main() {
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

		for _, event := range events {
			log.Printf("%d: %v", event.ID, event.Type)
			for k, v := range event.Data {
				log.Printf("\t%s: %v", k, v)
			}
			since = event.ID
		}
	}
}
