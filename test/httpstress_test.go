// Copyright (C) 2014 The Syncthing Authors.
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

// +build integration

package integration

import (
	"bytes"
	"crypto/tls"
	"errors"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestStressHTTP(t *testing.T) {
	log.Println("Cleaning...")
	err := removeAll("s2", "h2/index")
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Starting up...")
	sender := syncthingProcess{ // id1
		instance: "2",
		argv:     []string{"-home", "h2"},
		port:     8082,
		apiKey:   apiKey,
	}
	err = sender.start()
	if err != nil {
		t.Fatal(err)
	}

	// Create a client with reasonable timeouts on all stages of the request.

	tc := &tls.Config{InsecureSkipVerify: true}
	tr := &http.Transport{
		TLSClientConfig:       tc,
		DisableKeepAlives:     true,
		ResponseHeaderTimeout: 10 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   10 * time.Second,
	}

	var (
		requestsOK    = map[string]int{}
		requestsError = map[string]int{}
		firstError    error
		lock          sync.Mutex
	)

	gotError := func(ctx string, err error) {
		lock.Lock()
		requestsError[ctx]++
		if firstError == nil {
			firstError = err
		}
		lock.Unlock()
	}

	requestOK := func(ctx string) {
		lock.Lock()
		requestsOK[ctx]++
		lock.Unlock()
	}

	log.Println("Testing...")

	var wg sync.WaitGroup
	t0 := time.Now()

	// One thread with immediately closed connections
	wg.Add(1)
	go func() {
		defer wg.Done()
		for time.Since(t0).Seconds() < 30 {
			conn, err := net.Dial("tcp", "localhost:8082")
			if err != nil {
				gotError("Dial", err)
			} else {
				requestOK("Dial")
				conn.Close()
			}

			// At most 100 connects/sec
			time.Sleep(10 * time.Millisecond)
		}
	}()

	// 50 threads doing mixed HTTP and HTTPS requests
	for i := 0; i < 50; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Since(t0).Seconds() < 30 {
				proto := "http"
				if i%2 == 0 {
					proto = "https"
				}
				url := proto + "://localhost:8082/"
				resp, err := client.Get(url)

				if err != nil {
					gotError("Get "+proto, err)
					continue
				}

				bs, err := ioutil.ReadAll(resp.Body)
				resp.Body.Close()

				if err != nil {
					gotError("Read "+proto, err)
					continue
				}

				if !bytes.Contains(bs, []byte("</html>")) {
					err := errors.New("Incorrect response")
					gotError("Get "+proto, err)
					continue
				}

				requestOK(url)

				// At most 100 requests/sec
				time.Sleep(10 * time.Millisecond)
			}
		}()
	}

	wg.Wait()
	t.Logf("OK: %v reqs", requestsOK)
	t.Logf("Err: %v reqs", requestsError)

	if firstError != nil {
		t.Error(firstError)
	}

	err = sender.stop()
	if err != nil {
		t.Error(err)
	}
}
