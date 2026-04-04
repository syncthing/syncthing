// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/syncthing/syncthing/cmd/syncthing/internal/guiclient"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/model"
)

// Client is a typed REST API client for the Syncthing daemon.
type Client struct {
	http    *http.Client
	baseURL string
	apiKey  string
}

// NewClientFromConfig discovers credentials from the local config file.
func NewClientFromConfig() (*Client, error) {
	guiCfg, err := guiclient.LoadGUIConfig()
	if err != nil {
		return nil, err
	}
	return newClientFromGUI(guiCfg), nil
}

func newClientFromGUI(guiCfg config.GUIConfiguration) *Client {
	httpClient := guiclient.NewHTTPClient(guiCfg)
	httpClient.Timeout = 70 * time.Second // slightly more than event long-poll timeout

	return &Client{
		http:    httpClient,
		baseURL: guiclient.BaseURL(guiCfg),
		apiKey:  guiCfg.APIKey,
	}
}

// request executes an HTTP request and returns the response body.
func (c *Client) request(method, path string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequest(method, c.baseURL+"rest/"+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Api-Key", c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("invalid endpoint or API call")
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, errors.New("invalid API key")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status: %s\n%s", resp.Status, strings.TrimSpace(string(data)))
	}
	return data, nil
}

func (c *Client) get(path string) ([]byte, error) {
	return c.request(http.MethodGet, path, nil)
}

func (c *Client) post(path string) error {
	_, err := c.request(http.MethodPost, path, nil)
	return err
}

func (c *Client) requestJSON(method, path string, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = c.request(method, path, bytes.NewReader(data))
	return err
}

func (c *Client) delete(path string) error {
	_, err := c.request(http.MethodDelete, path, nil)
	return err
}

func getJSON[T any](c *Client, path string) (T, error) {
	var zero T
	data, err := c.get(path)
	if err != nil {
		return zero, err
	}
	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		return zero, fmt.Errorf("decoding %s: %w", path, err)
	}
	return result, nil
}

// --- Read Endpoints ---

// SystemStatus represents /rest/system/status.
type SystemStatus struct {
	MyID                    string                         `json:"myID"`
	Uptime                  int                            `json:"uptime"`
	Goroutines              int                            `json:"goroutines"`
	Alloc                   int64                          `json:"alloc"`
	Sys                     int64                          `json:"sys"`
	StartTime               string                         `json:"startTime"`
	URVersionMax            int                            `json:"urVersionMax"`
	ConnectionServiceStatus map[string]ConnectionSvcStatus `json:"connectionServiceStatus"`
	DiscoveryMethods        int                            `json:"discoveryMethods"`
	DiscoveryErrors         map[string]string              `json:"discoveryErrors"`
	DiscoveryStatus         map[string]ConnectionSvcStatus `json:"discoveryStatus"`
}

// ConnectionSvcStatus represents a single listener's status.
type ConnectionSvcStatus struct {
	Error        interface{} `json:"error"` // can be string or null
	LANAddresses []string    `json:"lanAddresses"`
	WANAddresses []string    `json:"wanAddresses"`
}

// SystemVersion represents /rest/system/version.
type SystemVersion struct {
	Version     string `json:"version"`
	LongVersion string `json:"longVersion"`
	OS          string `json:"os"`
	Arch        string `json:"arch"`
}

// ConnectionInfo holds per-device connection data.
type ConnectionInfo struct {
	Connected     bool   `json:"connected"`
	Paused        bool   `json:"paused"`
	ClientVersion string `json:"clientVersion"`
	Address       string `json:"address"`
	Type          string `json:"type"`
	IsLocal       bool   `json:"isLocal"`
	InBytesTotal  int64  `json:"inBytesTotal"`
	OutBytesTotal int64  `json:"outBytesTotal"`
}

// ConnectionsResponse represents /rest/system/connections.
type ConnectionsResponse struct {
	Connections map[string]ConnectionInfo `json:"connections"`
}

// SystemError represents a single error entry.
type SystemError struct {
	When    time.Time `json:"when"`
	Message string    `json:"message"`
}

// SystemErrorsResponse represents /rest/system/error.
type SystemErrorsResponse struct {
	Errors []SystemError `json:"errors"`
}

// DeviceStatistics represents per-device statistics.
type DeviceStatistics struct {
	LastSeen                time.Time `json:"lastSeen"`
	LastConnectionDurationS float64   `json:"lastConnectionDurationS"`
}

