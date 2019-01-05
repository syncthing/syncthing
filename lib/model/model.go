// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	stdsync "sync"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/connections"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/scanner"
	"github.com/syncthing/syncthing/lib/stats"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/upgrade"
	"github.com/syncthing/syncthing/lib/versioner"
	"github.com/thejerf/suture"
)

var locationLocal *time.Location

func init() {
	var err error
	locationLocal, err = time.LoadLocation("Local")
	if err != nil {
		panic(err.Error())
	}
}

// How many files to send in each Index/IndexUpdate message.
const (
	maxBatchSizeBytes = 250 * 1024 // Aim for making index messages no larger than 250 KiB (uncompressed)
	maxBatchSizeFiles = 1000       // Either way, don't include more files than this
)

type service interface {
	BringToFront(string)
	Override(*db.FileSet, func([]protocol.FileInfo))
	Revert(*db.FileSet, func([]protocol.FileInfo))
	DelayScan(d time.Duration)
	SchedulePull()              // something relevant changed, we should try a pull
	Jobs() ([]string, []string) // In progress, Queued
	Scan(subs []string) error
	Serve()
	Stop()
	CheckHealth() error
	Errors() []FileError
	WatchError() error

	getState() (folderState, time.Time, error)
	setState(state folderState)
	setError(err error)
}

type Availability struct {
	ID            protocol.DeviceID `json:"id"`
	FromTemporary bool              `json:"fromTemporary"`
}

type Model struct {
	*suture.Supervisor

	cfg               *config.Wrapper
	db                *db.Lowlevel
	finder            *db.BlockFinder
	progressEmitter   *ProgressEmitter
	id                protocol.DeviceID
	shortID           protocol.ShortID
	cacheIgnoredFiles bool
	protectedFiles    []string

	clientName    string
	clientVersion string

	fmut               sync.RWMutex                                           // protects the below
	folderCfgs         map[string]config.FolderConfiguration                  // folder -> cfg
	folderFiles        map[string]*db.FileSet                                 // folder -> files
	deviceStatRefs     map[protocol.DeviceID]*stats.DeviceStatisticsReference // deviceID -> statsRef
	folderIgnores      map[string]*ignore.Matcher                             // folder -> matcher object
	folderRunners      map[string]service                                     // folder -> puller or scanner
	folderRunnerTokens map[string][]suture.ServiceToken                       // folder -> tokens for puller or scanner
	folderStatRefs     map[string]*stats.FolderStatisticsReference            // folder -> statsRef
	folderRestartMuts  syncMutexMap                                           // folder -> restart mutex

	pmut                sync.RWMutex // protects the below
	conn                map[protocol.DeviceID]connections.Connection
	connRequestLimiters map[protocol.DeviceID]*byteSemaphore
	closed              map[protocol.DeviceID]chan struct{}
	helloMessages       map[protocol.DeviceID]protocol.HelloResult
	deviceDownloads     map[protocol.DeviceID]*deviceDownloadState
	remotePausedFolders map[protocol.DeviceID][]string // deviceID -> folders

	foldersRunning int32 // for testing only
}

type folderFactory func(*Model, config.FolderConfiguration, versioner.Versioner, fs.Filesystem) service

var (
	folderFactories = make(map[config.FolderType]folderFactory, 0)
)

var (
	errDeviceUnknown     = errors.New("unknown device")
	errDevicePaused      = errors.New("device is paused")
	errDeviceIgnored     = errors.New("device is ignored")
	ErrFolderPaused      = errors.New("folder is paused")
	errFolderNotRunning  = errors.New("folder is not running")
	errFolderMissing     = errors.New("no such folder")
	errNetworkNotAllowed = errors.New("network not allowed")
)

// NewModel creates and starts a new model. The model starts in read-only mode,
// where it sends index information to connected peers and responds to requests
// for file data without altering the local folder in any way.
func NewModel(cfg *config.Wrapper, id protocol.DeviceID, clientName, clientVersion string, ldb *db.Lowlevel, protectedFiles []string) *Model {
	m := &Model{
		Supervisor: suture.New("model", suture.Spec{
			Log: func(line string) {
				l.Debugln(line)
			},
			PassThroughPanics: true,
		}),
		cfg:                 cfg,
		db:                  ldb,
		finder:              db.NewBlockFinder(ldb),
		progressEmitter:     NewProgressEmitter(cfg),
		id:                  id,
		shortID:             id.Short(),
		cacheIgnoredFiles:   cfg.Options().CacheIgnoredFiles,
		protectedFiles:      protectedFiles,
		clientName:          clientName,
		clientVersion:       clientVersion,
		folderCfgs:          make(map[string]config.FolderConfiguration),
		folderFiles:         make(map[string]*db.FileSet),
		deviceStatRefs:      make(map[protocol.DeviceID]*stats.DeviceStatisticsReference),
		folderIgnores:       make(map[string]*ignore.Matcher),
		folderRunners:       make(map[string]service),
		folderRunnerTokens:  make(map[string][]suture.ServiceToken),
		folderStatRefs:      make(map[string]*stats.FolderStatisticsReference),
		conn:                make(map[protocol.DeviceID]connections.Connection),
		connRequestLimiters: make(map[protocol.DeviceID]*byteSemaphore),
		closed:              make(map[protocol.DeviceID]chan struct{}),
		helloMessages:       make(map[protocol.DeviceID]protocol.HelloResult),
		deviceDownloads:     make(map[protocol.DeviceID]*deviceDownloadState),
		remotePausedFolders: make(map[protocol.DeviceID][]string),
		fmut:                sync.NewRWMutex(),
		pmut:                sync.NewRWMutex(),
	}
	if cfg.Options().ProgressUpdateIntervalS > -1 {
		go m.progressEmitter.Serve()
	}
	scanLimiter.setCapacity(cfg.Options().MaxConcurrentScans)
	cfg.Subscribe(m)

	return m
}

// StartDeadlockDetector starts a deadlock detector on the models locks which
// causes panics in case the locks cannot be acquired in the given timeout
// period.
func (m *Model) StartDeadlockDetector(timeout time.Duration) {
	l.Infof("Starting deadlock detector with %v timeout", timeout)
	detector := newDeadlockDetector(timeout)
	detector.Watch("fmut", m.fmut)
	detector.Watch("pmut", m.pmut)
}

// StartFolder constructs the folder service and starts it.
func (m *Model) StartFolder(folder string) {
	m.fmut.Lock()
	m.pmut.Lock()
	folderType := m.startFolderLocked(folder)
	folderCfg := m.folderCfgs[folder]
	m.pmut.Unlock()
	m.fmut.Unlock()

	l.Infof("Ready to synchronize %s (%s)", folderCfg.Description(), folderType)
}

func (m *Model) startFolderLocked(folder string) config.FolderType {
	if err := m.checkFolderRunningLocked(folder); err == errFolderMissing {
		panic("cannot start nonexistent folder " + folder)
	} else if err == nil {
		panic("cannot start already running folder " + folder)
	}

	cfg := m.folderCfgs[folder]

	folderFactory, ok := folderFactories[cfg.Type]
	if !ok {
		panic(fmt.Sprintf("unknown folder type 0x%x", cfg.Type))
	}

	fs := m.folderFiles[folder]

	// Find any devices for which we hold the index in the db, but the folder
	// is not shared, and drop it.
	expected := mapDevices(cfg.DeviceIDs())
	for _, available := range fs.ListDevices() {
		if _, ok := expected[available]; !ok {
			l.Debugln("dropping", folder, "state for", available)
			fs.Drop(available)
		}
	}

	// Close connections to affected devices
	for _, id := range cfg.DeviceIDs() {
		m.closeLocked(id)
	}

	v, ok := fs.Sequence(protocol.LocalDeviceID), true
	indexHasFiles := ok && v > 0
	if !indexHasFiles {
		// It's a blank folder, so this may the first time we're looking at
		// it. Attempt to create and tag with our marker as appropriate. We
		// don't really do anything with errors at this point except warn -
		// if these things don't work, we still want to start the folder and
		// it'll show up as errored later.

		if err := cfg.CreateRoot(); err != nil {
			l.Warnln("Failed to create folder root directory", err)
		} else if err = cfg.CreateMarker(); err != nil {
			l.Warnln("Failed to create folder marker:", err)
		}
	}

	ver := cfg.Versioner()
	if service, ok := ver.(suture.Service); ok {
		// The versioner implements the suture.Service interface, so
		// expects to be run in the background in addition to being called
		// when files are going to be archived.
		token := m.Add(service)
		m.folderRunnerTokens[folder] = append(m.folderRunnerTokens[folder], token)
	}

	ffs := fs.MtimeFS()

	// These are our metadata files, and they should always be hidden.
	ffs.Hide(config.DefaultMarkerName)
	ffs.Hide(".stversions")
	ffs.Hide(".stignore")

	p := folderFactory(m, cfg, ver, ffs)

	m.folderRunners[folder] = p

	m.warnAboutOverwritingProtectedFiles(folder)

	token := m.Add(p)
	m.folderRunnerTokens[folder] = append(m.folderRunnerTokens[folder], token)

	return cfg.Type
}

func (m *Model) warnAboutOverwritingProtectedFiles(folder string) {
	if m.folderCfgs[folder].Type == config.FolderTypeSendOnly {
		return
	}

	// This is a bit of a hack.
	ffs := m.folderCfgs[folder].Filesystem()
	if ffs.Type() != fs.FilesystemTypeBasic {
		return
	}
	folderLocation := ffs.URI()
	ignores := m.folderIgnores[folder]

	var filesAtRisk []string
	for _, protectedFilePath := range m.protectedFiles {
		// check if file is synced in this folder
		if protectedFilePath != folderLocation && !fs.IsParent(protectedFilePath, folderLocation) {
			continue
		}

		// check if file is ignored
		relPath, _ := filepath.Rel(folderLocation, protectedFilePath)
		if ignores.Match(relPath).IsIgnored() {
			continue
		}

		filesAtRisk = append(filesAtRisk, protectedFilePath)
	}

	if len(filesAtRisk) > 0 {
		l.Warnln("Some protected files may be overwritten and cause issues. See https://docs.syncthing.net/users/config.html#syncing-configuration-files for more information. The at risk files are:", strings.Join(filesAtRisk, ", "))
	}
}

func (m *Model) AddFolder(cfg config.FolderConfiguration) {
	if len(cfg.ID) == 0 {
		panic("cannot add empty folder id")
	}

	if len(cfg.Path) == 0 {
		panic("cannot add empty folder path")
	}

	m.fmut.Lock()
	m.addFolderLocked(cfg)
	m.fmut.Unlock()
}

func (m *Model) addFolderLocked(cfg config.FolderConfiguration) {
	m.folderCfgs[cfg.ID] = cfg
	folderFs := cfg.Filesystem()
	m.folderFiles[cfg.ID] = db.NewFileSet(cfg.ID, folderFs, m.db)

	ignores := ignore.New(folderFs, ignore.WithCache(m.cacheIgnoredFiles))
	if err := ignores.Load(".stignore"); err != nil && !fs.IsNotExist(err) {
		l.Warnln("Loading ignores:", err)
	}
	m.folderIgnores[cfg.ID] = ignores
}

func (m *Model) RemoveFolder(cfg config.FolderConfiguration) {
	m.fmut.Lock()
	m.pmut.Lock()
	// Delete syncthing specific files
	cfg.Filesystem().RemoveAll(config.DefaultMarkerName)

	m.tearDownFolderLocked(cfg)
	// Remove it from the database
	db.DropFolder(m.db, cfg.ID)

	m.pmut.Unlock()
	m.fmut.Unlock()
}

