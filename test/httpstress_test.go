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

// +build integration

package integration_test

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
		log:    "2.out",
		argv:   []string{"-home", "h2"},
		port:   8082,
		apiKey: apiKey,
	}
	err = sender.start()
	if err != nil {
		t.Fatal(err)
	}

	tc := &tls.Config{InsecureSkipVerify: true}
	tr := &http.Transport{
		TLSClientConfig: tc,
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   2 * time.Second,
	}
	var wg sync.WaitGroup
	t0 := time.Now()

	var counter int
	var lock sync.Mutex

	errChan := make(chan error, 2)

	// One thread with immediately closed connections
	wg.Add(1)
	go func() {
		for time.Since(t0).Seconds() < 30 {
			conn, err := net.Dial("tcp", "localhost:8082")
			if err != nil {
				log.Println(err)
				errChan <- err
				return
			}
			conn.Close()
		}
		wg.Done()
	}()

	// 50 threads doing HTTP and HTTPS requests
	for i := 0; i < 50; i++ {
		i := i
		wg.Add(1)
		go func() {
			for time.Since(t0).Seconds() < 30 {
				proto := "http"
				if i%2 == 0 {
					proto = "https"
				}
				resp, err := client.Get(proto + "://localhost:8082/")
				if err != nil {
					errChan <- err
					return
				}
				bs, _ := ioutil.ReadAll(resp.Body)
				resp.Body.Close()
				if !bytes.Contains(bs, []byte("</html>")) {
					log.Printf("%s", bs)
					errChan <- errors.New("Incorrect response")
					return
				}

				lock.Lock()
				counter++
				lock.Unlock()
			}
			wg.Done()
		}()
	}

	go func() {
		wg.Wait()
		errChan <- nil
	}()

	err = <-errChan

	t.Logf("%.01f reqs/sec", float64(counter)/time.Since(t0).Seconds())

	sender.stop()
	if err != nil {
		t.Error(err)
	}
}
