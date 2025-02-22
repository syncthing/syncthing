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
	"github.com/thejerf/suture/v4"

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
	"github.com/syncthing/syncthing/lib/structutil"
	"github.com/syncthing/syncthing/lib/svcutil"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/testutil"
	"github.com/syncthing/syncthing/lib/tlsutil"
	"github.com/syncthing/syncthing/lib/ur"
)

var (
	confDir    = filepath.Join("testdata", "config")
	dev1       protocol.DeviceID
	apiCfg     = newMockedConfig()
	testAPIKey = "foobarbaz"
)

func withTestDefaults(guiCfg config.GUIConfiguration) config.GUIConfiguration {
	defaultGuiCfg := structutil.WithDefaults(config.GUIConfiguration{})

	if guiCfg.WebauthnRpId == "" {
		guiCfg.WebauthnRpId = defaultGuiCfg.WebauthnRpId
	}
	if len(guiCfg.WebauthnOrigins) == 0 {
		guiCfg.WebauthnOrigins = []string{"https://" + defaultGuiCfg.WebauthnRpId + ":8384"}
	}

	return guiCfg
}

func init() {
	dev1, _ = protocol.DeviceIDFromString("AIR6LPZ-7K4PTTV-UXQSMUU-CPQ5YWH-OEDFIIQ-JUG777G-2YQXXR5-YD6AWQR")
	apiCfg.GUIReturns(withTestDefaults(config.GUIConfiguration{APIKey: testAPIKey, RawAddress: "127.0.0.1:0"}))
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
		GUI: withTestDefaults(config.GUIConfiguration{
			RawAddress: "127.0.0.1:0",
			RawUseTLS:  false,
		}),
	}
	w := config.Wrap("/dev/null", cfg, protocol.LocalDeviceID, events.NoopLogger)

	mdb, _ := db.NewLowlevel(backend.OpenMemory(), events.NoopLogger)
	kdb := db.NewMiscDataNamespace(mdb)
	srvAbstract := New(protocol.LocalDeviceID, w, "", "syncthing", nil, nil, nil, events.NoopLogger, nil, nil, nil, nil, nil, nil, false, kdb)
	srv := srvAbstract.(*service)

	srv.started = make(chan startedTestMsg)

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

	baseURL, cancel, _, err := startHTTP(apiCfg)
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
	// Since running tests in parallel, the previous 1s timeout proved to be too short.
	// https://github.com/syncthing/syncthing/issues/9455
	timeout := 10 * time.Second
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

func getSessionCookie(cookies []*http.Cookie) (*http.Cookie, bool) {
	for _, cookie := range cookies {
		if cookie.MaxAge >= 0 && strings.HasPrefix(cookie.Name, "sessionid") {
			return cookie, true
		}
	}
	return nil, false
}