func (m *Model) tearDownFolderLocked(cfg config.FolderConfiguration) {
	// Close connections to affected devices
	// Must happen before stopping the folder service to abort ongoing
	// transmissions and thus allow timely service termination.
	for _, dev := range cfg.Devices {
		m.closeLocked(dev.DeviceID)
	}

	// Stop the services running for this folder and wait for them to finish
	// stopping to prevent races on restart.
	tokens := m.folderRunnerTokens[cfg.ID]
	m.pmut.Unlock()
	m.fmut.Unlock()
	for _, id := range tokens {
		m.RemoveAndWait(id, 0)
	}
	m.fmut.Lock()
	m.pmut.Lock()

	// Clean up our config maps
	delete(m.folderCfgs, cfg.ID)
	delete(m.folderFiles, cfg.ID)
	delete(m.folderIgnores, cfg.ID)
	delete(m.folderRunners, cfg.ID)
	delete(m.folderRunnerTokens, cfg.ID)
	delete(m.folderStatRefs, cfg.ID)
}

func (m *Model) RestartFolder(from, to config.FolderConfiguration) {
	if len(to.ID) == 0 {
		panic("bug: cannot restart empty folder ID")
	}
	if to.ID != from.ID {
		panic(fmt.Sprintf("bug: folder restart cannot change ID %q -> %q", from.ID, to.ID))
	}

	// This mutex protects the entirety of the restart operation, preventing
	// there from being more than one folder restart operation in progress
	// at any given time. The usual fmut/pmut stuff doesn't cover this,
	// because those locks are released while we are waiting for the folder
	// to shut down (and must be so because the folder might need them as
	// part of its operations before shutting down).
	restartMut := m.folderRestartMuts.Get(to.ID)
	restartMut.Lock()
	defer restartMut.Unlock()

	m.fmut.Lock()
	m.pmut.Lock()
	defer m.fmut.Unlock()
	defer m.pmut.Unlock()

	m.tearDownFolderLocked(from)
	if to.Paused {
		l.Infoln("Paused folder", to.Description())
	} else {
		m.addFolderLocked(to)
		folderType := m.startFolderLocked(to.ID)
		l.Infoln("Restarted folder", to.Description(), fmt.Sprintf("(%s)", folderType))
	}
}

func (m *Model) UsageReportingStats(version int, preview bool) map[string]interface{} {
	stats := make(map[string]interface{})
	if version >= 3 {
		// Block stats
		blockStatsMut.Lock()
		copyBlockStats := make(map[string]int)
		for k, v := range blockStats {
			copyBlockStats[k] = v
			if !preview {
				blockStats[k] = 0
			}
		}
		blockStatsMut.Unlock()
		stats["blockStats"] = copyBlockStats

		// Transport stats
		m.pmut.Lock()
		transportStats := make(map[string]int)
		for _, conn := range m.conn {
			transportStats[conn.Transport()]++
		}
		m.pmut.Unlock()
		stats["transportStats"] = transportStats

		// Ignore stats
		ignoreStats := map[string]int{
			"lines":           0,
			"inverts":         0,
			"folded":          0,
			"deletable":       0,
			"rooted":          0,
			"includes":        0,
			"escapedIncludes": 0,
			"doubleStars":     0,
			"stars":           0,
		}
		var seenPrefix [3]bool
		for folder := range m.cfg.Folders() {
			lines, _, err := m.GetIgnores(folder)
			if err != nil {
				continue
			}
			ignoreStats["lines"] += len(lines)

			for _, line := range lines {
				// Allow prefixes to be specified in any order, but only once.
				for {
					if strings.HasPrefix(line, "!") && !seenPrefix[0] {
						seenPrefix[0] = true
						line = line[1:]
						ignoreStats["inverts"] += 1
					} else if strings.HasPrefix(line, "(?i)") && !seenPrefix[1] {
						seenPrefix[1] = true
						line = line[4:]
						ignoreStats["folded"] += 1
					} else if strings.HasPrefix(line, "(?d)") && !seenPrefix[2] {
						seenPrefix[2] = true
						line = line[4:]
						ignoreStats["deletable"] += 1
					} else {
						seenPrefix[0] = false
						seenPrefix[1] = false
						seenPrefix[2] = false
						break
					}
				}

				// Noops, remove
				if strings.HasSuffix(line, "**") {
					line = line[:len(line)-2]
				}
				if strings.HasPrefix(line, "**/") {
					line = line[3:]
				}

				if strings.HasPrefix(line, "/") {
					ignoreStats["rooted"] += 1
				} else if strings.HasPrefix(line, "#include ") {
					ignoreStats["includes"] += 1
					if strings.Contains(line, "..") {
						ignoreStats["escapedIncludes"] += 1
					}
				}

				if strings.Contains(line, "**") {
					ignoreStats["doubleStars"] += 1
					// Remove not to trip up star checks.
					strings.Replace(line, "**", "", -1)
				}

				if strings.Contains(line, "*") {
					ignoreStats["stars"] += 1
				}
			}
		}
		stats["ignoreStats"] = ignoreStats
	}
	return stats
}

type ConnectionInfo struct {
	protocol.Statistics
	Connected     bool
	Paused        bool
	Address       string
	ClientVersion string
	Type          string
}

func (info ConnectionInfo) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"at":            info.At,
		"inBytesTotal":  info.InBytesTotal,
		"outBytesTotal": info.OutBytesTotal,
		"connected":     info.Connected,
		"paused":        info.Paused,
		"address":       info.Address,
		"clientVersion": info.ClientVersion,
		"type":          info.Type,
	})
}

// ConnectionStats returns a map with connection statistics for each device.
func (m *Model) ConnectionStats() map[string]interface{} {
	m.fmut.RLock()
	m.pmut.RLock()

	res := make(map[string]interface{})
	devs := m.cfg.Devices()
	conns := make(map[string]ConnectionInfo, len(devs))
	for device, deviceCfg := range devs {
		hello := m.helloMessages[device]
		versionString := hello.ClientVersion
		if hello.ClientName != "syncthing" {
			versionString = hello.ClientName + " " + hello.ClientVersion
		}
		ci := ConnectionInfo{
			ClientVersion: strings.TrimSpace(versionString),
			Paused:        deviceCfg.Paused,
		}
		if conn, ok := m.conn[device]; ok {
			ci.Type = conn.Type()
			ci.Connected = ok
			ci.Statistics = conn.Statistics()
			if addr := conn.RemoteAddr(); addr != nil {
				ci.Address = addr.String()
			}
		}

		conns[device.String()] = ci
	}

	res["connections"] = conns

	m.pmut.RUnlock()
	m.fmut.RUnlock()

	in, out := protocol.TotalInOut()
	res["total"] = ConnectionInfo{
		Statistics: protocol.Statistics{
			At:            time.Now(),
			InBytesTotal:  in,
			OutBytesTotal: out,
		},
	}

	return res
}

// DeviceStatistics returns statistics about each device
func (m *Model) DeviceStatistics() map[string]stats.DeviceStatistics {
	res := make(map[string]stats.DeviceStatistics)
	for id := range m.cfg.Devices() {
		res[id.String()] = m.deviceStatRef(id).GetStatistics()
	}
	return res
}

// FolderStatistics returns statistics about each folder
func (m *Model) FolderStatistics() map[string]stats.FolderStatistics {
	res := make(map[string]stats.FolderStatistics)
	for id := range m.cfg.Folders() {
		res[id] = m.folderStatRef(id).GetStatistics()
	}
	return res
}

type FolderCompletion struct {
	CompletionPct float64
	NeedBytes     int64
	NeedItems     int64
	GlobalBytes   int64
	NeedDeletes   int64
}

// Completion returns the completion status, in percent, for the given device
// and folder.
func (m *Model) Completion(device protocol.DeviceID, folder string) FolderCompletion {
	m.fmut.RLock()
	rf, ok := m.folderFiles[folder]
	m.fmut.RUnlock()
	if !ok {
		return FolderCompletion{} // Folder doesn't exist, so we hardly have any of it
	}

	tot := rf.GlobalSize().Bytes
	if tot == 0 {
		// Folder is empty, so we have all of it
		return FolderCompletion{
			CompletionPct: 100,
		}
	}

	m.pmut.RLock()
	counts := m.deviceDownloads[device].GetBlockCounts(folder)
	m.pmut.RUnlock()

	var need, items, fileNeed, downloaded, deletes int64
	rf.WithNeedTruncated(device, func(f db.FileIntf) bool {
		ft := f.(db.FileInfoTruncated)

		// If the file is deleted, we account it only in the deleted column.
		if ft.Deleted {
			deletes++
			return true
		}

		// This might might be more than it really is, because some blocks can be of a smaller size.
		downloaded = int64(counts[ft.Name] * int(ft.BlockSize()))

		fileNeed = ft.FileSize() - downloaded
		if fileNeed < 0 {
			fileNeed = 0
		}

		need += fileNeed
		items++

		return true
	})

	needRatio := float64(need) / float64(tot)
	completionPct := 100 * (1 - needRatio)

	// If the completion is 100% but there are deletes we need to handle,
	// drop it down a notch. Hack for consumers that look only at the
	// percentage (our own GUI does the same calculation as here on its own
	// and needs the same fixup).
	if need == 0 && deletes > 0 {
		completionPct = 95 // chosen by fair dice roll
	}

	l.Debugf("%v Completion(%s, %q): %f (%d / %d = %f)", m, device, folder, completionPct, need, tot, needRatio)

	return FolderCompletion{
		CompletionPct: completionPct,
		NeedBytes:     need,
		NeedItems:     items,
		GlobalBytes:   tot,
		NeedDeletes:   deletes,
	}
}

func addSizeOfFile(s *db.Counts, f db.FileIntf) {
	switch {
	case f.IsDeleted():
		s.Deleted++
	case f.IsDirectory():
		s.Directories++
	case f.IsSymlink():
		s.Symlinks++
	default:
		s.Files++
	}
	s.Bytes += f.FileSize()
}

// GlobalSize returns the number of files, deleted files and total bytes for all
// files in the global model.
func (m *Model) GlobalSize(folder string) db.Counts {
	m.fmut.RLock()
	defer m.fmut.RUnlock()
	if rf, ok := m.folderFiles[folder]; ok {
		return rf.GlobalSize()
	}
	return db.Counts{}
}

// LocalSize returns the number of files, deleted files and total bytes for all
// files in the local folder.
func (m *Model) LocalSize(folder string) db.Counts {
	m.fmut.RLock()
	defer m.fmut.RUnlock()
	if rf, ok := m.folderFiles[folder]; ok {
		return rf.LocalSize()
	}
	return db.Counts{}
}

// ReceiveOnlyChangedSize returns the number of files, deleted files and
// total bytes for all files that have changed locally in a receieve only
// folder.
func (m *Model) ReceiveOnlyChangedSize(folder string) db.Counts {
	m.fmut.RLock()
	defer m.fmut.RUnlock()
	if rf, ok := m.folderFiles[folder]; ok {
		return rf.ReceiveOnlyChangedSize()
	}
	return db.Counts{}
}

// NeedSize returns the number and total size of currently needed files.
func (m *Model) NeedSize(folder string) db.Counts {
	m.fmut.RLock()
	defer m.fmut.RUnlock()

	var result db.Counts
	if rf, ok := m.folderFiles[folder]; ok {
		cfg := m.folderCfgs[folder]
		rf.WithNeedTruncated(protocol.LocalDeviceID, func(f db.FileIntf) bool {
			if cfg.IgnoreDelete && f.IsDeleted() {
				return true
			}

			addSizeOfFile(&result, f)
			return true
		})
	}
	result.Bytes -= m.progressEmitter.BytesCompleted(folder)
	l.Debugf("%v NeedSize(%q): %v", m, folder, result)
	return result
}

