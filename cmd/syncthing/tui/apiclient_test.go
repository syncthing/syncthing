// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package tui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
)

func testClient(t *testing.T, handler http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &Client{
		http:    srv.Client(),
		baseURL: srv.URL + "/",
		apiKey:  "test-api-key",
	}
}

func TestClientPing(t *testing.T) {
	t.Parallel()
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/system/ping" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("X-Api-Key") != "test-api-key" {
			t.Errorf("missing or wrong API key header")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"ping": "pong"})
	}))
	if err := c.Ping(); err != nil {
		t.Fatal(err)
	}
}

func TestClientSystemStatus(t *testing.T) {
	t.Parallel()
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/system/status" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SystemStatus{
			MyID:   "DEVICE-1",
			Uptime: 3600,
			Alloc:  1024 * 1024,
		})
	}))

	status, err := c.SystemStatusGet()
	if err != nil {
		t.Fatal(err)
	}
	if status.MyID != "DEVICE-1" {
		t.Errorf("expected MyID DEVICE-1, got %s", status.MyID)
	}
	if status.Uptime != 3600 {
		t.Errorf("expected Uptime 3600, got %d", status.Uptime)
	}
}

func TestClientSystemVersion(t *testing.T) {
	t.Parallel()
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SystemVersion{
			Version: "v1.29.0",
			OS:      "linux",
			Arch:    "amd64",
		})
	}))

	ver, err := c.SystemVersionGet()
	if err != nil {
		t.Fatal(err)
	}
	if ver.Version != "v1.29.0" {
		t.Errorf("expected v1.29.0, got %s", ver.Version)
	}
}

func TestClientConnections(t *testing.T) {
	t.Parallel()
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ConnectionsResponse{
			Connections: map[string]ConnectionInfo{
				"DEV-1": {Connected: true, ClientVersion: "v1.29.0", InBytesTotal: 1024},
				"DEV-2": {Connected: false, Paused: true},
			},
		})
	}))

	conns, err := c.SystemConnectionsGet()
	if err != nil {
		t.Fatal(err)
	}
	if len(conns.Connections) != 2 {
		t.Errorf("expected 2 connections, got %d", len(conns.Connections))
	}
	if !conns.Connections["DEV-1"].Connected {
		t.Error("expected DEV-1 to be connected")
	}
}

func TestClientUnauthorized(t *testing.T) {
	t.Parallel()
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	}))

	_, err := c.SystemStatusGet()
	if err == nil {
		t.Fatal("expected error for unauthorized")
	}
	if err.Error() != "invalid API key" {
		t.Errorf("expected 'invalid API key', got %q", err.Error())
	}
}

func TestClientNotFound(t *testing.T) {
	t.Parallel()
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := c.SystemStatusGet()
	if err == nil {
		t.Fatal("expected error for not found")
	}
}

func TestClientFolderAdd(t *testing.T) {
	t.Parallel()
	var receivedMethod string
	var receivedPath string
	var receivedBody config.FolderConfiguration

	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedPath = r.URL.Path
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusOK)
	}))

	err := c.FolderAdd(config.FolderConfiguration{
		ID:    "test-folder",
		Label: "Test Folder",
		Path:  "/tmp/test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if receivedMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", receivedMethod)
	}
	if receivedPath != "/rest/config/folders" {
		t.Errorf("expected /rest/config/folders, got %s", receivedPath)
	}
	if receivedBody.ID != "test-folder" {
		t.Errorf("expected folder ID test-folder, got %s", receivedBody.ID)
	}
}

func TestClientDeviceAdd(t *testing.T) {
	t.Parallel()
	var receivedMethod string
	var receivedBody config.DeviceConfiguration

	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusOK)
	}))

	devID, _ := protocol.DeviceIDFromString("AAAAAAA-BBBBBBB-CCCCCCC-DDDDDDD-EEEEEEE-FFFFFFF-GGGGGGG-HHHHHHH")
	err := c.DeviceAdd(config.DeviceConfiguration{
		DeviceID: devID,
		Name:     "Test Device",
	})
	if err != nil {
		t.Fatal(err)
	}
	if receivedMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", receivedMethod)
	}
	if receivedBody.Name != "Test Device" {
		t.Errorf("expected name Test Device, got %s", receivedBody.Name)
	}
}

func TestClientPauseResume(t *testing.T) {
	t.Parallel()
	var lastPath string
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastPath = r.URL.Path + "?" + r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	}))

	if err := c.Pause("DEV-1"); err != nil {
		t.Fatal(err)
	}
	if lastPath != "/rest/system/pause?device=DEV-1" {
		t.Errorf("unexpected path: %s", lastPath)
	}

	if err := c.Resume(""); err != nil {
		t.Fatal(err)
	}
	if lastPath != "/rest/system/resume?" {
		t.Errorf("unexpected path: %s", lastPath)
	}
}

func TestClientFolderRemove(t *testing.T) {
	t.Parallel()
	var receivedMethod string
	var receivedPath string

	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))

	if err := c.FolderRemove("my-folder"); err != nil {
		t.Fatal(err)
	}
	if receivedMethod != http.MethodDelete {
		t.Errorf("expected DELETE, got %s", receivedMethod)
	}
	if receivedPath != "/rest/config/folders/my-folder" {
		t.Errorf("unexpected path: %s", receivedPath)
	}
}
