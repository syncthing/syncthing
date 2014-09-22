// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// +build ignore

package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"testing"
)

var (
	target    string
	authUser  string
	authPass  string
	csrfToken string
	csrfFile  string
	apiKey    string
)

var jsonEndpoints = []string{
	"/rest/completion?node=I6KAH76-66SLLLB-5PFXSOA-UFJCDZC-YAOMLEK-CP2GB32-BV5RQST-3PSROAU&repo=default",
	"/rest/config",
	"/rest/config/sync",
	"/rest/connections",
	"/rest/errors",
	"/rest/events",
	"/rest/lang",
	"/rest/model/version?repo=default",
	"/rest/model?repo=default",
	"/rest/need",
	"/rest/nodeid?id=I6KAH7666SLLLB5PFXSOAUFJCDZCYAOMLEKCP2GB32BV5RQST3PSROAU",
	"/rest/report",
	"/rest/system",
}

func main() {
	flag.StringVar(&target, "target", "localhost:8080", "Test target")
	flag.StringVar(&authUser, "user", "", "Username")
	flag.StringVar(&authPass, "pass", "", "Password")
	flag.StringVar(&csrfFile, "csrf", "", "CSRF token file")
	flag.StringVar(&apiKey, "api", "", "API key")
	flag.Parse()

	if len(csrfFile) > 0 {
		fd, err := os.Open(csrfFile)
		if err != nil {
			log.Fatal(err)
		}
		s := bufio.NewScanner(fd)
		for s.Scan() {
			csrfToken = s.Text()
		}
		fd.Close()
	}

	var tests []testing.InternalTest
	tests = append(tests, testing.InternalTest{"TestGetIndex", TestGetIndex})
	tests = append(tests, testing.InternalTest{"TestJSONEndpoints", TestJSONEndpoints})
	tests = append(tests, testing.InternalTest{"TestPOSTNoCSRF", TestPOSTNoCSRF})

	if len(authUser) > 0 {
		// If we expect authentication, verify that it fails with the wrong password and wrong API key
		tests = append(tests, testing.InternalTest{"TestJSONEndpointsNoAuth", TestJSONEndpointsNoAuth})
		tests = append(tests, testing.InternalTest{"TestJSONEndpointsIncorrectAuth", TestJSONEndpointsIncorrectAuth})
	}

	if len(csrfToken) > 0 {
		// If we have a CSRF token, verify that POST succeeds with it
		tests = append(tests, testing.InternalTest{"TestPostWitchCSRF", TestPostWitchCSRF})
		tests = append(tests, testing.InternalTest{"TestGetPostConfigOK", TestGetPostConfigOK})
		tests = append(tests, testing.InternalTest{"TestGetPostConfigFail", TestGetPostConfigFail})
	}

	fmt.Printf("Testing HTTP: CSRF=%v, API=%v, Auth=%v\n", len(csrfToken) > 0, len(apiKey) > 0, len(authUser) > 0)
	testing.Main(matcher, tests, nil, nil)
}

func matcher(s0, s1 string) (bool, error) {
	return true, nil
}

func TestGetIndex(t *testing.T) {
	res, err := get("/index.html")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 {
		t.Errorf("Status %d != 200", res.StatusCode)
	}
	if res.ContentLength < 1024 {
		t.Errorf("Length %d < 1024", res.ContentLength)
	}
	res.Body.Close()

	res, err = get("/")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 {
		t.Errorf("Status %d != 200", res.StatusCode)
	}
	if res.ContentLength < 1024 {
		t.Errorf("Length %d < 1024", res.ContentLength)
	}
	res.Body.Close()
}

func TestGetVersion(t *testing.T) {
	res, err := get("/rest/version")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("Status %d != 200", res.StatusCode)
	}
	ver, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	if !regexp.MustCompile(`v\d+\.\d+\.\d+`).Match(ver) {
		t.Errorf("Invalid version %q", ver)
	}
}

