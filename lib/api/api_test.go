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
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptoRand "crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/d4l3k/messagediff"
	webauthnProtocol "github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/protocol/webauthncbor"
	"github.com/go-webauthn/webauthn/protocol/webauthncose"
	"github.com/google/go-cmp/cmp"
	"github.com/syncthing/syncthing/lib/assets"
	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/config"
	connmocks "github.com/syncthing/syncthing/lib/connections/mocks"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/db/backend"
	discovermocks "github.com/syncthing/syncthing/lib/discover/mocks"
	"github.com/syncthing/syncthing/lib/events"
	eventmocks "github.com/syncthing/syncthing/lib/events/mocks"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/locations"
	"github.com/syncthing/syncthing/lib/logger"
	loggermocks "github.com/syncthing/syncthing/lib/logger/mocks"
	"github.com/syncthing/syncthing/lib/model"
	modelmocks "github.com/syncthing/syncthing/lib/model/mocks"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/svcutil"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/testutil"
	"github.com/syncthing/syncthing/lib/tlsutil"
	"github.com/syncthing/syncthing/lib/ur"
	"github.com/thejerf/suture/v4"
)

var (
	confDir    = filepath.Join("testdata", "config")
	dev1       protocol.DeviceID
	apiCfg     = newMockedConfig()
	testAPIKey = "foobarbaz"
)

func init() {
	dev1, _ = protocol.DeviceIDFromString("AIR6LPZ-7K4PTTV-UXQSMUU-CPQ5YWH-OEDFIIQ-JUG777G-2YQXXR5-YD6AWQR")
	apiCfg.GUIReturns(config.GUIConfiguration{APIKey: testAPIKey, RawAddress: "127.0.0.1:0"})
}

