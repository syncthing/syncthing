// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/d4l3k/messagediff"
	"github.com/syncthing/syncthing/lib/assets"
	"github.com/syncthing/syncthing/lib/config"
	connmocks "github.com/syncthing/syncthing/lib/connections/mocks"
	discovermocks "github.com/syncthing/syncthing/lib/discover/mocks"
	"github.com/syncthing/syncthing/lib/events"
	eventmocks "github.com/syncthing/syncthing/lib/events/mocks"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/locations"
	"github.com/syncthing/syncthing/lib/logger"
	loggermocks "github.com/syncthing/syncthing/lib/logger/mocks"
	modelmocks "github.com/syncthing/syncthing/lib/model/mocks"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/svcutil"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/tlsutil"
	"github.com/syncthing/syncthing/lib/ur"
	"github.com/syncthing/syncthing/lib/util"
	"github.com/thejerf/suture/v4"
)

var (
	confDir    = filepath.Join("testdata", "config")
	token      = filepath.Join(confDir, "csrftokens.txt")
	dev1       protocol.DeviceID
	apiCfg     = newMockedConfig()
	testAPIKey = "foobarbaz"
)

func init() {
	dev1, _ = protocol.DeviceIDFromString("AIR6LPZ-7K4PTTV-UXQSMUU-CPQ5YWH-OEDFIIQ-JUG777G-2YQXXR5-YD6AWQR")
	apiCfg.GUIReturns(config.GUIConfiguration{APIKey: testAPIKey})
}

func TestMain(m *testing.M) {
	orig := locations.GetBaseDir(locations.ConfigBaseDir)
	locations.SetBaseDir(locations.ConfigBaseDir, confDir)

	exitCode := m.Run()

	locations.SetBaseDir(locations.ConfigBaseDir, orig)

	os.Exit(exitCode)
}

func TestCSRFToken(t *testing.T) {
	t.Parallel()

	max := 250
	int := 5
	if testing.Short() {
		max = 20
		int = 2
	}

	m := newCsrfManager("unique", "prefix", config.GUIConfiguration{}, nil, "")

	t1 := m.newToken()
	t2 := m.newToken()

	t3 := m.newToken()
	if !m.validToken(t3) {
		t.Fatal("t3 should be valid")
	}

	for i := 0; i < max; i++ {
		if i%int == 0 {
			// t1 and t2 should remain valid by virtue of us checking them now
			// and then.
			if !m.validToken(t1) {
				t.Fatal("t1 should be valid at iteration", i)
			}
			if !m.validToken(t2) {
				t.Fatal("t2 should be valid at iteration", i)
			}
		}

		// The newly generated token is always valid
		t4 := m.newToken()
		if !m.validToken(t4) {
			t.Fatal("t4 should be valid at iteration", i)
		}
	}

	if m.validToken(t3) {
		t.Fatal("t3 should have expired by now")
	}
}

func TestStopAfterBrokenConfig(t *testing.T) {
	t.Parallel()

	cfg := config.Configuration{
		GUI: config.GUIConfiguration{
			RawAddress: "127.0.0.1:0",
			RawUseTLS:  false,
		},
	}
	w := config.Wrap("/dev/null", cfg, protocol.LocalDeviceID, events.NoopLogger)

	srv := New(protocol.LocalDeviceID, w, "", "syncthing", nil, nil, nil, events.NoopLogger, nil, nil, nil, nil, nil, nil, false).(*service)
	defer os.Remove(token)

	srv.started = make(chan string)

	sup := suture.New("test", svcutil.SpecWithDebugLogger(l))
	sup.Add(srv)
	ctx, cancel := context.WithCancel(context.Background())
	sup.ServeBackground(ctx)

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
	if err := srv.VerifyConfiguration(cfg, newCfg); err == nil {
		t.Fatal("Verify config should have failed")
	}

	cancel()
}

