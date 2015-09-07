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
	for {
		var b bytes.Buffer
		json.NewEncoder(&b).Encode(struct {
			URL string `json:"url"`
		}{
			uri.String(),
		})

		resp, err := http.Post(pool, "application/json", &b)
		if err != nil {
			if debug {
				log.Println("Error joining pool", pool, err)
			}
		} else if resp.StatusCode == 500 {
			if debug {
				bs, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					log.Println("Failed to read response body for", pool, err)
				} else {
					log.Println("Response for", pool, string(bs))
				}
				resp.Body.Close()
			}
		} else if resp.StatusCode == 429 {
			if debug {
				log.Println(pool, "under load, will retry in a minute")
			}
			time.Sleep(time.Minute)
			continue
		} else if resp.StatusCode == 200 {
			var x struct {
				EvictionIn time.Duration `json:"evictionIn"`
			}
			err := json.NewDecoder(resp.Body).Decode(&x)
			if err == nil {
				rejoin := x.EvictionIn - (x.EvictionIn / 5)
				if debug {
					log.Println("Joined", pool, "rejoining in", rejoin)
				}
				time.Sleep(rejoin)
				continue
			} else if debug {
				log.Println("Failed to deserialize respnse", err)
			}
		}
		time.Sleep(time.Hour)
	}
}