// FolderStatistics represents per-folder statistics.
type FolderStatistics struct {
	LastFile struct {
		At       time.Time `json:"at"`
		Filename string    `json:"filename"`
		Deleted  bool      `json:"deleted"`
	} `json:"lastFile"`
	LastScan time.Time `json:"lastScan"`
}

// FolderErrorsResponse represents /rest/folder/errors.
type FolderErrorsResponse struct {
	Errors  []FolderError `json:"errors"`
	Folder  string        `json:"folder"`
	Page    int           `json:"page"`
	Perpage int           `json:"perpage"`
}

// FolderError represents a single folder error.
type FolderError struct {
	Error string `json:"error"`
	Path  string `json:"path"`
}

// Event represents a single event from /rest/events.
type Event struct {
	ID       int             `json:"id"`
	GlobalID int             `json:"globalID"`
	Time     time.Time       `json:"time"`
	Type     string          `json:"type"`
	Data     json.RawMessage `json:"data"`
}

// RestartRequiredResponse represents /rest/config/restart-required.
type RestartRequiredResponse struct {
	RequiresRestart bool `json:"requiresRestart"`
}

// PendingDevice represents a pending device from /rest/cluster/pending/devices.
type PendingDevice struct {
	Time    time.Time `json:"time"`
	Name    string    `json:"name"`
	Address string    `json:"address"`
}

// PendingFolder represents a pending folder.
type PendingFolder struct {
	Time             time.Time `json:"time"`
	Label            string    `json:"label"`
	ReceiveEncrypted bool      `json:"receiveEncrypted"`
	RemoteEncrypted  bool      `json:"remoteEncrypted"`
}

// PendingFolderEntry represents a pending folder from /rest/cluster/pending/folders.
type PendingFolderEntry struct {
	OfferedBy map[string]PendingFolderOffer `json:"offeredBy"`
}

// PendingFolderOffer represents a single device's offer for a pending folder.
type PendingFolderOffer struct {
	Time             time.Time `json:"time"`
	Label            string    `json:"label"`
	ReceiveEncrypted bool      `json:"receiveEncrypted"`
	RemoteEncrypted  bool      `json:"remoteEncrypted"`
}

func (c *Client) Ping() error {
	_, err := c.get("system/ping")
	return err
}

func (c *Client) SystemStatusGet() (SystemStatus, error) {
	return getJSON[SystemStatus](c, "system/status")
}

func (c *Client) SystemVersionGet() (SystemVersion, error) {
	return getJSON[SystemVersion](c, "system/version")
}

func (c *Client) SystemConnectionsGet() (ConnectionsResponse, error) {
	return getJSON[ConnectionsResponse](c, "system/connections")
}

func (c *Client) SystemErrorsGet() ([]SystemError, error) {
	resp, err := getJSON[SystemErrorsResponse](c, "system/error")
	if err != nil {
		return nil, err
	}
	return resp.Errors, nil
}

func (c *Client) ConfigGet() (config.Configuration, error) {
	return getJSON[config.Configuration](c, "config")
}

func (c *Client) DBStatusGet(folderID string) (model.FolderSummary, error) {
	return getJSON[model.FolderSummary](c, "db/status?folder="+folderID)
}

func (c *Client) FolderErrorsGet(folderID string) ([]FolderError, error) {
	resp, err := getJSON[FolderErrorsResponse](c, "folder/errors?folder="+folderID)
	if err != nil {
		return nil, err
	}
	return resp.Errors, nil
}

func (c *Client) StatsDeviceGet() (map[string]DeviceStatistics, error) {
	return getJSON[map[string]DeviceStatistics](c, "stats/device")
}

func (c *Client) StatsFolderGet() (map[string]FolderStatistics, error) {
	return getJSON[map[string]FolderStatistics](c, "stats/folder")
}

func (c *Client) PendingDevicesGet() (map[string]PendingDevice, error) {
	return getJSON[map[string]PendingDevice](c, "cluster/pending/devices")
}

func (c *Client) PendingFoldersGet() (map[string]PendingFolderEntry, error) {
	return getJSON[map[string]PendingFolderEntry](c, "cluster/pending/folders?device=")
}

// DiscoveryEntry represents a single discovered device's addresses.
type DiscoveryEntry struct {
	Addresses []string `json:"addresses"`
}

