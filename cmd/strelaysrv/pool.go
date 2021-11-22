// Copyright (C) 2015 Audrius Butkevicius and Contributors.

package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"
)

const (
	httpStatusEnhanceYourCalm = 429
)

func poolHandler(pool string, uri *url.URL, mapping mapping, ownCert tls.Certificate) {
	if debug {
		log.Println("Joining", pool)
	}
	for {
		uriCopy := *uri
		uriCopy.Host = mapping.Address().String()

		var b bytes.Buffer
		json.NewEncoder(&b).Encode(struct {
			URL string `json:"url"`
		}{
			uriCopy.String(),
		})

		poolUrl, err := url.Parse(pool)
		if err != nil {
			log.Printf("Could not parse pool url '%s': %v", pool, err)
		}

		client := http.DefaultClient
		if poolUrl.Scheme == "https" {
			// Sent our certificate in join request
			client = &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{
						Certificates: []tls.Certificate{ownCert},
					},
				},
			}
		}

		resp, err := client.Post(pool, "application/json", &b)
		if err != nil {
			log.Printf("Error joining pool %v: HTTP request: %v", pool, err)
			time.Sleep(time.Minute)
			continue
		}

		bs, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Printf("Error joining pool %v: reading response: %v", pool, err)
			time.Sleep(time.Minute)
			continue
		}

		switch resp.StatusCode {
		case http.StatusOK:
			var x struct {
				EvictionIn time.Duration `json:"evictionIn"`
			}
			if err := json.Unmarshal(bs, &x); err == nil {
				rejoin := x.EvictionIn - (x.EvictionIn / 5)
				log.Printf("Joined pool %s, rejoining in %v", pool, rejoin)
				time.Sleep(rejoin)
				continue
			} else {
				log.Printf("Joined pool %s, failed to deserialize response: %v", pool, err)
			}

		case http.StatusInternalServerError:
			log.Printf("Failed to join %v: server error", pool)
			log.Printf("Response data: %s", bs)
			time.Sleep(time.Minute)
			continue

		case http.StatusBadRequest:
			log.Printf("Failed to join %v: request or check error", pool)
			log.Printf("Response data: %s", bs)
			time.Sleep(time.Minute)
			continue

		case httpStatusEnhanceYourCalm:
			log.Printf("Failed to join %v: under load (rate limiting)", pool)
			time.Sleep(time.Minute)
			continue

		case http.StatusUnauthorized:
			log.Printf("Failed to join %v: IP address not matching external address", pool)
			log.Println("Aborting")
			return

		default:
			log.Printf("Failed to join %v: unexpected status code from server: %d", pool, resp.StatusCode)
			log.Printf("Response data: %s", bs)
			time.Sleep(time.Minute)
			continue
		}

		time.Sleep(time.Hour)
	}
}