func hasSessionCookie(cookies []*http.Cookie) bool {
	_, ok := getSessionCookie(cookies)
	return ok
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

func httpPostCsrfAuth(url string, body any, xapikeyHeader, csrfTokenName, csrfTokenValue string, t *testing.T) *http.Response {
	t.Helper()
	return httpRequest(http.MethodPost, url, body, "", "", xapikeyHeader, "", csrfTokenName, csrfTokenValue, nil, t)
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
		cfg.GUIReturns(withTestDefaults(config.GUIConfiguration{
			User:                "üser",
			Password:            "$2a$10$IdIZTxTg/dCNuNEGlmLynOjqg4B1FvDKuIV5e0BB3pnWVHNb8.GSq", // bcrypt of "räksmörgås" in UTF-8
			RawAddress:          "127.0.0.1:0",
			APIKey:              testAPIKey,
			SendBasicAuthPrompt: sendBasicAuthPrompt,
		}))
		baseURL, cancel, _, err := startHTTP(cfg)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(cancel)
		url := baseURL + path

		t.Run(fmt.Sprintf("%d path", expectedOkStatus), func(t *testing.T) {
			t.Parallel()
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
	cfg.GUIReturns(withTestDefaults(config.GUIConfiguration{
		User:                "üser",
		Password:            "$2a$10$IdIZTxTg/dCNuNEGlmLynOjqg4B1FvDKuIV5e0BB3pnWVHNb8.GSq", // bcrypt of "räksmörgås" in UTF-8
		SendBasicAuthPrompt: false,
	}))
	baseURL, cancel, _, err := startHTTP(cfg)
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
	cfg.GUIReturns(withTestDefaults(config.GUIConfiguration{
		RawAddress: "127.0.0.1:0",
		APIKey:     testAPIKey,
	}))
	baseURL, cancel, _, err := startHTTP(cfg)
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

func startHTTP(cfg config.Wrapper) (string, context.CancelFunc, *webauthnService, error) {
	return startHTTPWithShutdownTimeout(cfg, 0)
}

func startHTTPWithShutdownTimeout(cfg config.Wrapper, shutdownTimeout time.Duration) (string, context.CancelFunc, *webauthnService, error) {
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
	startedChan := make(chan startedTestMsg)
	mockedSummary := &modelmocks.FolderSummaryService{}
	mockedSummary.SummaryReturns(new(model.FolderSummary), nil)

	// Instantiate the API service
	urService := ur.New(cfg, m, connections, false)
	mdb, _ := db.NewLowlevel(backend.OpenMemory(), events.NoopLogger)
	kdb := db.NewMiscDataNamespace(mdb)
	svcAbstract := New(protocol.LocalDeviceID, cfg, assetDir, "syncthing", m, eventSub, diskEventSub, events.NoopLogger, discoverer, connections, urService, mockedSummary, errorLog, systemLog, false, kdb)
	svc := svcAbstract.(*service)

	svc.started = startedChan

	if shutdownTimeout > 0*time.Millisecond {
		svc.shutdownTimeout = shutdownTimeout
	}

	// Actually start the API service
	supervisor := suture.New("API test", suture.Spec{
		PassThroughPanics: true,
	})
	supervisor.Add(svc)
	ctx, cancel := context.WithCancel(context.Background())
	supervisor.ServeBackground(ctx)

	// Make sure the API service is listening, and get the URL to use.
	startedMsg := <-startedChan
	webauthnService := startedMsg.webauthnService
	tcpAddr, err := net.ResolveTCPAddr("tcp", startedMsg.address)
	if err != nil {
		cancel()
		return "", cancel, webauthnService, fmt.Errorf("weird address from API service: %w", err)
	}

	host, _, _ := net.SplitHostPort(cfg.GUI().RawAddress)
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	baseURL := fmt.Sprintf("http://%s", net.JoinHostPort(host, strconv.Itoa(tcpAddr.Port)))

	return baseURL, cancel, webauthnService, nil
}

func TestCSRFRequired(t *testing.T) {
	t.Parallel()

	baseURL, cancel, _, err := startHTTP(apiCfg)
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

	baseURL, cancel, _, err := startHTTP(apiCfg)
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
	baseURL, cancel, _, err := startHTTP(apiCfg)
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
	cfg.GUIReturns(withTestDefaults(config.GUIConfiguration{RawAddress: "127.0.0.1:0"}))
	baseURL, cancel, _, err := startHTTP(cfg)
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
	cfg.GUIReturns(withTestDefaults(config.GUIConfiguration{
		RawAddress:            "127.0.0.1:0",
		InsecureSkipHostCheck: true,
	}))
	baseURL, cancel, _, err = startHTTP(cfg)
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
		baseURL, cancel, _, err = startHTTP(cfg)
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
	cfg.GUIReturns(withTestDefaults(config.GUIConfiguration{
		RawAddress: "[::1]:0",
	}))
	baseURL, cancel, _, err = startHTTP(cfg)
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

	baseURL, cancel, _, err := startHTTP(apiCfg)
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

	baseURL, cancel, _, err := startHTTP(apiCfg)
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
	svcAbstract := New(protocol.LocalDeviceID, cfg, "", "syncthing", nil, defSub, diskSub, events.NoopLogger, nil, nil, nil, nil, nil, nil, false, kdb)
	svc := svcAbstract.(*service)

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
		GUI: withTestDefaults(config.GUIConfiguration{
			RawAddress: "127.0.0.1:0",
			RawUseTLS:  false,
			APIKey:     testAPIKey,

			// Needed because GUIConfiguration.prepare() assigns this a random value if empty
			WebauthnUserId: []byte{0, 0, 0},
		}),
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
	baseURL, cancel, _, err := startHTTP(w)
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
	if err != nil {
		t.Fatal(err)
	}

	attObj, err := webauthncbor.Marshal(map[string]any{
		"fmt":      "none",
		"attStmt":  map[string]any{},
		"authData": authData,
	})
	if err != nil {
		t.Fatal(err)
	}

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
	if err != nil {
		t.Fatal(err)
	}
	clientDataJSONHash := sha256.Sum256(clientDataJSON)
	signedData := slices.Concat(authData, clientDataJSONHash[:])
	signedDataDigest := sha256.Sum256(signedData)

	sig, err := privateKey.Sign(cryptoRand.Reader, signedDataDigest[:], crypto.SHA256)
	if err != nil {
		t.Fatal(err)
	}

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

func (req *startWebauthnRegistrationResponse) finish(cred *webauthnProtocol.CredentialCreationResponse) finishWebauthnRegistrationRequest {
	return finishWebauthnRegistrationRequest{
		RequestID:  req.RequestID,
		Credential: *cred,
	}
}

func (req *startWebauthnAuthenticationResponse) finish(cred *webauthnProtocol.CredentialAssertionResponse, stayLoggedIn bool) finishWebauthnAuthenticationRequest {
	return finishWebauthnAuthenticationRequest{
		StayLoggedIn: stayLoggedIn,
		RequestID:    req.RequestID,
		Credential:   *cred,
	}
}

func TestWebauthnRegistration(t *testing.T) {
	t.Parallel()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), cryptoRand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicKeyCose, err := encodeCosePublicKey((privateKey.Public()).(*ecdsa.PublicKey))
	if err != nil {
		t.Fatal(err)
	}

	startServer := func(t *testing.T, credentials []config.WebauthnCredential) (string, string, string, *webauthnService, func(t *testing.T) startWebauthnRegistrationResponse, config.Wrapper) {
		cfg := newMockedConfig()
		cfg.GUIReturns(withTestDefaults(config.GUIConfiguration{
			User:                "user",
			RawAddress:          "127.0.0.1:0",
			APIKey:              testAPIKey,
			WebauthnCredentials: credentials,
		}))
		baseURL, cancel, webauthnService, err := startHTTP(cfg)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(cancel)

		cli := &http.Client{
			Timeout: 15 * time.Second,
		}
		resp, err := cli.Get(baseURL)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Unexpected status while getting CSRF token: %v", resp.Status)
		}
		resp.Body.Close()
		var csrfTokenName, csrfTokenValue string
		for _, cookie := range resp.Cookies() {
			if strings.HasPrefix(cookie.Name, "CSRF-Token") {
				csrfTokenName = cookie.Name
				csrfTokenValue = cookie.Value
				break
			}
		}
		if csrfTokenValue == "" {
			t.Fatalf("Failed to initialize test: no CSRF cookie returned from %v", baseURL)
		}

		getCreateOptions := func(t *testing.T) startWebauthnRegistrationResponse {
			startResp := httpPostCsrfAuth(baseURL+"/rest/config/gui/webauthn/register-start", nil, testAPIKey, csrfTokenName, csrfTokenValue, t)
			if startResp.StatusCode != http.StatusOK {
				t.Fatalf("Failed to start WebAuthn registration: status %d", startResp.StatusCode)
			}
			if hasSessionCookie(startResp.Cookies()) {
				t.Errorf("Expected no session cookie when starting WebAuthn registration")
			}

			var respBody startWebauthnRegistrationResponse
			err := unmarshalTo(startResp.Body, &respBody)
			if err != nil {
				t.Fatal(err)
			}

			return respBody
		}

		return baseURL, csrfTokenName, csrfTokenValue, webauthnService, getCreateOptions, cfg
	}

	t.Run("Can register a new WebAuthn credential", func(t *testing.T) {
		t.Parallel()
		baseURL, csrfTokenName, csrfTokenValue, _, getCreateOptions, cfg := startServer(t, nil)
		startResp := getCreateOptions(t)

		transports := []string{"transportA", "transportB"}
		cred := createWebauthnRegistrationResponse(startResp.Options, []byte{1, 2, 3, 4}, publicKeyCose, "https://localhost:8384", 42, transports, t)

		finishResp := httpPostCsrf(baseURL+"/rest/config/gui/webauthn/register-finish", startResp.finish(&cred), csrfTokenName, csrfTokenValue, t)
		if finishResp.StatusCode != http.StatusOK {
			t.Fatalf("Failed to finish WebAuthn registration: status %d", finishResp.StatusCode)
		}

		var pendingCred config.WebauthnCredential
		err := unmarshalTo(finishResp.Body, &pendingCred)
		if err != nil {
			t.Fatal(err)
		}

		if pendingCred.ID != base64.RawURLEncoding.EncodeToString([]byte{1, 2, 3, 4}) {
			t.Errorf("Wrong credential ID in registration success response: %v", pendingCred.ID)
		}

		if pendingCred.RpId != "localhost" {
			t.Errorf("Wrong RP ID in registration success response: %v", pendingCred.RpId)
		}
		if !(time.Since(pendingCred.CreateTime) < 10*time.Second) {
			t.Errorf("Wrong CreateTime in registration success response")
		}
		if !slices.Equal(pendingCred.Transports, transports) {
			t.Errorf("Wrong Transports in registration success response: %v != %v", transports, pendingCred.Transports)
		}
		if pendingCred.RequireUv {
			t.Errorf("Wrong RequireUv in registration success response")
		}
		if pendingCred.Nickname != "" {
			t.Errorf("Wrong Nickname in registration success response")
		}

		var volState WebauthnVolatileState
		getVolStateResp := httpGetCsrf(baseURL+"/rest/webauthn/state", csrfTokenName, csrfTokenValue, t)
		err = unmarshalTo(getVolStateResp.Body, &volState)
		if err != nil {
			t.Fatal(err)
		}
		credVolState := volState.Credentials[pendingCred.ID]
		if !(time.Since(credVolState.LastUseTime) < 10*time.Second) {
			t.Errorf("Wrong LastUseTime after registration success")
		}
		if credVolState.SignCount != 42 {
			t.Errorf("Wrong SignCount after registration success")
		}

		var conf config.Configuration
		getConfResp := httpGetCsrf(baseURL+"/rest/config", csrfTokenName, csrfTokenValue, t)
		if getConfResp.StatusCode != http.StatusOK {
			t.Fatalf("Failed to fetch config after WebAuthn registration: status %d", getConfResp.StatusCode)
		}
		err = unmarshalTo(getConfResp.Body, &conf)
		if err != nil {
			t.Fatal(err)
		}
		eligibleCredentials := cfg.GUI().EligibleWebAuthnCredentials(cfg.GUI())
		if err != nil {
			t.Fatal(err, "Failed to retrieve registered WebAuthn credentials")
		}
		if len(eligibleCredentials) != 0 {
			t.Errorf("Expected newly registered WebAuthn credential to not yet be committed to config")
		}
	})

	t.Run("Can register two WebAuthn credentials concurrently.", func(t *testing.T) {
		t.Parallel()
		baseURL, csrfTokenName, csrfTokenValue, _, getCreateOptions, _ := startServer(t, nil)
		startResp1 := getCreateOptions(t)
		startResp2 := getCreateOptions(t)

		transports := []string{"transportA", "transportB"}
		cred1 := createWebauthnRegistrationResponse(startResp1.Options, []byte{1, 2, 3, 4}, publicKeyCose, "https://localhost:8384", 42, transports, t)
		cred2 := createWebauthnRegistrationResponse(startResp2.Options, []byte{5, 6, 7, 8}, publicKeyCose, "https://localhost:8384", 37, transports, t)

		finishResp1 := httpPostCsrf(baseURL+"/rest/config/gui/webauthn/register-finish", startResp1.finish(&cred1), csrfTokenName, csrfTokenValue, t)
		if finishResp1.StatusCode != http.StatusOK {
			t.Errorf("Failed to finish 1st concurrent WebAuthn registration: status %d", finishResp1.StatusCode)
		}

		finishResp2 := httpPostCsrf(baseURL+"/rest/config/gui/webauthn/register-finish", startResp2.finish(&cred2), csrfTokenName, csrfTokenValue, t)
		if finishResp2.StatusCode != http.StatusOK {
			t.Errorf("Failed to finish 2nd concurrent WebAuthn registration: status %d", finishResp2.StatusCode)
		}
	})

	t.Run("WebAuthn registration times out after 10 minutes", func(t *testing.T) {
		t.Parallel()
		baseURL, csrfTokenName, csrfTokenValue, webauthnService, getCreateOptions, _ := startServer(t, nil)
		t0 := time.Now()
		webauthnService.timeNow = func() time.Time {
			return t0
		}
		startResp1 := getCreateOptions(t)
		startResp2 := getCreateOptions(t)

		if startResp1.Options.Response.Timeout != 10*60*1000 {
			t.Errorf("Expected PublicKeyCredentialCreationOptions.timeout to be set to 10 minutes, was: %v ms", startResp1.Options.Response.Timeout)
		}

		transports := []string{"transportA", "transportB"}
		cred1 := createWebauthnRegistrationResponse(startResp1.Options, []byte{1, 2, 3, 4}, publicKeyCose, "https://localhost:8384", 42, transports, t)
		cred2 := createWebauthnRegistrationResponse(startResp2.Options, []byte{5, 6, 7, 8}, publicKeyCose, "https://localhost:8384", 37, transports, t)

		webauthnService.timeNow = func() time.Time {
			return t0.Add(time.Minute*10 - time.Second*1)
		}
		finishResp1 := httpPostCsrf(baseURL+"/rest/config/gui/webauthn/register-finish", startResp1.finish(&cred1), csrfTokenName, csrfTokenValue, t)
		if finishResp1.StatusCode != http.StatusOK {
			t.Fatalf("WebAuthn registration failed: status %d", finishResp1.StatusCode)
		}

		webauthnService.timeNow = func() time.Time {
			return t0.Add(time.Minute*10 + time.Second*1)
		}
		finishResp2 := httpPostCsrf(baseURL+"/rest/config/gui/webauthn/register-finish", startResp2.finish(&cred2), csrfTokenName, csrfTokenValue, t)
		if finishResp2.StatusCode != http.StatusRequestTimeout {
			t.Errorf("Expected old WebAuthn registration to time out: status %d", finishResp2.StatusCode)
		}
		finishResp3 := httpPostCsrf(baseURL+"/rest/config/gui/webauthn/register-finish", startResp2.finish(&cred2), csrfTokenName, csrfTokenValue, t)
		if finishResp3.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected expired WebAuthn registration to have been deleted: status %d", finishResp3.StatusCode)
		}
	})

	t.Run("WebAuthn registration fails with wrong challenge", func(t *testing.T) {
		t.Parallel()
		baseURL, csrfTokenName, csrfTokenValue, _, getCreateOptions, _ := startServer(t, nil)
		startResp := getCreateOptions(t)

		cryptoRand.Reader.Read(startResp.Options.Response.Challenge)

		cred := createWebauthnRegistrationResponse(startResp.Options, []byte{1, 2, 3, 4}, publicKeyCose, "https://localhost:8384", 0, nil, t)
		finishResp := httpPostCsrf(baseURL+"/rest/config/gui/webauthn/register-finish", startResp.finish(&cred), csrfTokenName, csrfTokenValue, t)
		if finishResp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected failure to register WebAuthn credential with wrong challenge; status: %d", finishResp.StatusCode)
		}
	})

	t.Run("WebAuthn registration fails with wrong origin", func(t *testing.T) {
		t.Parallel()
		baseURL, csrfTokenName, csrfTokenValue, _, getCreateOptions, _ := startServer(t, nil)
		startResp := getCreateOptions(t)

		cred := createWebauthnRegistrationResponse(startResp.Options, []byte{1, 2, 3, 4}, publicKeyCose, "https://localhost", 0, nil, t)

		finishResp := httpPostCsrf(baseURL+"/rest/config/gui/webauthn/register-finish", startResp.finish(&cred), csrfTokenName, csrfTokenValue, t)
		if finishResp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected failure to register WebAuthn credential with wrong origin; status: %d", finishResp.StatusCode)
		}
	})

	t.Run("WebAuthn registration fails without user presence flag set", func(t *testing.T) {
		t.Parallel()
		baseURL, csrfTokenName, csrfTokenValue, _, getCreateOptions, _ := startServer(t, nil)
		startResp := getCreateOptions(t)
		cred := createWebauthnRegistrationResponse(startResp.Options, []byte{1, 2, 3, 4}, publicKeyCose, "https://localhost:8384", 0, nil, t)

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

		finishResp := httpPostCsrf(baseURL+"/rest/config/gui/webauthn/register-finish", startResp.finish(&cred), csrfTokenName, csrfTokenValue, t)
		if finishResp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected failure to register WebAuthn credential without user presence flag set; status: %d", finishResp.StatusCode)
		}
	})

	t.Run("WebAuthn registration fails with malformed public key", func(t *testing.T) {
		t.Parallel()
		baseURL, csrfTokenName, csrfTokenValue, _, getCreateOptions, _ := startServer(t, nil)
		startResp := getCreateOptions(t)
		corruptPublicKeyCose := bytes.Clone(publicKeyCose)
		corruptPublicKeyCose[7] ^= 0xff
		cred := createWebauthnRegistrationResponse(startResp.Options, []byte{1, 2, 3, 4}, corruptPublicKeyCose, "https://localhost:8384", 0, nil, t)

		finishResp := httpPostCsrf(baseURL+"/rest/config/gui/webauthn/register-finish", startResp.finish(&cred), csrfTokenName, csrfTokenValue, t)
		if finishResp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected failure to register WebAuthn credential with malformed public key; status: %d", finishResp.StatusCode)
		}
	})

	t.Run("WebAuthn registration fails with credential ID duplicated in config", func(t *testing.T) {
		t.Parallel()
		baseURL, csrfTokenName, csrfTokenValue, _, getCreateOptions, _ := startServer(t,
			[]config.WebauthnCredential{
				{
					ID:            base64.RawURLEncoding.EncodeToString([]byte{1, 2, 3, 4}),
					RpId:          "localhost",
					PublicKeyCose: base64.RawURLEncoding.EncodeToString(publicKeyCose),
				},
			},
		)
		startResp := getCreateOptions(t)
		cred := createWebauthnRegistrationResponse(startResp.Options, []byte{1, 2, 3, 4}, publicKeyCose, "https://localhost:8384", 0, nil, t)
		finishResp := httpPostCsrfAuth(baseURL+"/rest/config/gui/webauthn/register-finish", startResp.finish(&cred), testAPIKey, csrfTokenName, csrfTokenValue, t)
		if finishResp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected failure to register WebAuthn credential with duplicate credential ID; status: %d", finishResp.StatusCode)
		}
	})

	t.Run("WebAuthn registration fails with credential ID duplicated in pending credentials", func(t *testing.T) {
		t.Parallel()
		baseURL, csrfTokenName, csrfTokenValue, _, getCreateOptions, _ := startServer(t, nil)
		startResp := getCreateOptions(t)
		cred := createWebauthnRegistrationResponse(startResp.Options, []byte{1, 2, 3, 4}, publicKeyCose, "https://localhost:8384", 0, nil, t)
		finishResp := httpPostCsrf(baseURL+"/rest/config/gui/webauthn/register-finish", startResp.finish(&cred), csrfTokenName, csrfTokenValue, t)
		if finishResp.StatusCode != http.StatusOK {
			t.Fatalf("Expected WebAuthn credential registration to succeed; status: %d", finishResp.StatusCode)
		}

		startResp2 := getCreateOptions(t)
		cred2 := createWebauthnRegistrationResponse(startResp2.Options, []byte{1, 2, 3, 4}, publicKeyCose, "https://localhost:8384", 0, nil, t)
		finishResp2 := httpPostCsrf(baseURL+"/rest/config/gui/webauthn/register-finish", startResp2.finish(&cred2), csrfTokenName, csrfTokenValue, t)

		if finishResp2.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected failure to register WebAuthn credential with duplicate credential ID; status: %d", finishResp2.StatusCode)
		}
	})

	t.Run("WebAuthn registration can only be attempted once per challenge", func(t *testing.T) {
		t.Parallel()
		baseURL, csrfTokenName, csrfTokenValue, _, getCreateOptions, _ := startServer(t, nil)
		startResp := getCreateOptions(t)
		cred := createWebauthnRegistrationResponse(startResp.Options, []byte{1, 2, 3, 4}, publicKeyCose, "https://localhost", 0, nil, t)
		finishResp := httpPostCsrf(baseURL+"/rest/config/gui/webauthn/register-finish", startResp.finish(&cred), csrfTokenName, csrfTokenValue, t)
		if finishResp.StatusCode != http.StatusBadRequest {
			t.Fatalf("Expected WebAuthn credential registration to fail; status: %d", finishResp.StatusCode)
		}

		cred2 := createWebauthnRegistrationResponse(startResp.Options, []byte{5, 6, 7, 8}, publicKeyCose, "https://localhost:8384", 0, nil, t)
		finishResp2 := httpPostCsrf(baseURL+"/rest/config/gui/webauthn/register-finish", startResp.finish(&cred2), csrfTokenName, csrfTokenValue, t)

		if finishResp2.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected WebAuthn credential registration to fail with reused challenge; status: %d", finishResp2.StatusCode)
		}
	})
}