// NeedFolderFiles returns paginated list of currently needed files in
// progress, queued, and to be queued on next puller iteration, as well as the
// total number of files currently needed.
func (m *Model) NeedFolderFiles(folder string, page, perpage int) ([]db.FileInfoTruncated, []db.FileInfoTruncated, []db.FileInfoTruncated) {
	m.fmut.RLock()
	defer m.fmut.RUnlock()

	rf, ok := m.folderFiles[folder]
	if !ok {
		return nil, nil, nil
	}

	var progress, queued, rest []db.FileInfoTruncated
	var seen map[string]struct{}

	skip := (page - 1) * perpage
	get := perpage

	runner, ok := m.folderRunners[folder]
	if ok {
		allProgressNames, allQueuedNames := runner.Jobs()

		var progressNames, queuedNames []string
		progressNames, skip, get = getChunk(allProgressNames, skip, get)
		queuedNames, skip, get = getChunk(allQueuedNames, skip, get)

		progress = make([]db.FileInfoTruncated, len(progressNames))
		queued = make([]db.FileInfoTruncated, len(queuedNames))
		seen = make(map[string]struct{}, len(progressNames)+len(queuedNames))

		for i, name := range progressNames {
			if f, ok := rf.GetGlobalTruncated(name); ok {
				progress[i] = f
				seen[name] = struct{}{}
			}
		}

		for i, name := range queuedNames {
			if f, ok := rf.GetGlobalTruncated(name); ok {
				queued[i] = f
				seen[name] = struct{}{}
			}
		}
	}

	rest = make([]db.FileInfoTruncated, 0, perpage)
	cfg := m.folderCfgs[folder]
	rf.WithNeedTruncated(protocol.LocalDeviceID, func(f db.FileIntf) bool {
		if cfg.IgnoreDelete && f.IsDeleted() {
			return true
		}

		if skip > 0 {
			skip--
			return true
		}
		ft := f.(db.FileInfoTruncated)
		if _, ok := seen[ft.Name]; !ok {
			rest = append(rest, ft)
			get--
		}
		return get > 0
	})

	return progress, queued, rest
}

// LocalChangedFiles returns a paginated list of currently needed files in
// progress, queued, and to be queued on next puller iteration, as well as the
// total number of files currently needed.
func (m *Model) LocalChangedFiles(folder string, page, perpage int) []db.FileInfoTruncated {
	m.fmut.RLock()
	defer m.fmut.RUnlock()

	rf, ok := m.folderFiles[folder]
	if !ok {
		return nil
	}
	fcfg := m.folderCfgs[folder]
	if fcfg.Type != config.FolderTypeReceiveOnly {
		return nil
	}
	if rf.ReceiveOnlyChangedSize().TotalItems() == 0 {
		return nil
	}

	files := make([]db.FileInfoTruncated, 0, perpage)

	skip := (page - 1) * perpage
	get := perpage

	rf.WithHaveTruncated(protocol.LocalDeviceID, func(f db.FileIntf) bool {
		if !f.IsReceiveOnlyChanged() {
			return true
		}
		if skip > 0 {
			skip--
			return true
		}
		ft := f.(db.FileInfoTruncated)
		files = append(files, ft)
		get--
		return get > 0
	})

	return files
}

// RemoteNeedFolderFiles returns paginated list of currently needed files in
// progress, queued, and to be queued on next puller iteration, as well as the
// total number of files currently needed.
func (m *Model) RemoteNeedFolderFiles(device protocol.DeviceID, folder string, page, perpage int) ([]db.FileInfoTruncated, error) {
	m.fmut.RLock()
	m.pmut.RLock()
	if err := m.checkDeviceFolderConnectedLocked(device, folder); err != nil {
		m.pmut.RUnlock()
		m.fmut.RUnlock()
		return nil, err
	}
	rf := m.folderFiles[folder]
	m.pmut.RUnlock()
	m.fmut.RUnlock()

	files := make([]db.FileInfoTruncated, 0, perpage)
	skip := (page - 1) * perpage
	get := perpage
	rf.WithNeedTruncated(device, func(f db.FileIntf) bool {
		if skip > 0 {
			skip--
			return true
		}
		files = append(files, f.(db.FileInfoTruncated))
		get--
		return get > 0
	})

	return files, nil
}

// Index is called when a new device is connected and we receive their full index.
// Implements the protocol.Model interface.
func (m *Model) Index(deviceID protocol.DeviceID, folder string, fs []protocol.FileInfo) {
	m.handleIndex(deviceID, folder, fs, false)
}

// IndexUpdate is called for incremental updates to connected devices' indexes.
// Implements the protocol.Model interface.
func (m *Model) IndexUpdate(deviceID protocol.DeviceID, folder string, fs []protocol.FileInfo) {
	m.handleIndex(deviceID, folder, fs, true)
}

func (m *Model) handleIndex(deviceID protocol.DeviceID, folder string, fs []protocol.FileInfo, update bool) {
	op := "Index"
	if update {
		op += " update"
	}

	l.Debugf("%v (in): %s / %q: %d files", op, deviceID, folder, len(fs))

	if cfg, ok := m.cfg.Folder(folder); !ok || !cfg.SharedWith(deviceID) {
		l.Infof("%v for unexpected folder ID %q sent from device %q; ensure that the folder exists and that this device is selected under \"Share With\" in the folder configuration.", op, folder, deviceID)
		return
	} else if cfg.Paused {
		l.Debugf("%v for paused folder (ID %q) sent from device %q.", op, folder, deviceID)
		return
	}

	m.fmut.RLock()
	files, existing := m.folderFiles[folder]
	runner, running := m.folderRunners[folder]
	m.fmut.RUnlock()

	if !existing {
		l.Fatalf("%v for nonexistent folder %q", op, folder)
	}

	if running {
		defer runner.SchedulePull()
	} else if update {
		// Runner may legitimately not be set if this is the "cleanup" Index
		// message at startup.
		l.Fatalf("%v for not running folder %q", op, folder)
	}

	m.pmut.RLock()
	m.deviceDownloads[deviceID].Update(folder, makeForgetUpdate(fs))
	m.pmut.RUnlock()

	if !update {
		files.Drop(deviceID)
	}
	for i := range fs {
		// The local flags should never be transmitted over the wire. Make
		// sure they look like they weren't.
		fs[i].LocalFlags = 0
	}
	files.Update(deviceID, fs)

	events.Default.Log(events.RemoteIndexUpdated, map[string]interface{}{
		"device":  deviceID.String(),
		"folder":  folder,
		"items":   len(fs),
		"version": files.Sequence(deviceID),
	})
}

func (m *Model) ClusterConfig(deviceID protocol.DeviceID, cm protocol.ClusterConfig) {
	// Check the peer device's announced folders against our own. Emits events
	// for folders that we don't expect (unknown or not shared).
	// Also, collect a list of folders we do share, and if he's interested in
	// temporary indexes, subscribe the connection.

	tempIndexFolders := make([]string, 0, len(cm.Folders))

	m.pmut.RLock()
	conn, ok := m.conn[deviceID]
	hello := m.helloMessages[deviceID]
	m.pmut.RUnlock()
	if !ok {
		panic("bug: ClusterConfig called on closed or nonexistent connection")
	}

	dbLocation := filepath.Dir(m.db.Location())
	changed := false
	deviceCfg := m.cfg.Devices()[deviceID]

	// See issue #3802 - in short, we can't send modern symlink entries to older
	// clients.
	dropSymlinks := false
	if hello.ClientName == m.clientName && upgrade.CompareVersions(hello.ClientVersion, "v0.14.14") < 0 {
		l.Warnln("Not sending symlinks to old client", deviceID, "- please upgrade to v0.14.14 or newer")
		dropSymlinks = true
	}

	// Needs to happen outside of the fmut, as can cause CommitConfiguration
	if deviceCfg.AutoAcceptFolders {
		for _, folder := range cm.Folders {
			changed = m.handleAutoAccepts(deviceCfg, folder) || changed
		}
	}

	m.fmut.Lock()
	var paused []string
	for _, folder := range cm.Folders {
		cfg, ok := m.cfg.Folder(folder.ID)
		if !ok || !cfg.SharedWith(deviceID) {
			if deviceCfg.IgnoredFolder(folder.ID) {
				l.Infof("Ignoring folder %s from device %s since we are configured to", folder.Description(), deviceID)
				continue
			}
			m.cfg.AddOrUpdatePendingFolder(folder.ID, folder.Label, deviceID)
			events.Default.Log(events.FolderRejected, map[string]string{
				"folder":      folder.ID,
				"folderLabel": folder.Label,
				"device":      deviceID.String(),
			})
			l.Infof("Unexpected folder %s sent from device %q; ensure that the folder exists and that this device is selected under \"Share With\" in the folder configuration.", folder.Description(), deviceID)
			continue
		}
		if folder.Paused {
			paused = append(paused, folder.ID)
			continue
		}
		if cfg.Paused {
			continue
		}
		fs, ok := m.folderFiles[folder.ID]
		if !ok {
			// Shouldn't happen because !cfg.Paused, but might happen
			// if the folder is about to be unpaused, but not yet.
			continue
		}

		if !folder.DisableTempIndexes {
			tempIndexFolders = append(tempIndexFolders, folder.ID)
		}

		myIndexID := fs.IndexID(protocol.LocalDeviceID)
		mySequence := fs.Sequence(protocol.LocalDeviceID)
		var startSequence int64

		for _, dev := range folder.Devices {
			if dev.ID == m.id {
				// This is the other side's description of what it knows
				// about us. Lets check to see if we can start sending index
				// updates directly or need to send the index from start...

				if dev.IndexID == myIndexID {
					// They say they've seen our index ID before, so we can
					// send a delta update only.

					if dev.MaxSequence > mySequence {
						// Safety check. They claim to have more or newer
						// index data than we have - either we have lost
						// index data, or reset the index without resetting
						// the IndexID, or something else weird has
						// happened. We send a full index to reset the
						// situation.
						l.Infof("Device %v folder %s is delta index compatible, but seems out of sync with reality", deviceID, folder.Description())
						startSequence = 0
						continue
					}

					l.Debugf("Device %v folder %s is delta index compatible (mlv=%d)", deviceID, folder.Description(), dev.MaxSequence)
					startSequence = dev.MaxSequence
				} else if dev.IndexID != 0 {
					// They say they've seen an index ID from us, but it's
					// not the right one. Either they are confused or we
					// must have reset our database since last talking to
					// them. We'll start with a full index transfer.
					l.Infof("Device %v folder %s has mismatching index ID for us (%v != %v)", deviceID, folder.Description(), dev.IndexID, myIndexID)
					startSequence = 0
				}
			} else if dev.ID == deviceID && dev.IndexID != 0 {
				// This is the other side's description of themselves. We
				// check to see that it matches the IndexID we have on file,
				// otherwise we drop our old index data and expect to get a
				// completely new set.

				theirIndexID := fs.IndexID(deviceID)
				if dev.IndexID == 0 {
					// They're not announcing an index ID. This means they
					// do not support delta indexes and we should clear any
					// information we have from them before accepting their
					// index, which will presumably be a full index.
					fs.Drop(deviceID)
				} else if dev.IndexID != theirIndexID {
					// The index ID we have on file is not what they're
					// announcing. They must have reset their database and
					// will probably send us a full index. We drop any
					// information we have and remember this new index ID
					// instead.
					l.Infof("Device %v folder %s has a new index ID (%v)", deviceID, folder.Description(), dev.IndexID)
					fs.Drop(deviceID)
					fs.SetIndexID(deviceID, dev.IndexID)
				} else {
					// They're sending a recognized index ID and will most
					// likely use delta indexes. We might already have files
					// that we need to pull so let the folder runner know
					// that it should recheck the index data.
					if runner := m.folderRunners[folder.ID]; runner != nil {
						defer runner.SchedulePull()
					}
				}
			}
		}

		go sendIndexes(conn, folder.ID, fs, m.folderIgnores[folder.ID], startSequence, dbLocation, dropSymlinks)
	}

	m.pmut.Lock()
	m.remotePausedFolders[deviceID] = paused
	m.pmut.Unlock()

	// This breaks if we send multiple CM messages during the same connection.
	if len(tempIndexFolders) > 0 {
		m.pmut.RLock()
		conn, ok := m.conn[deviceID]
		m.pmut.RUnlock()
		// In case we've got ClusterConfig, and the connection disappeared
		// from infront of our nose.
		if ok {
			m.progressEmitter.temporaryIndexSubscribe(conn, tempIndexFolders)
		}
	}

	if deviceCfg.Introducer {
		foldersDevices, introduced := m.handleIntroductions(deviceCfg, cm)
		if introduced {
			changed = true
		}
		// If permitted, check if the introducer has unshare devices/folders with
		// some of the devices/folders that we know were introduced to us by him.
		if !deviceCfg.SkipIntroductionRemovals && m.handleDeintroductions(deviceCfg, cm, foldersDevices) {
			changed = true
		}
	}
	m.fmut.Unlock()

	if changed {
		if err := m.cfg.Save(); err != nil {
			l.Warnln("Failed to save config", err)
		}
	}
}

