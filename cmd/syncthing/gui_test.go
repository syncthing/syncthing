// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/d4l3k/messagediff"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/thejerf/suture"
)

func TestCSRFToken(t *testing.T) {
	t1 := newCsrfToken()
	t2 := newCsrfToken()

	t3 := newCsrfToken()
	if !validCsrfToken(t3) {
		t.Fatal("t3 should be valid")
	}

	for i := 0; i < 250; i++ {
		if i%5 == 0 {
			// t1 and t2 should remain valid by virtue of us checking them now
			// and then.
			if !validCsrfToken(t1) {
				t.Fatal("t1 should be valid at iteration", i)
			}
			if !validCsrfToken(t2) {
				t.Fatal("t2 should be valid at iteration", i)
			}
		}

		// The newly generated token is always valid
		t4 := newCsrfToken()
		if !validCsrfToken(t4) {
			t.Fatal("t4 should be valid at iteration", i)
		}
	}

	if validCsrfToken(t3) {
		t.Fatal("t3 should have expired by now")
	}
}

func TestStopAfterBrokenConfig(t *testing.T) {
	cfg := config.Configuration{
		GUI: config.GUIConfiguration{
			RawAddress: "127.0.0.1:0",
			RawUseTLS:  false,
		},
	}
	w := config.Wrap("/dev/null", cfg)

	srv, err := newAPIService(protocol.LocalDeviceID, w, "../../test/h1/https-cert.pem", "../../test/h1/https-key.pem", "", nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv.started = make(chan struct{})

	sup := suture.NewSimple("test")
	sup.Add(srv)
	sup.ServeBackground()

	<-srv.started

	// Service is now running, listening on a random port on localhost. Now we
	// request a config change to a completely invalid listen address. The
	// commit will fail and the service will be in a broken state.

	newCfg := config.Configuration{
		GUI: config.GUIConfiguration{
			RawAddress: "totally not a valid address",
			RawUseTLS:  false,
		},
	}
	if srv.CommitConfiguration(cfg, newCfg) {
		t.Fatal("Config commit should have failed")
	}

	// Nonetheless, it should be fine to Stop() it without panic.

	sup.Stop()
}

func TestAssetsDir(t *testing.T) {
	// For any given request to $FILE, we should return the first found of
	//  - assetsdir/$THEME/$FILE
	//  - compiled in asset $THEME/$FILE
	//  - assetsdir/default/$FILE
	//  - compiled in asset default/$FILE

	// The asset map contains compressed assets, so create a couple of gzip compressed assets here.
	buf := new(bytes.Buffer)
	gw := gzip.NewWriter(buf)
	gw.Write([]byte("default"))
	gw.Close()
	def := buf.Bytes()

	buf = new(bytes.Buffer)
	gw = gzip.NewWriter(buf)
	gw.Write([]byte("foo"))
	gw.Close()
	foo := buf.Bytes()

	e := embeddedStatic{
		theme:    "foo",
		mut:      sync.NewRWMutex(),
		assetDir: "testdata",
		assets: map[string][]byte{
			"foo/a":     foo, // overridden in foo/a
			"foo/b":     foo,
			"default/a": def, // overridden in default/a (but foo/a takes precedence)
			"default/b": def, // overridden in default/b (but foo/b takes precedence)
			"default/c": def,
		},
	}

	s := httptest.NewServer(e)
	defer s.Close()

	// assetsdir/foo/a exists, overrides compiled in
	expectURLToContain(t, s.URL+"/a", "overridden-foo")

	// foo/b is compiled in, default/b is overriden, return compiled in
	expectURLToContain(t, s.URL+"/b", "foo")

	// only exists as compiled in default/c so use that
	expectURLToContain(t, s.URL+"/c", "default")

	// only exists as overriden default/d so use that
	expectURLToContain(t, s.URL+"/d", "overridden-default")
}

func expectURLToContain(t *testing.T, url, exp string) {
	res, err := http.Get(url)
	if err != nil {
		t.Error(err)
		return
	}

	if res.StatusCode != 200 {
		t.Errorf("Got %s instead of 200 OK", res.Status)
		return
	}

	data, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Error(err)
		return
	}

	if string(data) != exp {
		t.Errorf("Got %q instead of %q on %q", data, exp, url)
		return
	}
}