func TestMain(m *testing.M) {
	orig := locations.GetBaseDir(locations.ConfigBaseDir)
	locations.SetBaseDir(locations.ConfigBaseDir, confDir)

	exitCode := m.Run()

	locations.SetBaseDir(locations.ConfigBaseDir, orig)

	os.Exit(exitCode)
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

	mdb, _ := db.NewLowlevel(backend.OpenMemory(), events.NoopLogger)
	kdb := db.NewMiscDataNamespace(mdb)
	srv := New(protocol.LocalDeviceID, w, "", "syncthing", nil, nil, nil, events.NoopLogger, nil, nil, nil, nil, nil, nil, false, kdb).(*service)

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
	t.Cleanup(cancel)

	cases := []httpTestCase{
		// /rest/db
		{
			URL:     "/rest/db/completion?device=" + protocol.LocalDeviceID.String() + "&folder=default",
			Code:    200,
			Type:    "application/json",
			Prefix:  "{",
			Timeout: 15 * time.Second,
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
		{
			URL:    "/rest/db/status?folder=default",
			Code:   200,
			Type:   "application/json",
			Prefix: "",
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
		tc := tc
		t.Run(tc.URL, func(t *testing.T) {
			t.Parallel()
			testHTTPRequest(t, baseURL, tc, testAPIKey)
		})
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

func hasSessionCookie(cookies []*http.Cookie) bool {
	for _, cookie := range cookies {
		if cookie.MaxAge >= 0 && strings.HasPrefix(cookie.Name, "sessionid") {
			return true
		}
	}
	return false
}

func hasDeleteSessionCookie(cookies []*http.Cookie) bool {
	for _, cookie := range cookies {
		if cookie.MaxAge < 0 && strings.HasPrefix(cookie.Name, "sessionid") {
			return true
		}
	}
	return false
}

func httpRequest(method string, url string, body any, basicAuthUsername, basicAuthPassword, xapikeyHeader, authorizationBearer, csrfTokenName, csrfTokenValue string, cookies []*http.Cookie, t *testing.T) *http.Response {
	t.Helper()

	var bodyReader io.Reader = nil
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		t.Fatal(err)
	}

	if basicAuthUsername != "" || basicAuthPassword != "" {
		req.SetBasicAuth(basicAuthUsername, basicAuthPassword)
	}

	if xapikeyHeader != "" {
		req.Header.Set("X-API-Key", xapikeyHeader)
	}

	if authorizationBearer != "" {
		req.Header.Set("Authorization", "Bearer "+authorizationBearer)
	}

	if csrfTokenName != "" && csrfTokenValue != "" {
		req.Header.Set("X-"+csrfTokenName, csrfTokenValue)
	}

	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	client := http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	return resp
}

func httpGet(url string, basicAuthUsername, basicAuthPassword, xapikeyHeader, authorizationBearer string, cookies []*http.Cookie, t *testing.T) *http.Response {
	t.Helper()
	return httpRequest(http.MethodGet, url, nil, basicAuthUsername, basicAuthPassword, xapikeyHeader, authorizationBearer, "", "", cookies, t)
}

func httpGetCsrf(url string, csrfTokenName, csrfTokenValue string, t *testing.T) *http.Response {
	t.Helper()
	return httpRequest(http.MethodGet, url, nil, "", "", "", "", csrfTokenName, csrfTokenValue, nil, t)
}

func httpPost(url string, body map[string]string, cookies []*http.Cookie, t *testing.T) *http.Response {
	t.Helper()
	return httpRequest(http.MethodPost, url, body, "", "", "", "", "", "", cookies, t)
}

func httpPostCsrf(url string, body any, csrfTokenName, csrfTokenValue string, t *testing.T) *http.Response {
	t.Helper()
	return httpRequest(http.MethodPost, url, body, "", "", "", "", csrfTokenName, csrfTokenValue, nil, t)
}

func httpPutCsrf(url string, body any, csrfTokenName, csrfTokenValue string, t *testing.T) *http.Response {
	t.Helper()
	return httpRequest(http.MethodPut, url, body, "", "", "", "", csrfTokenName, csrfTokenValue, nil, t)
}

func TestHTTPLogin(t *testing.T) {
	t.Parallel()

	httpGetBasicAuth := func(url string, username string, password string) *http.Response {
		t.Helper()
		return httpGet(url, username, password, "", "", nil, t)
	}

	httpGetXapikey := func(url string, xapikeyHeader string) *http.Response {
		t.Helper()
		return httpGet(url, "", "", xapikeyHeader, "", nil, t)
	}

	httpGetAuthorizationBearer := func(url string, bearer string) *http.Response {
		t.Helper()
		return httpGet(url, "", "", "", bearer, nil, t)
	}

	testWith := func(sendBasicAuthPrompt bool, expectedOkStatus int, expectedFailStatus int, path string) {
		cfg := newMockedConfig()
		cfg.GUIReturns(config.GUIConfiguration{
			User:                "üser",
			Password:            "$2a$10$IdIZTxTg/dCNuNEGlmLynOjqg4B1FvDKuIV5e0BB3pnWVHNb8.GSq", // bcrypt of "räksmörgås" in UTF-8
			RawAddress:          "127.0.0.1:0",
			APIKey:              testAPIKey,
			SendBasicAuthPrompt: sendBasicAuthPrompt,
		})
		baseURL, cancel, err := startHTTP(cfg)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(cancel)
		url := baseURL + path

		t.Run(fmt.Sprintf("%d path", expectedOkStatus), func(t *testing.T) {
			t.Run("no auth is rejected", func(t *testing.T) {
				t.Parallel()
				resp := httpGetBasicAuth(url, "", "")
				if resp.StatusCode != expectedFailStatus {
					t.Errorf("Unexpected non-%d return code %d for unauthed request", expectedFailStatus, resp.StatusCode)
				}
				if hasSessionCookie(resp.Cookies()) {
					t.Errorf("Unexpected session cookie for unauthed request")
				}
			})

			t.Run("incorrect password is rejected", func(t *testing.T) {
				t.Parallel()
				resp := httpGetBasicAuth(url, "üser", "rksmrgs")
				if resp.StatusCode != expectedFailStatus {
					t.Errorf("Unexpected non-%d return code %d for incorrect password", expectedFailStatus, resp.StatusCode)
				}
				if hasSessionCookie(resp.Cookies()) {
					t.Errorf("Unexpected session cookie for incorrect password")
				}
			})

			t.Run("incorrect username is rejected", func(t *testing.T) {
				t.Parallel()
				resp := httpGetBasicAuth(url, "user", "räksmörgås") // string literals in Go source code are in UTF-8
				if resp.StatusCode != expectedFailStatus {
					t.Errorf("Unexpected non-%d return code %d for incorrect username", expectedFailStatus, resp.StatusCode)
				}
				if hasSessionCookie(resp.Cookies()) {
					t.Errorf("Unexpected session cookie for incorrect username")
				}
			})

			t.Run("UTF-8 auth works", func(t *testing.T) {
				t.Parallel()
				resp := httpGetBasicAuth(url, "üser", "räksmörgås") // string literals in Go source code are in UTF-8
				if resp.StatusCode != expectedOkStatus {
					t.Errorf("Unexpected non-%d return code %d for authed request (UTF-8)", expectedOkStatus, resp.StatusCode)
				}
				if !hasSessionCookie(resp.Cookies()) {
					t.Errorf("Expected session cookie for authed request (UTF-8)")
				}
			})

			t.Run("Logout removes the session cookie", func(t *testing.T) {
				t.Parallel()
				resp := httpGetBasicAuth(url, "üser", "räksmörgås") // string literals in Go source code are in UTF-8
				if resp.StatusCode != expectedOkStatus {
					t.Errorf("Unexpected non-%d return code %d for authed request (UTF-8)", expectedOkStatus, resp.StatusCode)
				}
				if !hasSessionCookie(resp.Cookies()) {
					t.Errorf("Expected session cookie for authed request (UTF-8)")
				}
				logoutResp := httpPost(baseURL+"/rest/noauth/auth/logout", nil, resp.Cookies(), t)
				if !hasDeleteSessionCookie(logoutResp.Cookies()) {
					t.Errorf("Expected session cookie to be deleted for logout request")
				}
			})

			t.Run("Session cookie is invalid after logout", func(t *testing.T) {
				t.Parallel()
				loginResp := httpGetBasicAuth(url, "üser", "räksmörgås") // string literals in Go source code are in UTF-8
				if loginResp.StatusCode != expectedOkStatus {
					t.Errorf("Unexpected non-%d return code %d for authed request (UTF-8)", expectedOkStatus, loginResp.StatusCode)
				}
				if !hasSessionCookie(loginResp.Cookies()) {
					t.Errorf("Expected session cookie for authed request (UTF-8)")
				}

				resp := httpGet(url, "", "", "", "", loginResp.Cookies(), t)
				if resp.StatusCode != expectedOkStatus {
					t.Errorf("Unexpected non-%d return code %d for cookie-authed request (UTF-8)", expectedOkStatus, resp.StatusCode)
				}

				httpPost(baseURL+"/rest/noauth/auth/logout", nil, loginResp.Cookies(), t)
				resp = httpGet(url, "", "", "", "", loginResp.Cookies(), t)
				if resp.StatusCode != expectedFailStatus {
					t.Errorf("Expected session to be invalid (status %d) after logout, got status: %d", expectedFailStatus, resp.StatusCode)
				}
			})

			t.Run("ISO-8859-1 auth works", func(t *testing.T) {
				t.Parallel()
				resp := httpGetBasicAuth(url, "\xfcser", "r\xe4ksm\xf6rg\xe5s") // escaped ISO-8859-1
				if resp.StatusCode != expectedOkStatus {
					t.Errorf("Unexpected non-%d return code %d for authed request (ISO-8859-1)", expectedOkStatus, resp.StatusCode)
				}
				if !hasSessionCookie(resp.Cookies()) {
					t.Errorf("Expected session cookie for authed request (ISO-8859-1)")
				}
			})

			t.Run("bad X-API-Key is rejected", func(t *testing.T) {
				t.Parallel()
				resp := httpGetXapikey(url, testAPIKey+"X")
				if resp.StatusCode != expectedFailStatus {
					t.Errorf("Unexpected non-%d return code %d for bad API key", expectedFailStatus, resp.StatusCode)
				}
				if hasSessionCookie(resp.Cookies()) {
					t.Errorf("Unexpected session cookie for bad API key")
				}
			})

			t.Run("good X-API-Key is accepted", func(t *testing.T) {
				t.Parallel()
				resp := httpGetXapikey(url, testAPIKey)
				if resp.StatusCode != expectedOkStatus {
					t.Errorf("Unexpected non-%d return code %d for API key", expectedOkStatus, resp.StatusCode)
				}
				if hasSessionCookie(resp.Cookies()) {
					t.Errorf("Unexpected session cookie for API key")
				}
			})

			t.Run("bad Bearer is rejected", func(t *testing.T) {
				t.Parallel()
				resp := httpGetAuthorizationBearer(url, testAPIKey+"X")
				if resp.StatusCode != expectedFailStatus {
					t.Errorf("Unexpected non-%d return code %d for bad Authorization: Bearer", expectedFailStatus, resp.StatusCode)
				}
				if hasSessionCookie(resp.Cookies()) {
					t.Errorf("Unexpected session cookie for bad Authorization: Bearer")
				}
			})

			t.Run("good Bearer is accepted", func(t *testing.T) {
				t.Parallel()
				resp := httpGetAuthorizationBearer(url, testAPIKey)
				if resp.StatusCode != expectedOkStatus {
					t.Errorf("Unexpected non-%d return code %d for Authorization: Bearer", expectedOkStatus, resp.StatusCode)
				}
				if hasSessionCookie(resp.Cookies()) {
					t.Errorf("Unexpected session cookie for bad Authorization: Bearer")
				}
			})
		})
	}

	testWith(true, http.StatusOK, http.StatusOK, "/")
	testWith(true, http.StatusOK, http.StatusUnauthorized, "/meta.js")
	testWith(true, http.StatusNotFound, http.StatusUnauthorized, "/any-path/that/does/nooooooot/match-any/noauth-pattern")

	testWith(false, http.StatusOK, http.StatusOK, "/")
	testWith(false, http.StatusOK, http.StatusForbidden, "/meta.js")
	testWith(false, http.StatusNotFound, http.StatusForbidden, "/any-path/that/does/nooooooot/match-any/noauth-pattern")
}

func TestHtmlFormLogin(t *testing.T) {
	t.Parallel()

	cfg := newMockedConfig()
	cfg.GUIReturns(config.GUIConfiguration{
		User:                "üser",
		Password:            "$2a$10$IdIZTxTg/dCNuNEGlmLynOjqg4B1FvDKuIV5e0BB3pnWVHNb8.GSq", // bcrypt of "räksmörgås" in UTF-8
		SendBasicAuthPrompt: false,
	})
	baseURL, cancel, err := startHTTP(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cancel)

	loginUrl := baseURL + "/rest/noauth/auth/password"
	resourceUrl := baseURL + "/meta.js"
	resourceUrl404 := baseURL + "/any-path/that/does/nooooooot/match-any/noauth-pattern"

	performLogin := func(username string, password string) *http.Response {
		t.Helper()
		return httpPost(loginUrl, map[string]string{"username": username, "password": password}, nil, t)
	}

	performResourceRequest := func(url string, cookies []*http.Cookie) *http.Response {
		t.Helper()
		return httpGet(url, "", "", "", "", cookies, t)
	}

	testNoAuthPath := func(noAuthPath string) {
		t.Run("auth is not needed for "+noAuthPath, func(t *testing.T) {
			t.Parallel()
			resp := httpGet(baseURL+noAuthPath, "", "", "", "", nil, t)
			if resp.StatusCode != http.StatusOK {
				t.Errorf("Unexpected non-200 return code %d at %s", resp.StatusCode, noAuthPath)
			}
			if hasSessionCookie(resp.Cookies()) {
				t.Errorf("Unexpected session cookie at " + noAuthPath)
			}
		})
	}
	testNoAuthPath("/index.html")
	testNoAuthPath("/rest/svc/lang")

	t.Run("incorrect password is rejected with 403", func(t *testing.T) {
		t.Parallel()
		resp := performLogin("üser", "rksmrgs") // string literals in Go source code are in UTF-8
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("Unexpected non-403 return code %d for incorrect password", resp.StatusCode)
		}
		if hasSessionCookie(resp.Cookies()) {
			t.Errorf("Unexpected session cookie for incorrect password")
		}
		resp = performResourceRequest(resourceUrl, resp.Cookies())
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("Unexpected non-403 return code %d for incorrect password", resp.StatusCode)
		}
	})

	t.Run("incorrect username is rejected with 403", func(t *testing.T) {
		t.Parallel()
		resp := performLogin("user", "räksmörgås") // string literals in Go source code are in UTF-8
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("Unexpected non-403 return code %d for incorrect username", resp.StatusCode)
		}
		if hasSessionCookie(resp.Cookies()) {
			t.Errorf("Unexpected session cookie for incorrect username")
		}
		resp = performResourceRequest(resourceUrl, resp.Cookies())
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("Unexpected non-403 return code %d for incorrect username", resp.StatusCode)
		}
	})

	t.Run("UTF-8 auth works", func(t *testing.T) {
		t.Parallel()
		// JSON is always UTF-8, so ISO-8859-1 case is not applicable
		resp := performLogin("üser", "räksmörgås") // string literals in Go source code are in UTF-8
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("Unexpected non-204 return code %d for authed request (UTF-8)", resp.StatusCode)
		}
		resp = performResourceRequest(resourceUrl, resp.Cookies())
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Unexpected non-200 return code %d for authed request (UTF-8)", resp.StatusCode)
		}
	})

	t.Run("Logout removes the session cookie", func(t *testing.T) {
		t.Parallel()
		// JSON is always UTF-8, so ISO-8859-1 case is not applicable
		resp := performLogin("üser", "räksmörgås") // string literals in Go source code are in UTF-8
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("Unexpected non-204 return code %d for authed request (UTF-8)", resp.StatusCode)
		}
		logoutResp := httpPost(baseURL+"/rest/noauth/auth/logout", nil, resp.Cookies(), t)
		if !hasDeleteSessionCookie(logoutResp.Cookies()) {
			t.Errorf("Expected session cookie to be deleted for logout request")
		}
	})

	t.Run("Session cookie is invalid after logout", func(t *testing.T) {
		t.Parallel()
		// JSON is always UTF-8, so ISO-8859-1 case is not applicable
		loginResp := performLogin("üser", "räksmörgås") // string literals in Go source code are in UTF-8
		if loginResp.StatusCode != http.StatusNoContent {
			t.Errorf("Unexpected non-204 return code %d for authed request (UTF-8)", loginResp.StatusCode)
		}
		resp := performResourceRequest(resourceUrl, loginResp.Cookies())
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Unexpected non-200 return code %d for authed request (UTF-8)", resp.StatusCode)
		}
		httpPost(baseURL+"/rest/noauth/auth/logout", nil, loginResp.Cookies(), t)
		resp = performResourceRequest(resourceUrl, loginResp.Cookies())
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("Expected session to be invalid (status 403) after logout, got status: %d", resp.StatusCode)
		}
	})

	t.Run("form login is not applicable to other URLs", func(t *testing.T) {
		t.Parallel()
		resp := httpPost(baseURL+"/meta.js", map[string]string{"username": "üser", "password": "räksmörgås"}, nil, t)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("Unexpected non-403 return code %d for incorrect form login URL", resp.StatusCode)
		}
		if hasSessionCookie(resp.Cookies()) {
			t.Errorf("Unexpected session cookie for incorrect form login URL")
		}
	})

	t.Run("invalid URL returns 403 before auth and 404 after auth", func(t *testing.T) {
		t.Parallel()
		resp := performResourceRequest(resourceUrl404, nil)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("Unexpected non-403 return code %d for unauthed request", resp.StatusCode)
		}
		resp = performLogin("üser", "räksmörgås")
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("Unexpected non-204 return code %d for authed request", resp.StatusCode)
		}
		resp = performResourceRequest(resourceUrl404, resp.Cookies())
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Unexpected non-404 return code %d for authed request", resp.StatusCode)
		}
	})
}