// handleIntroductions handles adding devices/shares that are shared by an introducer device
func (m *Model) handleIntroductions(introducerCfg config.DeviceConfiguration, cm protocol.ClusterConfig) (folderDeviceSet, bool) {
	// This device is an introducer. Go through the announced lists of folders
	// and devices and add what we are missing, remove what we have extra that
	// has been introducer by the introducer.
	changed := false

	foldersDevices := make(folderDeviceSet)

	for _, folder := range cm.Folders {
		// Adds devices which we do not have, but the introducer has
		// for the folders that we have in common. Also, shares folders
		// with devices that we have in common, yet are currently not sharing
		// the folder.

		fcfg, ok := m.cfg.Folder(folder.ID)
		if !ok {
			// Don't have this folder, carry on.
			continue
		}

		for _, device := range folder.Devices {
			// No need to share with self.
			if device.ID == m.id {
				continue
			}

			foldersDevices.set(device.ID, folder.ID)

			if _, ok := m.cfg.Devices()[device.ID]; !ok {
				// The device is currently unknown. Add it to the config.
				m.introduceDevice(device, introducerCfg)
			} else if fcfg.SharedWith(device.ID) {
				// We already share the folder with this device, so
				// nothing to do.
				continue
			}

			// We don't yet share this folder with this device. Add the device
			// to sharing list of the folder.
			l.Infof("Sharing folder %s with %v (vouched for by introducer %v)", folder.Description(), device.ID, introducerCfg.DeviceID)
			fcfg.Devices = append(fcfg.Devices, config.FolderDeviceConfiguration{
				DeviceID:     device.ID,
				IntroducedBy: introducerCfg.DeviceID,
			})
			changed = true
		}

		if changed {
			m.cfg.SetFolder(fcfg)
		}
	}

	return foldersDevices, changed
}

// handleDeintroductions handles removals of devices/shares that are removed by an introducer device
func (m *Model) handleDeintroductions(introducerCfg config.DeviceConfiguration, cm protocol.ClusterConfig, foldersDevices folderDeviceSet) bool {
	changed := false
	devicesNotIntroduced := make(map[protocol.DeviceID]struct{})

	folders := m.cfg.FolderList()
	// Check if we should unshare some folders, if the introducer has unshared them.
	for i := range folders {
		for k := 0; k < len(folders[i].Devices); k++ {
			if folders[i].Devices[k].IntroducedBy != introducerCfg.DeviceID {
				devicesNotIntroduced[folders[i].Devices[k].DeviceID] = struct{}{}
				continue
			}
			if !foldersDevices.has(folders[i].Devices[k].DeviceID, folders[i].ID) {
				// We could not find that folder shared on the
				// introducer with the device that was introduced to us.
				// We should follow and unshare as well.
				l.Infof("Unsharing folder %s with %v as introducer %v no longer shares the folder with that device", folders[i].Description(), folders[i].Devices[k].DeviceID, folders[i].Devices[k].IntroducedBy)
				folders[i].Devices = append(folders[i].Devices[:k], folders[i].Devices[k+1:]...)
				k--
				changed = true
			}
		}
	}

	// Check if we should remove some devices, if the introducer no longer
	// shares any folder with them. Yet do not remove if we share other
	// folders that haven't been introduced by the introducer.
	devMap := m.cfg.Devices()
	devices := make([]config.DeviceConfiguration, 0, len(devMap))
	for deviceID, device := range devMap {
		if device.IntroducedBy == introducerCfg.DeviceID {
			if !foldersDevices.hasDevice(deviceID) {
				if _, ok := devicesNotIntroduced[deviceID]; !ok {
					// The introducer no longer shares any folder with the
					// device, remove the device.
					l.Infof("Removing device %v as introducer %v no longer shares any folders with that device", deviceID, device.IntroducedBy)
					changed = true
					continue
				}
				l.Infof("Would have removed %v as %v no longer shares any folders, yet there are other folders that are shared with this device that haven't been introduced by this introducer.", deviceID, device.IntroducedBy)
			}
		}
		devices = append(devices, device)
	}

	if changed {
		cfg := m.cfg.RawCopy()
		cfg.Folders = folders
		cfg.Devices = devices
		m.cfg.Replace(cfg)
	}

	return changed
}

// handleAutoAccepts handles adding and sharing folders for devices that have
// AutoAcceptFolders set to true.
func (m *Model) handleAutoAccepts(deviceCfg config.DeviceConfiguration, folder protocol.Folder) bool {
	if cfg, ok := m.cfg.Folder(folder.ID); !ok {
		defaultPath := m.cfg.Options().DefaultFolderPath
		defaultPathFs := fs.NewFilesystem(fs.FilesystemTypeBasic, defaultPath)
		pathAlternatives := []string{
			sanitizePath(folder.Label),
			sanitizePath(folder.ID),
		}
		for _, path := range pathAlternatives {
			if _, err := defaultPathFs.Lstat(path); !fs.IsNotExist(err) {
				continue
			}

			fcfg := config.NewFolderConfiguration(m.id, folder.ID, folder.Label, fs.FilesystemTypeBasic, filepath.Join(defaultPath, path))
			fcfg.Devices = append(fcfg.Devices, config.FolderDeviceConfiguration{
				DeviceID: deviceCfg.DeviceID,
			})
			// Need to wait for the waiter, as this calls CommitConfiguration,
			// which sets up the folder and as we return from this call,
			// ClusterConfig starts poking at m.folderFiles and other things
			// that might not exist until the config is committed.
			w, _ := m.cfg.SetFolder(fcfg)
			w.Wait()

			l.Infof("Auto-accepted %s folder %s at path %s", deviceCfg.DeviceID, folder.Description(), fcfg.Path)
			return true
		}
		l.Infof("Failed to auto-accept folder %s from %s due to path conflict", folder.Description(), deviceCfg.DeviceID)
		return false
	} else {
		for _, device := range cfg.DeviceIDs() {
			if device == deviceCfg.DeviceID {
				// Already shared nothing todo.
				return false
			}
		}
		cfg.Devices = append(cfg.Devices, config.FolderDeviceConfiguration{
			DeviceID: deviceCfg.DeviceID,
		})
		w, _ := m.cfg.SetFolder(cfg)
		w.Wait()
		l.Infof("Shared %s with %s due to auto-accept", folder.ID, deviceCfg.DeviceID)
		return true
	}
}

func (m *Model) introduceDevice(device protocol.Device, introducerCfg config.DeviceConfiguration) {
	addresses := []string{"dynamic"}
	for _, addr := range device.Addresses {
		if addr != "dynamic" {
			addresses = append(addresses, addr)
		}
	}

	l.Infof("Adding device %v to config (vouched for by introducer %v)", device.ID, introducerCfg.DeviceID)
	newDeviceCfg := config.DeviceConfiguration{
		DeviceID:     device.ID,
		Name:         device.Name,
		Compression:  introducerCfg.Compression,
		Addresses:    addresses,
		CertName:     device.CertName,
		IntroducedBy: introducerCfg.DeviceID,
	}

	// The introducers' introducers are also our introducers.
	if device.Introducer {
		l.Infof("Device %v is now also an introducer", device.ID)
		newDeviceCfg.Introducer = true
		newDeviceCfg.SkipIntroductionRemovals = device.SkipIntroductionRemovals
	}

	m.cfg.SetDevice(newDeviceCfg)
}

// Closed is called when a connection has been closed
func (m *Model) Closed(conn protocol.Connection, err error) {
	device := conn.ID()

	m.pmut.Lock()
	conn, ok := m.conn[device]
	if ok {
		m.progressEmitter.temporaryIndexUnsubscribe(conn)
	}
	delete(m.conn, device)
	delete(m.connRequestLimiters, device)
	delete(m.helloMessages, device)
	delete(m.deviceDownloads, device)
	delete(m.remotePausedFolders, device)
	closed := m.closed[device]
	delete(m.closed, device)
	m.pmut.Unlock()

	l.Infof("Connection to %s at %s closed: %v", device, conn.Name(), err)
	events.Default.Log(events.DeviceDisconnected, map[string]string{
		"id":    device.String(),
		"error": err.Error(),
	})
	close(closed)
}

// close will close the underlying connection for a given device
func (m *Model) close(device protocol.DeviceID) {
	m.pmut.Lock()
	m.closeLocked(device)
	m.pmut.Unlock()
}

// closeLocked will close the underlying connection for a given device
func (m *Model) closeLocked(device protocol.DeviceID) {
	conn, ok := m.conn[device]
	if !ok {
		// There is no connection to close
		return
	}

	closeRawConn(conn)
}

// Implements protocol.RequestResponse
type requestResponse struct {
	data   []byte
	closed chan struct{}
	once   stdsync.Once
}

func newRequestResponse(size int) *requestResponse {
	return &requestResponse{
		data:   protocol.BufferPool.Get(size),
		closed: make(chan struct{}),
	}
}

func (r *requestResponse) Data() []byte {
	return r.data
}

func (r *requestResponse) Close() {
	r.once.Do(func() {
		protocol.BufferPool.Put(r.data)
		close(r.closed)
	})
}

func (r *requestResponse) Wait() {
	<-r.closed
}