func TestWebauthnAuthentication(t *testing.T) {
	t.Parallel()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), cryptoRand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicKeyCose, err := encodeCosePublicKey((privateKey.Public()).(*ecdsa.PublicKey))
	if err != nil {
		t.Fatal(err)
	}

	startServer := func(t *testing.T, rpId string, origins []string, credentials []config.WebauthnCredential) (func(string, string, string, string) *http.Response, func(string, any) *http.Response, func() startWebauthnAuthenticationResponse, *webauthnService) {
		t.Helper()
		cfg := newMockedConfig()
		cfg.GUIReturns(withTestDefaults(config.GUIConfiguration{
			User:                "user",
			RawAddress:          "localhost:0",
			APIKey:              testAPIKey,
			WebauthnRpId:        rpId,
			WebauthnOrigins:     origins,
			WebauthnCredentials: credentials,
		}))
		baseURL, cancel, webauthnService, err := startHTTP(cfg)
		if err != nil {
			t.Fatal(err, "Failed to start HTTP server")
		}
		t.Cleanup(cancel)

		httpRequest := func(method string, url string, body any, xapikeyHeader, csrfTokenName, csrfTokenValue string) *http.Response {
			t.Helper()
			var bodyReader io.Reader = nil
			if body != nil {
				bodyBytes, err := json.Marshal(body)
				if err != nil {
					t.Fatal(err, "Failed to marshal HTTP request body")
				}
				bodyReader = bytes.NewReader(bodyBytes)
			}

			req, err := http.NewRequest(method, baseURL+url, bodyReader)
			if err != nil {
				t.Fatal(err, "Failed to construct HttpRequest")
			}

			if csrfTokenName != "" && csrfTokenValue != "" {
				req.Header.Set("X-"+csrfTokenName, csrfTokenValue)
			}

			if xapikeyHeader != "" {
				req.Header.Set("X-API-Key", xapikeyHeader)
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
			if err != nil {
				t.Fatal(err, "Failed to execute HTTP request")
			}

			return resp
		}

		httpGet := func(url string, xapikeyHeader, csrfTokenName, csrfTokenValue string) *http.Response {
			t.Helper()
			return httpRequest(http.MethodGet, url, nil, xapikeyHeader, csrfTokenName, csrfTokenValue)
		}

		httpPost := func(url string, body any) *http.Response {
			t.Helper()
			return httpRequest(http.MethodPost, url, body, "", "", "")
		}

		getAssertionOptions := func() startWebauthnAuthenticationResponse {
			t.Helper()
			startResp := httpPost("/rest/noauth/auth/webauthn-start", nil)
			if startResp.StatusCode != http.StatusOK {
				t.Fatalf("Failed to start WebAuthn authentication: status %d", startResp.StatusCode)
			}
			if hasSessionCookie(startResp.Cookies()) {
				t.Errorf("Expected no session cookie when starting WebAuthn authentication")
			}

			var respBody startWebauthnAuthenticationResponse
			err := unmarshalTo(startResp.Body, &respBody)
			if err != nil {
				t.Fatal(err, "Failed to unmarshal CredentialAssertion")
			}

			return respBody
		}

		return httpGet, httpPost, getAssertionOptions, webauthnService
	}

	t.Run("A credential that doesn't require UV", func(t *testing.T) {
		t.Parallel()
		credentials := []config.WebauthnCredential{
			{
				ID:            base64.RawURLEncoding.EncodeToString([]byte{1, 2, 3, 4}),
				RpId:          "localhost",
				PublicKeyCose: base64.RawURLEncoding.EncodeToString(publicKeyCose),
				RequireUv:     false,
			},
		}

		t.Run("can authenticate without UV", func(t *testing.T) {
			t.Parallel()
			httpGet, httpPost, getAssertionOptions, _ := startServer(t, "", nil, credentials)
			startResp := getAssertionOptions()

			cred := createWebauthnAssertionResponse(startResp.Options, []byte{1, 2, 3, 4}, privateKey, "https://localhost:8384", false, 42, t)

			finishResp := httpPost("/rest/noauth/auth/webauthn-finish", startResp.finish(&cred, false))
			if finishResp.StatusCode != http.StatusNoContent {
				t.Fatalf("Failed WebAuthn authentication: status %d", finishResp.StatusCode)
			}

			var csrfTokenName, csrfTokenValue string
			for _, cookie := range finishResp.Cookies() {
				if strings.HasPrefix(cookie.Name, "CSRF-Token") {
					csrfTokenName = cookie.Name
					csrfTokenValue = cookie.Value
					break
				}
			}

			var volState WebauthnVolatileState
			getVolStateResp := httpGet("/rest/webauthn/state", testAPIKey, csrfTokenName, csrfTokenValue)
			err := unmarshalTo(getVolStateResp.Body, &volState)
			if err != nil {
				t.Fatal(err)
			}
			credVolState, ok := volState.Credentials[cred.ID]
			if !ok {
				t.Fatalf("Failed to get credential state")
			}
			if !(time.Since(credVolState.LastUseTime) < 10*time.Second) {
				t.Errorf("Wrong LastUseTime after authentication success")
			}
			if credVolState.SignCount != 42 {
				t.Errorf("Wrong SignCount after authentication success")
			}
		})

		t.Run("can authenticate without UV even if a different credential requires UV", func(t *testing.T) {
			t.Parallel()
			_, httpPost, getAssertionOptions, _ := startServer(t, "", nil, []config.WebauthnCredential{
				credentials[0],
				{
					ID:            base64.RawURLEncoding.EncodeToString([]byte{5, 6, 7, 8}),
					RpId:          "localhost",
					PublicKeyCose: base64.RawURLEncoding.EncodeToString(publicKeyCose),
					RequireUv:     true,
				},
			})
			startResp := getAssertionOptions()

			cred := createWebauthnAssertionResponse(startResp.Options, []byte{1, 2, 3, 4}, privateKey, "https://localhost:8384", false, 1, t)

			finishResp := httpPost("/rest/noauth/auth/webauthn-finish", startResp.finish(&cred, false))
			if finishResp.StatusCode != http.StatusNoContent {
				t.Errorf("Failed WebAuthn authentication: status %d", finishResp.StatusCode)
			}
		})

		t.Run("can authenticate with UV", func(t *testing.T) {
			t.Parallel()
			_, httpPost, getAssertionOptions, _ := startServer(t, "", nil, credentials)
			startResp := getAssertionOptions()

			cred := createWebauthnAssertionResponse(startResp.Options, []byte{1, 2, 3, 4}, privateKey, "https://localhost:8384", true, 1, t)

			finishResp := httpPost("/rest/noauth/auth/webauthn-finish", startResp.finish(&cred, false))
			if finishResp.StatusCode != http.StatusNoContent {
				t.Errorf("Failed WebAuthn authentication: status %d", finishResp.StatusCode)
			}
		})
	})

	t.Run("A credential that requires UV", func(t *testing.T) {
		t.Parallel()
		credentials := []config.WebauthnCredential{
			{
				ID:            base64.RawURLEncoding.EncodeToString([]byte{1, 2, 3, 4}),
				RpId:          "localhost",
				PublicKeyCose: base64.RawURLEncoding.EncodeToString(publicKeyCose),
				RequireUv:     true,
			},
		}

		t.Run("cannot authenticate without UV", func(t *testing.T) {
			t.Parallel()
			_, httpPost, getAssertionOptions, _ := startServer(t, "", nil, credentials)
			startResp := getAssertionOptions()

			cred := createWebauthnAssertionResponse(startResp.Options, []byte{1, 2, 3, 4}, privateKey, "https://localhost:8384", false, 1, t)

			finishResp := httpPost("/rest/noauth/auth/webauthn-finish", startResp.finish(&cred, false))
			if finishResp.StatusCode != http.StatusConflict {
				t.Errorf("Expected WebAuthn authentication to fail without UV: status %d", finishResp.StatusCode)
			}
		})

		t.Run("cannot authenticate without UV even if a different credential does not require UV", func(t *testing.T) {
			t.Parallel()
			_, httpPost, getAssertionOptions, _ := startServer(t, "", nil, []config.WebauthnCredential{
				credentials[0],
				{
					ID:            base64.RawURLEncoding.EncodeToString([]byte{5, 6, 7, 8}),
					RpId:          "localhost",
					PublicKeyCose: base64.RawURLEncoding.EncodeToString(publicKeyCose),
					RequireUv:     false,
				},
			})
			startResp := getAssertionOptions()

			cred := createWebauthnAssertionResponse(startResp.Options, []byte{1, 2, 3, 4}, privateKey, "https://localhost:8384", false, 1, t)

			finishResp := httpPost("/rest/noauth/auth/webauthn-finish", startResp.finish(&cred, false))
			if finishResp.StatusCode != http.StatusConflict {
				t.Errorf("Expected WebAuthn authentication to fail without UV: status %d", finishResp.StatusCode)
			}
		})

		t.Run("can authenticate with UV", func(t *testing.T) {
			t.Parallel()
			_, httpPost, getAssertionOptions, _ := startServer(t, "", nil, credentials)
			startResp := getAssertionOptions()

			cred := createWebauthnAssertionResponse(startResp.Options, []byte{1, 2, 3, 4}, privateKey, "https://localhost:8384", true, 1, t)

			finishResp := httpPost("/rest/noauth/auth/webauthn-finish", startResp.finish(&cred, false))
			if finishResp.StatusCode != http.StatusNoContent {
				t.Errorf("Failed WebAuthn authentication: status %d", finishResp.StatusCode)
			}
		})
	})

	t.Run("With non-default RP ID and origin", func(t *testing.T) {
		t.Parallel()
		credentials := []config.WebauthnCredential{
			{
				ID:            base64.RawURLEncoding.EncodeToString([]byte{5, 6, 7, 8}),
				RpId:          "custom-host",
				PublicKeyCose: base64.RawURLEncoding.EncodeToString(publicKeyCose),
			},
		}

		t.Run("Can use a credential with matching RP ID", func(t *testing.T) {
			t.Parallel()
			_, httpPost, getAssertionOptions, _ := startServer(t, "custom-host", []string{"https://origin-other-than-rp-id"}, credentials)
			startResp := getAssertionOptions()

			cred := createWebauthnAssertionResponse(startResp.Options, []byte{5, 6, 7, 8}, privateKey, "https://origin-other-than-rp-id", false, 1, t)

			finishResp := httpPost("/rest/noauth/auth/webauthn-finish", startResp.finish(&cred, false))
			if finishResp.StatusCode != http.StatusNoContent {
				t.Errorf("Failed WebAuthn authentication: status %d", finishResp.StatusCode)
			}
		})

		t.Run("Cannot use a credential with non-matching RP ID", func(t *testing.T) {
			t.Parallel()
			_, httpPost, getAssertionOptions, _ := startServer(t, "custom-host", []string{"https://origin-other-than-rp-id"}, credentials)
			startResp := getAssertionOptions()
			startResp.Options.Response.RelyingPartyID = "localhost"

			cred := createWebauthnAssertionResponse(startResp.Options, []byte{5, 6, 7, 8}, privateKey, "https://origin-other-than-rp-id", false, 1, t)

			finishResp := httpPost("/rest/noauth/auth/webauthn-finish", startResp.finish(&cred, false))
			if finishResp.StatusCode != http.StatusForbidden {
				t.Errorf("Expected to fail WebAuthn authentication: status %d", finishResp.StatusCode)
			}
		})

		t.Run("Cannot use a credential with matching RP ID on the wrong origin", func(t *testing.T) {
			t.Parallel()
			_, httpPost, getAssertionOptions, _ := startServer(t, "custom-host", []string{"https://origin-other-than-rp-id"}, credentials)
			startResp := getAssertionOptions()

			cred := createWebauthnAssertionResponse(startResp.Options, []byte{5, 6, 7, 8}, privateKey, "https://localhost:8384", false, 1, t)

			finishResp := httpPost("/rest/noauth/auth/webauthn-finish", startResp.finish(&cred, false))
			if finishResp.StatusCode != http.StatusForbidden {
				t.Errorf("Expected to fail WebAuthn authentication: status %d", finishResp.StatusCode)
			}
		})
	})

	t.Run("Authentication fails", func(t *testing.T) {
		t.Parallel()
		credentials := []config.WebauthnCredential{
			{
				ID:            base64.RawURLEncoding.EncodeToString([]byte{1, 2, 3, 4}),
				RpId:          "localhost",
				PublicKeyCose: base64.RawURLEncoding.EncodeToString(publicKeyCose),
				RequireUv:     false,
			},
		}

		t.Run("with wrong challenge", func(t *testing.T) {
			t.Parallel()
			_, httpPost, getAssertionOptions, _ := startServer(t, "", nil, credentials)
			startResp := getAssertionOptions()

			cryptoRand.Reader.Read(startResp.Options.Response.Challenge)

			cred := createWebauthnAssertionResponse(startResp.Options, []byte{1, 2, 3, 4}, privateKey, "https://localhost:8384", false, 18, t)
			finishResp := httpPost("/rest/noauth/auth/webauthn-finish", startResp.finish(&cred, false))
			if finishResp.StatusCode != http.StatusForbidden {
				t.Errorf("Expected status 403, was: %v", finishResp.StatusCode)
			}
		})

		t.Run("with wrong RP ID", func(t *testing.T) {
			t.Parallel()
			_, httpPost, getAssertionOptions, _ := startServer(t, "localhost", nil, append(credentials,
				config.WebauthnCredential{
					ID:            base64.RawURLEncoding.EncodeToString([]byte{5, 6, 7, 8}),
					RpId:          "localhost",
					PublicKeyCose: base64.RawURLEncoding.EncodeToString(publicKeyCose),
					RequireUv:     false,
				}))
			startResp := getAssertionOptions()
			startResp.Options.Response.RelyingPartyID = "not-localhost"

			cred := createWebauthnAssertionResponse(startResp.Options, []byte{1, 2, 3, 4}, privateKey, "https://localhost:8384", false, 18, t)

			finishResp := httpPost("/rest/noauth/auth/webauthn-finish", startResp.finish(&cred, false))
			if finishResp.StatusCode != http.StatusForbidden {
				t.Errorf("Expected status 403, was: %v", finishResp.StatusCode)
			}
		})

		t.Run("with wrong origin", func(t *testing.T) {
			t.Parallel()
			_, httpPost, getAssertionOptions, _ := startServer(t, "", []string{"https://localhost:8384"}, credentials)
			startResp := getAssertionOptions()

			cred := createWebauthnAssertionResponse(startResp.Options, []byte{1, 2, 3, 4}, privateKey, "https://localhost", false, 18, t)

			finishResp := httpPost("/rest/noauth/auth/webauthn-finish", startResp.finish(&cred, false))
			if finishResp.StatusCode != http.StatusForbidden {
				t.Errorf("Expected status 403, was: %v", finishResp.StatusCode)
			}
		})

		t.Run("without user presence flag set", func(t *testing.T) {
			t.Parallel()
			_, httpPost, getAssertionOptions, _ := startServer(t, "", nil, credentials)
			startResp := getAssertionOptions()
			cred := createWebauthnAssertionResponse(startResp.Options, []byte{1, 2, 3, 4}, privateKey, "https://localhost:8384", false, 18, t)

			cred.AssertionResponse.AuthenticatorData[32] &= ^byte(webauthnProtocol.FlagUserPresent)

			finishResp := httpPost("/rest/noauth/auth/webauthn-finish", startResp.finish(&cred, false))
			if finishResp.StatusCode != http.StatusForbidden {
				t.Errorf("Expected status 403, was: %v", finishResp.StatusCode)
			}
		})

		t.Run("with signature by wrong private key", func(t *testing.T) {
			t.Parallel()
			_, httpPost, getAssertionOptions, _ := startServer(t, "", nil, credentials)
			startResp := getAssertionOptions()

			wrongPrivateKey, err := ecdsa.GenerateKey(elliptic.P256(), cryptoRand.Reader)
			if err != nil {
				t.Fatal(err)
			}

			cred := createWebauthnAssertionResponse(startResp.Options, []byte{1, 2, 3, 4}, wrongPrivateKey, "https://localhost:8384", false, 18, t)
			finishResp := httpPost("/rest/noauth/auth/webauthn-finish", startResp.finish(&cred, false))
			if finishResp.StatusCode != http.StatusForbidden {
				t.Errorf("Expected status 403, was: %v", finishResp.StatusCode)
			}
		})

		t.Run("with invalid signature", func(t *testing.T) {
			t.Parallel()
			_, httpPost, getAssertionOptions, _ := startServer(t, "", nil, credentials)
			startResp := getAssertionOptions()

			cred := createWebauthnAssertionResponse(startResp.Options, []byte{1, 2, 3, 4}, privateKey, "https://localhost:8384", false, 18, t)
			cred.AssertionResponse.Signature[17] ^= 0xff

			finishResp := httpPost("/rest/noauth/auth/webauthn-finish", startResp.finish(&cred, false))
			if finishResp.StatusCode != http.StatusForbidden {
				t.Errorf("Expected status 403, was: %v", finishResp.StatusCode)
			}
		})

		t.Run("with wrong credential ID", func(t *testing.T) {
			t.Parallel()
			_, httpPost, getAssertionOptions, _ := startServer(t, "", nil, credentials)
			startResp := getAssertionOptions()

			cred := createWebauthnAssertionResponse(startResp.Options, []byte{5, 6, 7, 8}, privateKey, "https://localhost:8384", false, 18, t)
			finishResp := httpPost("/rest/noauth/auth/webauthn-finish", startResp.finish(&cred, false))
			if finishResp.StatusCode != http.StatusForbidden {
				t.Errorf("Expected status 403, was: %v", finishResp.StatusCode)
			}
		})
	})

	t.Run("Authentication can only be attempted once per challenge", func(t *testing.T) {
		t.Parallel()
		credentials := []config.WebauthnCredential{
			{
				ID:            base64.RawURLEncoding.EncodeToString([]byte{1, 2, 3, 4}),
				RpId:          "localhost",
				PublicKeyCose: base64.RawURLEncoding.EncodeToString(publicKeyCose),
				RequireUv:     false,
			},
		}
		_, httpPost, getAssertionOptions, _ := startServer(t, "", nil, credentials)
		startResp := getAssertionOptions()

		cred := createWebauthnAssertionResponse(startResp.Options, []byte{5, 6, 7, 8}, privateKey, "https://localhost:8384", false, 18, t)
		finishResp := httpPost("/rest/noauth/auth/webauthn-finish", startResp.finish(&cred, false))
		if finishResp.StatusCode != http.StatusForbidden {
			t.Fatalf("Expected status 403, was: %v", finishResp.StatusCode)
		}

		cred2 := createWebauthnAssertionResponse(startResp.Options, []byte{1, 2, 3, 4}, privateKey, "https://localhost:8384", false, 18, t)
		finishResp2 := httpPost("/rest/noauth/auth/webauthn-finish", startResp.finish(&cred2, false))
		if finishResp2.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status 400, was: %v", finishResp2.StatusCode)
		}
	})

	t.Run("Can authenticate two sessions concurrently", func(t *testing.T) {
		t.Parallel()
		credentials := []config.WebauthnCredential{
			{
				ID:            base64.RawURLEncoding.EncodeToString([]byte{1, 2, 3, 4}),
				RpId:          "localhost",
				PublicKeyCose: base64.RawURLEncoding.EncodeToString(publicKeyCose),
				RequireUv:     false,
			},
			{
				ID:            base64.RawURLEncoding.EncodeToString([]byte{5, 6, 7, 8}),
				RpId:          "localhost",
				PublicKeyCose: base64.RawURLEncoding.EncodeToString(publicKeyCose),
				RequireUv:     false,
			},
		}
		_, httpPost, getAssertionOptions, _ := startServer(t, "", nil, credentials)
		startResp1 := getAssertionOptions()
		startResp2 := getAssertionOptions()

		cred1 := createWebauthnAssertionResponse(startResp1.Options, []byte{1, 2, 3, 4}, privateKey, "https://localhost:8384", false, 1, t)
		cred2 := createWebauthnAssertionResponse(startResp2.Options, []byte{5, 6, 7, 8}, privateKey, "https://localhost:8384", false, 1, t)
		delayedFatal := false

		finishResp1 := httpPost("/rest/noauth/auth/webauthn-finish", startResp1.finish(&cred1, false))
		if finishResp1.StatusCode != http.StatusNoContent {
			t.Errorf("Failed 1st concurrent WebAuthn authentication. Status: %v", finishResp1.StatusCode)
			delayedFatal = true
		}

		finishResp2 := httpPost("/rest/noauth/auth/webauthn-finish", startResp2.finish(&cred2, false))
		if finishResp2.StatusCode != http.StatusNoContent {
			t.Errorf("Failed 2nd concurrent WebAuthn authentication. Status: %v", finishResp2.StatusCode)
			delayedFatal = true
		}
		if delayedFatal {
			t.Fatal("Test failed")
		}

		sessionCookie1, ok := getSessionCookie(finishResp1.Cookies())
		if !ok {
			t.Error("Expected session cookie after 1st WebAuthn authentication success")
			delayedFatal = true
		}
		sessionCookie2, ok := getSessionCookie(finishResp2.Cookies())
		if !ok {
			t.Error("Expected session cookie after 2nd WebAuthn authentication success")
			delayedFatal = true
		}
		if delayedFatal {
			t.Fatal("Test failed")
		}

		if sessionCookie1.Value == sessionCookie2.Value {
			t.Error("Expected concurrent WebAuthn authentications to result in separate sessions")
		}
	})

	t.Run("WebAuthn authentication times out after 10 minutes", func(t *testing.T) {
		t.Parallel()
		credentials := []config.WebauthnCredential{
			{
				ID:            base64.RawURLEncoding.EncodeToString([]byte{1, 2, 3, 4}),
				RpId:          "localhost",
				PublicKeyCose: base64.RawURLEncoding.EncodeToString(publicKeyCose),
				RequireUv:     false,
			},
		}
		_, httpPost, getAssertionOptions, webauthnService := startServer(t, "", nil, credentials)
		startResp1 := getAssertionOptions()
		startResp2 := getAssertionOptions()

		if startResp1.Options.Response.Timeout != 10*60*1000 {
			t.Errorf("Expected PublicKeyCredentialRequestOptions.timeout to be set to 10 minutes, was: %v ms", startResp1.Options.Response.Timeout)
		}

		cred1 := createWebauthnAssertionResponse(startResp1.Options, []byte{1, 2, 3, 4}, privateKey, "https://localhost:8384", false, 1, t)
		cred2 := createWebauthnAssertionResponse(startResp2.Options, []byte{1, 2, 3, 4}, privateKey, "https://localhost:8384", false, 1, t)

		t0 := time.Now()
		webauthnService.timeNow = func() time.Time {
			return t0.Add(time.Minute*10 - time.Second*1)
		}
		finishResp1 := httpPost("/rest/noauth/auth/webauthn-finish", startResp1.finish(&cred1, false))
		if finishResp1.StatusCode != http.StatusNoContent {
			t.Fatalf("WebAuthn authentication failed. Status: %v", finishResp1.StatusCode)
		}

		webauthnService.timeNow = func() time.Time {
			return t0.Add(time.Minute*10 + time.Second*1)
		}
		finishResp2 := httpPost("/rest/noauth/auth/webauthn-finish", startResp2.finish(&cred2, false))
		if finishResp2.StatusCode != http.StatusRequestTimeout {
			t.Errorf("Expected old WebAuthn authentication to time out: status %d", finishResp2.StatusCode)
		}
		finishResp3 := httpPost("/rest/noauth/auth/webauthn-finish", startResp2.finish(&cred2, false))
		if finishResp3.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected expired WebAuthn authentication to have been deleted: status %d", finishResp3.StatusCode)
		}
	})

	t.Run("userVerification is set to", func(t *testing.T) {
		t.Parallel()
		credsWithRequireUv := func(aRequiresUv, bRequiresUv bool) []config.WebauthnCredential {
			return []config.WebauthnCredential{
				{
					ID:        base64.RawURLEncoding.EncodeToString([]byte{1, 2, 3, 4}),
					RpId:      "localhost",
					RequireUv: aRequiresUv,
				},
				{
					ID:        base64.RawURLEncoding.EncodeToString([]byte{5, 6, 7, 8}),
					RpId:      "localhost",
					RequireUv: bRequiresUv,
				},
			}
		}

		t.Run("discouraged if no credential requires UV", func(t *testing.T) {
			t.Parallel()
			_, _, getAssertionOptions, _ := startServer(t, "", nil, credsWithRequireUv(false, false))
			startResp := getAssertionOptions()
			if startResp.Options.Response.UserVerification != "discouraged" {
				t.Errorf("Expected userVerification: discouraged when no credential requires UV")
			}
		})

		t.Run("preferred if some but not all credentials require UV", func(t *testing.T) {
			t.Parallel()
			{
				_, _, getAssertionOptions, _ := startServer(t, "", nil, credsWithRequireUv(true, false))
				startResp := getAssertionOptions()
				if startResp.Options.Response.UserVerification != "preferred" {
					t.Errorf("Expected userVerification: preferred when some but not all credentials require UV")
				}
			}

			{
				_, _, getAssertionOptions, _ := startServer(t, "", nil, credsWithRequireUv(false, true))
				startResp := getAssertionOptions()
				if startResp.Options.Response.UserVerification != "preferred" {
					t.Errorf("Expected userVerification: preferred when some but not all credentials require UV")
				}
			}
		})

		t.Run("required if all credentials require UV", func(t *testing.T) {
			t.Parallel()
			_, _, getAssertionOptions, _ := startServer(t, "", nil, credsWithRequireUv(true, true))
			startResp := getAssertionOptions()
			if startResp.Options.Response.UserVerification != "required" {
				t.Errorf("Expected userVerification: required when all credentials require UV")
			}
		})
	})

	t.Run("Credentials with wrong RP ID are not eligible", func(t *testing.T) {
		t.Parallel()
		_, _, getAssertionOptions, _ := startServer(t, "", nil, []config.WebauthnCredential{
			{
				ID:   "AAAA",
				RpId: "rp-id-is-not-localhost",
			},
			{
				ID:   "BBBB",
				RpId: "localhost",
			},
		})
		startResp := getAssertionOptions()
		if len(startResp.Options.Response.AllowedCredentials) != 1 {
			t.Errorf("Expected only credentials with RpId=%s in allowCredentials, got: %v", "localhost", startResp.Options.Response.AllowedCredentials)
		}
		if startResp.Options.Response.AllowedCredentials[0].CredentialID.String() != "BBBB" {
			t.Errorf("Expected only credentials with RpId=%s in allowCredentials, got: %v", "localhost", startResp.Options.Response.AllowedCredentials)
		}
	})

	t.Run("Auth is required with a WebAuthn credential set", func(t *testing.T) {
		t.Parallel()
		httpGet, _, _, _ := startServer(t, "", nil, []config.WebauthnCredential{
			{
				ID:   "AAAA",
				RpId: "localhost",
			},
		})
		csrfRresp := httpGet("/", "", "", "")
		csrfRresp.Body.Close()
		var csrfTokenName, csrfTokenValue string
		for _, cookie := range csrfRresp.Cookies() {
			if strings.HasPrefix(cookie.Name, "CSRF-Token") {
				csrfTokenName = cookie.Name
				csrfTokenValue = cookie.Value
				break
			}
		}

		resp := httpGet("/rest/config", "", csrfTokenName, csrfTokenValue)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("Expected auth to be required with WebAuthn credential set")
		}
	})

	t.Run("No auth required when no password and no WebAuthn credentials set", func(t *testing.T) {
		t.Parallel()
		httpGet, _, _, _ := startServer(t, "rp-id-irrelevant", []string{"origin-irrelevant"}, []config.WebauthnCredential{})
		csrfRresp := httpGet("/", "", "", "")
		csrfRresp.Body.Close()
		var csrfTokenName, csrfTokenValue string
		for _, cookie := range csrfRresp.Cookies() {
			if strings.HasPrefix(cookie.Name, "CSRF-Token") {
				csrfTokenName = cookie.Name
				csrfTokenValue = cookie.Value
				break
			}
		}

		resp := httpGet("/rest/config", "", csrfTokenName, csrfTokenValue)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected no auth to be required with neither password nor WebAuthn credentials set")
		}
	})
}