func TestApiCache(t *testing.T) {
	t.Parallel()

	cfg := newMockedConfig()
	cfg.GUIReturns(config.GUIConfiguration{
		RawAddress: "127.0.0.1:0",
		APIKey:     testAPIKey,
	})
	baseURL, cancel, err := startHTTP(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cancel)

	httpGet := func(url string, bearer string) *http.Response {
		return httpGet(url, "", "", "", bearer, nil, t)
	}

	t.Run("meta.js has no-cache headers", func(t *testing.T) {
		t.Parallel()
		url := baseURL + "/meta.js"
		resp := httpGet(url, testAPIKey)
		if resp.Header.Get("Cache-Control") != "max-age=0, no-cache, no-store" {
			t.Errorf("Expected no-cache headers at %s", url)
		}
	})

	t.Run("/rest/ has no-cache headers", func(t *testing.T) {
		t.Parallel()
		url := baseURL + "/rest/system/version"
		resp := httpGet(url, testAPIKey)
		if resp.Header.Get("Cache-Control") != "max-age=0, no-cache, no-store" {
			t.Errorf("Expected no-cache headers at %s", url)
		}
	})
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
	mockedSummary.SummaryReturns(new(model.FolderSummary), nil)

	// Instantiate the API service
	urService := ur.New(cfg, m, connections, false)
	mdb, _ := db.NewLowlevel(backend.OpenMemory(), events.NoopLogger)
	kdb := db.NewMiscDataNamespace(mdb)
	svc := New(protocol.LocalDeviceID, cfg, assetDir, "syncthing", m, eventSub, diskEventSub, events.NoopLogger, discoverer, connections, urService, mockedSummary, errorLog, systemLog, false, kdb).(*service)
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
	t.Cleanup(cancel)

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

	if csrfTokenValue == "" {
		t.Fatal("Failed to initialize CSRF test: no CSRF cookie returned from " + baseURL)
	}

	t.Run("/rest without a token should fail", func(t *testing.T) {
		t.Parallel()
		resp, err := cli.Get(baseURL + "/rest/system/config")
		if err != nil {
			t.Fatal("Unexpected error from getting /rest/system/config:", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatal("Getting /rest/system/config without CSRF token should fail, not", resp.Status)
		}
	})

	t.Run("/rest with a token should succeed", func(t *testing.T) {
		t.Parallel()
		req, _ := http.NewRequest("GET", baseURL+"/rest/system/config", nil)
		req.Header.Set("X-"+csrfTokenName, csrfTokenValue)
		resp, err := cli.Do(req)
		if err != nil {
			t.Fatal("Unexpected error from getting /rest/system/config:", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatal("Getting /rest/system/config with CSRF token should succeed, not", resp.Status)
		}
	})

	t.Run("/rest with an incorrect API key should fail, X-API-Key version", func(t *testing.T) {
		t.Parallel()
		req, _ := http.NewRequest("GET", baseURL+"/rest/system/config", nil)
		req.Header.Set("X-API-Key", testAPIKey+"X")
		resp, err := cli.Do(req)
		if err != nil {
			t.Fatal("Unexpected error from getting /rest/system/config:", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatal("Getting /rest/system/config with incorrect API token should fail, not", resp.Status)
		}
	})

	t.Run("/rest with an incorrect API key should fail, Bearer auth version", func(t *testing.T) {
		t.Parallel()
		req, _ := http.NewRequest("GET", baseURL+"/rest/system/config", nil)
		req.Header.Set("Authorization", "Bearer "+testAPIKey+"X")
		resp, err := cli.Do(req)
		if err != nil {
			t.Fatal("Unexpected error from getting /rest/system/config:", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatal("Getting /rest/system/config with incorrect API token should fail, not", resp.Status)
		}
	})

	t.Run("/rest with the API key should succeed", func(t *testing.T) {
		t.Parallel()
		req, _ := http.NewRequest("GET", baseURL+"/rest/system/config", nil)
		req.Header.Set("X-API-Key", testAPIKey)
		resp, err := cli.Do(req)
		if err != nil {
			t.Fatal("Unexpected error from getting /rest/system/config:", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatal("Getting /rest/system/config with API key should succeed, not", resp.Status)
		}
	})

	t.Run("/rest with the API key as a bearer token should succeed", func(t *testing.T) {
		t.Parallel()
		req, _ := http.NewRequest("GET", baseURL+"/rest/system/config", nil)
		req.Header.Set("Authorization", "Bearer "+testAPIKey)
		resp, err := cli.Do(req)
		if err != nil {
			t.Fatal("Unexpected error from getting /rest/system/config:", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatal("Getting /rest/system/config with API key should succeed, not", resp.Status)
		}
	})
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

	if !testing.Short() {
		// A server bound to a wildcard address also doesn't do the check

		cfg = newMockedConfig()
		cfg.GUIReturns(config.GUIConfiguration{
			RawAddress: "0.0.0.0:0",
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
	mdb, _ := db.NewLowlevel(backend.OpenMemory(), events.NoopLogger)
	kdb := db.NewMiscDataNamespace(mdb)
	svc := New(protocol.LocalDeviceID, cfg, "", "syncthing", nil, defSub, diskSub, events.NoopLogger, nil, nil, nil, nil, nil, nil, false, kdb).(*service)

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

	ffs := fs.NewFilesystem(fs.FilesystemTypeFake, rand.String(32)+"?nostfolder=true")

	_ = ffs.Mkdir("dir", 0o755)
	_ = fs.WriteFile(ffs, "file", []byte("hello"), 0o644)
	_ = ffs.Mkdir("MiXEDCase", 0o755)

	// We expect completion to return the full path to the completed
	// directory, with an ending slash.
	dirPath := "dir" + pathSep
	mixedCaseDirPath := "MiXEDCase" + pathSep

	cases := []struct {
		current string
		returns []string
	}{
		// The directory without slash is completed to one with slash.
		{"dir", []string{"dir" + pathSep}},
		// With slash it's completed to its contents.
		// Dirs are given pathSeps.
		// Files are not returned.
		{"", []string{mixedCaseDirPath, dirPath}},
		// Globbing is automatic based on prefix.
		{"d", []string{dirPath}},
		{"di", []string{dirPath}},
		{"dir", []string{dirPath}},
		{"f", nil},
		{"q", nil},
		// Globbing is case-insensitive
		{"mixed", []string{mixedCaseDirPath}},
	}

	for _, tc := range cases {
		ret := browseFiles(ffs, tc.current)
		if !slices.Equal(ret, tc.returns) {
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

	if build.IsDarwin {
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

			// Needed because GUIConfiguration.prepare() assigns this a random value if empty
			WebauthnUserId: "AAAA",
			// Needed because WebauthnCredentials is nil by default,
			// but gets replaced with empty slice before the check for whether config has changed,
			// causing the config server to restart between the
			// PUT /rest/config/devices and GET /rest/config/devices/{id} calls below,
			// causing the latter to fail on connection refused.
			WebauthnCredentials: []config.WebauthnCredential{},
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
		t.Error("Expected 50 for MaxSendKbps, got", opts.MaxSendKbps)
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
	if !build.IsLinux {
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

func encodeCosePublicKey(publicKey *ecdsa.PublicKey) ([]byte, error) {
	publicKeyCose := webauthncose.EC2PublicKeyData{
		PublicKeyData: webauthncose.PublicKeyData{
			KeyType:   int64(webauthncose.EllipticKey),
			Algorithm: int64(webauthncose.AlgES256),
		},
		Curve:  int64(webauthncose.P256),
		XCoord: publicKey.X.Bytes(),
		YCoord: publicKey.Y.Bytes(),
	}
	publicKeyCoseBytes, err := webauthncbor.Marshal(publicKeyCose)
	if err != nil {
		return nil, err
	}
	return publicKeyCoseBytes, nil
}

func createWebauthnRegistrationResponse(
	options webauthnProtocol.CredentialCreation,
	credentialId []byte,
	publicKeyCose []byte,
	origin string,
	signCount byte,
	transports []string,
	t *testing.T,
) webauthnProtocol.CredentialCreationResponse {
	rpIdHash := sha256.Sum256([]byte(options.Response.RelyingParty.ID))
	signCountBytes := []byte{0, 0, 0, signCount}

	aaguid := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f}
	credentialIdLength := []byte{byte(len(credentialId) >> 8), byte(len(credentialId) & 0xff)}
	attestedCredentialData := append(append(append(aaguid, credentialIdLength...), credentialId...), publicKeyCose...)

	authData := append(append(append(
		rpIdHash[:],
		byte(webauthnProtocol.FlagAttestedCredentialData|webauthnProtocol.FlagUserPresent),
	),
		signCountBytes...),
		attestedCredentialData...,
	)

	clientData := webauthnProtocol.CollectedClientData{
		Type:      webauthnProtocol.CreateCeremony,
		Challenge: options.Response.Challenge.String(),
		Origin:    origin,
	}
	clientDataJSON, err := json.Marshal(clientData)
	testutil.FatalIfErr(t, err)

	attObj, err := webauthncbor.Marshal(map[string]any{
		"fmt":      "none",
		"attStmt":  map[string]any{},
		"authData": authData,
	})
	testutil.FatalIfErr(t, err)

	return webauthnProtocol.CredentialCreationResponse{
		PublicKeyCredential: webauthnProtocol.PublicKeyCredential{
			Credential: webauthnProtocol.Credential{
				ID:   webauthnProtocol.URLEncodedBase64(credentialId).String(),
				Type: "public-key",
			},
			RawID: webauthnProtocol.URLEncodedBase64(credentialId),
		},
		AttestationResponse: webauthnProtocol.AuthenticatorAttestationResponse{
			AuthenticatorResponse: webauthnProtocol.AuthenticatorResponse{
				ClientDataJSON: webauthnProtocol.URLEncodedBase64(clientDataJSON),
			},
			AttestationObject: webauthnProtocol.URLEncodedBase64(attObj),
		},
		Transports: transports,
	}
}

func createWebauthnAssertionResponse(
	options webauthnProtocol.CredentialAssertion,
	credentialId []byte,
	privateKey *ecdsa.PrivateKey,
	origin string,
	userVerified bool,
	signCount byte,
	t *testing.T,
) webauthnProtocol.CredentialAssertionResponse {
	rpIdHash := sha256.Sum256([]byte(options.Response.RelyingPartyID))
	signCountBytes := []byte{0, 0, 0, signCount}

	authData := append(append(
		rpIdHash[:],
		byte(webauthnProtocol.FlagUserPresent|testutil.IfExpr(userVerified, webauthnProtocol.FlagUserVerified, 0)),
	),
		signCountBytes...)

	clientData := webauthnProtocol.CollectedClientData{
		Type:      webauthnProtocol.AssertCeremony,
		Challenge: options.Response.Challenge.String(),
		Origin:    origin,
	}
	clientDataJSON, err := json.Marshal(clientData)
	testutil.FatalIfErr(t, err)
	clientDataJSONHash := sha256.Sum256(clientDataJSON)
	signedData := testutil.ConcatSlices(authData, clientDataJSONHash[:])
	signedDataDigest := sha256.Sum256(signedData)

	sig, err := privateKey.Sign(cryptoRand.Reader, signedDataDigest[:], crypto.SHA256)
	testutil.FatalIfErr(t, err)

	return webauthnProtocol.CredentialAssertionResponse{
		PublicKeyCredential: webauthnProtocol.PublicKeyCredential{
			Credential: webauthnProtocol.Credential{
				ID:   webauthnProtocol.URLEncodedBase64(credentialId).String(),
				Type: "public-key",
			},
			RawID: webauthnProtocol.URLEncodedBase64(credentialId),
		},
		AssertionResponse: webauthnProtocol.AuthenticatorAssertionResponse{
			AuthenticatorResponse: webauthnProtocol.AuthenticatorResponse{
				ClientDataJSON: webauthnProtocol.URLEncodedBase64(clientDataJSON),
			},
			AuthenticatorData: webauthnProtocol.URLEncodedBase64(authData),
			Signature:         webauthnProtocol.URLEncodedBase64(sig),
		},
	}
}

func TestWebauthnRegistration(t *testing.T) {
	t.Parallel()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), cryptoRand.Reader)
	testutil.FatalIfErr(t, err)
	publicKeyCose, err := encodeCosePublicKey((privateKey.Public()).(*ecdsa.PublicKey))
	testutil.FatalIfErr(t, err)

	startServer := func(t *testing.T, credentials []config.WebauthnCredential) (string, string, string, func(t *testing.T) webauthnProtocol.CredentialCreation) {
		cfg := newMockedConfig()
		cfg.GUIReturns(config.GUIConfiguration{
			User:                "user",
			RawAddress:          "127.0.0.1:0",
			WebauthnCredentials: credentials,
		})
		baseURL, cancel, err := startHTTP(cfg)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(cancel)

		cli := &http.Client{
			Timeout: 15 * time.Second,
		}
		resp, err := cli.Get(baseURL)
		testutil.FatalIfErr(t, err)
		testutil.AssertEqual(t, t.Fatalf, resp.StatusCode, http.StatusOK,
			"Unexpected status while getting CSRF token: %v", resp.Status)
		resp.Body.Close()
		var csrfTokenName, csrfTokenValue string
		for _, cookie := range resp.Cookies() {
			if strings.HasPrefix(cookie.Name, "CSRF-Token") {
				csrfTokenName = cookie.Name
				csrfTokenValue = cookie.Value
				break
			}
		}
		testutil.AssertNotEqual(t, t.Fatalf, csrfTokenValue, "",
			"Failed to initialize test: no CSRF cookie returned from %v", baseURL)

		getCreateOptions := func(t *testing.T) webauthnProtocol.CredentialCreation {
			startResp := httpPostCsrf(baseURL+"/rest/config/webauthn/register-start", nil, csrfTokenName, csrfTokenValue, t)
			testutil.AssertEqual(t, t.Fatalf, startResp.StatusCode, http.StatusOK,
				"Failed to start WebAuthn registration: status %d", startResp.StatusCode)
			testutil.AssertFalse(t, t.Errorf, hasSessionCookie(startResp.Cookies()),
				"Expected no session cookie when starting WebAuthn registration")

			var options webauthnProtocol.CredentialCreation
			testutil.FatalIfErr(t, unmarshalTo(startResp.Body, &options))

			return options
		}

		return baseURL, csrfTokenName, csrfTokenValue, getCreateOptions
	}

	t.Run("Can register a new WebAuthn credential", func(t *testing.T) {
		t.Parallel()
		baseURL, csrfTokenName, csrfTokenValue, getCreateOptions := startServer(t, nil)
		options := getCreateOptions(t)

		transports := []string{"transportA", "transportB"}
		cred := createWebauthnRegistrationResponse(options, []byte{1, 2, 3, 4}, publicKeyCose, "https://localhost:8384", 42, transports, t)

		finishResp := httpPostCsrf(baseURL+"/rest/config/webauthn/register-finish", cred, csrfTokenName, csrfTokenValue, t)
		testutil.AssertEqual(t, t.Fatalf, finishResp.StatusCode, http.StatusOK,
			"Failed to finish WebAuthn registration: status %d", finishResp.StatusCode)

		var pendingCred config.WebauthnCredential
		testutil.FatalIfErr(t, unmarshalTo(finishResp.Body, &pendingCred))

		testutil.AssertEqual(t, t.Errorf, pendingCred.ID, base64.URLEncoding.EncodeToString([]byte{1, 2, 3, 4}),
			"Wrong credential ID in registration success response")

		testutil.AssertEqual(t, t.Errorf, pendingCred.RpId, "localhost", "Wrong RP ID in registration success response")
		testutil.AssertLessThan(t, t.Errorf, time.Since(pendingCred.CreateTime), 10*time.Second,
			"Wrong CreateTime in registration success response")
		testutil.AssertLessThan(t, t.Errorf, time.Since(pendingCred.LastUseTime), 10*time.Second,
			"Wrong LastUseTime in registration success response")
		testutil.AssertPredicate(t, t.Errorf, slices.Equal, transports, pendingCred.Transports,
			"Wrong Transports in registration success response")
		testutil.AssertEqual(t, t.Errorf, false, pendingCred.RequireUv, "Wrong RequireUv in registration success response")
		testutil.AssertEqual(t, t.Errorf, 42, pendingCred.SignCount, "Wrong SignCount in registration success response")
		testutil.AssertEqual(t, t.Errorf, "", pendingCred.Nickname, "Wrong Nickname in registration success response")

		var conf config.Configuration
		getConfResp := httpGetCsrf(baseURL+"/rest/config", csrfTokenName, csrfTokenValue, t)
		testutil.AssertEqual(t, t.Fatalf, getConfResp.StatusCode, http.StatusOK,
			"Failed to fetch config after WebAuthn registration: status %d", getConfResp.StatusCode)
		testutil.FatalIfErr(t, unmarshalTo(getConfResp.Body, &conf))
		testutil.AssertEqual(t, t.Errorf, 0, len(conf.GUI.WebauthnCredentials),
			"Expected newly registered WebAuthn credential to not yet be committed to config")
	})

	t.Run("WebAuthn registration fails with wrong challenge", func(t *testing.T) {
		t.Parallel()
		baseURL, csrfTokenName, csrfTokenValue, getCreateOptions := startServer(t, nil)
		options := getCreateOptions(t)

		cryptoRand.Reader.Read(options.Response.Challenge)

		cred := createWebauthnRegistrationResponse(options, []byte{1, 2, 3, 4}, publicKeyCose, "https://localhost:8384", 0, nil, t)
		finishResp := httpPostCsrf(baseURL+"/rest/config/webauthn/register-finish", cred, csrfTokenName, csrfTokenValue, t)
		testutil.AssertEqual(t, t.Fatalf, finishResp.StatusCode, http.StatusBadRequest,
			"Expected failure to register WebAuthn credential with wrong challenge; status: %d", finishResp.StatusCode)
	})

	t.Run("WebAuthn registration fails with wrong origin", func(t *testing.T) {
		t.Parallel()
		baseURL, csrfTokenName, csrfTokenValue, getCreateOptions := startServer(t, nil)
		options := getCreateOptions(t)

		cred := createWebauthnRegistrationResponse(options, []byte{1, 2, 3, 4}, publicKeyCose, "https://localhost", 0, nil, t)

		finishResp := httpPostCsrf(baseURL+"/rest/config/webauthn/register-finish", cred, csrfTokenName, csrfTokenValue, t)
		testutil.AssertEqual(t, t.Fatalf, finishResp.StatusCode, http.StatusBadRequest,
			"Expected failure to register WebAuthn credential with wrong origin; status: %d", finishResp.StatusCode)
	})

	t.Run("WebAuthn registration fails without user presence flag set", func(t *testing.T) {
		t.Parallel()
		baseURL, csrfTokenName, csrfTokenValue, getCreateOptions := startServer(t, nil)
		options := getCreateOptions(t)
		cred := createWebauthnRegistrationResponse(options, []byte{1, 2, 3, 4}, publicKeyCose, "https://localhost:8384", 0, nil, t)

		var attObj webauthnProtocol.AttestationObject
		err := webauthncbor.Unmarshal(cred.AttestationResponse.AttestationObject, &attObj)
		if err != nil {
			t.Fatal(err)
		}
		// Set the UP flag bit to 0
		attObj.RawAuthData[32] &= ^byte(webauthnProtocol.FlagUserPresent)
		modAttObj, err := webauthncbor.Marshal(attObj)
		if err != nil {
			t.Fatal(err)
		}
		cred.AttestationResponse.AttestationObject = modAttObj

		finishResp := httpPostCsrf(baseURL+"/rest/config/webauthn/register-finish", cred, csrfTokenName, csrfTokenValue, t)
		testutil.AssertEqual(t, t.Fatalf, finishResp.StatusCode, http.StatusBadRequest,
			"Expected failure to register WebAuthn credential without user presence flag set; status: %d", finishResp.StatusCode)
	})

	t.Run("WebAuthn registration fails with malformed public key", func(t *testing.T) {
		t.Parallel()
		baseURL, csrfTokenName, csrfTokenValue, getCreateOptions := startServer(t, nil)
		options := getCreateOptions(t)
		corruptPublicKeyCose := bytes.Clone(publicKeyCose)
		corruptPublicKeyCose[7] ^= 0xff
		cred := createWebauthnRegistrationResponse(options, []byte{1, 2, 3, 4}, corruptPublicKeyCose, "https://localhost:8384", 0, nil, t)

		finishResp := httpPostCsrf(baseURL+"/rest/config/webauthn/register-finish", cred, csrfTokenName, csrfTokenValue, t)
		testutil.AssertEqual(t, t.Fatalf, finishResp.StatusCode, http.StatusBadRequest,
			"Expected failure to register WebAuthn credential with malformed public key; status: %d", finishResp.StatusCode)
	})

	t.Run("WebAuthn registration fails with credential ID duplicated in config", func(t *testing.T) {
		t.Parallel()
		baseURL, csrfTokenName, csrfTokenValue, getCreateOptions := startServer(t,
			[]config.WebauthnCredential{
				{
					ID:            base64.URLEncoding.EncodeToString([]byte{1, 2, 3, 4}),
					RpId:          "localhost",
					PublicKeyCose: base64.URLEncoding.EncodeToString(publicKeyCose),
					SignCount:     0,
					RequireUv:     false,
				},
			},
		)
		options := getCreateOptions(t)
		cred := createWebauthnRegistrationResponse(options, []byte{1, 2, 3, 4}, publicKeyCose, "https://localhost:8384", 0, nil, t)
		finishResp := httpPostCsrf(baseURL+"/rest/config/webauthn/register-finish", cred, csrfTokenName, csrfTokenValue, t)
		testutil.AssertEqual(t, t.Fatalf, finishResp.StatusCode, http.StatusBadRequest,
			"Expected failure to register WebAuthn credential with duplicate credential ID; status: %d", finishResp.StatusCode)
	})

	t.Run("WebAuthn registration fails with credential ID duplicated in pending credentials", func(t *testing.T) {
		t.Parallel()
		baseURL, csrfTokenName, csrfTokenValue, getCreateOptions := startServer(t, nil)
		options := getCreateOptions(t)
		cred := createWebauthnRegistrationResponse(options, []byte{1, 2, 3, 4}, publicKeyCose, "https://localhost:8384", 0, nil, t)
		finishResp := httpPostCsrf(baseURL+"/rest/config/webauthn/register-finish", cred, csrfTokenName, csrfTokenValue, t)
		testutil.AssertEqual(t, t.Fatalf, finishResp.StatusCode, http.StatusOK,
			"Expected WebAuthn credential registration to succeed; status: %d", finishResp.StatusCode)

		options2 := getCreateOptions(t)
		cred2 := createWebauthnRegistrationResponse(options2, []byte{1, 2, 3, 4}, publicKeyCose, "https://localhost:8384", 0, nil, t)
		finishResp2 := httpPostCsrf(baseURL+"/rest/config/webauthn/register-finish", cred2, csrfTokenName, csrfTokenValue, t)

		testutil.AssertEqual(t, t.Fatalf, finishResp2.StatusCode, http.StatusBadRequest,
			"Expected failure to register WebAuthn credential with duplicate credential ID; status: %d", finishResp2.StatusCode)
	})

	t.Run("WebAuthn registration can only be attempted once per challenge", func(t *testing.T) {
		t.Parallel()
		baseURL, csrfTokenName, csrfTokenValue, getCreateOptions := startServer(t, nil)
		options := getCreateOptions(t)
		cred := createWebauthnRegistrationResponse(options, []byte{1, 2, 3, 4}, publicKeyCose, "https://localhost", 0, nil, t)
		finishResp := httpPostCsrf(baseURL+"/rest/config/webauthn/register-finish", cred, csrfTokenName, csrfTokenValue, t)
		testutil.AssertEqual(t, t.Fatalf, finishResp.StatusCode, http.StatusBadRequest,
			"Expected WebAuthn credential registration to fail; status: %d", finishResp.StatusCode)

		cred2 := createWebauthnRegistrationResponse(options, []byte{5, 6, 7, 8}, publicKeyCose, "https://localhost:8384", 0, nil, t)
		finishResp2 := httpPostCsrf(baseURL+"/rest/config/webauthn/register-finish", cred2, csrfTokenName, csrfTokenValue, t)

		testutil.AssertEqual(t, t.Fatalf, finishResp2.StatusCode, http.StatusBadRequest,
			"Expected WebAuthn credential registration to fail with reused challenge; status: %d", finishResp2.StatusCode)
	})
}

func TestWebauthnAuthentication(t *testing.T) {
	t.Parallel()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), cryptoRand.Reader)
	testutil.FatalIfErr(t, err)
	publicKeyCose, err := encodeCosePublicKey((privateKey.Public()).(*ecdsa.PublicKey))
	testutil.FatalIfErr(t, err)

	startServer := func(t *testing.T, rpId, origin string, credentials []config.WebauthnCredential) (func(string, string, string) *http.Response, func(string, any) *http.Response, func() webauthnProtocol.CredentialAssertion) {
		t.Helper()
		cfg := newMockedConfig()
		cfg.GUIReturns(config.GUIConfiguration{
			User:                "user",
			RawAddress:          "localhost:0",
			WebauthnCredentials: credentials,
			WebauthnRpId:        rpId,
			WebauthnOrigin:      origin,
			RawUseTLS:           true,
		})
		baseURL, cancel, err := startHTTP(cfg)
		testutil.FatalIfErr(t, err, "Failed to start HTTP server")
		t.Cleanup(cancel)

		httpRequest := func(method string, url string, body any, csrfTokenName, csrfTokenValue string) *http.Response {
			t.Helper()
			var bodyReader io.Reader = nil
			if body != nil {
				bodyBytes, err := json.Marshal(body)
				testutil.FatalIfErr(t, err, "Failed to marshal HTTP request body")
				bodyReader = bytes.NewReader(bodyBytes)
			}

			req, err := http.NewRequest(method, baseURL+url, bodyReader)
			testutil.FatalIfErr(t, err, "Failed to construct HttpRequest")

			if csrfTokenName != "" && csrfTokenValue != "" {
				req.Header.Set("X-"+csrfTokenName, csrfTokenValue)
			}

			client := http.Client{
				Timeout: 15 * time.Second,
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{
						// Syncthing config requires TLS in order to enable WebAuthn without a password set
						InsecureSkipVerify: true,
					},
				},
			}
			resp, err := client.Do(req)
			testutil.FatalIfErr(t, err, "Failed to execute HTTP request")

			return resp
		}

		httpGet := func(url string, csrfTokenName, csrfTokenValue string) *http.Response {
			t.Helper()
			return httpRequest(http.MethodGet, url, nil, csrfTokenName, csrfTokenValue)
		}

		httpPost := func(url string, body any) *http.Response {
			t.Helper()
			return httpRequest(http.MethodPost, url, body, "", "")
		}

		getAssertionOptions := func() webauthnProtocol.CredentialAssertion {
			t.Helper()
			startResp := httpPost("/rest/noauth/auth/webauthn-start", nil)
			testutil.AssertEqual(t, t.Fatalf, startResp.StatusCode, http.StatusOK,
				"Failed to start WebAuthn registration: status %d", startResp.StatusCode)
			testutil.AssertFalse(t, t.Errorf, hasSessionCookie(startResp.Cookies()),
				"Expected no session cookie when starting WebAuthn registration")

			var options webauthnProtocol.CredentialAssertion
			testutil.FatalIfErr(t, unmarshalTo(startResp.Body, &options), "Failed to unmarshal CredentialAssertion")

			return options
		}

		return httpGet, httpPost, getAssertionOptions
	}

	type webauthnAuthResponseBody struct {
		StayLoggedIn bool
		Credential   webauthnProtocol.CredentialAssertionResponse
	}
	webauthnAuthResponse := func(stayLoggedIn bool, cred webauthnProtocol.CredentialAssertionResponse) webauthnAuthResponseBody {
		return webauthnAuthResponseBody{
			StayLoggedIn: stayLoggedIn,
			Credential:   cred,
		}
	}

	t.Run("A credential that doesn't require UV", func(t *testing.T) {
		credentials := []config.WebauthnCredential{
			{
				ID:            base64.URLEncoding.EncodeToString([]byte{1, 2, 3, 4}),
				RpId:          "localhost",
				PublicKeyCose: base64.URLEncoding.EncodeToString(publicKeyCose),
				SignCount:     0,
				RequireUv:     false,
			},
		}

		t.Run("can authenticate without UV", func(t *testing.T) {
			t.Parallel()
			_, httpPost, getAssertionOptions := startServer(t, "", "", credentials)
			options := getAssertionOptions()

			cred := createWebauthnAssertionResponse(options, []byte{1, 2, 3, 4}, privateKey, "https://localhost:8384", false, 1, t)

			finishResp := httpPost("/rest/noauth/auth/webauthn-finish", webauthnAuthResponse(false, cred))
			testutil.AssertEqual(t, t.Fatalf, finishResp.StatusCode, http.StatusNoContent,
				"Failed WebAuthn authentication: status %d", finishResp.StatusCode)
		})

		t.Run("can authenticate without UV even if a different credential requires UV", func(t *testing.T) {
			t.Parallel()
			_, httpPost, getAssertionOptions := startServer(t, "", "", []config.WebauthnCredential{
				credentials[0],
				{
					ID:            base64.URLEncoding.EncodeToString([]byte{5, 6, 7, 8}),
					RpId:          "localhost",
					PublicKeyCose: base64.URLEncoding.EncodeToString(publicKeyCose),
					SignCount:     0,
					RequireUv:     true,
				},
			})
			options := getAssertionOptions()

			cred := createWebauthnAssertionResponse(options, []byte{1, 2, 3, 4}, privateKey, "https://localhost:8384", false, 1, t)

			finishResp := httpPost("/rest/noauth/auth/webauthn-finish", webauthnAuthResponse(false, cred))
			testutil.AssertEqual(t, t.Fatalf, finishResp.StatusCode, http.StatusNoContent,
				"Failed WebAuthn authentication: status %d", finishResp.StatusCode)
		})

		t.Run("can authenticate with UV", func(t *testing.T) {
			t.Parallel()
			_, httpPost, getAssertionOptions := startServer(t, "", "", credentials)
			options := getAssertionOptions()

			cred := createWebauthnAssertionResponse(options, []byte{1, 2, 3, 4}, privateKey, "https://localhost:8384", true, 1, t)

			finishResp := httpPost("/rest/noauth/auth/webauthn-finish", webauthnAuthResponse(false, cred))
			testutil.AssertEqual(t, t.Fatalf, finishResp.StatusCode, http.StatusNoContent,
				"Failed WebAuthn authentication: status %d", finishResp.StatusCode)
		})
	})

	t.Run("A credential that requires UV", func(t *testing.T) {
		credentials := []config.WebauthnCredential{
			{
				ID:            base64.URLEncoding.EncodeToString([]byte{1, 2, 3, 4}),
				RpId:          "localhost",
				PublicKeyCose: base64.URLEncoding.EncodeToString(publicKeyCose),
				SignCount:     0,
				RequireUv:     true,
			},
		}

		t.Run("cannot authenticate without UV", func(t *testing.T) {
			t.Parallel()
			_, httpPost, getAssertionOptions := startServer(t, "", "", credentials)
			options := getAssertionOptions()

			cred := createWebauthnAssertionResponse(options, []byte{1, 2, 3, 4}, privateKey, "https://localhost:8384", false, 1, t)

			finishResp := httpPost("/rest/noauth/auth/webauthn-finish", webauthnAuthResponse(false, cred))
			testutil.AssertEqual(t, t.Fatalf, finishResp.StatusCode, http.StatusConflict,
				"Expected WebAuthn authentication to fail without UV: status %d", finishResp.StatusCode)
		})

		t.Run("cannot authenticate without UV even if a different credential does not require UV", func(t *testing.T) {
			t.Parallel()
			_, httpPost, getAssertionOptions := startServer(t, "", "", []config.WebauthnCredential{
				credentials[0],
				{
					ID:            base64.URLEncoding.EncodeToString([]byte{5, 6, 7, 8}),
					RpId:          "localhost",
					PublicKeyCose: base64.URLEncoding.EncodeToString(publicKeyCose),
					SignCount:     0,
					RequireUv:     false,
				},
			})
			options := getAssertionOptions()

			cred := createWebauthnAssertionResponse(options, []byte{1, 2, 3, 4}, privateKey, "https://localhost:8384", false, 1, t)

			finishResp := httpPost("/rest/noauth/auth/webauthn-finish", webauthnAuthResponse(false, cred))
			testutil.AssertEqual(t, t.Fatalf, finishResp.StatusCode, http.StatusConflict,
				"Expected WebAuthn authentication to fail without UV: status %d", finishResp.StatusCode)
		})

		t.Run("can authenticate with UV", func(t *testing.T) {
			t.Parallel()
			_, httpPost, getAssertionOptions := startServer(t, "", "", credentials)
			options := getAssertionOptions()

			cred := createWebauthnAssertionResponse(options, []byte{1, 2, 3, 4}, privateKey, "https://localhost:8384", true, 1, t)

			finishResp := httpPost("/rest/noauth/auth/webauthn-finish", webauthnAuthResponse(false, cred))
			testutil.AssertEqual(t, t.Fatalf, finishResp.StatusCode, http.StatusNoContent,
				"Failed WebAuthn authentication: status %d", finishResp.StatusCode)
		})
	})

	t.Run("With non-default RP ID and origin", func(t *testing.T) {
		credentials := []config.WebauthnCredential{
			{
				ID:            base64.URLEncoding.EncodeToString([]byte{1, 2, 3, 4}),
				RpId:          "localhost",
				PublicKeyCose: base64.URLEncoding.EncodeToString(publicKeyCose),
			},
			{
				ID:            base64.URLEncoding.EncodeToString([]byte{5, 6, 7, 8}),
				RpId:          "custom-host",
				PublicKeyCose: base64.URLEncoding.EncodeToString(publicKeyCose),
			},
		}

		t.Run("Can use a credential with matching RP ID", func(t *testing.T) {
			t.Parallel()
			_, httpPost, getAssertionOptions := startServer(t, "custom-host", "https://origin-other-than-rp-id", credentials)
			options := getAssertionOptions()

			cred := createWebauthnAssertionResponse(options, []byte{5, 6, 7, 8}, privateKey, "https://origin-other-than-rp-id", false, 1, t)

			finishResp := httpPost("/rest/noauth/auth/webauthn-finish", webauthnAuthResponse(false, cred))
			testutil.AssertEqual(t, t.Fatalf, finishResp.StatusCode, http.StatusNoContent,
				"Failed WebAuthn authentication: status %d", finishResp.StatusCode)
		})

		t.Run("Cannot use a credential with non-matching RP ID", func(t *testing.T) {
			t.Parallel()
			_, httpPost, getAssertionOptions := startServer(t, "custom-host", "https://origin-other-than-rp-id", credentials)
			options := getAssertionOptions()

			cred := createWebauthnAssertionResponse(options, []byte{1, 2, 3, 4}, privateKey, "https://origin-other-than-rp-id", false, 1, t)

			finishResp := httpPost("/rest/noauth/auth/webauthn-finish", webauthnAuthResponse(false, cred))
			testutil.AssertEqual(t, t.Fatalf, finishResp.StatusCode, http.StatusForbidden,
				"Expected to fail WebAuthn authentication: status %d", finishResp.StatusCode)
		})

		t.Run("Cannot use a credential with matching RP ID on the wrong origin", func(t *testing.T) {
			t.Parallel()
			_, httpPost, getAssertionOptions := startServer(t, "custom-host", "https://origin-other-than-rp-id", credentials)
			options := getAssertionOptions()

			cred := createWebauthnAssertionResponse(options, []byte{5, 6, 7, 8}, privateKey, "https://localhost:8384", false, 1, t)

			finishResp := httpPost("/rest/noauth/auth/webauthn-finish", webauthnAuthResponse(false, cred))
			testutil.AssertEqual(t, t.Fatalf, finishResp.StatusCode, http.StatusForbidden,
				"Expected to fail WebAuthn authentication: status %d", finishResp.StatusCode)
		})
	})

	t.Run("Authentication fails", func(t *testing.T) {
		credentials := []config.WebauthnCredential{
			{
				ID:            base64.URLEncoding.EncodeToString([]byte{1, 2, 3, 4}),
				RpId:          "localhost",
				PublicKeyCose: base64.URLEncoding.EncodeToString(publicKeyCose),
				SignCount:     17,
				RequireUv:     false,
			},
		}

		t.Run("with wrong challenge", func(t *testing.T) {
			t.Parallel()
			_, httpPost, getAssertionOptions := startServer(t, "", "", credentials)
			options := getAssertionOptions()

			cryptoRand.Reader.Read(options.Response.Challenge)

			cred := createWebauthnAssertionResponse(options, []byte{1, 2, 3, 4}, privateKey, "https://localhost:8384", false, 18, t)
			finishResp := httpPost("/rest/noauth/auth/webauthn-finish", webauthnAuthResponse(false, cred))
			testutil.AssertEqual(t, t.Fatalf, finishResp.StatusCode, http.StatusForbidden)
		})

		t.Run("with wrong RP ID", func(t *testing.T) {
			t.Parallel()
			_, httpPost, getAssertionOptions := startServer(t, "not-localhost", "", append(credentials,
				config.WebauthnCredential{
					ID:            base64.URLEncoding.EncodeToString([]byte{5, 6, 7, 8}),
					RpId:          "not-localhost",
					PublicKeyCose: base64.URLEncoding.EncodeToString(publicKeyCose),
					SignCount:     17,
					RequireUv:     false,
				}))
			options := getAssertionOptions()

			cred := createWebauthnAssertionResponse(options, []byte{1, 2, 3, 4}, privateKey, "https://localhost:8384", false, 18, t)

			finishResp := httpPost("/rest/noauth/auth/webauthn-finish", webauthnAuthResponse(false, cred))
			testutil.AssertEqual(t, t.Fatalf, finishResp.StatusCode, http.StatusForbidden)
		})

		t.Run("with wrong origin", func(t *testing.T) {
			t.Parallel()
			_, httpPost, getAssertionOptions := startServer(t, "", "", credentials)
			options := getAssertionOptions()

			cred := createWebauthnAssertionResponse(options, []byte{1, 2, 3, 4}, privateKey, "https://localhost", false, 18, t)

			finishResp := httpPost("/rest/noauth/auth/webauthn-finish", webauthnAuthResponse(false, cred))
			testutil.AssertEqual(t, t.Fatalf, finishResp.StatusCode, http.StatusForbidden)
		})

		t.Run("without user presence flag set", func(t *testing.T) {
			t.Parallel()
			_, httpPost, getAssertionOptions := startServer(t, "", "", credentials)
			options := getAssertionOptions()
			cred := createWebauthnAssertionResponse(options, []byte{1, 2, 3, 4}, privateKey, "https://localhost:8384", false, 18, t)

			cred.AssertionResponse.AuthenticatorData[32] &= ^byte(webauthnProtocol.FlagUserPresent)

			finishResp := httpPost("/rest/noauth/auth/webauthn-finish", webauthnAuthResponse(false, cred))
			testutil.AssertEqual(t, t.Fatalf, finishResp.StatusCode, http.StatusForbidden)
		})

		t.Run("with signature by wrong private key", func(t *testing.T) {
			t.Parallel()
			_, httpPost, getAssertionOptions := startServer(t, "", "", credentials)
			options := getAssertionOptions()

			wrongPrivateKey, err := ecdsa.GenerateKey(elliptic.P256(), cryptoRand.Reader)
			testutil.FatalIfErr(t, err)

			cred := createWebauthnAssertionResponse(options, []byte{1, 2, 3, 4}, wrongPrivateKey, "https://localhost:8384", false, 18, t)
			finishResp := httpPost("/rest/noauth/auth/webauthn-finish", webauthnAuthResponse(false, cred))
			testutil.AssertEqual(t, t.Fatalf, finishResp.StatusCode, http.StatusForbidden)
		})

		t.Run("with invalid signature", func(t *testing.T) {
			t.Parallel()
			_, httpPost, getAssertionOptions := startServer(t, "", "", credentials)
			options := getAssertionOptions()

			cred := createWebauthnAssertionResponse(options, []byte{1, 2, 3, 4}, privateKey, "https://localhost:8384", false, 18, t)
			cred.AssertionResponse.Signature[17] ^= 0xff

			finishResp := httpPost("/rest/noauth/auth/webauthn-finish", webauthnAuthResponse(false, cred))
			testutil.AssertEqual(t, t.Fatalf, finishResp.StatusCode, http.StatusForbidden)
		})

		t.Run("with wrong credential ID", func(t *testing.T) {
			t.Parallel()
			_, httpPost, getAssertionOptions := startServer(t, "", "", credentials)
			options := getAssertionOptions()

			cred := createWebauthnAssertionResponse(options, []byte{5, 6, 7, 8}, privateKey, "https://localhost:8384", false, 18, t)
			finishResp := httpPost("/rest/noauth/auth/webauthn-finish", webauthnAuthResponse(false, cred))
			testutil.AssertEqual(t, t.Fatalf, finishResp.StatusCode, http.StatusForbidden)
		})
	})

	t.Run("Authentication can only be attempted once per challenge", func(t *testing.T) {
		t.Parallel()
		credentials := []config.WebauthnCredential{
			{
				ID:            base64.URLEncoding.EncodeToString([]byte{1, 2, 3, 4}),
				RpId:          "localhost",
				PublicKeyCose: base64.URLEncoding.EncodeToString(publicKeyCose),
				SignCount:     17,
				RequireUv:     false,
			},
		}
		_, httpPost, getAssertionOptions := startServer(t, "", "", credentials)
		options := getAssertionOptions()

		cred := createWebauthnAssertionResponse(options, []byte{5, 6, 7, 8}, privateKey, "https://localhost:8384", false, 18, t)
		finishResp := httpPost("/rest/noauth/auth/webauthn-finish", webauthnAuthResponse(false, cred))
		testutil.AssertEqual(t, t.Fatalf, finishResp.StatusCode, http.StatusForbidden)

		cred2 := createWebauthnAssertionResponse(options, []byte{1, 2, 3, 4}, privateKey, "https://localhost:8384", false, 18, t)
		finishResp2 := httpPost("/rest/noauth/auth/webauthn-finish", webauthnAuthResponse(false, cred2))
		testutil.AssertEqual(t, t.Fatalf, finishResp2.StatusCode, http.StatusForbidden)
	})

	t.Run("userVerification is set to", func(t *testing.T) {
		credsWithRequireUv := func(aRequiresUv, bRequiresUv bool) []config.WebauthnCredential {
			return []config.WebauthnCredential{
				{
					ID:        base64.URLEncoding.EncodeToString([]byte{1, 2, 3, 4}),
					RpId:      "localhost",
					RequireUv: aRequiresUv,
				},
				{
					ID:        base64.URLEncoding.EncodeToString([]byte{5, 6, 7, 8}),
					RpId:      "localhost",
					RequireUv: bRequiresUv,
				},
			}
		}

		t.Run("discouraged if no credential requires UV", func(t *testing.T) {
			t.Parallel()
			_, _, getAssertionOptions := startServer(t, "", "", credsWithRequireUv(false, false))
			options := getAssertionOptions()
			testutil.AssertEqual(t, t.Errorf, options.Response.UserVerification, "discouraged",
				"Expected userVerification: discouraged when no credential requires UV")
		})

		t.Run("preferred if some but not all credentials require UV", func(t *testing.T) {
			t.Parallel()
			{
				_, _, getAssertionOptions := startServer(t, "", "", credsWithRequireUv(true, false))
				options := getAssertionOptions()
				testutil.AssertEqual(t, t.Errorf, options.Response.UserVerification, "preferred",
					"Expected userVerification: preferred when some but not all credentials require UV")
			}

			{
				_, _, getAssertionOptions := startServer(t, "", "", credsWithRequireUv(false, true))
				options := getAssertionOptions()
				testutil.AssertEqual(t, t.Errorf, options.Response.UserVerification, "preferred",
					"Expected userVerification: preferred when some but not all credentials require UV")
			}
		})

		t.Run("required if all credentials require UV", func(t *testing.T) {
			t.Parallel()
			_, _, getAssertionOptions := startServer(t, "", "", credsWithRequireUv(true, true))
			options := getAssertionOptions()
			testutil.AssertEqual(t, t.Errorf, options.Response.UserVerification, "required",
				"Expected userVerification: required when all credentials require UV")
		})
	})

	t.Run("Credentials with wrong RP ID are not eligible", func(t *testing.T) {
		t.Parallel()
		_, _, getAssertionOptions := startServer(t, "", "", []config.WebauthnCredential{
			{
				ID:   "AAAA",
				RpId: "rp-id-is-not-localhost",
			},
			{
				ID:   "BBBB",
				RpId: "localhost",
			},
		})
		options := getAssertionOptions()
		testutil.AssertEqual(t, t.Errorf, len(options.Response.AllowedCredentials), 1,
			"Expected only credentials with RpId=%s in allowCredentials, got: %v", "localhost", options.Response.AllowedCredentials)
		testutil.AssertEqual(t, t.Errorf, options.Response.AllowedCredentials[0].CredentialID.String(), "BBBB",
			"Expected only credentials with RpId=%s in allowCredentials, got: %v", "localhost", options.Response.AllowedCredentials)
	})

	t.Run("Auth is required with a WebAuthn credential set", func(t *testing.T) {
		t.Parallel()
		httpGet, _, _ := startServer(t, "", "", []config.WebauthnCredential{
			{
				ID:   "AAAA",
				RpId: "localhost",
			},
		})
		csrfRresp := httpGet("/", "", "")
		csrfRresp.Body.Close()
		var csrfTokenName, csrfTokenValue string
		for _, cookie := range csrfRresp.Cookies() {
			if strings.HasPrefix(cookie.Name, "CSRF-Token") {
				csrfTokenName = cookie.Name
				csrfTokenValue = cookie.Value
				break
			}
		}

		resp := httpGet("/rest/config", csrfTokenName, csrfTokenValue)
		testutil.AssertEqual(t, t.Errorf, resp.StatusCode, http.StatusForbidden,
			"Expected auth to be required with WebAuthn credential set")
	})

	t.Run("No auth required when no password and no WebAuthn credentials set", func(t *testing.T) {
		t.Parallel()
		httpGet, _, _ := startServer(t, "", "", []config.WebauthnCredential{})
		csrfRresp := httpGet("/", "", "")
		csrfRresp.Body.Close()
		var csrfTokenName, csrfTokenValue string
		for _, cookie := range csrfRresp.Cookies() {
			if strings.HasPrefix(cookie.Name, "CSRF-Token") {
				csrfTokenName = cookie.Name
				csrfTokenValue = cookie.Value
				break
			}
		}

		resp := httpGet("/rest/config", csrfTokenName, csrfTokenValue)
		testutil.AssertEqual(t, t.Errorf, resp.StatusCode, http.StatusOK,
			"Expected no auth to be required with neither password nor WebAuthn credentials set")
	})
}

func TestPasswordOrWebauthnAuthentication(t *testing.T) {
	t.Parallel()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), cryptoRand.Reader)
	testutil.FatalIfErr(t, err)
	publicKeyCose, err := encodeCosePublicKey((privateKey.Public()).(*ecdsa.PublicKey))
	testutil.FatalIfErr(t, err)

	startServer := func(t *testing.T) (func(string, any) *http.Response, func() webauthnProtocol.CredentialAssertion) {
		t.Helper()

		credentials := []config.WebauthnCredential{
			{
				ID:            base64.URLEncoding.EncodeToString([]byte{1, 2, 3, 4}),
				RpId:          "localhost",
				PublicKeyCose: base64.URLEncoding.EncodeToString(publicKeyCose),
				SignCount:     0,
				RequireUv:     false,
			},
		}
		password := "$2a$10$IdIZTxTg/dCNuNEGlmLynOjqg4B1FvDKuIV5e0BB3pnWVHNb8.GSq" // bcrypt of "räksmörgås" in UTF-8

		cfg := newMockedConfig()
		cfg.GUIReturns(config.GUIConfiguration{
			User:                "user",
			Password:            password,
			RawAddress:          "localhost:0",
			WebauthnCredentials: credentials,

			// Don't need TLS in this test because the password enables the auth middleware,
			// and there's no browser to prevent us from generating a WebAuthn response without HTTPS
			RawUseTLS: false,
		})
		baseURL, cancel, err := startHTTP(cfg)
		testutil.FatalIfErr(t, err, "Failed to start HTTP server")
		t.Cleanup(cancel)

		httpRequest := func(method string, url string, body any) *http.Response {
			t.Helper()
			var bodyReader io.Reader = nil
			if body != nil {
				bodyBytes, err := json.Marshal(body)
				testutil.FatalIfErr(t, err, "Failed to marshal HTTP request body")
				bodyReader = bytes.NewReader(bodyBytes)
			}

			req, err := http.NewRequest(method, baseURL+url, bodyReader)
			testutil.FatalIfErr(t, err, "Failed to construct HttpRequest")

			client := http.Client{
				Timeout: 15 * time.Second,
			}
			resp, err := client.Do(req)
			testutil.FatalIfErr(t, err, "Failed to execute HTTP request")

			return resp
		}

		httpPost := func(url string, body any) *http.Response {
			t.Helper()
			return httpRequest(http.MethodPost, url, body)
		}

		getAssertionOptions := func() webauthnProtocol.CredentialAssertion {
			startResp := httpPost("/rest/noauth/auth/webauthn-start", nil)
			testutil.AssertEqual(t, t.Fatalf, startResp.StatusCode, http.StatusOK,
				"Failed to start WebAuthn registration: status %d", startResp.StatusCode)
			testutil.AssertFalse(t, t.Errorf, hasSessionCookie(startResp.Cookies()),
				"Expected no session cookie when starting WebAuthn registration")

			var options webauthnProtocol.CredentialAssertion
			testutil.FatalIfErr(t, unmarshalTo(startResp.Body, &options), "Failed to unmarshal CredentialAssertion")

			return options
		}

		return httpPost, getAssertionOptions
	}

	type webauthnAuthResponseBody struct {
		StayLoggedIn bool
		Credential   webauthnProtocol.CredentialAssertionResponse
	}
	webauthnAuthResponse := func(stayLoggedIn bool, cred webauthnProtocol.CredentialAssertionResponse) webauthnAuthResponseBody {
		return webauthnAuthResponseBody{
			StayLoggedIn: stayLoggedIn,
			Credential:   cred,
		}
	}

	type passwordAuthResponseBody struct {
		Username     string
		Password     string
		StayLoggedIn bool
	}

	t.Run("Can log in with password instead of WebAuthn", func(t *testing.T) {
		t.Parallel()
		httpPost, _ := startServer(t)

		finishResp := httpPost("/rest/noauth/auth/password", passwordAuthResponseBody{
			Username:     "user",
			Password:     "räksmörgås",
			StayLoggedIn: false,
		})
		testutil.AssertEqual(t, t.Fatalf, finishResp.StatusCode, http.StatusNoContent,
			"Failed password authentication: status %d", finishResp.StatusCode)
	})

	t.Run("Can log in with WebAuthn instead of password", func(t *testing.T) {
		t.Parallel()
		httpPost, getAssertionOptions := startServer(t)
		options := getAssertionOptions()

		cred := createWebauthnAssertionResponse(options, []byte{1, 2, 3, 4}, privateKey, "https://localhost:8384", false, 1, t)

		finishResp := httpPost("/rest/noauth/auth/webauthn-finish", webauthnAuthResponse(false, cred))
		testutil.AssertEqual(t, t.Fatalf, finishResp.StatusCode, http.StatusNoContent,
			"Failed WebAuthn authentication: status %d", finishResp.StatusCode)
	})
}

func guiConfigEqual(a config.GUIConfiguration, b config.GUIConfiguration) bool {
	return cmp.Equal(a, b)
}

func TestWebauthnConfigChanges(t *testing.T) {
	t.Parallel()

	const testAPIKey = "foobarbaz"
	initialGuiCfg := config.GUIConfiguration{
		RawAddress:     "127.0.0.1:0",
		RawUseTLS:      false,
		APIKey:         testAPIKey,
		WebauthnUserId: "AAAA",
		WebauthnCredentials: []config.WebauthnCredential{
			{
				ID:            "AAAA",
				RpId:          "localhost",
				Nickname:      "Credential A",
				PublicKeyCose: base64.URLEncoding.EncodeToString([]byte{1, 2, 3, 4}),
				SignCount:     0,
				Transports:    []string{"transportA"},
				RequireUv:     false,
				CreateTime:    time.Now(),
				LastUseTime:   time.Now(),
			},
		},
	}

	initTest := func(t *testing.T) (config.Configuration, func(*testing.T) (func(string) *http.Response, func(string, string, any))) {
		cfg := config.Configuration{
			GUI: initialGuiCfg.Copy(),
		}

		tmpFile, err := os.CreateTemp("", "syncthing-testConfig-Webauthn-*")
		testutil.FatalIfErr(t, err, "Failed to create tmpfile for test")
		w := config.Wrap(tmpFile.Name(), cfg, protocol.LocalDeviceID, events.NoopLogger)
		tmpFile.Close()
		cfgCtx, cfgCancel := context.WithCancel(context.Background())
		go w.Serve(cfgCtx)
		t.Cleanup(func() {
			os.Remove(tmpFile.Name())
			cfgCancel()
		})

		startHttpServer := func(t *testing.T) (func(string) *http.Response, func(string, string, any)) {
			baseURL, cancel, err := startHTTP(w)
			t.Cleanup(cancel)
			testutil.FatalIfErr(t, err)

			cli := &http.Client{
				Timeout: 60 * time.Second,
			}

			do := func(req *http.Request, status int) *http.Response {
				t.Helper()
				req.Header.Set("X-API-Key", testAPIKey)
				resp, err := cli.Do(req)
				testutil.FatalIfErr(t, err)
				testutil.AssertEqual(t, t.Errorf, status, resp.StatusCode, "Expected status %v, got %v", status, resp.StatusCode)
				return resp
			}

			mod := func(method, path string, data interface{}) {
				t.Helper()
				bs, err := json.Marshal(data)
				testutil.FatalIfErr(t, err)
				req, _ := http.NewRequest(method, baseURL+path, bytes.NewReader(bs))
				do(req, http.StatusOK).Body.Close()
			}

			get := func(path string) *http.Response {
				t.Helper()
				req, _ := http.NewRequest(http.MethodGet, baseURL+path, nil)
				return do(req, http.StatusOK)
			}
			return get, mod
		}

		return cfg, startHttpServer
	}

	guiCfgPath := "/rest/config/gui"

	t.Run("Cannot add WebAuthn credential through just config", func(t *testing.T) {
		t.Parallel()
		cfg, startHttpServer := initTest(t)
		{
			_, mod := startHttpServer(t)
			guiCfg := cfg.GUI.Copy()
			guiCfg.WebauthnCredentials = append(
				guiCfg.WebauthnCredentials,
				config.WebauthnCredential{
					ID:            "BBBB",
					RpId:          "localhost",
					PublicKeyCose: base64.URLEncoding.EncodeToString([]byte{}),
					SignCount:     0,
					RequireUv:     false,
				},
			)
			mod(http.MethodPut, guiCfgPath, guiCfg)
		}
		{
			get, _ := startHttpServer(t)
			resp := get(guiCfgPath)
			var guiCfg config.GUIConfiguration
			testutil.FatalIfErr(t, unmarshalTo(resp.Body, &guiCfg))
			testutil.AssertPredicate(t, t.Errorf, guiConfigEqual, guiCfg, initialGuiCfg,
				"Expected not to be able to add WebAuthn credentials through just config. Updated config: %v", guiCfg)
		}
	})

	t.Run("Editing WebAuthn credential ID results in deleting the existing credential", func(t *testing.T) {
		t.Parallel()
		cfg, startHttpServer := initTest(t)
		{
			_, mod := startHttpServer(t)
			guiCfg := cfg.GUI.Copy()
			guiCfg.WebauthnCredentials[0].ID = "ZZZZ"
			mod(http.MethodPut, guiCfgPath, guiCfg)
		}
		{
			get, _ := startHttpServer(t)
			resp := get(guiCfgPath)
			var guiCfg config.GUIConfiguration
			testutil.FatalIfErr(t, unmarshalTo(resp.Body, &guiCfg))
			testutil.AssertEqual(t, t.Errorf, 0, len(guiCfg.WebauthnCredentials),
				"Expected attempt to edit WebAuthn credential ID to result in deleting the existing credential. Updated config: %v", guiCfg)
		}
	})

	testCanEditConfig := func(propName string, modify func(*config.GUIConfiguration), verify func(config.GUIConfiguration) bool) {
		t.Run(fmt.Sprintf("Can edit GUIConfiguration.%s", propName), func(t *testing.T) {
			t.Parallel()
			cfg, startHttpServer := initTest(t)
			{
				_, mod := startHttpServer(t)
				guiCfg := cfg.GUI.Copy()
				modify(&guiCfg)
				mod(http.MethodPut, guiCfgPath, guiCfg)
			}
			{
				get, _ := startHttpServer(t)
				resp := get(guiCfgPath)
				var guiCfg config.GUIConfiguration
				testutil.FatalIfErr(t, unmarshalTo(resp.Body, &guiCfg))
				testutil.AssertTrue(
					t,
					t.Errorf,
					!guiConfigEqual(guiCfg, initialGuiCfg) && verify(guiCfg),
					"Expected to be able to edit GUIConfiguration.%s. Updated config: %v", propName, guiCfg)
			}
		})
	}

	testCanEditConfig("WebauthnUserId", func(guiCfg *config.GUIConfiguration) {
		guiCfg.WebauthnUserId = "ABCDEFGH"
	}, func(guiCfg config.GUIConfiguration) bool {
		return guiCfg.WebauthnUserId == "ABCDEFGH"
	})
	testCanEditConfig("WebauthnRpId", func(guiCfg *config.GUIConfiguration) {
		guiCfg.WebauthnRpId = "no-longer-localhost"
	}, func(guiCfg config.GUIConfiguration) bool {
		return guiCfg.WebauthnRpId == "no-longer-localhost"
	})
	testCanEditConfig("WebauthnOrigin", func(guiCfg *config.GUIConfiguration) {
		guiCfg.WebauthnOrigin = "https://no-longer-localhost:8888"
	}, func(guiCfg config.GUIConfiguration) bool {
		return guiCfg.WebauthnOrigin == "https://no-longer-localhost:8888"
	})

	testCannotEditCredential := func(propName string, modify func(*config.GUIConfiguration)) {
		t.Run(fmt.Sprintf("Cannot edit WebAuthnCredential.%s", propName), func(t *testing.T) {
			t.Parallel()
			cfg, startHttpServer := initTest(t)
			{
				_, mod := startHttpServer(t)
				guiCfg := cfg.GUI.Copy()
				modify(&guiCfg)
				mod(http.MethodPut, guiCfgPath, guiCfg)
			}
			{
				get, _ := startHttpServer(t)
				resp := get(guiCfgPath)
				var guiCfg config.GUIConfiguration
				testutil.FatalIfErr(t, unmarshalTo(resp.Body, &guiCfg))
				testutil.AssertPredicate(t, t.Errorf, guiConfigEqual, guiCfg, initialGuiCfg,
					"Expected to not be able to edit %s of WebAuthn credential. Updated config: %v", propName, guiCfg)
			}
		})
	}

	testCannotEditCredential("RpId", func(guiCfg *config.GUIConfiguration) {
		guiCfg.WebauthnCredentials[0].RpId = "no-longer-locahost"
	})
	testCannotEditCredential("PublicKeyCose", func(guiCfg *config.GUIConfiguration) {
		guiCfg.WebauthnCredentials[0].PublicKeyCose = "BBBB"
	})
	testCannotEditCredential("SignCount", func(guiCfg *config.GUIConfiguration) {
		guiCfg.WebauthnCredentials[0].SignCount = 1337
	})
	testCannotEditCredential("Transports", func(guiCfg *config.GUIConfiguration) {
		guiCfg.WebauthnCredentials[0].Transports = []string{"transportA", "transportC"}
	})
	testCannotEditCredential("CreateTime", func(guiCfg *config.GUIConfiguration) {
		guiCfg.WebauthnCredentials[0].CreateTime = time.Now().Add(10 * time.Second)
	})
	testCannotEditCredential("LastUseTime", func(guiCfg *config.GUIConfiguration) {
		guiCfg.WebauthnCredentials[0].LastUseTime = time.Now().Add(10 * time.Second)
	})

	testCanEditCredential := func(propName string, modify func(*config.GUIConfiguration), verify func(config.GUIConfiguration) bool) {
		t.Run(fmt.Sprintf("Can edit WebauthnCredential.%s", propName), func(t *testing.T) {
			t.Parallel()
			cfg, startHttpServer := initTest(t)
			{
				_, mod := startHttpServer(t)
				guiCfg := cfg.GUI.Copy()
				modify(&guiCfg)
				mod(http.MethodPut, guiCfgPath, guiCfg)
			}
			{
				get, _ := startHttpServer(t)
				resp := get(guiCfgPath)
				var guiCfg config.GUIConfiguration
				testutil.FatalIfErr(t, unmarshalTo(resp.Body, &guiCfg))
				testutil.AssertTrue(t, t.Errorf,
					!guiConfigEqual(guiCfg, initialGuiCfg) && verify(guiCfg),
					"Expected to be able to edit %s of WebAuthn credential. Updated config: %v", propName, guiCfg)
			}
		})
	}

	testCanEditCredential("Nickname", func(guiCfg *config.GUIConfiguration) {
		guiCfg.WebauthnCredentials[0].Nickname = "Blåbärsmjölk"
	}, func(guiCfg config.GUIConfiguration) bool {
		return guiCfg.WebauthnCredentials[0].Nickname == "Blåbärsmjölk"
	})
	testCanEditCredential("RequireUv", func(guiCfg *config.GUIConfiguration) {
		guiCfg.WebauthnCredentials[0].RequireUv = true
	}, func(guiCfg config.GUIConfiguration) bool {
		return guiCfg.WebauthnCredentials[0].RequireUv == true
	})
}