// Request returns the specified data segment by reading it from local disk.
// Implements the protocol.Model interface.
func (m *Model) Request(deviceID protocol.DeviceID, folder, name string, size int32, offset int64, hash []byte, weakHash uint32, fromTemporary bool) (out protocol.RequestResponse, err error) {
	if size < 0 || offset < 0 {
		return nil, protocol.ErrInvalid
	}

	m.fmut.RLock()
	folderCfg, ok := m.folderCfgs[folder]
	folderIgnores := m.folderIgnores[folder]
	m.fmut.RUnlock()
	if !ok {
		// The folder might be already unpaused in the config, but not yet
		// in the model.
		l.Debugf("Request from %s for file %s in unstarted folder %q", deviceID, name, folder)
		return nil, protocol.ErrInvalid
	}

	if !folderCfg.SharedWith(deviceID) {
		l.Warnf("Request from %s for file %s in unshared folder %q", deviceID, name, folder)
		return nil, protocol.ErrNoSuchFile
	}
	if folderCfg.Paused {
		l.Debugf("Request from %s for file %s in paused folder %q", deviceID, name, folder)
		return nil, protocol.ErrInvalid
	}

	// Make sure the path is valid and in canonical form
	if name, err = fs.Canonicalize(name); err != nil {
		l.Debugf("Request from %s in folder %q for invalid filename %s", deviceID, folder, name)
		return nil, protocol.ErrInvalid
	}

	if deviceID != protocol.LocalDeviceID {
		l.Debugf("%v REQ(in): %s: %q / %q o=%d s=%d t=%v", m, deviceID, folder, name, offset, size, fromTemporary)
	}

	if fs.IsInternal(name) {
		l.Debugf("%v REQ(in) for internal file: %s: %q / %q o=%d s=%d", m, deviceID, folder, name, offset, size)
		return nil, protocol.ErrNoSuchFile
	}

	if folderIgnores.Match(name).IsIgnored() {
		l.Debugf("%v REQ(in) for ignored file: %s: %q / %q o=%d s=%d", m, deviceID, folder, name, offset, size)
		return nil, protocol.ErrNoSuchFile
	}

	folderFs := folderCfg.Filesystem()

	if err := osutil.TraversesSymlink(folderFs, filepath.Dir(name)); err != nil {
		l.Debugf("%v REQ(in) traversal check: %s - %s: %q / %q o=%d s=%d", m, err, deviceID, folder, name, offset, size)
		return nil, protocol.ErrNoSuchFile
	}

	// Restrict parallel requests by connection/device

	m.pmut.RLock()
	limiter := m.connRequestLimiters[deviceID]
	m.pmut.RUnlock()

	if limiter != nil {
		limiter.take(int(size))
	}

	// The requestResponse releases the bytes to the limiter when its Close method is called.
	res := newRequestResponse(int(size))
	defer func() {
		// Close it ourselves if it isn't returned due to an error
		if err != nil {
			res.Close()
		}
	}()

	if limiter != nil {
		go func() {
			res.Wait()
			limiter.give(int(size))
		}()
	}

	// Only check temp files if the flag is set, and if we are set to advertise
	// the temp indexes.
	if fromTemporary && !folderCfg.DisableTempIndexes {
		tempFn := fs.TempName(name)

		if info, err := folderFs.Lstat(tempFn); err != nil || !info.IsRegular() {
			// Reject reads for anything that doesn't exist or is something
			// other than a regular file.
			l.Debugf("%v REQ(in) failed stating temp file (%v): %s: %q / %q o=%d s=%d", m, err, deviceID, folder, name, offset, size)
			return nil, protocol.ErrNoSuchFile
		}
		err := readOffsetIntoBuf(folderFs, tempFn, offset, res.data)
		if err == nil && scanner.Validate(res.data, hash, weakHash) {
			return res, nil
		}
		// Fall through to reading from a non-temp file, just incase the temp
		// file has finished downloading.
	}

	if info, err := folderFs.Lstat(name); err != nil || !info.IsRegular() {
		// Reject reads for anything that doesn't exist or is something
		// other than a regular file.
		l.Debugf("%v REQ(in) failed stating file (%v): %s: %q / %q o=%d s=%d", m, err, deviceID, folder, name, offset, size)
		return nil, protocol.ErrNoSuchFile
	}

	if err := readOffsetIntoBuf(folderFs, name, offset, res.data); fs.IsNotExist(err) {
		l.Debugf("%v REQ(in) file doesn't exist: %s: %q / %q o=%d s=%d", m, deviceID, folder, name, offset, size)
		return nil, protocol.ErrNoSuchFile
	} else if err != nil {
		l.Debugf("%v REQ(in) failed reading file (%v): %s: %q / %q o=%d s=%d", m, err, deviceID, folder, name, offset, size)
		return nil, protocol.ErrGeneric
	}

	if !scanner.Validate(res.data, hash, weakHash) {
		m.recheckFile(deviceID, folderFs, folder, name, int(offset)/int(size), hash)
		l.Debugf("%v REQ(in) failed validating data (%v): %s: %q / %q o=%d s=%d", m, err, deviceID, folder, name, offset, size)
		return nil, protocol.ErrNoSuchFile
	}

	return res, nil
}

func (m *Model) recheckFile(deviceID protocol.DeviceID, folderFs fs.Filesystem, folder, name string, blockIndex int, hash []byte) {
	cf, ok := m.CurrentFolderFile(folder, name)
	if !ok {
		l.Debugf("%v recheckFile: %s: %q / %q: no current file", m, deviceID, folder, name)
		return
	}

	if cf.IsDeleted() || cf.IsInvalid() || cf.IsSymlink() || cf.IsDirectory() {
		l.Debugf("%v recheckFile: %s: %q / %q: not a regular file", m, deviceID, folder, name)
		return
	}

	if blockIndex >= len(cf.Blocks) {
		l.Debugf("%v recheckFile: %s: %q / %q i=%d: block index too far", m, deviceID, folder, name, blockIndex)
		return
	}

	block := cf.Blocks[blockIndex]

	// Seems to want a different version of the file, whatever.
	if !bytes.Equal(block.Hash, hash) {
		l.Debugf("%v recheckFile: %s: %q / %q i=%d: hash mismatch %x != %x", m, deviceID, folder, name, blockIndex, block.Hash, hash)
		return
	}

	// The hashes provided part of the request match what we expect to find according
	// to what we have in the database, yet the content we've read off the filesystem doesn't
	// Something is fishy, invalidate the file and rescan it.
	cf.SetMustRescan(m.shortID)

	// Update the index and tell others
	// The file will temporarily become invalid, which is ok as the content is messed up.
	m.updateLocalsFromScanning(folder, []protocol.FileInfo{cf})

	if err := m.ScanFolderSubdirs(folder, []string{name}); err != nil {
		l.Debugf("%v recheckFile: %s: %q / %q rescan: %s", m, deviceID, folder, name, err)
	} else {
		l.Debugf("%v recheckFile: %s: %q / %q", m, deviceID, folder, name)
	}
}

func (m *Model) CurrentFolderFile(folder string, file string) (protocol.FileInfo, bool) {
	m.fmut.RLock()
	fs, ok := m.folderFiles[folder]
	m.fmut.RUnlock()
	if !ok {
		return protocol.FileInfo{}, false
	}
	return fs.Get(protocol.LocalDeviceID, file)
}

func (m *Model) CurrentGlobalFile(folder string, file string) (protocol.FileInfo, bool) {
	m.fmut.RLock()
	fs, ok := m.folderFiles[folder]
	m.fmut.RUnlock()
	if !ok {
		return protocol.FileInfo{}, false
	}
	return fs.GetGlobal(file)
}

type cFiler struct {
	m *Model
	r string
}

// Implements scanner.CurrentFiler
func (cf cFiler) CurrentFile(file string) (protocol.FileInfo, bool) {
	return cf.m.CurrentFolderFile(cf.r, file)
}

// Connection returns the current connection for device, and a boolean whether a connection was found.
func (m *Model) Connection(deviceID protocol.DeviceID) (connections.Connection, bool) {
	m.pmut.RLock()
	cn, ok := m.conn[deviceID]
	m.pmut.RUnlock()
	if ok {
		m.deviceWasSeen(deviceID)
	}
	return cn, ok
}

func (m *Model) GetIgnores(folder string) ([]string, []string, error) {
	m.fmut.RLock()
	defer m.fmut.RUnlock()

	cfg, ok := m.folderCfgs[folder]
	if !ok {
		cfg, ok = m.cfg.Folders()[folder]
		if !ok {
			return nil, nil, fmt.Errorf("Folder %s does not exist", folder)
		}
	}

	// On creation a new folder with ignore patterns validly has no marker yet.
	if err := cfg.CheckPath(); err != nil && err != config.ErrMarkerMissing {
		return nil, nil, err
	}

	ignores, ok := m.folderIgnores[folder]
	if !ok {
		ignores = ignore.New(fs.NewFilesystem(cfg.FilesystemType, cfg.Path))
	}

	if err := ignores.Load(".stignore"); err != nil && !fs.IsNotExist(err) {
		return nil, nil, err
	}

	return ignores.Lines(), ignores.Patterns(), nil
}

func (m *Model) SetIgnores(folder string, content []string) error {
	cfg, ok := m.cfg.Folders()[folder]
	if !ok {
		return fmt.Errorf("folder %s does not exist", cfg.Description())
	}

	err := cfg.CheckPath()
	if err == config.ErrPathMissing {
		if err = cfg.CreateRoot(); err != nil {
			return fmt.Errorf("failed to create folder root: %v", err)
		}
		err = cfg.CheckPath()
	}
	if err != nil && err != config.ErrMarkerMissing {
		return err
	}

	if err := ignore.WriteIgnores(cfg.Filesystem(), ".stignore", content); err != nil {
		l.Warnln("Saving .stignore:", err)
		return err
	}

	m.fmut.RLock()
	runner, ok := m.folderRunners[folder]
	m.fmut.RUnlock()
	if ok {
		return runner.Scan(nil)
	}
	return nil
}

// OnHello is called when an device connects to us.
// This allows us to extract some information from the Hello message
// and add it to a list of known devices ahead of any checks.
func (m *Model) OnHello(remoteID protocol.DeviceID, addr net.Addr, hello protocol.HelloResult) error {
	if m.cfg.IgnoredDevice(remoteID) {
		return errDeviceIgnored
	}

	cfg, ok := m.cfg.Device(remoteID)
	if !ok {
		m.cfg.AddOrUpdatePendingDevice(remoteID, hello.DeviceName, addr.String())
		events.Default.Log(events.DeviceRejected, map[string]string{
			"name":    hello.DeviceName,
			"device":  remoteID.String(),
			"address": addr.String(),
		})
		return errDeviceUnknown
	}

	if cfg.Paused {
		return errDevicePaused
	}

	if len(cfg.AllowedNetworks) > 0 {
		if !connections.IsAllowedNetwork(addr.String(), cfg.AllowedNetworks) {
			return errNetworkNotAllowed
		}
	}

	return nil
}

// GetHello is called when we are about to connect to some remote device.
func (m *Model) GetHello(id protocol.DeviceID) protocol.HelloIntf {
	name := ""
	if _, ok := m.cfg.Device(id); ok {
		name = m.cfg.MyName()
	}
	return &protocol.Hello{
		DeviceName:    name,
		ClientName:    m.clientName,
		ClientVersion: m.clientVersion,
	}
}

// AddConnection adds a new peer connection to the model. An initial index will
// be sent to the connected peer, thereafter index updates whenever the local
// folder changes.
func (m *Model) AddConnection(conn connections.Connection, hello protocol.HelloResult) {
	deviceID := conn.ID()
	device, ok := m.cfg.Device(deviceID)
	if !ok {
		l.Infoln("Trying to add connection to unknown device")
		return
	}

	m.pmut.Lock()
	if oldConn, ok := m.conn[deviceID]; ok {
		l.Infoln("Replacing old connection", oldConn, "with", conn, "for", deviceID)
		// There is an existing connection to this device that we are
		// replacing. We must close the existing connection and wait for the
		// close to complete before adding the new connection. We do the
		// actual close without holding pmut as the connection will call
		// back into Closed() for the cleanup.
		closed := m.closed[deviceID]
		m.pmut.Unlock()
		closeRawConn(oldConn)
		<-closed
		m.pmut.Lock()
	}

	m.conn[deviceID] = conn
	m.closed[deviceID] = make(chan struct{})
	m.deviceDownloads[deviceID] = newDeviceDownloadState()
	// 0: default, <0: no limiting
	switch {
	case device.MaxRequestKiB > 0:
		m.connRequestLimiters[deviceID] = newByteSemaphore(1024 * device.MaxRequestKiB)
	case device.MaxRequestKiB == 0:
		m.connRequestLimiters[deviceID] = newByteSemaphore(1024 * defaultPullerPendingKiB)
	}

	m.helloMessages[deviceID] = hello

	event := map[string]string{
		"id":            deviceID.String(),
		"deviceName":    hello.DeviceName,
		"clientName":    hello.ClientName,
		"clientVersion": hello.ClientVersion,
		"type":          conn.Type(),
	}

	addr := conn.RemoteAddr()
	if addr != nil {
		event["addr"] = addr.String()
	}

	events.Default.Log(events.DeviceConnected, event)

	l.Infof(`Device %s client is "%s %s" named "%s" at %s`, deviceID, hello.ClientName, hello.ClientVersion, hello.DeviceName, conn)

	conn.Start()
	m.pmut.Unlock()

	// Acquires fmut, so has to be done outside of pmut.
	cm := m.generateClusterConfig(deviceID)
	conn.ClusterConfig(cm)

	if (device.Name == "" || m.cfg.Options().OverwriteRemoteDevNames) && hello.DeviceName != "" {
		device.Name = hello.DeviceName
		m.cfg.SetDevice(device)
		m.cfg.Save()
	}

	m.deviceWasSeen(deviceID)
}