func TestAssetsDir(t *testing.T) {
	t.Parallel()

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
	def := assets.Asset{
		Content: buf.String(),
		Gzipped: true,
	}

	buf = new(bytes.Buffer)
	gw = gzip.NewWriter(buf)
	gw.Write([]byte("foo"))
	gw.Close()
	foo := assets.Asset{
		Content: buf.String(),
		Gzipped: true,
	}

	e := &staticsServer{
		theme:    "foo",
		mut:      sync.NewRWMutex(),
		assetDir: "testdata",
		assets: map[string]assets.Asset{
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

	// foo/b is compiled in, default/b is overridden, return compiled in
	expectURLToContain(t, s.URL+"/b", "foo")

	// only exists as compiled in default/c so use that
	expectURLToContain(t, s.URL+"/c", "default")

	// only exists as overridden default/d so use that
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

	data, err := io.ReadAll(res.Body)
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
	t.Parallel()

	names := dirNames("testdata")
	expected := []string{"config", "default", "foo", "testfolder"}
	if diff, equal := messagediff.PrettyDiff(expected, names); !equal {
		t.Errorf("Unexpected dirNames return: %#v\n%s", names, diff)
	}
}

type httpTestCase struct {
	URL     string        // URL to check
	Code    int           // Expected result code
	Type    string        // Expected content type
	Prefix  string        // Expected result prefix
	Timeout time.Duration // Defaults to a second
}

func TestAPIServiceRequests(t *testing.T) {
	t.Parallel()

	baseURL, cancel, err := startHTTP(apiCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()

	cases := []httpTestCase{
		// /rest/db
		{
			URL:    "/rest/db/completion?device=" + protocol.LocalDeviceID.String() + "&folder=default",
			Code:   200,
			Type:   "application/json",
			Prefix: "{",
		},
		{
			URL:  "/rest/db/file?folder=default&file=something",
			Code: 404,
		},
		{
			URL:    "/rest/db/ignores?folder=default",
			Code:   200,
			Type:   "application/json",
			Prefix: "{",
		},
		{
			URL:    "/rest/db/need?folder=default",
			Code:   200,
			Type:   "application/json",
			Prefix: "{",
		},
		{
			URL:    "/rest/db/status?folder=default",
			Code:   200,
			Type:   "application/json",
			Prefix: "{",
		},
		{
			URL:    "/rest/db/browse?folder=default",
			Code:   200,
			Type:   "application/json",
			Prefix: "null",
		},

		// /rest/stats
		{
			URL:    "/rest/stats/device",
			Code:   200,
			Type:   "application/json",
			Prefix: "null",
		},
		{
			URL:    "/rest/stats/folder",
			Code:   200,
			Type:   "application/json",
			Prefix: "null",
		},

		// /rest/svc
		{
			URL:    "/rest/svc/deviceid?id=" + protocol.LocalDeviceID.String(),
			Code:   200,
			Type:   "application/json",
			Prefix: "{",
		},
		{
			URL:    "/rest/svc/lang",
			Code:   200,
			Type:   "application/json",
			Prefix: "[",
		},
		{
			URL:     "/rest/svc/report",
			Code:    200,
			Type:    "application/json",
			Prefix:  "{",
			Timeout: 5 * time.Second,
		},

		// /rest/system
		{
			URL:    "/rest/system/browse?current=~",
			Code:   200,
			Type:   "application/json",
			Prefix: "[",
		},
		{
			URL:    "/rest/system/config",
			Code:   200,
			Type:   "application/json",
			Prefix: "{",
		},
		{
			URL:    "/rest/system/config/insync",
			Code:   200,
			Type:   "application/json",
			Prefix: "{",
		},
		{
			URL:    "/rest/system/connections",
			Code:   200,
			Type:   "application/json",
			Prefix: "null",
		},
		{
			URL:    "/rest/system/discovery",
			Code:   200,
			Type:   "application/json",
			Prefix: "{",
		},
		{
			URL:    "/rest/system/error?since=0",
			Code:   200,
			Type:   "application/json",
			Prefix: "{",
		},
		{
			URL:    "/rest/system/ping",
			Code:   200,
			Type:   "application/json",
			Prefix: "{",
		},
		{
			URL:    "/rest/system/status",
			Code:   200,
			Type:   "application/json",
			Prefix: "{",
		},
		{
			URL:    "/rest/system/version",
			Code:   200,
			Type:   "application/json",
			Prefix: "{",
		},
		{
			URL:    "/rest/system/debug",
			Code:   200,
			Type:   "application/json",
			Prefix: "{",
		},
		{
			URL:    "/rest/system/log?since=0",
			Code:   200,
			Type:   "application/json",
			Prefix: "{",
		},
		{
			URL:    "/rest/system/log.txt?since=0",
			Code:   200,
			Type:   "text/plain",
			Prefix: "",
		},

		// /rest/config
		{
			URL:    "/rest/config",
			Code:   200,
			Type:   "application/json",
			Prefix: "",
		},
		{
			URL:    "/rest/config/folders",
			Code:   200,
			Type:   "application/json",
			Prefix: "",
		},
		{
			URL:    "/rest/config/folders/missing",
			Code:   404,
			Type:   "text/plain",
			Prefix: "",
		},
		{
			URL:    "/rest/config/devices",
			Code:   200,
			Type:   "application/json",
			Prefix: "",
		},
		{
			URL:    "/rest/config/devices/illegalid",
			Code:   400,
			Type:   "text/plain",
			Prefix: "",
		},
		{
			URL:    "/rest/config/devices/" + protocol.GlobalDeviceID.String(),
			Code:   404,
			Type:   "text/plain",
			Prefix: "",
		},
		{
			URL:    "/rest/config/options",
			Code:   200,
			Type:   "application/json",
			Prefix: "{",
		},
		{
			URL:    "/rest/config/gui",
			Code:   200,
			Type:   "application/json",
			Prefix: "{",
		},
		{
			URL:    "/rest/config/ldap",
			Code:   200,
			Type:   "application/json",
			Prefix: "{",
		},
	}

	for _, tc := range cases {
		t.Log("Testing", tc.URL, "...")
		testHTTPRequest(t, baseURL, tc, testAPIKey)
	}
}

// testHTTPRequest tries the given test case, comparing the result code,
// content type, and result prefix.
func testHTTPRequest(t *testing.T, baseURL string, tc httpTestCase, apikey string) {
	timeout := time.Second
	if tc.Timeout > 0 {
		timeout = tc.Timeout
	}
	cli := &http.Client{
		Timeout: timeout,
	}

	req, err := http.NewRequest("GET", baseURL+tc.URL, nil)
	if err != nil {
		t.Errorf("Unexpected error requesting %s: %v", tc.URL, err)
		return
	}
	req.Header.Set("X-API-Key", apikey)

	resp, err := cli.Do(req)
	if err != nil {
		t.Errorf("Unexpected error requesting %s: %v", tc.URL, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != tc.Code {
		t.Errorf("Get on %s should have returned status code %d, not %s", tc.URL, tc.Code, resp.Status)
		return
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, tc.Type) {
		t.Errorf("The content type on %s should be %q, not %q", tc.URL, tc.Type, ct)
		return
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("Unexpected error reading %s: %v", tc.URL, err)
		return
	}

	if !bytes.HasPrefix(data, []byte(tc.Prefix)) {
		t.Errorf("Returned data from %s does not have prefix %q: %s", tc.URL, tc.Prefix, data)
		return
	}
}

func TestHTTPLogin(t *testing.T) {
	t.Parallel()

	cfg := newMockedConfig()
	cfg.GUIReturns(config.GUIConfiguration{
		User:     "üser",
		Password: "$2a$10$IdIZTxTg/dCNuNEGlmLynOjqg4B1FvDKuIV5e0BB3pnWVHNb8.GSq", // bcrypt of "räksmörgås" in UTF-8
	})
	baseURL, cancel, err := startHTTP(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()

	// Verify rejection when not using authorization

	req, _ := http.NewRequest("GET", baseURL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Unexpected non-401 return code %d for unauthed request", resp.StatusCode)
	}

	// Verify that incorrect password is rejected

	req.SetBasicAuth("üser", "rksmrgs")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Unexpected non-401 return code %d for incorrect password", resp.StatusCode)
	}

	// Verify that incorrect username is rejected

	req.SetBasicAuth("user", "räksmörgås") // string literals in Go source code are in UTF-8
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Unexpected non-401 return code %d for incorrect username", resp.StatusCode)
	}

	// Verify that UTF-8 auth works

	req.SetBasicAuth("üser", "räksmörgås") // string literals in Go source code are in UTF-8
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Unexpected non-200 return code %d for authed request (UTF-8)", resp.StatusCode)
	}

	// Verify that ISO-8859-1 auth

	req.SetBasicAuth("\xfcser", "r\xe4ksm\xf6rg\xe5s") // escaped ISO-8859-1
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Unexpected non-200 return code %d for authed request (ISO-8859-1)", resp.StatusCode)
	}
}

func startHTTP(cfg config.Wrapper) (string, context.CancelFunc, error) {
	m := new(modelmocks.Model)
	assetDir := "../../gui"
	eventSub := new(eventmocks.BufferedSubscription)
	diskEventSub := new(eventmocks.BufferedSubscription)
	discoverer := new(discovermocks.Manager)
	connections := new(connmocks.Service)
	errorLog := new(loggermocks.Recorder)
	systemLog := new(loggermocks.Recorder)
	for _, l := range []*loggermocks.Recorder{errorLog, systemLog} {
		l.SinceReturns([]logger.Line{
			{
				When:    time.Now(),
				Message: "Test message",
			},
		})
	}
	addrChan := make(chan string)
	mockedSummary := &modelmocks.FolderSummaryService{}
	mockedSummary.SummaryReturns(map[string]interface{}{"mocked": true}, nil)

	// Instantiate the API service
	urService := ur.New(cfg, m, connections, false)
	svc := New(protocol.LocalDeviceID, cfg, assetDir, "syncthing", m, eventSub, diskEventSub, events.NoopLogger, discoverer, connections, urService, mockedSummary, errorLog, systemLog, false).(*service)
	defer os.Remove(token)
	svc.started = addrChan

	// Actually start the API service
	supervisor := suture.New("API test", suture.Spec{
		PassThroughPanics: true,
	})
	supervisor.Add(svc)
	ctx, cancel := context.WithCancel(context.Background())
	supervisor.ServeBackground(ctx)

	// Make sure the API service is listening, and get the URL to use.
	addr := <-addrChan
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		cancel()
		return "", cancel, fmt.Errorf("weird address from API service: %w", err)
	}

	host, _, _ := net.SplitHostPort(cfg.GUI().RawAddress)
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	baseURL := fmt.Sprintf("http://%s", net.JoinHostPort(host, strconv.Itoa(tcpAddr.Port)))

	return baseURL, cancel, nil
}

func TestCSRFRequired(t *testing.T) {
	t.Parallel()

	baseURL, cancel, err := startHTTP(apiCfg)
	if err != nil {
		t.Fatal("Unexpected error from getting base URL:", err)
	}
	defer cancel()

	cli := &http.Client{
		Timeout: time.Minute,
	}

	// Getting the base URL (i.e. "/") should succeed.

	resp, err := cli.Get(baseURL)
	if err != nil {
		t.Fatal("Unexpected error from getting base URL:", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatal("Getting base URL should succeed, not", resp.Status)
	}

	// Find the returned CSRF token for future use

	var csrfTokenName, csrfTokenValue string
	for _, cookie := range resp.Cookies() {
		if strings.HasPrefix(cookie.Name, "CSRF-Token") {
			csrfTokenName = cookie.Name
			csrfTokenValue = cookie.Value
			break
		}
	}

	// Calling on /rest without a token should fail

	resp, err = cli.Get(baseURL + "/rest/system/config")
	if err != nil {
		t.Fatal("Unexpected error from getting /rest/system/config:", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatal("Getting /rest/system/config without CSRF token should fail, not", resp.Status)
	}

	// Calling on /rest with a token should succeed

	req, _ := http.NewRequest("GET", baseURL+"/rest/system/config", nil)
	req.Header.Set("X-"+csrfTokenName, csrfTokenValue)
	resp, err = cli.Do(req)
	if err != nil {
		t.Fatal("Unexpected error from getting /rest/system/config:", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatal("Getting /rest/system/config with CSRF token should succeed, not", resp.Status)
	}

	// Calling on /rest with the API key should succeed

	req, _ = http.NewRequest("GET", baseURL+"/rest/system/config", nil)
	req.Header.Set("X-API-Key", testAPIKey)
	resp, err = cli.Do(req)
	if err != nil {
		t.Fatal("Unexpected error from getting /rest/system/config:", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatal("Getting /rest/system/config with API key should succeed, not", resp.Status)
	}
}

func TestRandomString(t *testing.T) {
	t.Parallel()

	baseURL, cancel, err := startHTTP(apiCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	cli := &http.Client{
		Timeout: time.Second,
	}

	// The default should be to return a 32 character random string

	for _, url := range []string{"/rest/svc/random/string", "/rest/svc/random/string?length=-1", "/rest/svc/random/string?length=yo"} {
		req, _ := http.NewRequest("GET", baseURL+url, nil)
		req.Header.Set("X-API-Key", testAPIKey)
		resp, err := cli.Do(req)
		if err != nil {
			t.Fatal(err)
		}

		var res map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
			t.Fatal(err)
		}
		if len(res["random"]) != 32 {
			t.Errorf("Expected 32 random characters, got %q of length %d", res["random"], len(res["random"]))
		}
	}

	// We can ask for a different length if we like

	req, _ := http.NewRequest("GET", baseURL+"/rest/svc/random/string?length=27", nil)
	req.Header.Set("X-API-Key", testAPIKey)
	resp, err := cli.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	var res map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatal(err)
	}
	if len(res["random"]) != 27 {
		t.Errorf("Expected 27 random characters, got %q of length %d", res["random"], len(res["random"]))
	}
}

func TestConfigPostOK(t *testing.T) {
	t.Parallel()

	cfg := bytes.NewBuffer([]byte(`{
		"version": 15,
		"folders": [
			{
				"id": "foo",
				"path": "TestConfigPostOK"
			}
		]
	}`))

	resp, err := testConfigPost(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Error("Expected 200 OK, not", resp.Status)
	}
	os.RemoveAll("TestConfigPostOK")
}

func TestConfigPostDupFolder(t *testing.T) {
	t.Parallel()

	cfg := bytes.NewBuffer([]byte(`{
		"version": 15,
		"folders": [
			{"id": "foo"},
			{"id": "foo"}
		]
	}`))

	resp, err := testConfigPost(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Error("Expected 400 Bad Request, not", resp.Status)
	}
}

func testConfigPost(data io.Reader) (*http.Response, error) {
	baseURL, cancel, err := startHTTP(apiCfg)
	if err != nil {
		return nil, err
	}
	defer cancel()
	cli := &http.Client{
		Timeout: time.Second,
	}

	req, _ := http.NewRequest("POST", baseURL+"/rest/system/config", data)
	req.Header.Set("X-API-Key", testAPIKey)
	return cli.Do(req)
}

func TestHostCheck(t *testing.T) {
	t.Parallel()

	// An API service bound to localhost should reject non-localhost host Headers

	cfg := newMockedConfig()
	cfg.GUIReturns(config.GUIConfiguration{RawAddress: "127.0.0.1:0"})
	baseURL, cancel, err := startHTTP(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()

	// A normal HTTP get to the localhost-bound service should succeed

	resp, err := http.Get(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Error("Regular HTTP get: expected 200 OK, not", resp.Status)
	}

	// A request with a suspicious Host header should fail

	req, _ := http.NewRequest("GET", baseURL, nil)
	req.Host = "example.com"
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Error("Suspicious Host header: expected 403 Forbidden, not", resp.Status)
	}

	// A request with an explicit "localhost:8384" Host header should pass

	req, _ = http.NewRequest("GET", baseURL, nil)
	req.Host = "localhost:8384"
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Error("Explicit localhost:8384: expected 200 OK, not", resp.Status)
	}

	// A request with an explicit "localhost" Host header (no port) should pass

	req, _ = http.NewRequest("GET", baseURL, nil)
	req.Host = "localhost"
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Error("Explicit localhost: expected 200 OK, not", resp.Status)
	}

	// A server with InsecureSkipHostCheck set behaves differently

	cfg = newMockedConfig()
	cfg.GUIReturns(config.GUIConfiguration{
		RawAddress:            "127.0.0.1:0",
		InsecureSkipHostCheck: true,
	})
	baseURL, cancel, err = startHTTP(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()

	// A request with a suspicious Host header should be allowed

	req, _ = http.NewRequest("GET", baseURL, nil)
	req.Host = "example.com"
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Error("Incorrect host header, check disabled: expected 200 OK, not", resp.Status)
	}

	// A server bound to a wildcard address also doesn't do the check

	cfg = newMockedConfig()
	cfg.GUIReturns(config.GUIConfiguration{
		RawAddress:            "0.0.0.0:0",
		InsecureSkipHostCheck: true,
	})
	baseURL, cancel, err = startHTTP(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()

	// A request with a suspicious Host header should be allowed

	req, _ = http.NewRequest("GET", baseURL, nil)
	req.Host = "example.com"
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Error("Incorrect host header, wildcard bound: expected 200 OK, not", resp.Status)
	}

	// This should all work over IPv6 as well

	if runningInContainer() {
		// Working IPv6 in Docker can't be taken for granted.
		return
	}

	cfg = newMockedConfig()
	cfg.GUIReturns(config.GUIConfiguration{
		RawAddress: "[::1]:0",
	})
	baseURL, cancel, err = startHTTP(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()

	// A normal HTTP get to the localhost-bound service should succeed

	resp, err = http.Get(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Error("Regular HTTP get (IPv6): expected 200 OK, not", resp.Status)
	}

	// A request with a suspicious Host header should fail

	req, _ = http.NewRequest("GET", baseURL, nil)
	req.Host = "example.com"
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Error("Suspicious Host header (IPv6): expected 403 Forbidden, not", resp.Status)
	}

	// A request with an explicit "localhost:8384" Host header should pass

	req, _ = http.NewRequest("GET", baseURL, nil)
	req.Host = "localhost:8384"
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Error("Explicit localhost:8384 (IPv6): expected 200 OK, not", resp.Status)
	}
}

func TestAddressIsLocalhost(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		address string
		result  bool
	}{
		// These are all valid localhost addresses
		{"localhost", true},
		{"LOCALHOST", true},
		{"localhost.", true},
		{"::1", true},
		{"127.0.0.1", true},
		{"127.23.45.56", true},
		{"localhost:8080", true},
		{"LOCALHOST:8000", true},
		{"localhost.:8080", true},
		{"[::1]:8080", true},
		{"127.0.0.1:8080", true},
		{"127.23.45.56:8080", true},
		{"www.localhost", true},
		{"www.localhost:8080", true},

		// These are all non-localhost addresses
		{"example.com", false},
		{"example.com:8080", false},
		{"localhost.com", false},
		{"localhost.com:8080", false},
		{"192.0.2.10", false},
		{"192.0.2.10:8080", false},
		{"0.0.0.0", false},
		{"0.0.0.0:8080", false},
		{"::", false},
		{"[::]:8080", false},
		{":8080", false},
	}

	for _, tc := range testcases {
		result := addressIsLocalhost(tc.address)
		if result != tc.result {
			t.Errorf("addressIsLocalhost(%q)=%v, expected %v", tc.address, result, tc.result)
		}
	}
}

