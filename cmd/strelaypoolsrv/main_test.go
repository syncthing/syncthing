// Copyright © 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

func init() {
	for i := 0; i < 10; i++ {
		u := fmt.Sprintf("permanent%d", i)
		permanentRelays = append(permanentRelays, &relay{URL: u})
	}

	knownRelays = []*relay{
		{URL: "known1"},
		{URL: "known2"},
		{URL: "known3"},
	}

	mut = new(sync.RWMutex)
}

// Regression test: handleGetRequest should not modify permanentRelays.
func TestHandleGetRequest(t *testing.T) {
	needcap := len(permanentRelays) + len(knownRelays)
	if needcap > cap(permanentRelays) {
		t.Fatalf("test setup failed: need cap(permanentRelays) >= %d, have %d",
			needcap, cap(permanentRelays))
	}

	w := httptest.NewRecorder()
	w.Body = new(bytes.Buffer)
	handleGetRequest(w, httptest.NewRequest("GET", "/", nil))

	result := make(map[string][]*relay)
	err := json.NewDecoder(w.Body).Decode(&result)
	if err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	relays := result["relays"]
	expect, actual := len(knownRelays)+len(permanentRelays), len(relays)
	if actual != expect {
		t.Errorf("expected %d relays, got %d", expect, actual)
	}

	// Check for changes in permanentRelays.
	for i, r := range permanentRelays {
		switch {
		case !strings.HasPrefix(r.URL, "permanent"):
			t.Errorf("relay %q among permanent relays", r.URL)
		case r.URL != fmt.Sprintf("permanent%d", i):
			t.Error("order of permanent relays changed")
		}
	}
}

func TestCanonicalizeQueryValues(t *testing.T) {
	// This just demonstrates and validates the uri.Parse/String stuff in
	// regards to query strings.

	in := "http://example.com/?some weird= query^value"
	exp := "http://example.com/?some+weird=+query%5Evalue"

	uri, err := url.Parse(in)
	if err != nil {
		t.Fatal(err)
	}

	str := uri.String()
	if str != in {
		// Just re-encoding the URL doesn't sanitize the query string.
		t.Errorf("expected %q, got %q", in, str)
	}

	uri.RawQuery = uri.Query().Encode()
	str = uri.String()
	if str != exp {
		// The query string is now in correct format.
		t.Errorf("expected %q, got %q", exp, str)
	}
}