func (m *Model) DownloadProgress(device protocol.DeviceID, folder string, updates []protocol.FileDownloadProgressUpdate) {
	m.fmut.RLock()
	cfg, ok := m.folderCfgs[folder]
	m.fmut.RUnlock()

	if !ok || cfg.DisableTempIndexes || !cfg.SharedWith(device) {
		return
	}

	m.pmut.RLock()
	m.deviceDownloads[device].Update(folder, updates)
	state := m.deviceDownloads[device].GetBlockCounts(folder)
	m.pmut.RUnlock()

	events.Default.Log(events.RemoteDownloadProgress, map[string]interface{}{
		"device": device.String(),
		"folder": folder,
		"state":  state,
	})
}

func (m *Model) deviceStatRef(deviceID protocol.DeviceID) *stats.DeviceStatisticsReference {
	m.fmut.Lock()
	defer m.fmut.Unlock()

	if sr, ok := m.deviceStatRefs[deviceID]; ok {
		return sr
	}

	sr := stats.NewDeviceStatisticsReference(m.db, deviceID.String())
	m.deviceStatRefs[deviceID] = sr
	return sr
}

func (m *Model) deviceWasSeen(deviceID protocol.DeviceID) {
	m.deviceStatRef(deviceID).WasSeen()
}

func (m *Model) folderStatRef(folder string) *stats.FolderStatisticsReference {
	m.fmut.Lock()
	defer m.fmut.Unlock()

	sr, ok := m.folderStatRefs[folder]
	if !ok {
		sr = stats.NewFolderStatisticsReference(m.db, folder)
		m.folderStatRefs[folder] = sr
	}
	return sr
}

func (m *Model) receivedFile(folder string, file protocol.FileInfo) {
	m.folderStatRef(folder).ReceivedFile(file.Name, file.IsDeleted())
}

func sendIndexes(conn protocol.Connection, folder string, fs *db.FileSet, ignores *ignore.Matcher, prevSequence int64, dbLocation string, dropSymlinks bool) {
	deviceID := conn.ID()
	var err error

	l.Debugf("Starting sendIndexes for %s to %s at %s (slv=%d)", folder, deviceID, conn, prevSequence)
	defer l.Debugf("Exiting sendIndexes for %s to %s at %s: %v", folder, deviceID, conn, err)

	// We need to send one index, regardless of whether there is something to send or not
	prevSequence, err = sendIndexTo(prevSequence, conn, folder, fs, ignores, dbLocation, dropSymlinks)

	// Subscribe to LocalIndexUpdated (we have new information to send) and
	// DeviceDisconnected (it might be us who disconnected, so we should
	// exit).
	sub := events.Default.Subscribe(events.LocalIndexUpdated | events.DeviceDisconnected)
	defer events.Default.Unsubscribe(sub)

	for err == nil {
		if conn.Closed() {
			// Our work is done.
			return
		}

		// While we have sent a sequence at least equal to the one
		// currently in the database, wait for the local index to update. The
		// local index may update for other folders than the one we are
		// sending for.
		if fs.Sequence(protocol.LocalDeviceID) <= prevSequence {
			sub.Poll(time.Minute)
			continue
		}

		prevSequence, err = sendIndexTo(prevSequence, conn, folder, fs, ignores, dbLocation, dropSymlinks)

		// Wait a short amount of time before entering the next loop. If there
		// are continuous changes happening to the local index, this gives us
		// time to batch them up a little.
		time.Sleep(250 * time.Millisecond)
	}
}

// sendIndexTo sends file infos with a sequence number higher than prevSequence and
// returns the highest sent sequence number.
func sendIndexTo(prevSequence int64, conn protocol.Connection, folder string, fs *db.FileSet, ignores *ignore.Matcher, dbLocation string, dropSymlinks bool) (int64, error) {
	deviceID := conn.ID()
	initial := prevSequence == 0
	batch := newFileInfoBatch(nil)
	batch.flushFn = func(fs []protocol.FileInfo) error {
		l.Debugf("Sending indexes for %s to %s at %s: %d files (<%d bytes)", folder, deviceID, conn, len(batch.infos), batch.size)
		if initial {
			initial = false
			return conn.Index(folder, fs)
		}
		return conn.IndexUpdate(folder, fs)
	}

	var err error
	var f protocol.FileInfo
	fs.WithHaveSequence(prevSequence+1, func(fi db.FileIntf) bool {
		if err = batch.flushIfFull(); err != nil {
			return false
		}

		if shouldDebug() {
			if fi.SequenceNo() < prevSequence+1 {
				panic(fmt.Sprintln("sequence lower than requested, got:", fi.SequenceNo(), ", asked to start at:", prevSequence+1))
			}
			if f.Sequence > 0 && fi.SequenceNo() <= f.Sequence {
				panic(fmt.Sprintln("non-increasing sequence, current:", fi.SequenceNo(), "<= previous:", f.Sequence))
			}
		}

		if shouldDebug() {
			if fi.SequenceNo() < prevSequence+1 {
				panic(fmt.Sprintln("sequence lower than requested, got:", fi.SequenceNo(), ", asked to start at:", prevSequence+1))
			}
			if f.Sequence > 0 && fi.SequenceNo() <= f.Sequence {
				panic(fmt.Sprintln("non-increasing sequence, current:", fi.SequenceNo(), "<= previous:", f.Sequence))
			}
		}

		f = fi.(protocol.FileInfo)

		// Mark the file as invalid if any of the local bad stuff flags are set.
		f.RawInvalid = f.IsInvalid()
		// If the file is marked LocalReceive (i.e., changed locally on a
		// receive only folder) we do not want it to ever become the
		// globally best version, invalid or not.
		if f.IsReceiveOnlyChanged() {
			f.Version = protocol.Vector{}
		}
		f.LocalFlags = 0 // never sent externally

		if dropSymlinks && f.IsSymlink() {
			// Do not send index entries with symlinks to clients that can't
			// handle it. Fixes issue #3802. Once both sides are upgraded, a
			// rescan (i.e., change) of the symlink is required for it to
			// sync again, due to delta indexes.
			return true
		}

		batch.append(f)
		return true
	})
	if err != nil {
		return prevSequence, err
	}

	err = batch.flush()

	// True if there was nothing to be sent
	if f.Sequence == 0 {
		return prevSequence, err
	}

	return f.Sequence, err
}

func (m *Model) updateLocalsFromScanning(folder string, fs []protocol.FileInfo) {
	m.updateLocals(folder, fs)

	m.fmut.RLock()
	folderCfg := m.folderCfgs[folder]
	m.fmut.RUnlock()

	m.diskChangeDetected(folderCfg, fs, events.LocalChangeDetected)
}

func (m *Model) updateLocalsFromPulling(folder string, fs []protocol.FileInfo) {
	m.updateLocals(folder, fs)

	m.fmut.RLock()
	folderCfg := m.folderCfgs[folder]
	m.fmut.RUnlock()

	m.diskChangeDetected(folderCfg, fs, events.RemoteChangeDetected)
}

func (m *Model) updateLocals(folder string, fs []protocol.FileInfo) {
	m.fmut.RLock()
	files := m.folderFiles[folder]
	m.fmut.RUnlock()
	if files == nil {
		// The folder doesn't exist.
		return
	}
	files.Update(protocol.LocalDeviceID, fs)

	filenames := make([]string, len(fs))
	for i, file := range fs {
		filenames[i] = file.Name
	}

	events.Default.Log(events.LocalIndexUpdated, map[string]interface{}{
		"folder":    folder,
		"items":     len(fs),
		"filenames": filenames,
		"version":   files.Sequence(protocol.LocalDeviceID),
	})
}

func (m *Model) diskChangeDetected(folderCfg config.FolderConfiguration, files []protocol.FileInfo, typeOfEvent events.EventType) {
	for _, file := range files {
		if file.IsInvalid() {
			continue
		}

		objType := "file"
		action := "modified"

		switch {
		case file.IsDeleted():
			action = "deleted"

		// If our local vector is version 1 AND it is the only version
		// vector so far seen for this file then it is a new file.  Else if
		// it is > 1 it's not new, and if it is 1 but another shortId
		// version vector exists then it is new for us but created elsewhere
		// so the file is still not new but modified by us. Only if it is
		// truly new do we change this to 'added', else we leave it as
		// 'modified'.
		case len(file.Version.Counters) == 1 && file.Version.Counters[0].Value == 1:
			action = "added"
		}

		if file.IsSymlink() {
			objType = "symlink"
		} else if file.IsDirectory() {
			objType = "dir"
		}

		// Two different events can be fired here based on what EventType is passed into function
		events.Default.Log(typeOfEvent, map[string]string{
			"folder":     folderCfg.ID,
			"folderID":   folderCfg.ID, // incorrect, deprecated, kept for historical compliance
			"label":      folderCfg.Label,
			"action":     action,
			"type":       objType,
			"path":       filepath.FromSlash(file.Name),
			"modifiedBy": file.ModifiedBy.String(),
		})
	}
}

func (m *Model) requestGlobal(deviceID protocol.DeviceID, folder, name string, offset int64, size int, hash []byte, weakHash uint32, fromTemporary bool) ([]byte, error) {
	m.pmut.RLock()
	nc, ok := m.conn[deviceID]
	m.pmut.RUnlock()

	if !ok {
		return nil, fmt.Errorf("requestGlobal: no such device: %s", deviceID)
	}

	l.Debugf("%v REQ(out): %s: %q / %q o=%d s=%d h=%x wh=%x ft=%t", m, deviceID, folder, name, offset, size, hash, weakHash, fromTemporary)

	return nc.Request(folder, name, offset, size, hash, weakHash, fromTemporary)
}

func (m *Model) ScanFolders() map[string]error {
	m.fmut.RLock()
	folders := make([]string, 0, len(m.folderCfgs))
	for folder := range m.folderCfgs {
		folders = append(folders, folder)
	}
	m.fmut.RUnlock()

	errors := make(map[string]error, len(m.folderCfgs))
	errorsMut := sync.NewMutex()

	wg := sync.NewWaitGroup()
	wg.Add(len(folders))
	for _, folder := range folders {
		folder := folder
		go func() {
			err := m.ScanFolder(folder)
			if err != nil {
				errorsMut.Lock()
				errors[folder] = err
				errorsMut.Unlock()

				// Potentially sets the error twice, once in the scanner just
				// by doing a check, and once here, if the error returned is
				// the same one as returned by CheckHealth, though
				// duplicate set is handled by setError.
				m.fmut.RLock()
				srv := m.folderRunners[folder]
				m.fmut.RUnlock()
				srv.setError(err)
			}
			wg.Done()
		}()
	}
	wg.Wait()
	return errors
}

func (m *Model) ScanFolder(folder string) error {
	return m.ScanFolderSubdirs(folder, nil)
}