func TestAccessControlAllowOriginHeader(t *testing.T) {
	t.Parallel()

	baseURL, cancel, err := startHTTP(apiCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	cli := &http.Client{
		Timeout: time.Second,
	}

	req, _ := http.NewRequest("GET", baseURL+"/rest/system/status", nil)
	req.Header.Set("X-API-Key", testAPIKey)
	resp, err := cli.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatal("GET on /rest/system/status should succeed, not", resp.Status)
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("GET on /rest/system/status should return a 'Access-Control-Allow-Origin: *' header")
	}
}

func TestOptionsRequest(t *testing.T) {
	t.Parallel()

	baseURL, cancel, err := startHTTP(apiCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	cli := &http.Client{
		Timeout: time.Second,
	}

	req, _ := http.NewRequest("OPTIONS", baseURL+"/rest/system/status", nil)
	resp, err := cli.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatal("OPTIONS on /rest/system/status should succeed, not", resp.Status)
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("OPTIONS on /rest/system/status should return a 'Access-Control-Allow-Origin: *' header")
	}
	if resp.Header.Get("Access-Control-Allow-Methods") != "GET, POST, PUT, PATCH, DELETE, OPTIONS" {
		t.Fatal("OPTIONS on /rest/system/status should return a 'Access-Control-Allow-Methods: GET, POST, PUT, PATCH, DELETE, OPTIONS' header")
	}
	if resp.Header.Get("Access-Control-Allow-Headers") != "Content-Type, X-API-Key" {
		t.Fatal("OPTIONS on /rest/system/status should return a 'Access-Control-Allow-Headers: Content-Type, X-API-KEY' header")
	}
}

