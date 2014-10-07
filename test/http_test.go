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
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

var jsonEndpoints = []string{
	"/rest/completion?device=I6KAH76-66SLLLB-5PFXSOA-UFJCDZC-YAOMLEK-CP2GB32-BV5RQST-3PSROAU&folder=default",
	"/rest/config",
	"/rest/config/sync",
	"/rest/connections",
	"/rest/errors",
	"/rest/events",
	"/rest/lang",
	"/rest/model?folder=default",
	"/rest/need",
	"/rest/deviceid?id=I6KAH7666SLLLB5PFXSOAUFJCDZCYAOMLEKCP2GB32BV5RQST3PSROAU",
	"/rest/report",
	"/rest/system",
}

func TestGetIndex(t *testing.T) {
	st := syncthingProcess{
		argv: []string{"-home", "h2"},
		port: 8082,
		log:  "2.out",
	}
	err := st.start()
	if err != nil {
		t.Fatal(err)
	}
	defer st.stop()

	res, err := st.get("/index.html")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 {
		t.Errorf("Status %d != 200", res.StatusCode)
	}
	if res.ContentLength < 1024 {
		t.Errorf("Length %d < 1024", res.ContentLength)
	}
	if res.Header.Get("Set-Cookie") == "" {
		t.Error("No set-cookie header")
	}
	res.Body.Close()

	res, err = st.get("/")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 {
		t.Errorf("Status %d != 200", res.StatusCode)
	}
	if res.ContentLength < 1024 {
		t.Errorf("Length %d < 1024", res.ContentLength)
	}
	if res.Header.Get("Set-Cookie") == "" {
		t.Error("No set-cookie header")
	}
	res.Body.Close()
}

func TestGetIndexAuth(t *testing.T) {
	st := syncthingProcess{
		argv: []string{"-home", "h1"},
		port: 8081,
		log:  "1.out",
	}
	err := st.start()
	if err != nil {
		t.Fatal(err)
	}
	defer st.stop()

	// Without auth should give 401

	res, err := http.Get("http://127.0.0.1:8081/")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 401 {
		t.Errorf("Status %d != 401", res.StatusCode)
	}

	// With wrong username/password should give 401

	req, err := http.NewRequest("GET", "http://127.0.0.1:8081/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetBasicAuth("testuser", "wrongpass")

	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 401 {
		t.Fatalf("Status %d != 401", res.StatusCode)
	}

	// With correct username/password should succeed

	req, err = http.NewRequest("GET", "http://127.0.0.1:8081/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetBasicAuth("testuser", "testpass")

	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("Status %d != 200", res.StatusCode)
	}
}

func TestGetJSON(t *testing.T) {
	st := syncthingProcess{
		argv: []string{"-home", "h2"},
		port: 8082,
		log:  "2.out",
	}
	err := st.start()
	if err != nil {
		t.Fatal(err)
	}
	defer st.stop()

	for _, path := range jsonEndpoints {
		res, err := st.get(path)
		if err != nil {
			t.Error(err)
		}

		if ct := res.Header.Get("Content-Type"); ct != "application/json; charset=utf-8" {
			t.Errorf("Incorrect Content-Type %q for %q", ct, path)
		}

		var intf interface{}
		err = json.NewDecoder(res.Body).Decode(&intf)
		res.Body.Close()

		if err != nil {
			t.Error(err)
		}
	}
}

func TestPOSTWithoutCSRF(t *testing.T) {
	st := syncthingProcess{
		argv: []string{"-home", "h2"},
		port: 8082,
		log:  "2.out",
	}
	err := st.start()
	if err != nil {
		t.Fatal(err)
	}
	defer st.stop()

	// Should fail without CSRF

	req, err := http.NewRequest("POST", "http://127.0.0.1:8082/rest/error/clear", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatalf("Status %d != 403 for POST", res.StatusCode)
	}

	// Get CSRF

	req, err = http.NewRequest("GET", "http://127.0.0.1:8082/", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	hdr := res.Header.Get("Set-Cookie")
	if !strings.Contains(hdr, "CSRF-Token") {
		t.Error("Missing CSRF-Token in", hdr)
	}

	// Should succeed with CSRF

	req, err = http.NewRequest("POST", "http://127.0.0.1:8082/rest/error/clear", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-CSRF-Token", hdr[len("CSRF-Token="):])
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("Status %d != 200 for POST", res.StatusCode)
	}

	// Should fail with incorrect CSRF

	req, err = http.NewRequest("POST", "http://127.0.0.1:8082/rest/error/clear", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-CSRF-Token", hdr[len("CSRF-Token="):]+"X")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatalf("Status %d != 403 for POST", res.StatusCode)
	}
}