func TestPasswordOrWebauthnAuthentication(t *testing.T) {
	t.Parallel()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), cryptoRand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicKeyCose, err := encodeCosePublicKey((privateKey.Public()).(*ecdsa.PublicKey))
	if err != nil {
		t.Fatal(err)
	}

	startServer := func(t *testing.T) (func(string, any) *http.Response, func() startWebauthnAuthenticationResponse) {
		t.Helper()

		password := "$2a$10$IdIZTxTg/dCNuNEGlmLynOjqg4B1FvDKuIV5e0BB3pnWVHNb8.GSq" // bcrypt of "räksmörgås" in UTF-8

		cfg := newMockedConfig()
		cfg.GUIReturns(withTestDefaults(config.GUIConfiguration{
			User:       "user",
			Password:   password,
			RawAddress: "localhost:0",
			WebauthnCredentials: []config.WebauthnCredential{
				{
					ID:            base64.RawURLEncoding.EncodeToString([]byte{1, 2, 3, 4}),
					RpId:          "localhost",
					PublicKeyCose: base64.RawURLEncoding.EncodeToString(publicKeyCose),
					RequireUv:     false,
				},
			},
		}))
		baseURL, cancel, _, err := startHTTP(cfg)
		if err != nil {
			t.Fatal(err, "Failed to start HTTP server")
		}
		t.Cleanup(cancel)

		httpRequest := func(method string, url string, body any) *http.Response {
			t.Helper()
			var bodyReader io.Reader = nil
			if body != nil {
				bodyBytes, err := json.Marshal(body)
				if err != nil {
					t.Fatal(err, "Failed to marshal HTTP request body")
				}
				bodyReader = bytes.NewReader(bodyBytes)
			}

			req, err := http.NewRequest(method, baseURL+url, bodyReader)
			if err != nil {
				t.Fatal(err, "Failed to construct HttpRequest")
			}

			client := http.Client{
				Timeout: 15 * time.Second,
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err, "Failed to execute HTTP request")
			}

			return resp
		}

		httpPost := func(url string, body any) *http.Response {
			t.Helper()
			return httpRequest(http.MethodPost, url, body)
		}

		getAssertionOptions := func() startWebauthnAuthenticationResponse {
			startResp := httpPost("/rest/noauth/auth/webauthn-start", nil)
			if startResp.StatusCode != http.StatusOK {
				t.Fatalf("Failed to start WebAuthn registration: status %d", startResp.StatusCode)
			}
			if hasSessionCookie(startResp.Cookies()) {
				t.Errorf("Expected no session cookie when starting WebAuthn registration")
			}

			var respBody startWebauthnAuthenticationResponse
			err := unmarshalTo(startResp.Body, &respBody)
			if err != nil {
				t.Fatal(err, "Failed to unmarshal CredentialAssertion")
			}

			return respBody
		}

		return httpPost, getAssertionOptions
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
		if finishResp.StatusCode != http.StatusNoContent {
			t.Fatalf("Failed password authentication: status %d", finishResp.StatusCode)
		}
	})

	t.Run("Can log in with WebAuthn instead of password", func(t *testing.T) {
		t.Parallel()
		httpPost, getAssertionOptions := startServer(t)
		startResp := getAssertionOptions()

		cred := createWebauthnAssertionResponse(startResp.Options, []byte{1, 2, 3, 4}, privateKey, "https://localhost:8384", false, 1, t)

		finishResp := httpPost("/rest/noauth/auth/webauthn-finish", startResp.finish(&cred, false))
		if finishResp.StatusCode != http.StatusNoContent {
			t.Fatalf("Failed WebAuthn authentication: status %d", finishResp.StatusCode)
		}
	})
}