func TestEventMasks(t *testing.T) {
	t.Parallel()

	cfg := newMockedConfig()
	defSub := new(eventmocks.BufferedSubscription)
	diskSub := new(eventmocks.BufferedSubscription)
	svc := New(protocol.LocalDeviceID, cfg, "", "syncthing", nil, defSub, diskSub, events.NoopLogger, nil, nil, nil, nil, nil, nil, false).(*service)
	defer os.Remove(token)

	if mask := svc.getEventMask(""); mask != DefaultEventMask {
		t.Errorf("incorrect default mask %x != %x", int64(mask), int64(DefaultEventMask))
	}

	expected := events.FolderSummary | events.LocalChangeDetected
	if mask := svc.getEventMask("FolderSummary,LocalChangeDetected"); mask != expected {
		t.Errorf("incorrect parsed mask %x != %x", int64(mask), int64(expected))
	}

	expected = 0
	if mask := svc.getEventMask("WeirdEvent,something else that doesn't exist"); mask != expected {
		t.Errorf("incorrect parsed mask %x != %x", int64(mask), int64(expected))
	}

	if res := svc.getEventSub(DefaultEventMask); res != defSub {
		t.Errorf("should have returned the given default event sub")
	}
	if res := svc.getEventSub(DiskEventMask); res != diskSub {
		t.Errorf("should have returned the given disk event sub")
	}
	if res := svc.getEventSub(events.LocalIndexUpdated); res == nil || res == defSub || res == diskSub {
		t.Errorf("should have returned a valid, non-default event sub")
	}
}

