// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// Package rc provides remote control of a Syncthing process via the REST API.
package rc

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/dialer"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/model"
	"github.com/syncthing/syncthing/lib/protocol"
)

// APIKey is set via the STGUIAPIKEY variable when we launch the binary, to
// ensure that we have API access regardless of authentication settings.
const APIKey = "592A47BC-A7DF-4C2F-89E0-A80B3E5094EE"

type Process struct {
	// Set at initialization
	addr string

	// Set by eventLoop()
	eventMut      sync.Mutex
	id            protocol.DeviceID
	folders       []string
	startComplete chan struct{}
	stopped       chan struct{}
	stopErr       error
	sequence      map[string]map[string]int64 // Folder ID => Device ID => Sequence
	done          map[string]bool             // Folder ID => 100%

	cmd   *exec.Cmd
	logfd *os.File
}

// NewProcess returns a new Process talking to Syncthing at the specified address.
// Example: NewProcess("127.0.0.1:8082")
func NewProcess(addr string) *Process {
	p := &Process{
		addr:          addr,
		sequence:      make(map[string]map[string]int64),
		done:          make(map[string]bool),
		startComplete: make(chan struct{}),
		stopped:       make(chan struct{}),
	}
	return p
}

func (p *Process) ID() protocol.DeviceID {
	return p.id
}

// LogTo creates the specified log file and ensures that stdout and stderr
// from the Start()ed process is redirected there. Must be called before
// Start().
func (p *Process) LogTo(filename string) error {
	if p.cmd != nil {
		panic("logfd cannot be set with an existing cmd")
	}

	if p.logfd != nil {
		p.logfd.Close()
	}

	fd, err := os.Create(filename)
	if err != nil {
		return err
	}
	p.logfd = fd
	return nil
}

// Start runs the specified Syncthing binary with the given arguments.
// Syncthing should be configured to provide an API on the address given to
// NewProcess. Event processing is started.
func (p *Process) Start(bin string, args ...string) error {
	cmd := exec.Command(bin, args...)
	if p.logfd != nil {
		cmd.Stdout = p.logfd
		cmd.Stderr = p.logfd
	}
	cmd.Env = append(os.Environ(), "STNORESTART=1", "STGUIAPIKEY="+APIKey)

	err := cmd.Start()
	if err != nil {
		return err
	}

	p.cmd = cmd
	go p.eventLoop()
	go p.wait()

	return nil
}

func (p *Process) wait() {
	p.cmd.Wait()

	if p.logfd != nil {
		p.stopErr = p.checkForProblems(p.logfd)
	}

	close(p.stopped)
}

// AwaitStartup waits for the Syncthing process to start and perform initial
// scans of all folders.
func (p *Process) AwaitStartup() {
	select {
	case <-p.startComplete:
	case <-p.stopped:
	}
}

// Stop stops the running Syncthing process. If the process was logging to a
// local file (set by LogTo), the log file will be opened and checked for
// panics and data races. The presence of either will be signalled in the form
// of a returned error.
func (p *Process) Stop() (*os.ProcessState, error) {
	select {
	case <-p.stopped:
		return p.cmd.ProcessState, p.stopErr
	default:
	}

	if _, err := p.Post("/rest/system/shutdown", nil); err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		// Unexpected EOF is somewhat expected here, as we may exit before
		// returning something sensible.
		return nil, err
	}

	<-p.stopped

	return p.cmd.ProcessState, p.stopErr
}

// Stopped returns a channel that will be closed when Syncthing has stopped.
func (p *Process) Stopped() chan struct{} {
	return p.stopped
}

// Get performs an HTTP GET and returns the bytes and/or an error. Any non-200
// return code is returned as an error.
func (p *Process) Get(path string) ([]byte, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			DialContext:       dialer.DialContext,
			Proxy:             http.ProxyFromEnvironment,
			DisableKeepAlives: true,
		},
	}

	url := fmt.Sprintf("http://%s%s", p.addr, path)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("X-Api-Key", APIKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	return p.readResponse(resp)
}