func (m *Model) ScanFolderSubdirs(folder string, subs []string) error {
	m.fmut.RLock()
	if err := m.checkFolderRunningLocked(folder); err != nil {
		m.fmut.RUnlock()
		return err
	}
	runner := m.folderRunners[folder]
	m.fmut.RUnlock()

	return runner.Scan(subs)
}

func (m *Model) DelayScan(folder string, next time.Duration) {
	m.fmut.Lock()
	runner, ok := m.folderRunners[folder]
	m.fmut.Unlock()
	if !ok {
		return
	}
	runner.DelayScan(next)
}

// numHashers returns the number of hasher routines to use for a given folder,
// taking into account configuration and available CPU cores.
func (m *Model) numHashers(folder string) int {
	m.fmut.Lock()
	folderCfg := m.folderCfgs[folder]
	numFolders := len(m.folderCfgs)
	m.fmut.Unlock()

	if folderCfg.Hashers > 0 {
		// Specific value set in the config, use that.
		return folderCfg.Hashers
	}

	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		// Interactive operating systems; don't load the system too heavily by
		// default.
		return 1
	}

	// For other operating systems and architectures, lets try to get some
	// work done... Divide the available CPU cores among the configured
	// folders.
	if perFolder := runtime.GOMAXPROCS(-1) / numFolders; perFolder > 0 {
		return perFolder
	}

	return 1
}

// generateClusterConfig returns a ClusterConfigMessage that is correct for
// the given peer device
func (m *Model) generateClusterConfig(device protocol.DeviceID) protocol.ClusterConfig {
	var message protocol.ClusterConfig

	m.fmut.RLock()
	defer m.fmut.RUnlock()

	for _, folderCfg := range m.cfg.FolderList() {
		if !folderCfg.SharedWith(device) {
			continue
		}

		protocolFolder := protocol.Folder{
			ID:                 folderCfg.ID,
			Label:              folderCfg.Label,
			ReadOnly:           folderCfg.Type == config.FolderTypeSendOnly,
			IgnorePermissions:  folderCfg.IgnorePerms,
			IgnoreDelete:       folderCfg.IgnoreDelete,
			DisableTempIndexes: folderCfg.DisableTempIndexes,
			Paused:             folderCfg.Paused,
		}

		var fs *db.FileSet
		if !folderCfg.Paused {
			fs = m.folderFiles[folderCfg.ID]
		}

		for _, device := range folderCfg.Devices {
			deviceCfg, _ := m.cfg.Device(device.DeviceID)

			protocolDevice := protocol.Device{
				ID:          deviceCfg.DeviceID,
				Name:        deviceCfg.Name,
				Addresses:   deviceCfg.Addresses,
				Compression: deviceCfg.Compression,
				CertName:    deviceCfg.CertName,
				Introducer:  deviceCfg.Introducer,
			}

			if fs != nil {
				if deviceCfg.DeviceID == m.id {
					protocolDevice.IndexID = fs.IndexID(protocol.LocalDeviceID)
					protocolDevice.MaxSequence = fs.Sequence(protocol.LocalDeviceID)
				} else {
					protocolDevice.IndexID = fs.IndexID(deviceCfg.DeviceID)
					protocolDevice.MaxSequence = fs.Sequence(deviceCfg.DeviceID)
				}
			}

			protocolFolder.Devices = append(protocolFolder.Devices, protocolDevice)
		}

		message.Folders = append(message.Folders, protocolFolder)
	}

	return message
}

func (m *Model) State(folder string) (string, time.Time, error) {
	m.fmut.RLock()
	runner, ok := m.folderRunners[folder]
	m.fmut.RUnlock()
	if !ok {
		// The returned error should be an actual folder error, so returning
		// errors.New("does not exist") or similar here would be
		// inappropriate.
		return "", time.Time{}, nil
	}
	state, changed, err := runner.getState()
	return state.String(), changed, err
}

func (m *Model) FolderErrors(folder string) ([]FileError, error) {
	m.fmut.RLock()
	defer m.fmut.RUnlock()
	if err := m.checkFolderRunningLocked(folder); err != nil {
		return nil, err
	}
	return m.folderRunners[folder].Errors(), nil
}

func (m *Model) WatchError(folder string) error {
	m.fmut.RLock()
	defer m.fmut.RUnlock()
	if err := m.checkFolderRunningLocked(folder); err != nil {
		return err
	}
	return m.folderRunners[folder].WatchError()
}

func (m *Model) Override(folder string) {
	// Grab the runner and the file set.

	m.fmut.RLock()
	fs, fsOK := m.folderFiles[folder]
	runner, runnerOK := m.folderRunners[folder]
	m.fmut.RUnlock()
	if !fsOK || !runnerOK {
		return
	}

	// Run the override, taking updates as if they came from scanning.

	runner.Override(fs, func(files []protocol.FileInfo) {
		m.updateLocalsFromScanning(folder, files)
	})
}

func (m *Model) Revert(folder string) {
	// Grab the runner and the file set.

	m.fmut.RLock()
	fs, fsOK := m.folderFiles[folder]
	runner, runnerOK := m.folderRunners[folder]
	m.fmut.RUnlock()
	if !fsOK || !runnerOK {
		return
	}

	// Run the revert, taking updates as if they came from scanning.

	runner.Revert(fs, func(files []protocol.FileInfo) {
		m.updateLocalsFromScanning(folder, files)
	})
}

// CurrentSequence returns the change version for the given folder.
// This is guaranteed to increment if the contents of the local folder has
// changed.
func (m *Model) CurrentSequence(folder string) (int64, bool) {
	m.fmut.RLock()
	fs, ok := m.folderFiles[folder]
	m.fmut.RUnlock()
	if !ok {
		// The folder might not exist, since this can be called with a user
		// specified folder name from the REST interface.
		return 0, false
	}

	return fs.Sequence(protocol.LocalDeviceID), true
}

// RemoteSequence returns the change version for the given folder, as
// sent by remote peers. This is guaranteed to increment if the contents of
// the remote or global folder has changed.
func (m *Model) RemoteSequence(folder string) (int64, bool) {
	m.fmut.RLock()
	defer m.fmut.RUnlock()

	fs, ok := m.folderFiles[folder]
	cfg := m.folderCfgs[folder]
	if !ok {
		// The folder might not exist, since this can be called with a user
		// specified folder name from the REST interface.
		return 0, false
	}

	var ver int64
	for _, device := range cfg.Devices {
		ver += fs.Sequence(device.DeviceID)
	}

	return ver, true
}

func (m *Model) GlobalDirectoryTree(folder, prefix string, levels int, dirsonly bool) map[string]interface{} {
	m.fmut.RLock()
	files, ok := m.folderFiles[folder]
	m.fmut.RUnlock()
	if !ok {
		return nil
	}

	output := make(map[string]interface{})
	sep := string(filepath.Separator)
	prefix = osutil.NativeFilename(prefix)

	if prefix != "" && !strings.HasSuffix(prefix, sep) {
		prefix = prefix + sep
	}

	files.WithPrefixedGlobalTruncated(prefix, func(fi db.FileIntf) bool {
		f := fi.(db.FileInfoTruncated)

		// Don't include the prefix itself.
		if f.IsInvalid() || f.IsDeleted() || strings.HasPrefix(prefix, f.Name) {
			return true
		}

		f.Name = strings.Replace(f.Name, prefix, "", 1)

		var dir, base string
		if f.IsDirectory() && !f.IsSymlink() {
			dir = f.Name
		} else {
			dir = filepath.Dir(f.Name)
			base = filepath.Base(f.Name)
		}

		if levels > -1 && strings.Count(f.Name, sep) > levels {
			return true
		}

		last := output
		if dir != "." {
			for _, path := range strings.Split(dir, sep) {
				directory, ok := last[path]
				if !ok {
					newdir := make(map[string]interface{})
					last[path] = newdir
					last = newdir
				} else {
					last = directory.(map[string]interface{})
				}
			}
		}

		if !dirsonly && base != "" {
			last[base] = []interface{}{
				f.ModTime(), f.FileSize(),
			}
		}

		return true
	})

	return output
}