func TestBrowse(t *testing.T) {
	t.Parallel()

	pathSep := string(os.PathSeparator)

	tmpDir, err := os.MkdirTemp("", "syncthing")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	if err := os.Mkdir(filepath.Join(tmpDir, "dir"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "file"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(tmpDir, "MiXEDCase"), 0755); err != nil {
		t.Fatal(err)
	}

	// We expect completion to return the full path to the completed
	// directory, with an ending slash.
	dirPath := filepath.Join(tmpDir, "dir") + pathSep
	mixedCaseDirPath := filepath.Join(tmpDir, "MiXEDCase") + pathSep

	cases := []struct {
		current string
		returns []string
	}{
		// The direcotory without slash is completed to one with slash.
		{tmpDir, []string{tmpDir + pathSep}},
		// With slash it's completed to its contents.
		// Dirs are given pathSeps.
		// Files are not returned.
		{tmpDir + pathSep, []string{mixedCaseDirPath, dirPath}},
		// Globbing is automatic based on prefix.
		{tmpDir + pathSep + "d", []string{dirPath}},
		{tmpDir + pathSep + "di", []string{dirPath}},
		{tmpDir + pathSep + "dir", []string{dirPath}},
		{tmpDir + pathSep + "f", nil},
		{tmpDir + pathSep + "q", nil},
		// Globbing is case-insensitve
		{tmpDir + pathSep + "mixed", []string{mixedCaseDirPath}},
	}

	for _, tc := range cases {
		ret := browseFiles(tc.current, fs.FilesystemTypeBasic)
		if !util.EqualStrings(ret, tc.returns) {
			t.Errorf("browseFiles(%q) => %q, expected %q", tc.current, ret, tc.returns)
		}
	}
}

func TestPrefixMatch(t *testing.T) {
	t.Parallel()

	cases := []struct {
		s        string
		prefix   string
		expected int
	}{
		{"aaaA", "aaa", matchExact},
		{"AAAX", "BBB", noMatch},
		{"AAAX", "aAa", matchCaseIns},
		{"äÜX", "äü", matchCaseIns},
	}

	for _, tc := range cases {
		ret := checkPrefixMatch(tc.s, tc.prefix)
		if ret != tc.expected {
			t.Errorf("checkPrefixMatch(%q, %q) => %v, expected %v", tc.s, tc.prefix, ret, tc.expected)
		}
	}
}

func TestShouldRegenerateCertificate(t *testing.T) {
	// Self signed certificates expiring in less than a month are errored so we
	// can regenerate in time.
	crt, err := tlsutil.NewCertificateInMemory("foo.example.com", 29)
	if err != nil {
		t.Fatal(err)
	}
	if err := shouldRegenerateCertificate(crt); err == nil {
		t.Error("expected expiry error")
	}

	// Certificates with at least 31 days of life left are fine.
	crt, err = tlsutil.NewCertificateInMemory("foo.example.com", 31)
	if err != nil {
		t.Fatal(err)
	}
	if err := shouldRegenerateCertificate(crt); err != nil {
		t.Error("expected no error:", err)
	}

	if runtime.GOOS == "darwin" {
		// Certificates with too long an expiry time are not allowed on macOS
		crt, err = tlsutil.NewCertificateInMemory("foo.example.com", 1000)
		if err != nil {
			t.Fatal(err)
		}
		if err := shouldRegenerateCertificate(crt); err == nil {
			t.Error("expected expiry error")
		}
	}
}

func TestConfigChanges(t *testing.T) {
	t.Parallel()

	const testAPIKey = "foobarbaz"
	cfg := config.Configuration{
		GUI: config.GUIConfiguration{
			RawAddress: "127.0.0.1:0",
			RawUseTLS:  false,
			APIKey:     testAPIKey,
		},
	}
	tmpFile, err := os.CreateTemp("", "syncthing-testConfig-")
	if err != nil {
		panic(err)
	}
	defer os.Remove(tmpFile.Name())
	w := config.Wrap(tmpFile.Name(), cfg, protocol.LocalDeviceID, events.NoopLogger)
	tmpFile.Close()
	cfgCtx, cfgCancel := context.WithCancel(context.Background())
	go w.Serve(cfgCtx)
	defer cfgCancel()
	baseURL, cancel, err := startHTTP(w)
	if err != nil {
		t.Fatal("Unexpected error from getting base URL:", err)
	}
	defer cancel()

	cli := &http.Client{
		Timeout: time.Minute,
	}

	do := func(req *http.Request, status int) *http.Response {
		t.Helper()
		req.Header.Set("X-API-Key", testAPIKey)
		resp, err := cli.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != status {
			t.Errorf("Expected status %v, got %v", status, resp.StatusCode)
		}
		return resp
	}

	mod := func(method, path string, data interface{}) {
		t.Helper()
		bs, err := json.Marshal(data)
		if err != nil {
			t.Fatal(err)
		}
		req, _ := http.NewRequest(method, baseURL+path, bytes.NewReader(bs))
		do(req, http.StatusOK).Body.Close()
	}

	get := func(path string) *http.Response {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, baseURL+path, nil)
		return do(req, http.StatusOK)
	}

	dev1Path := "/rest/config/devices/" + dev1.String()

	// Create device
	mod(http.MethodPut, "/rest/config/devices", []config.DeviceConfiguration{{DeviceID: dev1}})

	// Check its there
	get(dev1Path).Body.Close()

	// Modify just a single attribute
	mod(http.MethodPatch, dev1Path, map[string]bool{"Paused": true})

	// Check that attribute
	resp := get(dev1Path)
	var dev config.DeviceConfiguration
	if err := unmarshalTo(resp.Body, &dev); err != nil {
		t.Fatal(err)
	}
	if !dev.Paused {
		t.Error("Expected device to be paused")
	}

	folder2Path := "/rest/config/folders/folder2"

	// Create a folder and add another
	mod(http.MethodPut, "/rest/config/folders", []config.FolderConfiguration{{ID: "folder1", Path: "folder1"}})
	mod(http.MethodPut, folder2Path, config.FolderConfiguration{ID: "folder2", Path: "folder2"})

	// Check they are there
	get("/rest/config/folders/folder1").Body.Close()
	get(folder2Path).Body.Close()

	// Modify just a single attribute
	mod(http.MethodPatch, folder2Path, map[string]bool{"Paused": true})

	// Check that attribute
	resp = get(folder2Path)
	var folder config.FolderConfiguration
	if err := unmarshalTo(resp.Body, &folder); err != nil {
		t.Fatal(err)
	}
	if !dev.Paused {
		t.Error("Expected folder to be paused")
	}

	// Delete folder2
	req, _ := http.NewRequest(http.MethodDelete, baseURL+folder2Path, nil)
	do(req, http.StatusOK)

	// Check folder1 is still there and folder2 gone
	get("/rest/config/folders/folder1").Body.Close()
	req, _ = http.NewRequest(http.MethodGet, baseURL+folder2Path, nil)
	do(req, http.StatusNotFound)

	mod(http.MethodPatch, "/rest/config/options", map[string]int{"maxSendKbps": 50})
	resp = get("/rest/config/options")
	var opts config.OptionsConfiguration
	if err := unmarshalTo(resp.Body, &opts); err != nil {
		t.Fatal(err)
	}
	if opts.MaxSendKbps != 50 {
		t.Error("Exepcted 50 for MaxSendKbps, got", opts.MaxSendKbps)
	}
}

func TestSanitizedHostname(t *testing.T) {
	cases := []struct {
		in, out string
	}{
		{"foo.BAR-baz", "foo.bar-baz"},
		{"~.~-Min 1:a Räksmörgås-dator 😀😎 ~.~-", "min1araksmorgas-dator"},
		{"Vicenç-PC", "vicenc-pc"},
		{"~.~-~.~-", ""},
		{"", ""},
	}

	for _, tc := range cases {
		res, err := sanitizedHostname(tc.in)
		if tc.out == "" && err == nil {
			t.Errorf("%q should cause error", tc.in)
		} else if res != tc.out {
			t.Errorf("%q => %q, expected %q", tc.in, res, tc.out)
		}
	}
}

// runningInContainer returns true if we are inside Docker or LXC. It might
// be prone to false negatives if things change in the future, but likely
// not false positives.
func runningInContainer() bool {
	if runtime.GOOS != "linux" {
		return false
	}

	bs, err := os.ReadFile("/proc/1/cgroup")
	if err != nil {
		return false
	}
	if bytes.Contains(bs, []byte("/docker/")) {
		return true
	}
	if bytes.Contains(bs, []byte("/lxc/")) {
		return true
	}
	return false
}
