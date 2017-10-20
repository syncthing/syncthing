// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
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
	"github.com/syncthing/syncthing/lib/weakhash"
	"github.com/thejerf/suture"
)

// How many files to send in each Index/IndexUpdate message.
const (
	maxBatchSizeBytes = 250 * 1024 // Aim for making index messages no larger than 250 KiB (uncompressed)
	maxBatchSizeFiles = 1000       // Either way, don't include more files than this
)

type service interface {
	BringToFront(string)
	DelayScan(d time.Duration)
	IndexUpdated()              // Remote index was updated notification
	IgnoresUpdated()            // ignore matcher was updated notification
	Jobs() ([]string, []string) // In progress, Queued
	Scan(subs []string) error
	Serve()
	Stop()
	BlockStats() map[string]int

	getState() (folderState, time.Time, error)
	setState(state folderState)
	clearError()
	setError(err error)
}

type Availability struct {
	ID            protocol.DeviceID `json:"id"`
	FromTemporary bool              `json:"fromTemporary"`
}

type Model struct {
	*suture.Supervisor

	cfg               *config.Wrapper
	db                *db.Instance
	finder            *db.BlockFinder
	progressEmitter   *ProgressEmitter
	id                protocol.DeviceID
	shortID           protocol.ShortID
	cacheIgnoredFiles bool
	protectedFiles    []string

	clientName    string
	clientVersion string

	folderCfgs         map[string]config.FolderConfiguration                  // folder -> cfg
	folderFiles        map[string]*db.FileSet                                 // folder -> files
	folderDevices      folderDeviceSet                                        // folder -> deviceIDs
	deviceFolders      map[protocol.DeviceID][]string                         // deviceID -> folders
	deviceStatRefs     map[protocol.DeviceID]*stats.DeviceStatisticsReference // deviceID -> statsRef
	folderIgnores      map[string]*ignore.Matcher                             // folder -> matcher object
	folderRunners      map[string]service                                     // folder -> puller or scanner
	folderRunnerTokens map[string][]suture.ServiceToken                       // folder -> tokens for puller or scanner
	folderStatRefs     map[string]*stats.FolderStatisticsReference            // folder -> statsRef
	fmut               sync.RWMutex                                           // protects the above

	conn                map[protocol.DeviceID]connections.Connection
	closed              map[protocol.DeviceID]chan struct{}
	helloMessages       map[protocol.DeviceID]protocol.HelloResult
	deviceDownloads     map[protocol.DeviceID]*deviceDownloadState
	remotePausedFolders map[protocol.DeviceID][]string // deviceID -> folders
	pmut                sync.RWMutex                   // protects the above
}

type folderFactory func(*Model, config.FolderConfiguration, versioner.Versioner, fs.Filesystem) service

var (
	folderFactories = make(map[config.FolderType]folderFactory, 0)
)

var (
	errFolderPathMissing   = errors.New("folder path missing")
	errFolderMarkerMissing = errors.New("folder marker missing")
	errDeviceUnknown       = errors.New("unknown device")
	errDevicePaused        = errors.New("device is paused")
	errDeviceIgnored       = errors.New("device is ignored")
	errFolderPaused        = errors.New("folder is paused")
	errFolderMissing       = errors.New("no such folder")
	errNetworkNotAllowed   = errors.New("network not allowed")
)