// Post performs an HTTP POST and returns the bytes and/or an error. Any
// non-200 return code is returned as an error.
func (p *Process) Post(path string, data io.Reader) ([]byte, error) {
	client := &http.Client{
		Timeout: 600 * time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}
	url := fmt.Sprintf("http://%s%s", p.addr, path)
	req, err := http.NewRequest(http.MethodPost, url, data)
	if err != nil {
		return nil, err
	}

	req.Header.Add("X-Api-Key", APIKey)
	req.Header.Add("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	return p.readResponse(resp)
}

type Event struct {
	ID   int
	Time time.Time
	Type string
	Data interface{}
}

func (p *Process) Events(since int) ([]Event, error) {
	bs, err := p.Get(fmt.Sprintf("/rest/events?since=%d&timeout=10", since))
	if err != nil {
		return nil, err
	}

	var evs []Event
	dec := json.NewDecoder(bytes.NewReader(bs))
	dec.UseNumber()
	err = dec.Decode(&evs)
	if err != nil {
		return nil, fmt.Errorf("events: %w in %q", err, bs)
	}
	return evs, err
}

func (p *Process) Rescan(folder string) error {
	_, err := p.Post("/rest/db/scan?folder="+url.QueryEscape(folder), nil)
	return err
}

func (p *Process) RescanDelay(folder string, delaySeconds int) error {
	_, err := p.Post(fmt.Sprintf("/rest/db/scan?folder=%s&next=%d", url.QueryEscape(folder), delaySeconds), nil)
	return err
}

func (p *Process) RescanSub(folder string, sub string, delaySeconds int) error {
	return p.RescanSubs(folder, []string{sub}, delaySeconds)
}

func (p *Process) RescanSubs(folder string, subs []string, delaySeconds int) error {
	data := url.Values{}
	data.Set("folder", folder)
	for _, sub := range subs {
		data.Add("sub", sub)
	}
	data.Set("next", strconv.Itoa(delaySeconds))
	_, err := p.Post("/rest/db/scan?"+data.Encode(), nil)
	return err
}

func (p *Process) ConfigInSync() (bool, error) {
	bs, err := p.Get("/rest/system/config/insync")
	if err != nil {
		return false, err
	}
	return bytes.Contains(bs, []byte("true")), nil
}

func (p *Process) GetConfig() (config.Configuration, error) {
	var cfg config.Configuration
	bs, err := p.Get("/rest/system/config")
	if err != nil {
		return cfg, err
	}

	err = json.Unmarshal(bs, &cfg)
	return cfg, err
}

func (p *Process) PostConfig(cfg config.Configuration) error {
	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(cfg); err != nil {
		return err
	}
	_, err := p.Post("/rest/system/config", buf)
	return err
}

func (p *Process) PauseDevice(dev protocol.DeviceID) error {
	_, err := p.Post("/rest/system/pause?device="+dev.String(), nil)
	return err
}

func (p *Process) ResumeDevice(dev protocol.DeviceID) error {
	_, err := p.Post("/rest/system/resume?device="+dev.String(), nil)
	return err
}

func (p *Process) PauseAll() error {
	_, err := p.Post("/rest/system/pause", nil)
	return err
}

func (p *Process) ResumeAll() error {
	_, err := p.Post("/rest/system/resume", nil)
	return err
}

func InSync(folder string, ps ...*Process) bool {
	for _, p := range ps {
		p.eventMut.Lock()
	}
	defer func() {
		for _, p := range ps {
			p.eventMut.Unlock()
		}
	}()

	for i := range ps {
		// If our latest FolderSummary didn't report 100%, then we are not done.

		if !ps[i].done[folder] {
			l.Debugf("done = ps[%d].done[%q] = false", i, folder)
			return false
		}

		// Check Sequence for each device. The local version seen by remote
		// devices should be the same as what it has locally, or the index
		// hasn't been sent yet.

		sourceID := ps[i].id.String()
		sourceSeq := ps[i].sequence[folder][sourceID]
		l.Debugf("sourceSeq = ps[%d].sequence[%q][%q] = %d", i, folder, sourceID, sourceSeq)
		for j := range ps {
			if i != j {
				remoteSeq := ps[j].sequence[folder][sourceID]
				if remoteSeq != sourceSeq {
					l.Debugf("remoteSeq = ps[%d].sequence[%q][%q] = %d", j, folder, sourceID, remoteSeq)
					return false
				}
			}
		}
	}

	return true
}

func AwaitSync(folder string, ps ...*Process) {
	for {
		time.Sleep(250 * time.Millisecond)
		if InSync(folder, ps...) {
			return
		}
	}
}

type Model struct {
	GlobalBytes   int
	GlobalDeleted int
	GlobalFiles   int
	InSyncBytes   int
	InSyncFiles   int
	Invalid       string
	LocalBytes    int
	LocalDeleted  int
	LocalFiles    int
	NeedBytes     int
	NeedFiles     int
	State         string
	StateChanged  time.Time
	Version       int
}

func (p *Process) Model(folder string) (Model, error) {
	bs, err := p.Get("/rest/db/status?folder=" + url.QueryEscape(folder))
	if err != nil {
		return Model{}, err
	}

	var res Model
	if err := json.Unmarshal(bs, &res); err != nil {
		return Model{}, err
	}

	l.Debugf("%+v", res)

	return res, nil
}

func (*Process) readResponse(resp *http.Response) ([]byte, error) {
	bs, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return bs, err
	}
	if resp.StatusCode != http.StatusOK {
		return bs, errors.New(resp.Status)
	}
	return bs, nil
}