func TestDirNames(t *testing.T) {
	names := dirNames("testdata")
	expected := []string{"default", "foo", "testfolder"}
	if diff, equal := messagediff.PrettyDiff(expected, names); !equal {
		t.Errorf("Unexpected dirNames return: %#v\n%s", names, diff)
	}
}

func TestAPIServiceRequests(t *testing.T) {
	model := new(mockedModel)
	cfg := new(mockedConfig)
	httpsCertFile := "../../test/h1/https-cert.pem"
	httpsKeyFile := "../../test/h1/https-key.pem"
	assetDir := "../../gui"
	eventSub := new(mockedEventSub)
	discoverer := new(mockedCachingMux)
	relayService := new(mockedRelayService)
	errorLog := new(mockedLoggerRecorder)
	systemLog := new(mockedLoggerRecorder)

	// Instantiate the API service
	svc, err := newAPIService(protocol.LocalDeviceID, cfg, httpsCertFile, httpsKeyFile, assetDir, model,
		eventSub, discoverer, relayService, errorLog, systemLog)
	if err != nil {
		t.Fatal(err)
	}
	_ = svc

	// Make sure the API service is listening, and get the URL to use.
	addr := svc.listener.Addr()
	if addr == nil {
		t.Fatal("Nil listening address from API service")
	}
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr.String())
	if err != nil {
		t.Fatal("Weird address from API service:", err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", tcpAddr.Port)

	// Actually start the API service
	supervisor := suture.NewSimple("API test")
	supervisor.Add(svc)
	supervisor.ServeBackground()

	// Try requests to common URLs, all of which should succeed and return
	// some sort of JSON object.
	urls := []string{
		"/rest/system/status",
		"/rest/system/config",
		"/rest/system/config/insync",
		// "/rest/system/connections", does not return an object in the empty case ("null"), should be fixed
		"/rest/system/discovery",
		"/rest/system/error?since=0",
		"/rest/system/ping",
		// "/rest/system/upgrade", depends on Github API, not good for testing
		"/rest/system/version",
		"/rest/system/debug",
		"/rest/system/log?since=0",
	}
	for _, url := range urls {
		t.Log("Testing", url, "...")
		testHTTPJSONObject(t, baseURL+url)
	}
}

// testHTTPJSONObject tries the given URL and verifies that the HTTP request
// succeeds and that a JSON object (something beginning with "{") is
// returned. Returns the object data, or nil on failure.
func testHTTPJSONObject(t *testing.T, url string) []byte {
	resp := testHTTPRequest(t, url)
	if resp == nil {
		return nil
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Errorf("The content type on %s should be application/json, not %q", url, ct)
		return nil
	}

	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("Unexpected error reading %s: %v", url, err)
		return nil
	}

	if !bytes.HasPrefix(data, []byte("{")) {
		t.Errorf("Returned data from %s does not look like a JSON object: %s", url, data)
		return nil
	}
	return data
}

// testHTTPRequest performs a HTTP GET request and verifies that the
// response is successfull (code 200). Returns the *http.Response or nil on
// failure.
func testHTTPRequest(t *testing.T, url string) *http.Response {
	cli := &http.Client{
		Timeout: time.Second,
	}
	resp, err := cli.Get(url)
	if err != nil {
		t.Errorf("Unexpected error requesting %s: %v", url, err)
		return nil
	}
	if resp.StatusCode != 200 {
		t.Errorf("Get on %s should have returned status code 200, not %s", url, resp.Status)
		return nil
	}
	return resp
}