func TestGetVersionNoCSRF(t *testing.T) {
	r, err := http.NewRequest("GET", "http://"+target+"/rest/version", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(authUser) > 0 {
		r.SetBasicAuth(authUser, authPass)
	}
	res, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 403 {
		t.Fatalf("Status %d != 403", res.StatusCode)
	}
}

func TestJSONEndpoints(t *testing.T) {
	for _, p := range jsonEndpoints {
		res, err := get(p)
		if err != nil {
			t.Error(err)
			continue
		}
		if res.StatusCode != 200 {
			t.Errorf("Status %d != 200 for %q", res.StatusCode, p)
			continue
		}
		if ct := res.Header.Get("Content-Type"); ct != "application/json; charset=utf-8" {
			t.Errorf("Content-Type %q != \"application/json\" for %q", ct, p)
			continue
		}
	}
}

func TestPOSTNoCSRF(t *testing.T) {
	r, err := http.NewRequest("POST", "http://"+target+"/rest/error/clear", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(authUser) > 0 {
		r.SetBasicAuth(authUser, authPass)
	}
	res, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 403 && res.StatusCode != 401 {
		t.Fatalf("Status %d != 403/401 for POST", res.StatusCode)
	}
}

func TestPostWitchCSRF(t *testing.T) {
	r, err := http.NewRequest("POST", "http://"+target+"/rest/error/clear", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(csrfToken) > 0 {
		r.Header.Set("X-CSRF-Token", csrfToken)
	}
	if len(authUser) > 0 {
		r.SetBasicAuth(authUser, authPass)
	}
	res, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("Status %d != 200 for POST", res.StatusCode)
	}
}

func TestGetPostConfigOK(t *testing.T) {
	// Get config
	r, err := http.NewRequest("GET", "http://"+target+"/rest/config", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(csrfToken) > 0 {
		r.Header.Set("X-CSRF-Token", csrfToken)
	}
	if len(authUser) > 0 {
		r.SetBasicAuth(authUser, authPass)
	}
	res, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("Status %d != 200 for POST", res.StatusCode)
	}
	bs, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	// Post same config back
	r, err = http.NewRequest("POST", "http://"+target+"/rest/config", bytes.NewBuffer(bs))
	if err != nil {
		t.Fatal(err)
	}
	if len(csrfToken) > 0 {
		r.Header.Set("X-CSRF-Token", csrfToken)
	}
	if len(authUser) > 0 {
		r.SetBasicAuth(authUser, authPass)
	}
	res, err = http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("Status %d != 200 for POST", res.StatusCode)
	}
}

func TestGetPostConfigFail(t *testing.T) {
	// Get config
	r, err := http.NewRequest("GET", "http://"+target+"/rest/config", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(csrfToken) > 0 {
		r.Header.Set("X-CSRF-Token", csrfToken)
	}
	if len(authUser) > 0 {
		r.SetBasicAuth(authUser, authPass)
	}
	res, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("Status %d != 200 for POST", res.StatusCode)
	}
	bs, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	// Post same config back, with some characters missing to create a syntax error
	r, err = http.NewRequest("POST", "http://"+target+"/rest/config", bytes.NewBuffer(bs[2:]))
	if err != nil {
		t.Fatal(err)
	}
	if len(csrfToken) > 0 {
		r.Header.Set("X-CSRF-Token", csrfToken)
	}
	if len(authUser) > 0 {
		r.SetBasicAuth(authUser, authPass)
	}
	res, err = http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 500 {
		t.Fatalf("Status %d != 500 for POST", res.StatusCode)
	}
}

func TestJSONEndpointsNoAuth(t *testing.T) {
	for _, p := range jsonEndpoints {
		r, err := http.NewRequest("GET", "http://"+target+p, nil)
		if err != nil {
			t.Error(err)
			continue
		}
		if len(csrfToken) > 0 {
			r.Header.Set("X-CSRF-Token", csrfToken)
		}
		res, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Error(err)
			continue
		}
		if res.StatusCode != 403 && res.StatusCode != 401 {
			t.Errorf("Status %d != 403/401 for %q", res.StatusCode, p)
			continue
		}
	}
}

func TestJSONEndpointsIncorrectAuth(t *testing.T) {
	for _, p := range jsonEndpoints {
		r, err := http.NewRequest("GET", "http://"+target+p, nil)
		if err != nil {
			t.Error(err)
			continue
		}
		if len(csrfToken) > 0 {
			r.Header.Set("X-CSRF-Token", csrfToken)
		}
		r.SetBasicAuth("wronguser", "wrongpass")
		res, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Error(err)
			continue
		}
		if res.StatusCode != 403 && res.StatusCode != 401 {
			t.Errorf("Status %d != 403/401 for %q", res.StatusCode, p)
			continue
		}
	}
}

func get(path string) (*http.Response, error) {
	r, err := http.NewRequest("GET", "http://"+target+path, nil)
	if err != nil {
		return nil, err
	}
	if len(authUser) > 0 {
		r.SetBasicAuth(authUser, authPass)
	}
	if len(apiKey) > 0 {
		r.Header.Set("X-API-Key", apiKey)
	}
	return http.DefaultClient.Do(r)
}