func (p *Process) checkForProblems(logfd *os.File) error {
	fd, err := os.Open(logfd.Name())
	if err != nil {
		return err
	}
	defer fd.Close()

	raceConditionStart := []byte("WARNING: DATA RACE")
	raceConditionSep := []byte("==================")
	panicConditionStart := []byte("panic:")
	panicConditionSep := []byte("[") // fallback if we don't already know our ID
	if p.id.String() != "" {
		panicConditionSep = []byte(p.id.String()[:5])
	}
	sc := bufio.NewScanner(fd)
	race := false
	_panic := false

	for sc.Scan() {
		line := sc.Bytes()
		if race || _panic {
			if bytes.Contains(line, panicConditionSep) {
				_panic = false
				continue
			}
			fmt.Printf("%s\n", line)
			if bytes.Contains(line, raceConditionSep) {
				race = false
			}
		} else if bytes.Contains(line, raceConditionStart) {
			fmt.Printf("%s\n", raceConditionSep)
			fmt.Printf("%s\n", raceConditionStart)
			race = true
			if err == nil {
				err = errors.New("Race condition detected")
			}
		} else if bytes.Contains(line, panicConditionStart) {
			_panic = true
			if err == nil {
				err = errors.New("Panic detected")
			}
		}
	}

	return err
}

func (p *Process) eventLoop() {
	since := 0
	notScanned := make(map[string]struct{})
	start := time.Now()
	for {
		select {
		case <-p.stopped:
			return
		default:
		}

		evs, err := p.Events(since)
		if err != nil {
			if time.Since(start) < 5*time.Second {
				// The API has probably not started yet, lets give it some time.
				continue
			}

			// If we're stopping, no need to print the error.
			select {
			case <-p.stopped:
				return
			default:
			}

			log.Println("eventLoop: events:", err)
			continue
		}

		for _, ev := range evs {
			if ev.ID != since+1 {
				slog.Warn("Event ID jumped", "from", since, "to", ev.ID)
			}
			since = ev.ID

			switch ev.Type {
			case "Starting":
				// The Starting event tells us where the configuration is. Load
				// it and populate our list of folders.

				data := ev.Data.(map[string]interface{})
				id, err := protocol.DeviceIDFromString(data["myID"].(string))
				if err != nil {
					log.Println("eventLoop: DeviceIdFromString:", err)
					continue
				}
				p.id = id

				home := data["home"].(string)
				w, _, err := config.Load(filepath.Join(home, "config.xml"), protocol.LocalDeviceID, events.NoopLogger)
				if err != nil {
					log.Println("eventLoop: Starting:", err)
					continue
				}
				for id := range w.Folders() {
					p.eventMut.Lock()
					p.folders = append(p.folders, id)
					p.eventMut.Unlock()
					notScanned[id] = struct{}{}
				}

				l.Debugln("Started", p.id)

			case "StateChanged":
				// When a folder changes to idle, we tick it off by removing
				// it from p.notScanned.

				if len(p.folders) == 0 {
					// We haven't parsed the config yet, shouldn't happen
					panic("race, or lost startup event")
				}

				select {
				case <-p.startComplete:
				default:
					data := ev.Data.(map[string]interface{})
					to := data["to"].(string)
					if to == "idle" {
						folder := data["folder"].(string)
						delete(notScanned, folder)
						if len(notScanned) == 0 {
							close(p.startComplete)
						}
					}
				}

			case "LocalIndexUpdated":
				data := ev.Data.(map[string]interface{})
				folder := data["folder"].(string)
				p.eventMut.Lock()
				m := p.updateSequenceLocked(folder, p.id.String(), data["sequence"])
				p.done[folder] = false
				l.Debugf("LocalIndexUpdated %v %v done=false\n\t%+v", p.id, folder, m)
				p.eventMut.Unlock()

			case "RemoteIndexUpdated":
				data := ev.Data.(map[string]interface{})
				device := data["device"].(string)
				folder := data["folder"].(string)
				p.eventMut.Lock()
				m := p.updateSequenceLocked(folder, device, data["sequence"])
				p.done[folder] = false
				l.Debugf("RemoteIndexUpdated %v %v done=false\n\t%+v", p.id, folder, m)
				p.eventMut.Unlock()

			case "FolderSummary":
				data := ev.Data.(map[string]interface{})
				folder := data["folder"].(string)
				summary := data["summary"].(map[string]interface{})
				need, _ := summary["needTotalItems"].(json.Number).Int64()
				done := need == 0
				p.eventMut.Lock()
				m := p.updateSequenceLocked(folder, p.id.String(), summary["sequence"])
				p.done[folder] = done
				l.Debugf("FolderSummary %v %v\n\t%+v\n\t%+v", p.id, folder, p.done, m)
				p.eventMut.Unlock()

			case "FolderCompletion":
				data := ev.Data.(map[string]interface{})
				device := data["device"].(string)
				folder := data["folder"].(string)
				p.eventMut.Lock()
				m := p.updateSequenceLocked(folder, device, data["sequence"])
				l.Debugln("FolderCompletion", p.id, folder, m)
				p.eventMut.Unlock()
			}
		}
	}
}