func (m *Model) GetFolderVersions(folder string) (map[string][]versioner.FileVersion, error) {
	fcfg, ok := m.cfg.Folder(folder)
	if !ok {
		return nil, errFolderMissing
	}

	files := make(map[string][]versioner.FileVersion)

	filesystem := fcfg.Filesystem()
	err := filesystem.Walk(".stversions", func(path string, f fs.FileInfo, err error) error {
		// Skip root (which is ok to be a symlink)
		if path == ".stversions" {
			return nil
		}

		// Ignore symlinks
		if f.IsSymlink() {
			return fs.SkipDir
		}

		// No records for directories
		if f.IsDir() {
			return nil
		}

		// Strip .stversions prefix.
		path = strings.TrimPrefix(path, ".stversions"+string(fs.PathSeparator))

		name, tag := versioner.UntagFilename(path)
		// Something invalid
		if name == "" || tag == "" {
			return nil
		}

		name = osutil.NormalizedFilename(name)

		versionTime, err := time.ParseInLocation(versioner.TimeFormat, tag, locationLocal)
		if err != nil {
			return nil
		}

		files[name] = append(files[name], versioner.FileVersion{
			VersionTime: versionTime.Truncate(time.Second),
			ModTime:     f.ModTime().Truncate(time.Second),
			Size:        f.Size(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	return files, nil
}

func (m *Model) RestoreFolderVersions(folder string, versions map[string]time.Time) (map[string]string, error) {
	fcfg, ok := m.cfg.Folder(folder)
	if !ok {
		return nil, errFolderMissing
	}

	filesystem := fcfg.Filesystem()
	ver := fcfg.Versioner()

	restore := make(map[string]string)
	errors := make(map[string]string)

	// Validation
	for file, version := range versions {
		file = osutil.NativeFilename(file)
		tag := version.In(locationLocal).Truncate(time.Second).Format(versioner.TimeFormat)
		versionedTaggedFilename := filepath.Join(".stversions", versioner.TagFilename(file, tag))
		// Check that the thing we've been asked to restore is actually a file
		// and that it exists.
		if info, err := filesystem.Lstat(versionedTaggedFilename); err != nil {
			errors[file] = err.Error()
			continue
		} else if !info.IsRegular() {
			errors[file] = "not a file"
			continue
		}

		// Check that the target location of where we are supposed to restore
		// either does not exist, or is actually a file.
		if info, err := filesystem.Lstat(file); err == nil && !info.IsRegular() {
			errors[file] = "cannot replace a non-file"
			continue
		} else if err != nil && !fs.IsNotExist(err) {
			errors[file] = err.Error()
			continue
		}

		restore[file] = versionedTaggedFilename
	}

	// Execution
	var err error
	for target, source := range restore {
		err = nil
		if _, serr := filesystem.Lstat(target); serr == nil {
			if ver != nil {
				err = osutil.InWritableDir(ver.Archive, filesystem, target)
			} else {
				err = osutil.InWritableDir(filesystem.Remove, filesystem, target)
			}
		}

		filesystem.MkdirAll(filepath.Dir(target), 0755)
		if err == nil {
			err = osutil.Copy(filesystem, source, target)
		}

		if err != nil {
			errors[target] = err.Error()
			continue
		}
	}

	// Trigger scan
	if !fcfg.FSWatcherEnabled {
		m.ScanFolder(folder)
	}

	return errors, nil
}

func (m *Model) Availability(folder string, file protocol.FileInfo, block protocol.BlockInfo) []Availability {
	// The slightly unusual locking sequence here is because we need to hold
	// pmut for the duration (as the value returned from foldersFiles can
	// get heavily modified on Close()), but also must acquire fmut before
	// pmut. (The locks can be *released* in any order.)
	m.fmut.RLock()
	m.pmut.RLock()
	defer m.pmut.RUnlock()

	fs, ok := m.folderFiles[folder]
	cfg := m.folderCfgs[folder]
	m.fmut.RUnlock()

	if !ok {
		return nil
	}

	var availabilities []Availability
next:
	for _, device := range fs.Availability(file.Name) {
		for _, pausedFolder := range m.remotePausedFolders[device] {
			if pausedFolder == folder {
				continue next
			}
		}
		_, ok := m.conn[device]
		if ok {
			availabilities = append(availabilities, Availability{ID: device, FromTemporary: false})
		}
	}

	for _, device := range cfg.Devices {
		if m.deviceDownloads[device.DeviceID].Has(folder, file.Name, file.Version, int32(block.Offset/int64(file.BlockSize()))) {
			availabilities = append(availabilities, Availability{ID: device.DeviceID, FromTemporary: true})
		}
	}

	return availabilities
}

// BringToFront bumps the given files priority in the job queue.
func (m *Model) BringToFront(folder, file string) {
	m.pmut.RLock()
	defer m.pmut.RUnlock()

	runner, ok := m.folderRunners[folder]
	if ok {
		runner.BringToFront(file)
	}
}

func (m *Model) ResetFolder(folder string) {
	l.Infof("Cleaning data for folder %q", folder)
	db.DropFolder(m.db, folder)
}

func (m *Model) String() string {
	return fmt.Sprintf("model@%p", m)
}

func (m *Model) VerifyConfiguration(from, to config.Configuration) error {
	return nil
}

func (m *Model) CommitConfiguration(from, to config.Configuration) bool {
	// TODO: This should not use reflect, and should take more care to try to handle stuff without restart.

	// Go through the folder configs and figure out if we need to restart or not.

	fromFolders := mapFolders(from.Folders)
	toFolders := mapFolders(to.Folders)
	for folderID, cfg := range toFolders {
		if _, ok := fromFolders[folderID]; !ok {
			// A folder was added.
			if cfg.Paused {
				l.Infoln("Paused folder", cfg.Description())
			} else {
				l.Infoln("Adding folder", cfg.Description())
				m.AddFolder(cfg)
				m.StartFolder(folderID)
			}
		}
	}

	for folderID, fromCfg := range fromFolders {
		toCfg, ok := toFolders[folderID]
		if !ok {
			// The folder was removed.
			m.RemoveFolder(fromCfg)
			continue
		}

		// This folder exists on both sides. Settings might have changed.
		// Check if anything differs that requires a restart.
		if !reflect.DeepEqual(fromCfg.RequiresRestartOnly(), toCfg.RequiresRestartOnly()) {
			m.RestartFolder(fromCfg, toCfg)
		}

		// Emit the folder pause/resume event
		if fromCfg.Paused != toCfg.Paused {
			eventType := events.FolderResumed
			if toCfg.Paused {
				eventType = events.FolderPaused
			}
			events.Default.Log(eventType, map[string]string{"id": toCfg.ID, "label": toCfg.Label})
		}
	}

	// Removing a device. We actually don't need to do anything.
	// Because folder config has changed (since the device lists do not match)
	// Folders for that had device got "restarted", which involves killing
	// connections to all devices that we were sharing the folder with.
	// At some point model.Close() will get called for that device which will
	// clean residue device state that is not part of any folder.

	// Pausing a device, unpausing is handled by the connection service.
	fromDevices := from.DeviceMap()
	toDevices := to.DeviceMap()
	for deviceID, toCfg := range toDevices {
		fromCfg, ok := fromDevices[deviceID]
		if !ok || fromCfg.Paused == toCfg.Paused {
			continue
		}

		// Ignored folder was removed, reconnect to retrigger the prompt.
		if len(fromCfg.IgnoredFolders) > len(toCfg.IgnoredFolders) {
			m.close(deviceID)
		}

		if toCfg.Paused {
			l.Infoln("Pausing", deviceID)
			m.close(deviceID)
			events.Default.Log(events.DevicePaused, map[string]string{"device": deviceID.String()})
		} else {
			events.Default.Log(events.DeviceResumed, map[string]string{"device": deviceID.String()})
		}
	}

	scanLimiter.setCapacity(to.Options.MaxConcurrentScans)

	// Some options don't require restart as those components handle it fine
	// by themselves. Compare the options structs containing only the
	// attributes that require restart and act apprioriately.
	if !reflect.DeepEqual(from.Options.RequiresRestartOnly(), to.Options.RequiresRestartOnly()) {
		l.Debugln(m, "requires restart, options differ")
		return false
	}

	return true
}

// checkFolderRunningLocked returns nil if the folder is up and running and a
// descriptive error if not.
// Need to hold (read) lock on m.fmut when calling this.
func (m *Model) checkFolderRunningLocked(folder string) error {
	_, ok := m.folderRunners[folder]
	if ok {
		return nil
	}

	if cfg, ok := m.cfg.Folder(folder); !ok {
		return errFolderMissing
	} else if cfg.Paused {
		return ErrFolderPaused
	}

	return errFolderNotRunning
}

// checkFolderDeviceStatusLocked first checks the folder and then whether the
// given device is connected and shares this folder.
// Need to hold (read) lock on both m.fmut and m.pmut when calling this.
func (m *Model) checkDeviceFolderConnectedLocked(device protocol.DeviceID, folder string) error {
	if err := m.checkFolderRunningLocked(folder); err != nil {
		return err
	}

	if cfg, ok := m.cfg.Device(device); !ok {
		return errDeviceUnknown
	} else if cfg.Paused {
		return errDevicePaused
	}

	if _, ok := m.conn[device]; !ok {
		return errors.New("device is not connected")
	}

	if cfg, ok := m.cfg.Folder(folder); !ok || !cfg.SharedWith(device) {
		return errors.New("folder is not shared with device")
	}
	return nil
}

// mapFolders returns a map of folder ID to folder configuration for the given
// slice of folder configurations.
func mapFolders(folders []config.FolderConfiguration) map[string]config.FolderConfiguration {
	m := make(map[string]config.FolderConfiguration, len(folders))
	for _, cfg := range folders {
		m[cfg.ID] = cfg
	}
	return m
}

// mapDevices returns a map of device ID to nothing for the given slice of
// device IDs.
func mapDevices(devices []protocol.DeviceID) map[protocol.DeviceID]struct{} {
	m := make(map[protocol.DeviceID]struct{}, len(devices))
	for _, dev := range devices {
		m[dev] = struct{}{}
	}
	return m
}

// Skips `skip` elements and retrieves up to `get` elements from a given slice.
// Returns the resulting slice, plus how much elements are left to skip or
// copy to satisfy the values which were provided, given the slice is not
// big enough.
func getChunk(data []string, skip, get int) ([]string, int, int) {
	l := len(data)
	if l <= skip {
		return []string{}, skip - l, get
	} else if l < skip+get {
		return data[skip:l], 0, get - (l - skip)
	}
	return data[skip : skip+get], 0, 0
}

func closeRawConn(conn io.Closer) error {
	if conn, ok := conn.(*tls.Conn); ok {
		// If the underlying connection is a *tls.Conn, Close() does more
		// than it says on the tin. Specifically, it sends a TLS alert
		// message, which might block forever if the connection is dead
		// and we don't have a deadline set.
		conn.SetWriteDeadline(time.Now().Add(250 * time.Millisecond))
	}
	return conn.Close()
}

func stringSliceWithout(ss []string, s string) []string {
	for i := range ss {
		if ss[i] == s {
			copy(ss[i:], ss[i+1:])
			ss = ss[:len(ss)-1]
			return ss
		}
	}
	return ss
}

func readOffsetIntoBuf(fs fs.Filesystem, file string, offset int64, buf []byte) error {
	fd, err := fs.Open(file)
	if err != nil {
		l.Debugln("readOffsetIntoBuf.Open", file, err)
		return err
	}

	defer fd.Close()
	_, err = fd.ReadAt(buf, offset)
	if err != nil {
		l.Debugln("readOffsetIntoBuf.ReadAt", file, err)
	}
	return err
}

// makeForgetUpdate takes an index update and constructs a download progress update
// causing to forget any progress for files which we've just been sent.
func makeForgetUpdate(files []protocol.FileInfo) []protocol.FileDownloadProgressUpdate {
	updates := make([]protocol.FileDownloadProgressUpdate, 0, len(files))
	for _, file := range files {
		if file.IsSymlink() || file.IsDirectory() || file.IsDeleted() {
			continue
		}
		updates = append(updates, protocol.FileDownloadProgressUpdate{
			Name:       file.Name,
			Version:    file.Version,
			UpdateType: protocol.UpdateTypeForget,
		})
	}
	return updates
}

// folderDeviceSet is a set of (folder, deviceID) pairs
type folderDeviceSet map[string]map[protocol.DeviceID]struct{}

// set adds the (dev, folder) pair to the set
func (s folderDeviceSet) set(dev protocol.DeviceID, folder string) {
	devs, ok := s[folder]
	if !ok {
		devs = make(map[protocol.DeviceID]struct{})
		s[folder] = devs
	}
	devs[dev] = struct{}{}
}

// has returns true if the (dev, folder) pair is in the set
func (s folderDeviceSet) has(dev protocol.DeviceID, folder string) bool {
	_, ok := s[folder][dev]
	return ok
}

// hasDevice returns true if the device is set on any folder
func (s folderDeviceSet) hasDevice(dev protocol.DeviceID) bool {
	for _, devices := range s {
		if _, ok := devices[dev]; ok {
			return true
		}
	}
	return false
}

type fileInfoBatch struct {
	infos   []protocol.FileInfo
	size    int
	flushFn func([]protocol.FileInfo) error
}

func newFileInfoBatch(fn func([]protocol.FileInfo) error) *fileInfoBatch {
	return &fileInfoBatch{
		infos:   make([]protocol.FileInfo, 0, maxBatchSizeFiles),
		flushFn: fn,
	}
}

func (b *fileInfoBatch) append(f protocol.FileInfo) {
	b.infos = append(b.infos, f)
	b.size += f.ProtoSize()
}

func (b *fileInfoBatch) flushIfFull() error {
	if len(b.infos) == maxBatchSizeFiles || b.size > maxBatchSizeBytes {
		return b.flush()
	}
	return nil
}

func (b *fileInfoBatch) flush() error {
	if len(b.infos) == 0 {
		return nil
	}
	if err := b.flushFn(b.infos); err != nil {
		return err
	}
	b.reset()
	return nil
}

func (b *fileInfoBatch) reset() {
	b.infos = b.infos[:0]
	b.size = 0
}

// syncMutexMap is a type safe wrapper for a sync.Map that holds mutexes
type syncMutexMap struct {
	inner stdsync.Map
}

func (m *syncMutexMap) Get(key string) sync.Mutex {
	v, _ := m.inner.LoadOrStore(key, sync.NewMutex())
	return v.(sync.Mutex)
}

// sanitizePath takes a string that might contain all kinds of special
// characters and makes a valid, similar, path name out of it.
//
// Spans of invalid characters are replaced by a single space. Invalid
// characters are control characters, the things not allowed in file names
// in Windows, and common shell metacharacters. Even if asterisks and pipes
// and stuff are allowed on Unixes in general they might not be allowed by
// the filesystem and may surprise the user and cause shell oddness. This
// function is intended for file names we generate on behalf of the user,
// and surprising them with odd shell characters in file names is unkind.
//
// We include whitespace in the invalid characters so that multiple
// whitespace is collapsed to a single space. Additionally, whitespace at
// either end is removed.
func sanitizePath(path string) string {
	invalid := regexp.MustCompile(`([[:cntrl:]]|[<>:"'/\\|?*\n\r\t \[\]\{\};:!@$%&^#])+`)
	return strings.TrimSpace(invalid.ReplaceAllString(path, " "))
}