// DiscoveryGet returns discovered devices and their addresses.
func (c *Client) DiscoveryGet() (map[string]DiscoveryEntry, error) {
	return getJSON[map[string]DiscoveryEntry](c, "system/discovery")
}

func (c *Client) RestartRequiredGet() (bool, error) {
	resp, err := getJSON[RestartRequiredResponse](c, "config/restart-required")
	if err != nil {
		return false, err
	}
	return resp.RequiresRestart, nil
}

// EventsGet long-polls for events since the given ID.
func (c *Client) EventsGet(ctx context.Context, since int, timeout int) ([]Event, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	path := fmt.Sprintf("events?since=%d&timeout=%d", since, timeout)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"rest/"+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Api-Key", c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("events: HTTP %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var evts []Event
	if err := json.NewDecoder(resp.Body).Decode(&evts); err != nil {
		return nil, fmt.Errorf("events: %w", err)
	}
	return evts, nil
}

// --- Write Endpoints ---

func (c *Client) FolderAdd(cfg config.FolderConfiguration) error {
	return c.requestJSON(http.MethodPost, "config/folders", cfg)
}

func (c *Client) FolderUpdate(cfg config.FolderConfiguration) error {
	return c.requestJSON(http.MethodPut, "config/folders/"+cfg.ID, cfg)
}

func (c *Client) FolderRemove(id string) error {
	return c.delete("config/folders/" + id)
}

func (c *Client) DeviceAdd(cfg config.DeviceConfiguration) error {
	return c.requestJSON(http.MethodPost, "config/devices", cfg)
}

func (c *Client) DeviceUpdate(cfg config.DeviceConfiguration) error {
	return c.requestJSON(http.MethodPut, "config/devices/"+cfg.DeviceID.String(), cfg)
}

func (c *Client) DeviceRemove(id string) error {
	return c.delete("config/devices/" + id)
}

func (c *Client) ConfigFolderGet(id string) (config.FolderConfiguration, error) {
	return getJSON[config.FolderConfiguration](c, "config/folders/"+id)
}

func (c *Client) ConfigDeviceGet(id string) (config.DeviceConfiguration, error) {
	return getJSON[config.DeviceConfiguration](c, "config/devices/"+id)
}

func (c *Client) PendingDeviceDismiss(deviceID string) error {
	return c.delete("cluster/pending/devices?device=" + deviceID)
}

func (c *Client) PendingFolderDismiss(deviceID, folderID string) error {
	return c.delete("cluster/pending/folders?device=" + deviceID + "&folder=" + folderID)
}

func (c *Client) Pause(deviceID string) error {
	path := "system/pause"
	if deviceID != "" {
		path += "?device=" + deviceID
	}
	return c.post(path)
}

func (c *Client) Resume(deviceID string) error {
	path := "system/resume"
	if deviceID != "" {
		path += "?device=" + deviceID
	}
	return c.post(path)
}

func (c *Client) Scan(folderID string) error {
	return c.post("db/scan?folder=" + folderID)
}

func (c *Client) Override(folderID string) error {
	return c.post("db/override?folder=" + folderID)
}

func (c *Client) Revert(folderID string) error {
	return c.post("db/revert?folder=" + folderID)
}

func (c *Client) Restart() error {
	return c.post("system/restart")
}

func (c *Client) Shutdown() error {
	return c.post("system/shutdown")
}

func (c *Client) ErrorsClear() error {
	return c.post("system/error/clear")
}

// LogEntry represents a single log message from /rest/system/log.
type LogEntry struct {
	When    time.Time `json:"when"`
	Message string    `json:"message"`
	Level   int       `json:"level"`
}

// LogResponse represents the /rest/system/log response.
type LogResponse struct {
	Messages []LogEntry `json:"messages"`
}

// SystemLogGet fetches recent daemon log entries.
func (c *Client) SystemLogGet() ([]LogEntry, error) {
	resp, err := getJSON[LogResponse](c, "system/log")
	if err != nil {
		return nil, err
	}
	return resp.Messages, nil
}

func (c *Client) GUIConfigGet() (config.GUIConfiguration, error) {
	return getJSON[config.GUIConfiguration](c, "config/gui")
}

func (c *Client) GUIConfigPatch(gui config.GUIConfiguration) error {
	return c.requestJSON(http.MethodPatch, "config/gui", gui)
}