// NewModel creates and starts a new model. The model starts in read-only mode,
// where it sends index information to connected peers and responds to requests
// for file data without altering the local folder in any way.
func NewModel(cfg *config.Wrapper, id protocol.DeviceID, clientName, clientVersion string, ldb *db.Instance, protectedFiles []string) *Model {
	m := &Model{
		Supervisor: suture.New("model", suture.Spec{
			Log: func(line string) {
				l.Debugln(line)
			},
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
		folderDevices:       make(folderDeviceSet),
		deviceFolders:       make(map[protocol.DeviceID][]string),
		deviceStatRefs:      make(map[protocol.DeviceID]*stats.DeviceStatisticsReference),
		folderIgnores:       make(map[string]*ignore.Matcher),
		folderRunners:       make(map[string]service),
		folderRunnerTokens:  make(map[string][]suture.ServiceToken),
		folderStatRefs:      make(map[string]*stats.FolderStatisticsReference),
		conn:                make(map[protocol.DeviceID]connections.Connection),
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
	cfg, ok := m.folderCfgs[folder]
	if !ok {
		panic("cannot start nonexistent folder " + cfg.Description())
	}

	_, ok = m.folderRunners[folder]
	if ok {
		panic("cannot start already running folder " + cfg.Description())
	}

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
			fs.Replace(available, nil)
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

		// Directory permission bits. Will be filtered down to something
		// sane by umask on Unixes.

		cfg.CreateRoot()

		if err := cfg.CreateMarker(); err != nil {
			l.Warnln("Creating folder marker:", err)
		}
	}

	var ver versioner.Versioner
	if len(cfg.Versioning.Type) > 0 {
		versionerFactory, ok := versioner.Factories[cfg.Versioning.Type]
		if !ok {
			l.Fatalf("Requested versioning type %q that does not exist", cfg.Versioning.Type)
		}

		ver = versionerFactory(folder, cfg.Filesystem(), cfg.Versioning.Params)
		if service, ok := ver.(suture.Service); ok {
			// The versioner implements the suture.Service interface, so
			// expects to be run in the background in addition to being called
			// when files are going to be archived.
			token := m.Add(service)
			m.folderRunnerTokens[folder] = append(m.folderRunnerTokens[folder], token)
		}
	}

	ffs := fs.MtimeFS()

	// These are our metadata files, and they should always be hidden.
	ffs.Hide(".stfolder")
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
		if !strings.HasPrefix(protectedFilePath, folderLocation) {
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

	for _, device := range cfg.Devices {
		m.folderDevices.set(device.DeviceID, cfg.ID)
		m.deviceFolders[device.DeviceID] = append(m.deviceFolders[device.DeviceID], cfg.ID)
	}

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
	cfg.Filesystem().RemoveAll(".stfolder")

	m.tearDownFolderLocked(cfg.ID)
	// Remove it from the database
	db.DropFolder(m.db, cfg.ID)

	m.pmut.Unlock()
	m.fmut.Unlock()
}

func (m *Model) tearDownFolderLocked(folder string) {
	// Stop the services running for this folder
	for _, id := range m.folderRunnerTokens[folder] {
		m.Remove(id)
	}

	// Close connections to affected devices
	for dev := range m.folderDevices[folder] {
		if conn, ok := m.conn[dev]; ok {
			closeRawConn(conn)
		}
	}

	// Clean up our config maps
	delete(m.folderCfgs, folder)
	delete(m.folderFiles, folder)
	delete(m.folderDevices, folder)
	delete(m.folderIgnores, folder)
	delete(m.folderRunners, folder)
	delete(m.folderRunnerTokens, folder)
	delete(m.folderStatRefs, folder)
	for dev, folders := range m.deviceFolders {
		m.deviceFolders[dev] = stringSliceWithout(folders, folder)
	}
}

func (m *Model) RestartFolder(cfg config.FolderConfiguration) {
	if len(cfg.ID) == 0 {
		panic("cannot add empty folder id")
	}

	m.fmut.Lock()
	m.pmut.Lock()

	m.tearDownFolderLocked(cfg.ID)
	if !cfg.Paused {
		m.addFolderLocked(cfg)
		folderType := m.startFolderLocked(cfg.ID)
		l.Infoln("Restarted folder", cfg.Description(), fmt.Sprintf("(%s)", folderType))
	} else {
		l.Infoln("Paused folder", cfg.Description())
	}

	m.pmut.Unlock()
	m.fmut.Unlock()
}

func (m *Model) UsageReportingStats(version int) map[string]interface{} {
	stats := make(map[string]interface{})
	if version >= 3 {
		// Block stats
		m.fmut.Lock()
		blockStats := make(map[string]int)
		for _, folder := range m.folderRunners {
			for k, v := range folder.BlockStats() {
				blockStats[k] += v
			}
		}
		m.fmut.Unlock()
		stats["blockStats"] = blockStats

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
	GlobalBytes   int64
	NeedDeletes   int64
}

// Completion returns the completion status, in percent, for the given device
// and folder.
func (m *Model) Completion(device protocol.DeviceID, folder string) FolderCompletion {
	m.fmut.RLock()
	rf, ok := m.folderFiles[folder]
	ignores := m.folderIgnores[folder]
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

	var need, fileNeed, downloaded, deletes int64
	rf.WithNeedTruncated(device, func(f db.FileIntf) bool {
		if ignores.Match(f.FileName()).IsIgnored() {
			return true
		}

		ft := f.(db.FileInfoTruncated)

		// If the file is deleted, we account it only in the deleted column.
		if ft.Deleted {
			deletes++
			return true
		}

		// This might might be more than it really is, because some blocks can be of a smaller size.
		downloaded = int64(counts[ft.Name] * protocol.BlockSize)

		fileNeed = ft.FileSize() - downloaded
		if fileNeed < 0 {
			fileNeed = 0
		}

		need += fileNeed
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

// NeedSize returns the number and total size of currently needed files.
func (m *Model) NeedSize(folder string) db.Counts {
	m.fmut.RLock()
	defer m.fmut.RUnlock()

	var result db.Counts
	if rf, ok := m.folderFiles[folder]; ok {
		ignores := m.folderIgnores[folder]
		cfg := m.folderCfgs[folder]
		rf.WithNeedTruncated(protocol.LocalDeviceID, func(f db.FileIntf) bool {
			if shouldIgnore(f, ignores, cfg.IgnoreDelete) {
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
func (m *Model) NeedFolderFiles(folder string, page, perpage int) ([]db.FileInfoTruncated, []db.FileInfoTruncated, []db.FileInfoTruncated, int) {
	m.fmut.RLock()
	defer m.fmut.RUnlock()

	total := 0

	rf, ok := m.folderFiles[folder]
	if !ok {
		return nil, nil, nil, 0
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
	ignores := m.folderIgnores[folder]
	cfg := m.folderCfgs[folder]
	rf.WithNeedTruncated(protocol.LocalDeviceID, func(f db.FileIntf) bool {
		if shouldIgnore(f, ignores, cfg.IgnoreDelete) {
			return true
		}

		total++
		if skip > 0 {
			skip--
			return true
		}
		if get > 0 {
			ft := f.(db.FileInfoTruncated)
			if _, ok := seen[ft.Name]; !ok {
				rest = append(rest, ft)
				get--
			}
		}
		return true
	})

	return progress, queued, rest, total
}

// Index is called when a new device is connected and we receive their full index.
// Implements the protocol.Model interface.
func (m *Model) Index(deviceID protocol.DeviceID, folder string, fs []protocol.FileInfo) {
	l.Debugf("IDX(in): %s %q: %d files", deviceID, folder, len(fs))

	if !m.folderSharedWith(folder, deviceID) {
		l.Debugf("Unexpected folder ID %q sent from device %q; ensure that the folder exists and that this device is selected under \"Share With\" in the folder configuration.", folder, deviceID)
		return
	}

	m.fmut.RLock()
	files, ok := m.folderFiles[folder]
	runner := m.folderRunners[folder]
	m.fmut.RUnlock()

	if !ok {
		l.Fatalf("Index for nonexistent folder %q", folder)
	}

	if runner != nil {
		// Runner may legitimately not be set if this is the "cleanup" Index
		// message at startup.
		defer runner.IndexUpdated()
	}

	m.pmut.RLock()
	m.deviceDownloads[deviceID].Update(folder, makeForgetUpdate(fs))
	m.pmut.RUnlock()

	files.Replace(deviceID, fs)

	events.Default.Log(events.RemoteIndexUpdated, map[string]interface{}{
		"device":  deviceID.String(),
		"folder":  folder,
		"items":   len(fs),
		"version": files.Sequence(deviceID),
	})
}

// IndexUpdate is called for incremental updates to connected devices' indexes.
// Implements the protocol.Model interface.
func (m *Model) IndexUpdate(deviceID protocol.DeviceID, folder string, fs []protocol.FileInfo) {
	l.Debugf("%v IDXUP(in): %s / %q: %d files", m, deviceID, folder, len(fs))

	if !m.folderSharedWith(folder, deviceID) {
		l.Debugf("Update for unexpected folder ID %q sent from device %q; ensure that the folder exists and that this device is selected under \"Share With\" in the folder configuration.", folder, deviceID)
		return
	}

	m.fmut.RLock()
	files := m.folderFiles[folder]
	runner, ok := m.folderRunners[folder]
	m.fmut.RUnlock()

	if !ok {
		l.Fatalf("IndexUpdate for nonexistent folder %q", folder)
	}

	m.pmut.RLock()
	m.deviceDownloads[deviceID].Update(folder, makeForgetUpdate(fs))
	m.pmut.RUnlock()

	files.Update(deviceID, fs)

	events.Default.Log(events.RemoteIndexUpdated, map[string]interface{}{
		"device":  deviceID.String(),
		"folder":  folder,
		"items":   len(fs),
		"version": files.Sequence(deviceID),
	})

	runner.IndexUpdated()
}

func (m *Model) folderSharedWith(folder string, deviceID protocol.DeviceID) bool {
	m.fmut.RLock()
	shared := m.folderSharedWithLocked(folder, deviceID)
	m.fmut.RUnlock()
	return shared
}

func (m *Model) folderSharedWithLocked(folder string, deviceID protocol.DeviceID) bool {
	for _, nfolder := range m.deviceFolders[deviceID] {
		if nfolder == folder {
			return true
		}
	}
	return false
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

	// See issue #3802 - in short, we can't send modern symlink entries to older
	// clients.
	dropSymlinks := false
	if hello.ClientName == m.clientName && upgrade.CompareVersions(hello.ClientVersion, "v0.14.14") < 0 {
		l.Warnln("Not sending symlinks to old client", deviceID, "- please upgrade to v0.14.14 or newer")
		dropSymlinks = true
	}

	m.fmut.Lock()
	var paused []string
	for _, folder := range cm.Folders {
		if folder.Paused {
			paused = append(paused, folder.ID)
			continue
		}

		if cfg, ok := m.cfg.Folder(folder.ID); ok && cfg.Paused {
			continue
		}

		if m.cfg.IgnoredFolder(folder.ID) {
			l.Infof("Ignoring folder %s from device %s since we are configured to", folder.Description(), deviceID)
			continue
		}

		if !m.folderSharedWithLocked(folder.ID, deviceID) {
			events.Default.Log(events.FolderRejected, map[string]string{
				"folder":      folder.ID,
				"folderLabel": folder.Label,
				"device":      deviceID.String(),
			})
			l.Infof("Unexpected folder %s sent from device %q; ensure that the folder exists and that this device is selected under \"Share With\" in the folder configuration.", folder.Description(), deviceID)
			continue
		}
		if !folder.DisableTempIndexes {
			tempIndexFolders = append(tempIndexFolders, folder.ID)
		}

		fs := m.folderFiles[folder.ID]
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
					fs.Replace(deviceID, nil)
				} else if dev.IndexID != theirIndexID {
					// The index ID we have on file is not what they're
					// announcing. They must have reset their database and
					// will probably send us a full index. We drop any
					// information we have and remember this new index ID
					// instead.
					l.Infof("Device %v folder %s has a new index ID (%v)", deviceID, folder.Description(), dev.IndexID)
					fs.Replace(deviceID, nil)
					fs.SetIndexID(deviceID, dev.IndexID)
				} else {
					// They're sending a recognized index ID and will most
					// likely use delta indexes. We might already have files
					// that we need to pull so let the folder runner know
					// that it should recheck the index data.
					if runner := m.folderRunners[folder.ID]; runner != nil {
						defer runner.IndexUpdated()
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

	var changed = false
	if deviceCfg := m.cfg.Devices()[deviceID]; deviceCfg.Introducer {
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
		// We don't have this folder, skip.
		if _, ok := m.folderDevices[folder.ID]; !ok {
			continue
		}

		// Adds devices which we do not have, but the introducer has
		// for the folders that we have in common. Also, shares folders
		// with devices that we have in common, yet are currently not sharing
		// the folder.
	nextDevice:
		for _, device := range folder.Devices {
			foldersDevices.set(device.ID, folder.ID)

			if _, ok := m.cfg.Devices()[device.ID]; !ok {
				// The device is currently unknown. Add it to the config.
				m.introduceDevice(device, introducerCfg)
				changed = true
			}

			for _, er := range m.deviceFolders[device.ID] {
				if er == folder.ID {
					// We already share the folder with this device, so
					// nothing to do.
					continue nextDevice
				}
			}

			// We don't yet share this folder with this device. Add the device
			// to sharing list of the folder.
			m.introduceDeviceToFolder(device, folder, introducerCfg)
			changed = true
		}
	}

	return foldersDevices, changed
}

// handleIntroductions handles removals of devices/shares that are removed by an introducer device
func (m *Model) handleDeintroductions(introducerCfg config.DeviceConfiguration, cm protocol.ClusterConfig, foldersDevices folderDeviceSet) bool {
	changed := false
	foldersIntroducedByOthers := make(folderDeviceSet)

	// Check if we should unshare some folders, if the introducer has unshared them.
	for _, folderCfg := range m.cfg.Folders() {
		folderChanged := false
		for i := 0; i < len(folderCfg.Devices); i++ {
			if folderCfg.Devices[i].IntroducedBy == introducerCfg.DeviceID {
				if !foldersDevices.has(folderCfg.Devices[i].DeviceID, folderCfg.ID) {
					// We could not find that folder shared on the
					// introducer with the device that was introduced to us.
					// We should follow and unshare aswell.
					l.Infof("Unsharing folder %s with %v as introducer %v no longer shares the folder with that device", folderCfg.Description(), folderCfg.Devices[i].DeviceID, folderCfg.Devices[i].IntroducedBy)
					folderCfg.Devices = append(folderCfg.Devices[:i], folderCfg.Devices[i+1:]...)
					i--
					folderChanged = true
				}
			} else {
				foldersIntroducedByOthers.set(folderCfg.Devices[i].DeviceID, folderCfg.ID)
			}
		}

		// We've modified the folder, hence update it.
		if folderChanged {
			m.cfg.SetFolder(folderCfg)
			changed = true
		}
	}

	// Check if we should remove some devices, if the introducer no longer
	// shares any folder with them. Yet do not remove if we share other
	// folders that haven't been introduced by the introducer.
	for _, device := range m.cfg.Devices() {
		if device.IntroducedBy == introducerCfg.DeviceID {
			if !foldersDevices.hasDevice(device.DeviceID) {
				if foldersIntroducedByOthers.hasDevice(device.DeviceID) {
					l.Infof("Would have removed %v as %v no longer shares any folders, yet there are other folders that are shared with this device that haven't been introduced by this introducer.", device.DeviceID, device.IntroducedBy)
					continue
				}
				// The introducer no longer shares any folder with the
				// device, remove the device.
				l.Infof("Removing device %v as introducer %v no longer shares any folders with that device", device.DeviceID, device.IntroducedBy)
				m.cfg.RemoveDevice(device.DeviceID)
				changed = true
			}
		}
	}

	return changed
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

func (m *Model) introduceDeviceToFolder(device protocol.Device, folder protocol.Folder, introducerCfg config.DeviceConfiguration) {
	l.Infof("Sharing folder %s with %v (vouched for by introducer %v)", folder.Description(), device.ID, introducerCfg.DeviceID)

	m.deviceFolders[device.ID] = append(m.deviceFolders[device.ID], folder.ID)
	m.folderDevices.set(device.ID, folder.ID)

	folderCfg := m.cfg.Folders()[folder.ID]
	folderCfg.Devices = append(folderCfg.Devices, config.FolderDeviceConfiguration{
		DeviceID:     device.ID,
		IntroducedBy: introducerCfg.DeviceID,
	})
	m.cfg.SetFolder(folderCfg)
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
	delete(m.helloMessages, device)
	delete(m.deviceDownloads, device)
	delete(m.remotePausedFolders, device)
	closed := m.closed[device]
	delete(m.closed, device)
	m.pmut.Unlock()

	l.Infof("Connection to %s closed: %v", device, err)
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

// Request returns the specified data segment by reading it from local disk.
// Implements the protocol.Model interface.
func (m *Model) Request(deviceID protocol.DeviceID, folder, name string, offset int64, hash []byte, fromTemporary bool, buf []byte) error {
	if offset < 0 {
		return protocol.ErrInvalid
	}

	if !m.folderSharedWith(folder, deviceID) {
		l.Warnf("Request from %s for file %s in unshared folder %q", deviceID, name, folder)
		return protocol.ErrNoSuchFile
	}
	if deviceID != protocol.LocalDeviceID {
		l.Debugf("%v REQ(in): %s: %q / %q o=%d s=%d t=%v", m, deviceID, folder, name, offset, len(buf), fromTemporary)
	}
	m.fmut.RLock()
	folderCfg := m.folderCfgs[folder]
	folderIgnores := m.folderIgnores[folder]
	m.fmut.RUnlock()

	folderFs := folderCfg.Filesystem()

	// Having passed the rootedJoinedPath check above, we know "name" is
	// acceptable relative to "folderPath" and in canonical form, so we can
	// trust it.

	if fs.IsInternal(name) {
		l.Debugf("%v REQ(in) for internal file: %s: %q / %q o=%d s=%d", m, deviceID, folder, name, offset, len(buf))
		return protocol.ErrNoSuchFile
	}

	if folderIgnores.Match(name).IsIgnored() {
		l.Debugf("%v REQ(in) for ignored file: %s: %q / %q o=%d s=%d", m, deviceID, folder, name, offset, len(buf))
		return protocol.ErrNoSuchFile
	}

	if err := osutil.TraversesSymlink(folderFs, filepath.Dir(name)); err != nil {
		l.Debugf("%v REQ(in) traversal check: %s - %s: %q / %q o=%d s=%d", m, err, deviceID, folder, name, offset, len(buf))
		return protocol.ErrNoSuchFile
	}

	// Only check temp files if the flag is set, and if we are set to advertise
	// the temp indexes.
	if fromTemporary && !folderCfg.DisableTempIndexes {
		tempFn := fs.TempName(name)

		if info, err := folderFs.Lstat(tempFn); err != nil || !info.IsRegular() {
			// Reject reads for anything that doesn't exist or is something
			// other than a regular file.
			return protocol.ErrNoSuchFile
		}

		if err := readOffsetIntoBuf(folderFs, tempFn, offset, buf); err == nil {
			return nil
		}
		// Fall through to reading from a non-temp file, just incase the temp
		// file has finished downloading.
	}

	if info, err := folderFs.Lstat(name); err != nil || !info.IsRegular() {
		// Reject reads for anything that doesn't exist or is something
		// other than a regular file.
		return protocol.ErrNoSuchFile
	}

	err := readOffsetIntoBuf(folderFs, name, offset, buf)
	if fs.IsNotExist(err) {
		return protocol.ErrNoSuchFile
	} else if err != nil {
		return protocol.ErrGeneric
	}
	return nil
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

// ConnectedTo returns true if we are connected to the named device.
func (m *Model) ConnectedTo(deviceID protocol.DeviceID) bool {
	m.pmut.RLock()
	_, ok := m.conn[deviceID]
	m.pmut.RUnlock()
	if ok {
		m.deviceWasSeen(deviceID)
	}
	return ok
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

	if err := m.checkFolderPath(cfg); err != nil {
		return nil, nil, err
	}

	ignores, ok := m.folderIgnores[folder]
	if ok {
		return ignores.Lines(), ignores.Patterns(), nil
	}

	ignores = ignore.New(fs.NewFilesystem(cfg.FilesystemType, cfg.Path))
	if err := ignores.Load(".stignore"); err != nil && !fs.IsNotExist(err) {
		return nil, nil, err
	}

	return ignores.Lines(), ignores.Patterns(), nil
}

func (m *Model) SetIgnores(folder string, content []string) error {
	cfg, ok := m.cfg.Folders()[folder]
	if !ok {
		return fmt.Errorf("Folder %s does not exist", folder)
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

	l.Infof(`Device %s client is "%s %s" named "%s"`, deviceID, hello.ClientName, hello.ClientVersion, hello.DeviceName)

	conn.Start()
	m.pmut.Unlock()

	// Acquires fmut, so has to be done outside of pmut.
	cm := m.generateClusterConfig(deviceID)
	conn.ClusterConfig(cm)

	device, ok := m.cfg.Devices()[deviceID]
	if ok && (device.Name == "" || m.cfg.Options().OverwriteRemoteDevNames) {
		device.Name = hello.DeviceName
		m.cfg.SetDevice(device)
		m.cfg.Save()
	}

	m.deviceWasSeen(deviceID)
}

func (m *Model) DownloadProgress(device protocol.DeviceID, folder string, updates []protocol.FileDownloadProgressUpdate) {
	if !m.folderSharedWith(folder, device) {
		return
	}

	m.fmut.RLock()
	cfg, ok := m.folderCfgs[folder]
	m.fmut.RUnlock()

	if !ok || cfg.Type == config.FolderTypeSendOnly || cfg.DisableTempIndexes {
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

func sendIndexes(conn protocol.Connection, folder string, fs *db.FileSet, ignores *ignore.Matcher, startSequence int64, dbLocation string, dropSymlinks bool) {
	deviceID := conn.ID()
	name := conn.Name()
	var err error

	l.Debugf("sendIndexes for %s-%s/%q starting (slv=%d)", deviceID, name, folder, startSequence)
	defer l.Debugf("sendIndexes for %s-%s/%q exiting: %v", deviceID, name, folder, err)

	minSequence, err := sendIndexTo(startSequence, conn, folder, fs, ignores, dbLocation, dropSymlinks)

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
		if fs.Sequence(protocol.LocalDeviceID) <= minSequence {
			sub.Poll(time.Minute)
			continue
		}

		minSequence, err = sendIndexTo(minSequence, conn, folder, fs, ignores, dbLocation, dropSymlinks)

		// Wait a short amount of time before entering the next loop. If there
		// are continuous changes happening to the local index, this gives us
		// time to batch them up a little.
		time.Sleep(250 * time.Millisecond)
	}
}

func sendIndexTo(minSequence int64, conn protocol.Connection, folder string, fs *db.FileSet, ignores *ignore.Matcher, dbLocation string, dropSymlinks bool) (int64, error) {
	deviceID := conn.ID()
	name := conn.Name()
	batch := make([]protocol.FileInfo, 0, maxBatchSizeFiles)
	batchSizeBytes := 0
	initial := minSequence == 0
	maxSequence := minSequence
	var err error

	sorter := NewIndexSorter(dbLocation)
	defer sorter.Close()

	fs.WithHave(protocol.LocalDeviceID, func(fi db.FileIntf) bool {
		f := fi.(protocol.FileInfo)
		if f.Sequence <= minSequence {
			return true
		}

		if f.Sequence > maxSequence {
			maxSequence = f.Sequence
		}

		if dropSymlinks && f.IsSymlink() {
			// Do not send index entries with symlinks to clients that can't
			// handle it. Fixes issue #3802. Once both sides are upgraded, a
			// rescan (i.e., change) of the symlink is required for it to
			// sync again, due to delta indexes.
			return true
		}

		sorter.Append(f)
		return true
	})

	sorter.Sorted(func(f protocol.FileInfo) bool {
		if len(batch) == maxBatchSizeFiles || batchSizeBytes > maxBatchSizeBytes {
			if initial {
				if err = conn.Index(folder, batch); err != nil {
					return false
				}
				l.Debugf("sendIndexes for %s-%s/%q: %d files (<%d bytes) (initial index)", deviceID, name, folder, len(batch), batchSizeBytes)
				initial = false
			} else {
				if err = conn.IndexUpdate(folder, batch); err != nil {
					return false
				}
				l.Debugf("sendIndexes for %s-%s/%q: %d files (<%d bytes) (batched update)", deviceID, name, folder, len(batch), batchSizeBytes)
			}

			batch = make([]protocol.FileInfo, 0, maxBatchSizeFiles)
			batchSizeBytes = 0
		}

		batch = append(batch, f)
		batchSizeBytes += f.ProtoSize()
		return true
	})

	if initial && err == nil {
		err = conn.Index(folder, batch)
		if err == nil {
			l.Debugf("sendIndexes for %s-%s/%q: %d files (small initial index)", deviceID, name, folder, len(batch))
		}
	} else if len(batch) > 0 && err == nil {
		err = conn.IndexUpdate(folder, batch)
		if err == nil {
			l.Debugf("sendIndexes for %s-%s/%q: %d files (last batch)", deviceID, name, folder, len(batch))
		}
	}

	return maxSequence, err
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
		objType := "file"
		action := "modified"

		// If our local vector is version 1 AND it is the only version
		// vector so far seen for this file then it is a new file.  Else if
		// it is > 1 it's not new, and if it is 1 but another shortId
		// version vector exists then it is new for us but created elsewhere
		// so the file is still not new but modified by us. Only if it is
		// truly new do we change this to 'added', else we leave it as
		// 'modified'.
		if len(file.Version.Counters) == 1 && file.Version.Counters[0].Value == 1 {
			action = "added"
		}

		if file.IsDirectory() {
			objType = "dir"
		}
		if file.IsDeleted() {
			action = "deleted"
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

func (m *Model) requestGlobal(deviceID protocol.DeviceID, folder, name string, offset int64, size int, hash []byte, fromTemporary bool) ([]byte, error) {
	m.pmut.RLock()
	nc, ok := m.conn[deviceID]
	m.pmut.RUnlock()

	if !ok {
		return nil, fmt.Errorf("requestGlobal: no such device: %s", deviceID)
	}

	l.Debugf("%v REQ(out): %s: %q / %q o=%d s=%d h=%x ft=%t", m, deviceID, folder, name, offset, size, hash, fromTemporary)

	return nc.Request(folder, name, offset, size, hash, fromTemporary)
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
				// the same one as returned by CheckFolderHealth, though
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
	m.fmut.Lock()
	runner, okRunner := m.folderRunners[folder]
	cfg, okCfg := m.folderCfgs[folder]
	m.fmut.Unlock()

	if !okRunner {
		if okCfg && cfg.Paused {
			return errFolderPaused
		}
		return errFolderMissing
	}

	return runner.Scan(subs)
}

func (m *Model) internalScanFolderSubdirs(ctx context.Context, folder string, subDirs []string) error {
	for i := 0; i < len(subDirs); i++ {
		sub := osutil.NativeFilename(subDirs[i])

		if sub == "" {
			// A blank subdirs means to scan the entire folder. We can trim
			// the subDirs list and go on our way.
			subDirs = nil
			break
		}

		// We test each path by joining with "root". What we join with is
		// not relevant, we just want the dotdot escape detection here. For
		// historical reasons we may get paths that end in a slash. We
		// remove that first to allow the rootedJoinedPath to pass.
		sub = strings.TrimRight(sub, string(fs.PathSeparator))
		subDirs[i] = sub
	}

	m.fmut.Lock()
	fset := m.folderFiles[folder]
	folderCfg := m.folderCfgs[folder]
	ignores := m.folderIgnores[folder]
	runner, ok := m.folderRunners[folder]
	m.fmut.Unlock()
	mtimefs := fset.MtimeFS()

	// Check if the ignore patterns changed as part of scanning this folder.
	// If they did we should schedule a pull of the folder so that we
	// request things we might have suddenly become unignored and so on.

	oldHash := ignores.Hash()
	defer func() {
		if ignores.Hash() != oldHash {
			l.Debugln("Folder", folder, "ignore patterns changed; triggering puller")
			runner.IgnoresUpdated()
		}
	}()

	if !ok {
		if folderCfg.Paused {
			return errFolderPaused
		}
		return errFolderMissing
	}

	if err := m.CheckFolderHealth(folder); err != nil {
		runner.setError(err)
		l.Infof("Stopping folder %s due to error: %s", folderCfg.Description(), err)
		return err
	}

	if err := ignores.Load(".stignore"); err != nil && !fs.IsNotExist(err) {
		err = fmt.Errorf("loading ignores: %v", err)
		runner.setError(err)
		l.Infof("Stopping folder %s due to error: %s", folderCfg.Description(), err)
		return err
	}

	// Clean the list of subitems to ensure that we start at a known
	// directory, and don't scan subdirectories of things we've already
	// scanned.
	subDirs = unifySubs(subDirs, func(f string) bool {
		_, ok := fset.Get(protocol.LocalDeviceID, f)
		return ok
	})

	runner.setState(FolderScanning)

	fchan, err := scanner.Walk(ctx, scanner.Config{
		Folder:                folderCfg.ID,
		Subs:                  subDirs,
		Matcher:               ignores,
		BlockSize:             protocol.BlockSize,
		TempLifetime:          time.Duration(m.cfg.Options().KeepTemporariesH) * time.Hour,
		CurrentFiler:          cFiler{m, folder},
		Filesystem:            mtimefs,
		IgnorePerms:           folderCfg.IgnorePerms,
		AutoNormalize:         folderCfg.AutoNormalize,
		Hashers:               m.numHashers(folder),
		ShortID:               m.shortID,
		ProgressTickIntervalS: folderCfg.ScanProgressIntervalS,
		UseWeakHashes:         weakhash.Enabled,
	})

	if err != nil {
		// The error we get here is likely an OS level error, which might not be
		// as readable as our health check errors. Check if we can get a health
		// check error first, and use that if it's available.
		if ferr := m.CheckFolderHealth(folder); ferr != nil {
			err = ferr
		}
		runner.setError(err)
		return err
	}

	batch := make([]protocol.FileInfo, 0, maxBatchSizeFiles)
	batchSizeBytes := 0

	for f := range fchan {
		if len(batch) == maxBatchSizeFiles || batchSizeBytes > maxBatchSizeBytes {
			if err := m.CheckFolderHealth(folder); err != nil {
				l.Infof("Stopping folder %s mid-scan due to folder error: %s", folderCfg.Description(), err)
				return err
			}
			m.updateLocalsFromScanning(folder, batch)
			batch = batch[:0]
			batchSizeBytes = 0
		}
		batch = append(batch, f)
		batchSizeBytes += f.ProtoSize()
	}

	if err := m.CheckFolderHealth(folder); err != nil {
		l.Infof("Stopping folder %s mid-scan due to folder error: %s", folderCfg.Description(), err)
		return err
	} else if len(batch) > 0 {
		m.updateLocalsFromScanning(folder, batch)
	}

	if len(subDirs) == 0 {
		// If we have no specific subdirectories to traverse, set it to one
		// empty prefix so we traverse the entire folder contents once.
		subDirs = []string{""}
	}

	// Do a scan of the database for each prefix, to check for deleted and
	// ignored files.
	batch = batch[:0]
	batchSizeBytes = 0
	for _, sub := range subDirs {
		var iterError error

		fset.WithPrefixedHaveTruncated(protocol.LocalDeviceID, sub, func(fi db.FileIntf) bool {
			f := fi.(db.FileInfoTruncated)
			if len(batch) == maxBatchSizeFiles || batchSizeBytes > maxBatchSizeBytes {
				if err := m.CheckFolderHealth(folder); err != nil {
					iterError = err
					return false
				}
				m.updateLocalsFromScanning(folder, batch)
				batch = batch[:0]
				batchSizeBytes = 0
			}

			switch {
			case !f.IsInvalid() && ignores.Match(f.Name).IsIgnored():
				// File was valid at last pass but has been ignored. Set invalid bit.
				l.Debugln("setting invalid bit on ignored", f)
				nf := protocol.FileInfo{
					Name:          f.Name,
					Type:          f.Type,
					Size:          f.Size,
					ModifiedS:     f.ModifiedS,
					ModifiedNs:    f.ModifiedNs,
					ModifiedBy:    m.id.Short(),
					Permissions:   f.Permissions,
					NoPermissions: f.NoPermissions,
					Invalid:       true,
					Version:       f.Version, // The file is still the same, so don't bump version
				}
				batch = append(batch, nf)
				batchSizeBytes += nf.ProtoSize()

			case !f.IsInvalid() && !f.IsDeleted():
				// The file is valid and not deleted. Lets check if it's
				// still here.

				if _, err := mtimefs.Lstat(f.Name); err != nil {
					// We don't specifically verify that the error is
					// fs.IsNotExist because there is a corner case when a
					// directory is suddenly transformed into a file. When that
					// happens, files that were in the directory (that is now a
					// file) are deleted but will return a confusing error ("not a
					// directory") when we try to Lstat() them.

					nf := protocol.FileInfo{
						Name:       f.Name,
						Type:       f.Type,
						Size:       0,
						ModifiedS:  f.ModifiedS,
						ModifiedNs: f.ModifiedNs,
						ModifiedBy: m.id.Short(),
						Deleted:    true,
						Version:    f.Version.Update(m.shortID),
					}

					batch = append(batch, nf)
					batchSizeBytes += nf.ProtoSize()
				}
			}
			return true
		})

		if iterError != nil {
			l.Infof("Stopping folder %s mid-scan due to folder error: %s", folderCfg.Description(), iterError)
			return iterError
		}
	}

	if err := m.CheckFolderHealth(folder); err != nil {
		l.Infof("Stopping folder %s mid-scan due to folder error: %s", folderCfg.Description(), err)
		return err
	} else if len(batch) > 0 {
		m.updateLocalsFromScanning(folder, batch)
	}

	m.folderStatRef(folder).ScanCompleted()
	runner.setState(FolderIdle)
	return nil
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
	// The list of folders in the message is sorted, so we always get the
	// same order.
	folders := m.deviceFolders[device]
	sort.Strings(folders)

	for _, folder := range folders {
		folderCfg := m.cfg.Folders()[folder]
		fs := m.folderFiles[folder]

		protocolFolder := protocol.Folder{
			ID:                 folder,
			Label:              folderCfg.Label,
			ReadOnly:           folderCfg.Type == config.FolderTypeSendOnly,
			IgnorePermissions:  folderCfg.IgnorePerms,
			IgnoreDelete:       folderCfg.IgnoreDelete,
			DisableTempIndexes: folderCfg.DisableTempIndexes,
			Paused:             folderCfg.Paused,
		}

		// Devices are sorted, so we always get the same order.
		for _, device := range m.folderDevices.sortedDevices(folder) {
			deviceCfg := m.cfg.Devices()[device]

			var indexID protocol.IndexID
			var maxSequence int64
			if device == m.id {
				indexID = fs.IndexID(protocol.LocalDeviceID)
				maxSequence = fs.Sequence(protocol.LocalDeviceID)
			} else {
				indexID = fs.IndexID(device)
				maxSequence = fs.Sequence(device)
			}

			protocolDevice := protocol.Device{
				ID:          device,
				Name:        deviceCfg.Name,
				Addresses:   deviceCfg.Addresses,
				Compression: deviceCfg.Compression,
				CertName:    deviceCfg.CertName,
				Introducer:  deviceCfg.Introducer,
				IndexID:     indexID,
				MaxSequence: maxSequence,
			}

			protocolFolder.Devices = append(protocolFolder.Devices, protocolDevice)
		}
		message.Folders = append(message.Folders, protocolFolder)
	}
	m.fmut.RUnlock()

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

func (m *Model) Override(folder string) {
	m.fmut.RLock()
	fs, ok := m.folderFiles[folder]
	runner := m.folderRunners[folder]
	m.fmut.RUnlock()
	if !ok {
		return
	}

	runner.setState(FolderScanning)
	batch := make([]protocol.FileInfo, 0, maxBatchSizeFiles)
	batchSizeBytes := 0
	fs.WithNeed(protocol.LocalDeviceID, func(fi db.FileIntf) bool {
		need := fi.(protocol.FileInfo)
		if len(batch) == maxBatchSizeFiles || batchSizeBytes > maxBatchSizeBytes {
			m.updateLocalsFromScanning(folder, batch)
			batch = batch[:0]
			batchSizeBytes = 0
		}

		have, ok := fs.Get(protocol.LocalDeviceID, need.Name)
		if !ok || have.Name != need.Name {
			// We are missing the file
			need.Deleted = true
			need.Blocks = nil
			need.Version = need.Version.Update(m.shortID)
			need.Size = 0
		} else {
			// We have the file, replace with our version
			have.Version = have.Version.Merge(need.Version).Update(m.shortID)
			need = have
		}
		need.Sequence = 0
		batch = append(batch, need)
		batchSizeBytes += need.ProtoSize()
		return true
	})
	if len(batch) > 0 {
		m.updateLocalsFromScanning(folder, batch)
	}
	runner.setState(FolderIdle)
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
	if !ok {
		// The folder might not exist, since this can be called with a user
		// specified folder name from the REST interface.
		return 0, false
	}

	var ver int64
	for device := range m.folderDevices[folder] {
		ver += fs.Sequence(device)
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

		if f.IsInvalid() || f.IsDeleted() || f.Name == prefix {
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

func (m *Model) Availability(folder, file string, version protocol.Vector, block protocol.BlockInfo) []Availability {
	// The slightly unusual locking sequence here is because we need to hold
	// pmut for the duration (as the value returned from foldersFiles can
	// get heavily modified on Close()), but also must acquire fmut before
	// pmut. (The locks can be *released* in any order.)
	m.fmut.RLock()
	m.pmut.RLock()
	defer m.pmut.RUnlock()

	fs, ok := m.folderFiles[folder]
	devices := m.folderDevices[folder]
	m.fmut.RUnlock()

	if !ok {
		return nil
	}

	var availabilities []Availability
next:
	for _, device := range fs.Availability(file) {
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

	for device := range devices {
		if m.deviceDownloads[device].Has(folder, file, version, int32(block.Offset/protocol.BlockSize)) {
			availabilities = append(availabilities, Availability{ID: device, FromTemporary: true})
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

// CheckFolderHealth checks the folder for common errors and returns the
// current folder error, or nil if the folder is healthy.
func (m *Model) CheckFolderHealth(id string) error {
	folder, ok := m.cfg.Folders()[id]
	if !ok {
		return errFolderMissing
	}

	// Check for folder errors, with the most serious and specific first and
	// generic ones like out of space on the home disk later. Note the
	// inverted error flow (err==nil checks) here.

	err := m.checkFolderPath(folder)
	if err == nil {
		err = m.checkFolderFreeSpace(folder)
	}
	if err == nil {
		err = m.checkHomeDiskFree()
	}

	// Set or clear the error on the runner, which also does logging and
	// generates events and stuff.
	m.runnerExchangeError(folder, err)

	return err
}

// checkFolderPath returns nil if the folder path exists and has the marker file.
func (m *Model) checkFolderPath(folder config.FolderConfiguration) error {
	fs := folder.Filesystem()

	if fi, err := fs.Stat("."); err != nil || !fi.IsDir() {
		return errFolderPathMissing
	}

	if !folder.HasMarker() {
		return errFolderMarkerMissing
	}

	return nil
}

// checkFolderFreeSpace returns nil if the folder has the required amount of
// free space, or if folder free space checking is disabled.
func (m *Model) checkFolderFreeSpace(folder config.FolderConfiguration) error {
	return m.checkFreeSpace(folder.MinDiskFree, folder.Filesystem())
}

// checkHomeDiskFree returns nil if the home disk has the required amount of
// free space, or if home disk free space checking is disabled.
func (m *Model) checkHomeDiskFree() error {
	fs := fs.NewFilesystem(fs.FilesystemTypeBasic, filepath.Dir(m.cfg.ConfigPath()))
	return m.checkFreeSpace(m.cfg.Options().MinHomeDiskFree, fs)
}

func (m *Model) checkFreeSpace(req config.Size, fs fs.Filesystem) error {
	val := req.BaseValue()
	if val <= 0 {
		return nil
	}

	usage, err := fs.Usage(".")
	if req.Percentage() {
		freePct := (float64(usage.Free) / float64(usage.Total)) * 100
		if err == nil && freePct < val {
			return fmt.Errorf("insufficient space in %v %v: %f %% < %v", fs.Type(), fs.URI(), freePct, req)
		}
	} else {
		if err == nil && float64(usage.Free) < val {
			return fmt.Errorf("insufficient space in %v %v: %v < %v", fs.Type(), fs.URI(), usage.Free, req)
		}
	}

	return nil
}

// runnerExchangeError sets the given error (which way be nil) on the folder
// runner. If the error differs from any previous error, logging and events
// happen.
func (m *Model) runnerExchangeError(folder config.FolderConfiguration, err error) {
	m.fmut.RLock()
	runner, runnerExists := m.folderRunners[folder.ID]
	m.fmut.RUnlock()

	var oldErr error
	if runnerExists {
		_, _, oldErr = runner.getState()
	}

	if err != nil {
		if oldErr != nil && oldErr.Error() != err.Error() {
			l.Infof("Folder %s error changed: %q -> %q", folder.Description(), oldErr, err)
		} else if oldErr == nil {
			l.Warnf("Stopping folder %s - %v", folder.Description(), err)
		}
		if runnerExists {
			runner.setError(err)
		}
	} else if oldErr != nil {
		l.Infof("Folder %q error is cleared, restarting", folder.ID)
		if runnerExists {
			runner.clearError()
		}
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
				l.Infoln(m, "Paused folder", cfg.Description())
				cfg.CreateRoot()
			} else {
				l.Infoln(m, "Adding folder", cfg.Description())
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
		// Check if anything differs, apart from the label.
		toCfgCopy := toCfg
		fromCfgCopy := fromCfg
		fromCfgCopy.Label = ""
		toCfgCopy.Label = ""

		if !reflect.DeepEqual(fromCfgCopy, toCfgCopy) {
			m.RestartFolder(toCfg)
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
	fromDevices := mapDeviceConfigs(from.Devices)
	toDevices := mapDeviceConfigs(to.Devices)
	for deviceID, toCfg := range toDevices {
		fromCfg, ok := fromDevices[deviceID]
		if !ok || fromCfg.Paused == toCfg.Paused {
			continue
		}

		if toCfg.Paused {
			l.Infoln("Pausing", deviceID)
			m.close(deviceID)
			events.Default.Log(events.DevicePaused, map[string]string{"device": deviceID.String()})
		} else {
			events.Default.Log(events.DeviceResumed, map[string]string{"device": deviceID.String()})
		}
	}

	// Some options don't require restart as those components handle it fine
	// by themselves.
	from.Options.URAccepted = to.Options.URAccepted
	from.Options.URSeen = to.Options.URSeen
	from.Options.URUniqueID = to.Options.URUniqueID
	from.Options.ListenAddresses = to.Options.ListenAddresses
	from.Options.RelaysEnabled = to.Options.RelaysEnabled
	from.Options.UnackedNotificationIDs = to.Options.UnackedNotificationIDs
	from.Options.MaxRecvKbps = to.Options.MaxRecvKbps
	from.Options.MaxSendKbps = to.Options.MaxSendKbps
	from.Options.LimitBandwidthInLan = to.Options.LimitBandwidthInLan
	from.Options.StunKeepaliveS = to.Options.StunKeepaliveS
	from.Options.StunServers = to.Options.StunServers
	// All of the other generic options require restart. Or at least they may;
	// removing this check requires going through those options carefully and
	// making sure there are individual services that handle them correctly.
	// This code is the "original" requires-restart check and protects other
	// components that haven't yet been converted to VerifyConfig/CommitConfig
	// handling.
	if !reflect.DeepEqual(from.Options, to.Options) {
		l.Debugln(m, "requires restart, options differ")
		return false
	}

	return true
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

// mapDeviceConfigs returns a map of device ID to device configuration for the given
// slice of folder configurations.
func mapDeviceConfigs(devices []config.DeviceConfiguration) map[protocol.DeviceID]config.DeviceConfiguration {
	m := make(map[protocol.DeviceID]config.DeviceConfiguration, len(devices))
	for _, dev := range devices {
		m[dev.DeviceID] = dev
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

// The exists function is expected to return true for all known paths
// (excluding "" and ".")
func unifySubs(dirs []string, exists func(dir string) bool) []string {
	subs := trimUntilParentKnown(dirs, exists)
	sort.Strings(subs)
	return simplifySortedPaths(subs)
}

func trimUntilParentKnown(dirs []string, exists func(dir string) bool) []string {
	var subs []string
	for _, sub := range dirs {
		for sub != "" && !fs.IsInternal(sub) {
			sub = filepath.Clean(sub)
			parent := filepath.Dir(sub)
			if parent == "." || exists(parent) {
				break
			}
			sub = parent
			if sub == "." || sub == string(filepath.Separator) {
				// Shortcut. We are going to scan the full folder, so we can
				// just return an empty list of subs at this point.
				return nil
			}
		}
		if sub == "" {
			return nil
		}
		subs = append(subs, sub)
	}
	return subs
}

func simplifySortedPaths(subs []string) []string {
	var cleaned []string
next:
	for _, sub := range subs {
		for _, existing := range cleaned {
			if sub == existing || strings.HasPrefix(sub, existing+string(fs.PathSeparator)) {
				continue next
			}
		}
		cleaned = append(cleaned, sub)
	}

	return cleaned
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

// shouldIgnore returns true when a file should be excluded from processing
func shouldIgnore(file db.FileIntf, matcher *ignore.Matcher, ignoreDelete bool) bool {
	switch {
	case ignoreDelete && file.IsDeleted():
		// ignoreDelete first because it's a very cheap test so a win if it
		// succeeds, and we might in the long run accumulate quite a few
		// deleted files.
		return true

	case matcher.ShouldIgnore(file.FileName()):
		return true
	}

	return false
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

// sortedDevices returns the list of devices for a given folder, sorted
func (s folderDeviceSet) sortedDevices(folder string) []protocol.DeviceID {
	devs := make([]protocol.DeviceID, 0, len(s[folder]))
	for dev := range s[folder] {
		devs = append(devs, dev)
	}
	sort.Sort(protocol.DeviceIDs(devs))
	return devs
}