func TestWebauthnConfigChanges(t *testing.T) {
	t.Parallel()

	// This test needs a longer-than-default shutdown timeout when running on GitHub Actions
	shutdownTimeout := testutil.IfExpr(os.Getenv("CI") == "true", 1000*time.Millisecond, 0)

	initialWebauthnCredentials := []config.WebauthnCredential{
		{
			ID:            "AAAA",
			RpId:          "localhost",
			Nickname:      "Credential A",
			PublicKeyCose: base64.RawURLEncoding.EncodeToString([]byte{1, 2, 3, 4}),
			Transports:    []string{"transportA"},
			RequireUv:     false,
			CreateTime:    time.Now(),
		},
	}

	initConfig := func(t *testing.T) config.Wrapper {
		const testAPIKey = "foobarbaz"
		cfg := config.Configuration{
			GUI: withTestDefaults((&config.GUIConfiguration{
				RawAddress:          "127.0.0.1:0",
				APIKey:              testAPIKey,
				WebauthnUserId:      []byte{0, 0, 0},
				WebauthnCredentials: initialWebauthnCredentials,
			}).Copy()),
		}

		tmpFile, err := os.CreateTemp("", "syncthing-testConfig-Webauthn-*")
		if err != nil {
			t.Fatal(err, "Failed to create tmpfile for test")
		}
		w := config.Wrap(tmpFile.Name(), cfg, protocol.LocalDeviceID, events.NoopLogger)
		tmpFile.Close()
		cfgCtx, cfgCancel := context.WithCancel(context.Background())
		go w.Serve(cfgCtx)
		t.Cleanup(func() {
			os.Remove(tmpFile.Name())
			cfgCancel()
		})
		return w
	}

	startHttpServer := func(t *testing.T, w config.Wrapper) (func(string) *http.Response, func(string, string, any)) {
		baseURL, cancel, _, err := startHTTPWithShutdownTimeout(w, shutdownTimeout)
		t.Cleanup(cancel)
		if err != nil {
			t.Fatal(err)
		}

		cli := &http.Client{
			Timeout: 60 * time.Second,
		}

		do := func(req *http.Request, status int) *http.Response {
			t.Helper()
			req.Header.Set("X-API-Key", testAPIKey)
			resp, err := cli.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			if status != resp.StatusCode {
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

		return get, mod
	}

	cfgPath := "/rest/config"

	t.Run("Cannot add WebAuthn credential through just config", func(t *testing.T) {
		t.Parallel()
		w := initConfig(t)
		get, mod := startHttpServer(t, w)
		{
			cfg := w.RawCopy()
			cfg.GUI.WebauthnCredentials = append(
				cfg.GUI.WebauthnCredentials,
				config.WebauthnCredential{
					ID:            "BBBB",
					RpId:          "localhost",
					PublicKeyCose: base64.RawURLEncoding.EncodeToString([]byte{}),
					RequireUv:     false,
				},
			)
			mod(http.MethodPut, cfgPath, cfg)
		}
		{
			resp := get(cfgPath)
			var cfg config.Configuration
			err := unmarshalTo(resp.Body, &cfg)
			if err != nil {
				t.Fatal(err)
			}
			if !cmp.Equal(cfg.GUI.WebauthnCredentials, initialWebauthnCredentials) {
				t.Errorf("Expected not to be able to add WebAuthn credentials through just config. Updated credentials: %v", cfg.GUI.WebauthnCredentials)
			}
		}
	})

	t.Run("Editing WebAuthn credential ID results in deleting the existing credential", func(t *testing.T) {
		t.Parallel()
		w := initConfig(t)
		{
			_, mod := startHttpServer(t, w)
			cfg := w.RawCopy()
			cfg.GUI.WebauthnCredentials[0].ID = "ZZZZ"
			mod(http.MethodPut, cfgPath, cfg)
		}
		{
			get, _ := startHttpServer(t, w)
			resp := get(cfgPath)
			var cfg config.Configuration
			err := unmarshalTo(resp.Body, &cfg)
			if err != nil {
				t.Fatal(err)
			}
			if 0 != len(cfg.GUI.WebauthnCredentials) {
				t.Errorf("Expected attempt to edit WebAuthn credential ID to result in deleting the existing credential. Updated credentials: %v", cfg.GUI.WebauthnCredentials)
			}
		}
	})

	testCannotEditCredential := func(propName string, modify func([]config.WebauthnCredential)) {
		t.Run(fmt.Sprintf("Cannot edit WebAuthnCredential.%s", propName), func(t *testing.T) {
			t.Parallel()
			w := initConfig(t)
			get, mod := startHttpServer(t, w)
			{
				cfg := w.RawCopy()
				modify(cfg.GUI.WebauthnCredentials)
				mod(http.MethodPut, cfgPath, cfg)
			}
			{
				resp := get(cfgPath)
				var cfg config.Configuration
				err := unmarshalTo(resp.Body, &cfg)
				if err != nil {
					t.Fatal(err)
				}
				if !cmp.Equal(cfg.GUI.WebauthnCredentials, initialWebauthnCredentials) {
					t.Errorf("Expected to not be able to edit %s of WebAuthn credential. Updated credentials: %v", propName, cfg.GUI.WebauthnCredentials)
				}
			}
		})
	}

	testCannotEditCredential("RpId", func(credentials []config.WebauthnCredential) {
		credentials[0].RpId = "no-longer-locahost"
	})
	testCannotEditCredential("PublicKeyCose", func(credentials []config.WebauthnCredential) {
		credentials[0].PublicKeyCose = "BBBB"
	})
	testCannotEditCredential("Transports", func(credentials []config.WebauthnCredential) {
		credentials[0].Transports = []string{"transportA", "transportC"}
	})
	testCannotEditCredential("CreateTime", func(credentials []config.WebauthnCredential) {
		credentials[0].CreateTime = time.Now().Add(10 * time.Second)
	})

	testCanEditCredential := func(propName string, modify func([]config.WebauthnCredential), verify func([]config.WebauthnCredential) bool) {
		t.Run(fmt.Sprintf("Can edit WebauthnCredential.%s", propName), func(t *testing.T) {
			t.Parallel()
			w := initConfig(t)
			{
				_, mod := startHttpServer(t, w)
				cfg := w.RawCopy()
				modify(cfg.GUI.WebauthnCredentials)
				mod(http.MethodPut, cfgPath, cfg)
			}
			{
				get, _ := startHttpServer(t, w)
				resp := get(cfgPath)
				var cfg config.Configuration
				err := unmarshalTo(resp.Body, &cfg)
				if err != nil {
					t.Fatal(err)
				}
				if !(!cmp.Equal(cfg.GUI.WebauthnCredentials, initialWebauthnCredentials) && verify(cfg.GUI.WebauthnCredentials)) {
					t.Errorf("Expected to be able to edit %s of WebAuthn credential. Updated credentials: %v", propName, cfg.GUI.WebauthnCredentials)
				}
			}
		})
	}

	testCanEditCredential("Nickname", func(credentials []config.WebauthnCredential) {
		credentials[0].Nickname = "Blåbärsmjölk"
	}, func(credentials []config.WebauthnCredential) bool {
		return credentials[0].Nickname == "Blåbärsmjölk"
	})
	testCanEditCredential("RequireUv", func(credentials []config.WebauthnCredential) {
		credentials[0].RequireUv = true
	}, func(credentials []config.WebauthnCredential) bool {
		return credentials[0].RequireUv == true
	})
}
