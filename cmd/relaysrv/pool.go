// Copyright (C) 2015 Audrius Butkevicius and Contributors (see the CONTRIBUTORS file).

package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"time"
)

func poolHandler(pool string, uri *url.URL) {
	if debug {
		log.Println("Joining", pool)
	}
	for {
		var b bytes.Buffer
		json.NewEncoder(&b).Encode(struct {
			URL string `json:"url"`
		}{
			uri.String(),
		})

		resp, err := http.Post(pool, "application/json", &b)
		if err != nil {
			log.Println("Error joining pool", pool, err)
		} else if resp.StatusCode == 500 {
			bs, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Println("Failed to join", pool, "due to an internal server error. Could not read response:", err)
			} else {
				log.Println("Failed to join", pool, "due to an internal server error:", string(bs))
			}
			resp.Body.Close()
		} else if resp.StatusCode == 429 {
			log.Println(pool, "under load, will retry in a minute")
			time.Sleep(time.Minute)
			continue
		} else if resp.StatusCode == 403 {
			log.Println(pool, "failed to join due to IP address not matching external address. Aborting")
			return
		} else if resp.StatusCode == 200 {
			var x struct {
				EvictionIn time.Duration `json:"evictionIn"`
			}
			err := json.NewDecoder(resp.Body).Decode(&x)
			if err == nil {
				rejoin := x.EvictionIn - (x.EvictionIn / 5)
				log.Println("Joined", pool, "rejoining in", rejoin)
				time.Sleep(rejoin)
				continue
			} else {
				log.Println("Failed to deserialize response", err)
			}
		} else {
			log.Println(pool, "unknown response type from server", resp.StatusCode)
		}
		time.Sleep(time.Hour)
	}
}