func (p *Process) updateSequenceLocked(folder, device string, sequenceIntf interface{}) map[string]int64 {
	sequence, _ := sequenceIntf.(json.Number).Int64()
	m := p.sequence[folder]
	if m == nil {
		m = make(map[string]int64)
	}
	m[device] = sequence
	p.sequence[folder] = m
	return m
}

type ConnectionStats struct {
	Address       string
	Type          string
	Connected     bool
	Paused        bool
	ClientVersion string
	InBytesTotal  int64
	OutBytesTotal int64
}

func (p *Process) Connections() (map[string]ConnectionStats, error) {
	bs, err := p.Get("/rest/system/connections")
	if err != nil {
		return nil, err
	}

	var res map[string]ConnectionStats
	if err := json.Unmarshal(bs, &res); err != nil {
		return nil, err
	}

	return res, nil
}

type SystemStatus struct {
	Alloc         int64
	Goroutines    int
	MyID          protocol.DeviceID
	PathSeparator string
	StartTime     time.Time
	Sys           int64
	Themes        []string
	Tilde         string
	Uptime        int
}

func (p *Process) SystemStatus() (SystemStatus, error) {
	bs, err := p.Get("/rest/system/status")
	if err != nil {
		return SystemStatus{}, err
	}

	var res SystemStatus
	if err := json.Unmarshal(bs, &res); err != nil {
		return SystemStatus{}, err
	}

	return res, nil
}

type SystemVersion struct {
	Arch        string
	Codename    string
	LongVersion string
	OS          string
	Version     string
}

func (p *Process) SystemVersion() (SystemVersion, error) {
	bs, err := p.Get("/rest/system/version")
	if err != nil {
		return SystemVersion{}, err
	}

	var res SystemVersion
	if err := json.Unmarshal(bs, &res); err != nil {
		return SystemVersion{}, err
	}

	return res, nil
}

func (p *Process) RemoteInSync(folder string, dev protocol.DeviceID) (bool, error) {
	bs, err := p.Get(fmt.Sprintf("/rest/db/completion?folder=%v&device=%v", url.QueryEscape(folder), dev))
	if err != nil {
		return false, err
	}

	var comp model.FolderCompletion
	if err := json.Unmarshal(bs, &comp); err != nil {
		return false, err
	}

	return comp.NeedItems+comp.NeedDeletes == 0, nil
}
