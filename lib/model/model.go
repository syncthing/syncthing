// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:generate counterfeiter -o mocks/model.go --fake-name Model . Model

package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	stdsync "sync"
	"time"

	"github.com/pkg/errors"
	"github.com/thejerf/suture/v4"

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
	"github.com/syncthing/syncthing/lib/svcutil"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/ur/contract"
	"github.com/syncthing/syncthing/lib/versioner"
)

// How many files to send in each Index/IndexUpdate message.
const (
	maxBatchSizeBytes = 250 * 1024 // Aim for making index messages no larger than 250 KiB (uncompressed)
	maxBatchSizeFiles = 1000       // Either way, don't include more files than this
)

type service interface {
	suture.Service
	BringToFront(string)
	Override()
	Revert()
	DelayScan(d time.Duration)
	SchedulePull()                                    // something relevant changed, we should try a pull
	Jobs(page, perpage int) ([]string, []string, int) // In progress, Queued, skipped
	Scan(subs []string) error
	Errors() []FileError
	WatchError() error
	ScheduleForceRescan(path string)
	GetStatistics() (stats.FolderStatistics, error)

	getState() (folderState, time.Time, error)
}

type Availability struct {
	ID            protocol.DeviceID `json:"id"`
	FromTemporary bool              `json:"fromTemporary"`
}

type Model interface {
	suture.Service

	connections.Model

	ResetFolder(folder string)
	DelayScan(folder string, next time.Duration)
	ScanFolder(folder string) error
	ScanFolders() map[string]error
	ScanFolderSubdirs(folder string, subs []string) error
	State(folder string) (string, time.Time, error)
	FolderErrors(folder string) ([]FileError, error)
	WatchError(folder string) error
	Override(folder string)
	Revert(folder string)
	BringToFront(folder, file string)
	LoadIgnores(folder string) ([]string, []string, error)
	CurrentIgnores(folder string) ([]string, []string, error)
	SetIgnores(folder string, content []string) error

	GetFolderVersions(folder string) (map[string][]versioner.FileVersion, error)
	RestoreFolderVersions(folder string, versions map[string]time.Time) (map[string]error, error)

	DBSnapshot(folder string) (*db.Snapshot, error)
	NeedFolderFiles(folder string, page, perpage int) ([]db.FileInfoTruncated, []db.FileInfoTruncated, []db.FileInfoTruncated, error)
	RemoteNeedFolderFiles(folder string, device protocol.DeviceID, page, perpage int) ([]db.FileInfoTruncated, error)
	LocalChangedFolderFiles(folder string, page, perpage int) ([]db.FileInfoTruncated, error)
	FolderProgressBytesCompleted(folder string) int64

	CurrentFolderFile(folder string, file string) (protocol.FileInfo, bool, error)
	CurrentGlobalFile(folder string, file string) (protocol.FileInfo, bool, error)
	Availability(folder string, file protocol.FileInfo, block protocol.BlockInfo) ([]Availability, error)

	Completion(device protocol.DeviceID, folder string) (FolderCompletion, error)
	ConnectionStats() map[string]interface{}
	DeviceStatistics() (map[protocol.DeviceID]stats.DeviceStatistics, error)
	FolderStatistics() (map[string]stats.FolderStatistics, error)
	UsageReportingStats(report *contract.Report, version int, preview bool)

	PendingDevices() (map[protocol.DeviceID]db.ObservedDevice, error)
	PendingFolders(device protocol.DeviceID) (map[string]db.PendingFolder, error)

	StartDeadlockDetector(timeout time.Duration)
	GlobalDirectoryTree(folder, prefix string, levels int, dirsOnly bool) ([]*TreeEntry, error)
}

type model struct {
	*suture.Supervisor

	// constructor parameters
	cfg            config.Wrapper
	id             protocol.DeviceID
	clientName     string
	clientVersion  string
	db             *db.Lowlevel
	protectedFiles []string
	evLogger       events.Logger

	// constant or concurrency safe fields
	finder          *db.BlockFinder
	progressEmitter *ProgressEmitter
	shortID         protocol.ShortID
	// globalRequestLimiter limits the amount of data in concurrent incoming
	// requests
	globalRequestLimiter *byteSemaphore
	// folderIOLimiter limits the number of concurrent I/O heavy operations,
	// such as scans and pulls.
	folderIOLimiter *byteSemaphore
	fatalChan       chan error
	started         chan struct{}

	// fields protected by fmut
	fmut                           sync.RWMutex
	folderCfgs                     map[string]config.FolderConfiguration                  // folder -> cfg
	folderFiles                    map[string]*db.FileSet                                 // folder -> files
	deviceStatRefs                 map[protocol.DeviceID]*stats.DeviceStatisticsReference // deviceID -> statsRef
	folderIgnores                  map[string]*ignore.Matcher                             // folder -> matcher object
	folderRunners                  map[string]service                                     // folder -> puller or scanner
	folderRunnerToken              map[string]suture.ServiceToken                         // folder -> token for folder runner
	folderRestartMuts              syncMutexMap                                           // folder -> restart mutex
	folderVersioners               map[string]versioner.Versioner                         // folder -> versioner (may be nil)
	folderEncryptionPasswordTokens map[string][]byte                                      // folder -> encryption token (may be missing, and only for encryption type folders)
	folderEncryptionFailures       map[string]map[protocol.DeviceID]error                 // folder -> device -> error regarding encryption consistency (may be missing)

	// fields protected by pmut
	pmut                sync.RWMutex
	conn                map[protocol.DeviceID]protocol.Connection
	connRequestLimiters map[protocol.DeviceID]*byteSemaphore
	closed              map[protocol.DeviceID]chan struct{}
	helloMessages       map[protocol.DeviceID]protocol.Hello
	deviceDownloads     map[protocol.DeviceID]*deviceDownloadState
	remotePausedFolders map[protocol.DeviceID]map[string]struct{} // deviceID -> folders
	indexSenders        map[protocol.DeviceID]*indexSenderRegistry

	// for testing only
	foldersRunning int32
}

type folderFactory func(*model, *db.FileSet, *ignore.Matcher, config.FolderConfiguration, versioner.Versioner, events.Logger, *byteSemaphore) service

var (
	folderFactories = make(map[config.FolderType]folderFactory)
)

var (
	errDeviceUnknown     = errors.New("unknown device")
	errDevicePaused      = errors.New("device is paused")
	errDeviceIgnored     = errors.New("device is ignored")
	errDeviceRemoved     = errors.New("device has been removed")
	ErrFolderPaused      = errors.New("folder is paused")
	ErrFolderNotRunning  = errors.New("folder is not running")
	ErrFolderMissing     = errors.New("no such folder")
	errNetworkNotAllowed = errors.New("network not allowed")
	errNoVersioner       = errors.New("folder has no versioner")
	// errors about why a connection is closed
	errReplacingConnection             = errors.New("replacing connection")
	errStopped                         = errors.New("Syncthing is being stopped")
	errEncryptionInvConfigLocal        = errors.New("can't encrypt data for a device when the folder type is receiveEncrypted")
	errEncryptionInvConfigRemote       = errors.New("remote has encrypted data and encrypts that data for us - this is impossible")
	errEncryptionNotEncryptedLocal     = errors.New("folder is announced as encrypted, but not configured thus")
	errEncryptionNotEncryptedRemote    = errors.New("folder is configured to be encrypted but not announced thus")
	errEncryptionNotEncryptedUntrusted = errors.New("device is untrusted, but configured to receive not encrypted data")
	errEncryptionPassword              = errors.New("different encryption passwords used")
	errEncryptionNeedToken             = errors.New("require password token for receive-encrypted token")
	errMissingRemoteInClusterConfig    = errors.New("remote device missing in cluster config")
	errMissingLocalInClusterConfig     = errors.New("local device missing in cluster config")
	errConnLimitReached                = errors.New("connection limit reached")
	// messages for failure reports
	failureUnexpectedGenerateCCError = "unexpected error occurred in generateClusterConfig"
)

// NewModel creates and starts a new model. The model starts in read-only mode,
// where it sends index information to connected peers and responds to requests
// for file data without altering the local folder in any way.
func NewModel(cfg config.Wrapper, id protocol.DeviceID, clientName, clientVersion string, ldb *db.Lowlevel, protectedFiles []string, evLogger events.Logger) Model {
	spec := svcutil.SpecWithDebugLogger(l)
	m := &model{
		Supervisor: suture.New("model", spec),

		// constructor parameters
		cfg:            cfg,
		id:             id,
		clientName:     clientName,
		clientVersion:  clientVersion,
		db:             ldb,
		protectedFiles: protectedFiles,
		evLogger:       evLogger,

		// constant or concurrency safe fields
		finder:               db.NewBlockFinder(ldb),
		progressEmitter:      NewProgressEmitter(cfg, evLogger),
		shortID:              id.Short(),
		globalRequestLimiter: newByteSemaphore(1024 * cfg.Options().MaxConcurrentIncomingRequestKiB()),
		folderIOLimiter:      newByteSemaphore(cfg.Options().MaxFolderConcurrency()),
		fatalChan:            make(chan error),
		started:              make(chan struct{}),

		// fields protected by fmut
		fmut:                           sync.NewRWMutex(),
		folderCfgs:                     make(map[string]config.FolderConfiguration),
		folderFiles:                    make(map[string]*db.FileSet),
		deviceStatRefs:                 make(map[protocol.DeviceID]*stats.DeviceStatisticsReference),
		folderIgnores:                  make(map[string]*ignore.Matcher),
		folderRunners:                  make(map[string]service),
		folderRunnerToken:              make(map[string]suture.ServiceToken),
		folderVersioners:               make(map[string]versioner.Versioner),
		folderEncryptionPasswordTokens: make(map[string][]byte),
		folderEncryptionFailures:       make(map[string]map[protocol.DeviceID]error),

		// fields protected by pmut
		pmut:                sync.NewRWMutex(),
		conn:                make(map[protocol.DeviceID]protocol.Connection),
		connRequestLimiters: make(map[protocol.DeviceID]*byteSemaphore),
		closed:              make(map[protocol.DeviceID]chan struct{}),
		helloMessages:       make(map[protocol.DeviceID]protocol.Hello),
		deviceDownloads:     make(map[protocol.DeviceID]*deviceDownloadState),
		remotePausedFolders: make(map[protocol.DeviceID]map[string]struct{}),
		indexSenders:        make(map[protocol.DeviceID]*indexSenderRegistry),
	}
	for devID := range cfg.Devices() {
		m.deviceStatRefs[devID] = stats.NewDeviceStatisticsReference(m.db, devID)
	}
	m.Add(m.progressEmitter)
	m.Add(svcutil.AsService(m.serve, m.String()))

	return m
}

func (m *model) serve(ctx context.Context) error {
	defer m.closeAllConnectionsAndWait()

	cfg := m.cfg.Subscribe(m)
	defer m.cfg.Unsubscribe(m)

	if err := m.initFolders(cfg); err != nil {
		close(m.started)
		return svcutil.AsFatalErr(err, svcutil.ExitError)
	}

	close(m.started)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-m.fatalChan:
		return svcutil.AsFatalErr(err, svcutil.ExitError)
	}
}

func (m *model) initFolders(cfg config.Configuration) error {
	clusterConfigDevices := make(deviceIDSet, len(cfg.Devices))
	for _, folderCfg := range cfg.Folders {
		if folderCfg.Paused {
			folderCfg.CreateRoot()
			continue
		}
		err := m.newFolder(folderCfg, cfg.Options.CacheIgnoredFiles)
		if err != nil {
			return err
		}
		clusterConfigDevices.add(folderCfg.DeviceIDs())
	}

	ignoredDevices := observedDeviceSet(m.cfg.IgnoredDevices())
	m.cleanPending(cfg.DeviceMap(), cfg.FolderMap(), ignoredDevices, nil)

	m.sendClusterConfig(clusterConfigDevices.AsSlice())
	return nil
}

func (m *model) closeAllConnectionsAndWait() {
	m.pmut.RLock()
	closed := make([]chan struct{}, 0, len(m.conn))
	for id, conn := range m.conn {
		closed = append(closed, m.closed[id])
		go conn.Close(errStopped)
	}
	m.pmut.RUnlock()
	for _, c := range closed {
		<-c
	}
}

func (m *model) fatal(err error) {
	select {
	case m.fatalChan <- err:
	default:
	}
}

// StartDeadlockDetector starts a deadlock detector on the models locks which
// causes panics in case the locks cannot be acquired in the given timeout
// period.
func (m *model) StartDeadlockDetector(timeout time.Duration) {
	l.Infof("Starting deadlock detector with %v timeout", timeout)
	detector := newDeadlockDetector(timeout)
	detector.Watch("fmut", m.fmut)
	detector.Watch("pmut", m.pmut)
}

// Need to hold lock on m.fmut when calling this.
func (m *model) addAndStartFolderLocked(cfg config.FolderConfiguration, fset *db.FileSet, cacheIgnoredFiles bool) {
	ignores := ignore.New(cfg.Filesystem(), ignore.WithCache(cacheIgnoredFiles))
	if cfg.Type != config.FolderTypeReceiveEncrypted {
		if err := ignores.Load(".stignore"); err != nil && !fs.IsNotExist(err) {
			l.Warnln("Loading ignores:", err)
		}
	}

	m.addAndStartFolderLockedWithIgnores(cfg, fset, ignores)
}

// Only needed for testing, use addAndStartFolderLocked instead.
func (m *model) addAndStartFolderLockedWithIgnores(cfg config.FolderConfiguration, fset *db.FileSet, ignores *ignore.Matcher) {
	m.folderCfgs[cfg.ID] = cfg
	m.folderFiles[cfg.ID] = fset
	m.folderIgnores[cfg.ID] = ignores

	_, ok := m.folderRunners[cfg.ID]
	if ok {
		l.Warnln("Cannot start already running folder", cfg.Description())
		panic("cannot start already running folder")
	}

	folderFactory, ok := folderFactories[cfg.Type]
	if !ok {
		panic(fmt.Sprintf("unknown folder type 0x%x", cfg.Type))
	}

	folder := cfg.ID

	// Find any devices for which we hold the index in the db, but the folder
	// is not shared, and drop it.
	expected := mapDevices(cfg.DeviceIDs())
	for _, available := range fset.ListDevices() {
		if _, ok := expected[available]; !ok {
			l.Debugln("dropping", folder, "state for", available)
			fset.Drop(available)
		}
	}

	v, ok := fset.Sequence(protocol.LocalDeviceID), true
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

	if cfg.Type == config.FolderTypeReceiveEncrypted {
		if encryptionToken, err := readEncryptionToken(cfg); err == nil {
			m.folderEncryptionPasswordTokens[folder] = encryptionToken
		} else if !fs.IsNotExist(err) {
			l.Warnf("Failed to read encryption token: %v", err)
		}
	}

	// These are our metadata files, and they should always be hidden.
	ffs := cfg.Filesystem()
	_ = ffs.Hide(config.DefaultMarkerName)
	_ = ffs.Hide(".stversions")
	_ = ffs.Hide(".stignore")

	var ver versioner.Versioner
	if cfg.Versioning.Type != "" {
		var err error
		ver, err = versioner.New(cfg)
		if err != nil {
			panic(fmt.Errorf("creating versioner: %w", err))
		}
	}
	m.folderVersioners[folder] = ver

	p := folderFactory(m, fset, ignores, cfg, ver, m.evLogger, m.folderIOLimiter)

	m.folderRunners[folder] = p

	m.warnAboutOverwritingProtectedFiles(cfg, ignores)

	m.folderRunnerToken[folder] = m.Add(p)

	l.Infof("Ready to synchronize %s (%s)", cfg.Description(), cfg.Type)
}

func (m *model) warnAboutOverwritingProtectedFiles(cfg config.FolderConfiguration, ignores *ignore.Matcher) {
	if cfg.Type == config.FolderTypeSendOnly {
		return
	}

	// This is a bit of a hack.
	ffs := cfg.Filesystem()
	if ffs.Type() != fs.FilesystemTypeBasic {
		return
	}
	folderLocation := ffs.URI()

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

func (m *model) removeFolder(cfg config.FolderConfiguration) {
	m.fmut.RLock()
	token, ok := m.folderRunnerToken[cfg.ID]
	m.fmut.RUnlock()
	if ok {
		m.RemoveAndWait(token, 0)
	}

	// We need to hold both fmut and pmut and must acquire locks in the same
	// order always. (The locks can be *released* in any order.)
	m.fmut.Lock()
	m.pmut.RLock()

	isPathUnique := true
	for folderID, folderCfg := range m.folderCfgs {
		if folderID != cfg.ID && folderCfg.Path == cfg.Path {
			isPathUnique = false
			break
		}
	}
	if isPathUnique {
		// Remove (if empty and removable) or move away (if non-empty or
		// otherwise not removable) Syncthing-specific marker files.
		fs := cfg.Filesystem()
		if err := fs.Remove(config.DefaultMarkerName); err != nil {
			moved := config.DefaultMarkerName + time.Now().Format(".removed-20060102-150405")
			_ = fs.Rename(config.DefaultMarkerName, moved)
		}
	}

	m.cleanupFolderLocked(cfg)
	for _, r := range m.indexSenders {
		r.remove(cfg.ID)
	}

	m.fmut.Unlock()
	m.pmut.RUnlock()

	// Remove it from the database
	db.DropFolder(m.db, cfg.ID)
}

// Need to hold lock on m.fmut when calling this.
func (m *model) cleanupFolderLocked(cfg config.FolderConfiguration) {
	// clear up our config maps
	delete(m.folderCfgs, cfg.ID)
	delete(m.folderFiles, cfg.ID)
	delete(m.folderIgnores, cfg.ID)
	delete(m.folderRunners, cfg.ID)
	delete(m.folderRunnerToken, cfg.ID)
	delete(m.folderVersioners, cfg.ID)
}

func (m *model) restartFolder(from, to config.FolderConfiguration, cacheIgnoredFiles bool) error {
	if len(to.ID) == 0 {
		panic("bug: cannot restart empty folder ID")
	}
	if to.ID != from.ID {
		l.Warnf("bug: folder restart cannot change ID %q -> %q", from.ID, to.ID)
		panic("bug: folder restart cannot change ID")
	}
	folder := to.ID

	// This mutex protects the entirety of the restart operation, preventing
	// there from being more than one folder restart operation in progress
	// at any given time. The usual fmut/pmut stuff doesn't cover this,
	// because those locks are released while we are waiting for the folder
	// to shut down (and must be so because the folder might need them as
	// part of its operations before shutting down).
	restartMut := m.folderRestartMuts.Get(folder)
	restartMut.Lock()
	defer restartMut.Unlock()

	m.fmut.RLock()
	token, ok := m.folderRunnerToken[from.ID]
	m.fmut.RUnlock()
	if ok {
		m.RemoveAndWait(token, 0)
	}

	m.fmut.Lock()
	defer m.fmut.Unlock()

	// Cache the (maybe) existing fset before it's removed by cleanupFolderLocked
	fset := m.folderFiles[folder]
	fsetNil := fset == nil

	m.cleanupFolderLocked(from)
	if !to.Paused {
		if fsetNil {
			// Create a new fset. Might take a while and we do it under
			// locking, but it's unsafe to create fset:s concurrently so
			// that's the price we pay.
			var err error
			fset, err = db.NewFileSet(folder, to.Filesystem(), m.db)
			if err != nil {
				return fmt.Errorf("restarting %v: %w", to.Description(), err)
			}
		}
		m.addAndStartFolderLocked(to, fset, cacheIgnoredFiles)
	}

	// Care needs to be taken because we already hold fmut and the lock order
	// must be the same everywhere. As fmut is acquired first, this is fine.
	// toDeviceIDs := to.DeviceIDs()
	m.pmut.RLock()
	for _, id := range to.DeviceIDs() {
		indexSenders, ok := m.indexSenders[id]
		if !ok {
			continue
		}
		// In case the folder was newly shared with us we already got a
		// cluster config and wont necessarily get another soon - start
		// sending indexes if connected.
		if to.Paused {
			indexSenders.pause(to.ID)
		} else if !from.SharedWith(indexSenders.deviceID) || fsetNil || from.Paused {
			indexSenders.resume(to, fset)
		}
	}
	m.pmut.RUnlock()

	var infoMsg string
	switch {
	case to.Paused:
		infoMsg = "Paused"
	case from.Paused:
		infoMsg = "Unpaused"
	default:
		infoMsg = "Restarted"
	}
	l.Infof("%v folder %v (%v)", infoMsg, to.Description(), to.Type)

	return nil
}

func (m *model) newFolder(cfg config.FolderConfiguration, cacheIgnoredFiles bool) error {
	// Creating the fileset can take a long time (metadata calculation) so
	// we do it outside of the lock.
	fset, err := db.NewFileSet(cfg.ID, cfg.Filesystem(), m.db)
	if err != nil {
		return fmt.Errorf("adding %v: %w", cfg.Description(), err)
	}

	m.fmut.Lock()
	defer m.fmut.Unlock()

	// Cluster configs might be received and processed before reaching this
	// point, i.e. before the folder is started. If that's the case, start
	// index senders here.
	m.pmut.RLock()
	for _, id := range cfg.DeviceIDs() {
		if is, ok := m.indexSenders[id]; ok {
			is.resume(cfg, fset)
		}
	}
	m.pmut.RUnlock()

	m.addAndStartFolderLocked(cfg, fset, cacheIgnoredFiles)
	return nil
}

func (m *model) UsageReportingStats(report *contract.Report, version int, preview bool) {
	if version >= 3 {
		// Block stats
		blockStatsMut.Lock()
		for k, v := range blockStats {
			switch k {
			case "total":
				report.BlockStats.Total = v
			case "renamed":
				report.BlockStats.Renamed = v
			case "reused":
				report.BlockStats.Reused = v
			case "pulled":
				report.BlockStats.Pulled = v
			case "copyOrigin":
				report.BlockStats.CopyOrigin = v
			case "copyOriginShifted":
				report.BlockStats.CopyOriginShifted = v
			case "copyElsewhere":
				report.BlockStats.CopyElsewhere = v
			}
			// Reset counts, as these are incremental
			if !preview {
				blockStats[k] = 0
			}
		}
		blockStatsMut.Unlock()

		// Transport stats
		m.pmut.RLock()
		for _, conn := range m.conn {
			report.TransportStats[conn.Transport()]++
		}
		m.pmut.RUnlock()

		// Ignore stats
		var seenPrefix [3]bool
		for folder := range m.cfg.Folders() {
			lines, _, err := m.CurrentIgnores(folder)
			if err != nil {
				continue
			}
			report.IgnoreStats.Lines += len(lines)

			for _, line := range lines {
				// Allow prefixes to be specified in any order, but only once.
				for {
					if strings.HasPrefix(line, "!") && !seenPrefix[0] {
						seenPrefix[0] = true
						line = line[1:]
						report.IgnoreStats.Inverts++
					} else if strings.HasPrefix(line, "(?i)") && !seenPrefix[1] {
						seenPrefix[1] = true
						line = line[4:]
						report.IgnoreStats.Folded++
					} else if strings.HasPrefix(line, "(?d)") && !seenPrefix[2] {
						seenPrefix[2] = true
						line = line[4:]
						report.IgnoreStats.Deletable++
					} else {
						seenPrefix[0] = false
						seenPrefix[1] = false
						seenPrefix[2] = false
						break
					}
				}

				// Noops, remove
				line = strings.TrimSuffix(line, "**")
				line = strings.TrimPrefix(line, "**/")

				if strings.HasPrefix(line, "/") {
					report.IgnoreStats.Rooted++
				} else if strings.HasPrefix(line, "#include ") {
					report.IgnoreStats.Includes++
					if strings.Contains(line, "..") {
						report.IgnoreStats.EscapedIncludes++
					}
				}

				if strings.Contains(line, "**") {
					report.IgnoreStats.DoubleStars++
					// Remove not to trip up star checks.
					line = strings.ReplaceAll(line, "**", "")
				}

				if strings.Contains(line, "*") {
					report.IgnoreStats.Stars++
				}
			}
		}
	}
}

type ConnectionInfo struct {
	protocol.Statistics
	Connected     bool
	Paused        bool
	Address       string
	ClientVersion string
	Type          string
	Crypto        string
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
		"crypto":        info.Crypto,
	})
}

// NumConnections returns the current number of active connected devices.
func (m *model) NumConnections() int {
	m.pmut.RLock()
	defer m.pmut.RUnlock()
	return len(m.conn)
}

// ConnectionStats returns a map with connection statistics for each device.
func (m *model) ConnectionStats() map[string]interface{} {
	m.pmut.RLock()
	defer m.pmut.RUnlock()

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
			ci.Crypto = conn.Crypto()
			ci.Connected = ok
			ci.Statistics = conn.Statistics()
			if addr := conn.RemoteAddr(); addr != nil {
				ci.Address = addr.String()
			}
		}

		conns[device.String()] = ci
	}

	res["connections"] = conns

	in, out := protocol.TotalInOut()
	res["total"] = ConnectionInfo{
		Statistics: protocol.Statistics{
			At:            time.Now().Truncate(time.Second),
			InBytesTotal:  in,
			OutBytesTotal: out,
		},
	}

	return res
}

// DeviceStatistics returns statistics about each device
func (m *model) DeviceStatistics() (map[protocol.DeviceID]stats.DeviceStatistics, error) {
	m.fmut.RLock()
	defer m.fmut.RUnlock()
	res := make(map[protocol.DeviceID]stats.DeviceStatistics, len(m.deviceStatRefs))
	for id, sr := range m.deviceStatRefs {
		stats, err := sr.GetStatistics()
		if err != nil {
			return nil, err
		}
		res[id] = stats
	}
	return res, nil
}

// FolderStatistics returns statistics about each folder
func (m *model) FolderStatistics() (map[string]stats.FolderStatistics, error) {
	res := make(map[string]stats.FolderStatistics)
	m.fmut.RLock()
	defer m.fmut.RUnlock()
	for id, runner := range m.folderRunners {
		stats, err := runner.GetStatistics()
		if err != nil {
			return nil, err
		}
		res[id] = stats
	}
	return res, nil
}

type FolderCompletion struct {
	CompletionPct float64
	GlobalBytes   int64
	NeedBytes     int64
	GlobalItems   int
	NeedItems     int
	NeedDeletes   int
	Sequence      int64
}

func newFolderCompletion(global, need db.Counts, sequence int64) FolderCompletion {
	comp := FolderCompletion{
		GlobalBytes: global.Bytes,
		NeedBytes:   need.Bytes,
		GlobalItems: global.Files + global.Directories + global.Symlinks,
		NeedItems:   need.Files + need.Directories + need.Symlinks,
		NeedDeletes: need.Deleted,
		Sequence:    sequence,
	}
	comp.setComplectionPct()
	return comp
}

func (comp *FolderCompletion) add(other FolderCompletion) {
	comp.GlobalBytes += other.GlobalBytes
	comp.NeedBytes += other.NeedBytes
	comp.GlobalItems += other.GlobalItems
	comp.NeedItems += other.NeedItems
	comp.NeedDeletes += other.NeedDeletes
	comp.setComplectionPct()
}

func (comp *FolderCompletion) setComplectionPct() {
	if comp.GlobalBytes == 0 {
		comp.CompletionPct = 100
	} else {
		needRatio := float64(comp.NeedBytes) / float64(comp.GlobalBytes)
		comp.CompletionPct = 100 * (1 - needRatio)
	}

	// If the completion is 100% but there are deletes we need to handle,
	// drop it down a notch. Hack for consumers that look only at the
	// percentage (our own GUI does the same calculation as here on its own
	// and needs the same fixup).
	if comp.NeedBytes == 0 && comp.NeedDeletes > 0 {
		comp.CompletionPct = 95 // chosen by fair dice roll
	}
}

// Map returns the members as a map, e.g. used in api to serialize as Json.
func (comp FolderCompletion) Map() map[string]interface{} {
	return map[string]interface{}{
		"completion":  comp.CompletionPct,
		"globalBytes": comp.GlobalBytes,
		"needBytes":   comp.NeedBytes,
		"globalItems": comp.GlobalItems,
		"needItems":   comp.NeedItems,
		"needDeletes": comp.NeedDeletes,
		"sequence":    comp.Sequence,
	}
}

// Completion returns the completion status, in percent with some counters,
// for the given device and folder. The device can be any known device ID
// (including the local device) or explicitly protocol.LocalDeviceID. An
// empty folder string means the aggregate of all folders shared with the
// given device.
func (m *model) Completion(device protocol.DeviceID, folder string) (FolderCompletion, error) {
	// The user specifically asked for our own device ID. Internally that is
	// known as protocol.LocalDeviceID so translate.
	if device == m.id {
		device = protocol.LocalDeviceID
	}

	if folder != "" {
		// We want completion for a specific folder.
		return m.folderCompletion(device, folder)
	}

	// We want completion for all (shared) folders as an aggregate.
	var comp FolderCompletion
	for _, fcfg := range m.cfg.FolderList() {
		if device == protocol.LocalDeviceID || fcfg.SharedWith(device) {
			folderComp, err := m.folderCompletion(device, fcfg.ID)
			if err != nil {
				return FolderCompletion{}, err
			}
			comp.add(folderComp)
		}
	}
	return comp, nil
}

func (m *model) folderCompletion(device protocol.DeviceID, folder string) (FolderCompletion, error) {
	m.fmut.RLock()
	err := m.checkFolderRunningLocked(folder)
	rf := m.folderFiles[folder]
	m.fmut.RUnlock()
	if err != nil {
		return FolderCompletion{}, err
	}

	snap, err := rf.Snapshot()
	if err != nil {
		return FolderCompletion{}, err
	}
	defer snap.Release()

	m.pmut.RLock()
	downloaded := m.deviceDownloads[device].BytesDownloaded(folder)
	m.pmut.RUnlock()

	need := snap.NeedSize(device)
	need.Bytes -= downloaded
	// This might might be more than it really is, because some blocks can be of a smaller size.
	if need.Bytes < 0 {
		need.Bytes = 0
	}

	comp := newFolderCompletion(snap.GlobalSize(), need, snap.Sequence(device))

	l.Debugf("%v Completion(%s, %q): %v", m, device, folder, comp.Map())
	return comp, nil
}

// DBSnapshot returns a snapshot of the database content relevant to the given folder.
func (m *model) DBSnapshot(folder string) (*db.Snapshot, error) {
	m.fmut.RLock()
	err := m.checkFolderRunningLocked(folder)
	rf := m.folderFiles[folder]
	m.fmut.RUnlock()
	if err != nil {
		return nil, err
	}
	return rf.Snapshot()
}

func (m *model) FolderProgressBytesCompleted(folder string) int64 {
	return m.progressEmitter.BytesCompleted(folder)
}

// NeedFolderFiles returns paginated list of currently needed files in
// progress, queued, and to be queued on next puller iteration.
func (m *model) NeedFolderFiles(folder string, page, perpage int) ([]db.FileInfoTruncated, []db.FileInfoTruncated, []db.FileInfoTruncated, error) {
	m.fmut.RLock()
	rf, rfOk := m.folderFiles[folder]
	runner, runnerOk := m.folderRunners[folder]
	cfg := m.folderCfgs[folder]
	m.fmut.RUnlock()

	if !rfOk {
		return nil, nil, nil, ErrFolderMissing
	}

	snap, err := rf.Snapshot()
	if err != nil {
		return nil, nil, nil, err
	}
	defer snap.Release()
	var progress, queued, rest []db.FileInfoTruncated
	var seen map[string]struct{}

	p := newPager(page, perpage)

	if runnerOk {
		progressNames, queuedNames, skipped := runner.Jobs(page, perpage)

		progress = make([]db.FileInfoTruncated, len(progressNames))
		queued = make([]db.FileInfoTruncated, len(queuedNames))
		seen = make(map[string]struct{}, len(progressNames)+len(queuedNames))

		for i, name := range progressNames {
			if f, ok := snap.GetGlobalTruncated(name); ok {
				progress[i] = f
				seen[name] = struct{}{}
			}
		}

		for i, name := range queuedNames {
			if f, ok := snap.GetGlobalTruncated(name); ok {
				queued[i] = f
				seen[name] = struct{}{}
			}
		}

		p.get -= len(seen)
		if p.get == 0 {
			return progress, queued, nil, nil
		}
		p.toSkip -= skipped
	}

	rest = make([]db.FileInfoTruncated, 0, perpage)
	snap.WithNeedTruncated(protocol.LocalDeviceID, func(f protocol.FileIntf) bool {
		if cfg.IgnoreDelete && f.IsDeleted() {
			return true
		}

		if p.skip() {
			return true
		}
		ft := f.(db.FileInfoTruncated)
		if _, ok := seen[ft.Name]; !ok {
			rest = append(rest, ft)
			p.get--
		}
		return p.get > 0
	})

	return progress, queued, rest, nil
}

// RemoteNeedFolderFiles returns paginated list of currently needed files in
// progress, queued, and to be queued on next puller iteration, as well as the
// total number of files currently needed.
func (m *model) RemoteNeedFolderFiles(folder string, device protocol.DeviceID, page, perpage int) ([]db.FileInfoTruncated, error) {
	m.fmut.RLock()
	rf, ok := m.folderFiles[folder]
	m.fmut.RUnlock()

	if !ok {
		return nil, ErrFolderMissing
	}

	snap, err := rf.Snapshot()
	if err != nil {
		return nil, err
	}
	defer snap.Release()

	files := make([]db.FileInfoTruncated, 0, perpage)
	p := newPager(page, perpage)
	snap.WithNeedTruncated(device, func(f protocol.FileIntf) bool {
		if p.skip() {
			return true
		}
		files = append(files, f.(db.FileInfoTruncated))
		return !p.done()
	})
	return files, nil
}

func (m *model) LocalChangedFolderFiles(folder string, page, perpage int) ([]db.FileInfoTruncated, error) {
	m.fmut.RLock()
	rf, ok := m.folderFiles[folder]
	cfg := m.folderCfgs[folder]
	m.fmut.RUnlock()

	if !ok {
		return nil, ErrFolderMissing
	}

	snap, err := rf.Snapshot()
	if err != nil {
		return nil, err
	}
	defer snap.Release()

	if snap.ReceiveOnlyChangedSize().TotalItems() == 0 {
		return nil, nil
	}

	p := newPager(page, perpage)
	recvEnc := cfg.Type == config.FolderTypeReceiveEncrypted
	files := make([]db.FileInfoTruncated, 0, perpage)

	snap.WithHaveTruncated(protocol.LocalDeviceID, func(f protocol.FileIntf) bool {
		if !f.IsReceiveOnlyChanged() || (recvEnc && f.IsDeleted()) {
			return true
		}
		if p.skip() {
			return true
		}
		ft := f.(db.FileInfoTruncated)
		files = append(files, ft)
		return !p.done()
	})

	return files, nil
}

type pager struct {
	toSkip, get int
}

func newPager(page, perpage int) *pager {
	return &pager{
		toSkip: (page - 1) * perpage,
		get:    perpage,
	}
}

func (p *pager) skip() bool {
	if p.toSkip == 0 {
		return false
	}
	p.toSkip--
	return true
}

func (p *pager) done() bool {
	if p.get > 0 {
		p.get--
	}
	return p.get == 0
}

// Index is called when a new device is connected and we receive their full index.
// Implements the protocol.Model interface.
func (m *model) Index(deviceID protocol.DeviceID, folder string, fs []protocol.FileInfo) error {
	return m.handleIndex(deviceID, folder, fs, false)
}

// IndexUpdate is called for incremental updates to connected devices' indexes.
// Implements the protocol.Model interface.
func (m *model) IndexUpdate(deviceID protocol.DeviceID, folder string, fs []protocol.FileInfo) error {
	return m.handleIndex(deviceID, folder, fs, true)
}

func (m *model) handleIndex(deviceID protocol.DeviceID, folder string, fs []protocol.FileInfo, update bool) error {
	op := "Index"
	if update {
		op += " update"
	}

	l.Debugf("%v (in): %s / %q: %d files", op, deviceID, folder, len(fs))

	if cfg, ok := m.cfg.Folder(folder); !ok || !cfg.SharedWith(deviceID) {
		l.Infof("%v for unexpected folder ID %q sent from device %q; ensure that the folder exists and that this device is selected under \"Share With\" in the folder configuration.", op, folder, deviceID)
		return errors.Wrap(ErrFolderMissing, folder)
	} else if cfg.Paused {
		l.Debugf("%v for paused folder (ID %q) sent from device %q.", op, folder, deviceID)
		return errors.Wrap(ErrFolderPaused, folder)
	}

	m.fmut.RLock()
	files, existing := m.folderFiles[folder]
	runner, running := m.folderRunners[folder]
	m.fmut.RUnlock()

	if !existing {
		l.Infof("%v for nonexistent folder %q", op, folder)
		return errors.Wrap(ErrFolderMissing, folder)
	}

	if running {
		defer runner.SchedulePull()
	}

	m.pmut.RLock()
	downloads := m.deviceDownloads[deviceID]
	m.pmut.RUnlock()
	downloads.Update(folder, makeForgetUpdate(fs))

	if !update {
		files.Drop(deviceID)
	}
	for i := range fs {
		// The local attributes should never be transmitted over the wire.
		// Make sure they look like they weren't.
		fs[i].LocalFlags = 0
		fs[i].VersionHash = nil
	}
	files.Update(deviceID, fs)

	seq := files.Sequence(deviceID)
	m.evLogger.Log(events.RemoteIndexUpdated, map[string]interface{}{
		"device":   deviceID.String(),
		"folder":   folder,
		"items":    len(fs),
		"sequence": seq,
		"version":  seq, // legacy for sequence
	})

	return nil
}

func (m *model) ClusterConfig(deviceID protocol.DeviceID, cm protocol.ClusterConfig) error {
	// Check the peer device's announced folders against our own. Emits events
	// for folders that we don't expect (unknown or not shared).
	// Also, collect a list of folders we do share, and if he's interested in
	// temporary indexes, subscribe the connection.

	m.pmut.RLock()
	indexSenderRegistry, ok := m.indexSenders[deviceID]
	m.pmut.RUnlock()
	if !ok {
		panic("bug: ClusterConfig called on closed or nonexistent connection")
	}

	deviceCfg, ok := m.cfg.Device(deviceID)
	if !ok {
		l.Debugln("Device disappeared from config while processing cluster-config")
		return errDeviceUnknown
	}

	// Assemble the device information from the connected device about
	// themselves and us for all folders.
	ccDeviceInfos := make(map[string]*indexSenderStartInfo, len(cm.Folders))
	for _, folder := range cm.Folders {
		info := &indexSenderStartInfo{}
		for _, dev := range folder.Devices {
			if dev.ID == m.id {
				info.local = dev
			} else if dev.ID == deviceID {
				info.remote = dev
			}
			if info.local.ID != protocol.EmptyDeviceID && info.remote.ID != protocol.EmptyDeviceID {
				break
			}
		}
		if info.remote.ID == protocol.EmptyDeviceID {
			l.Infof("Device %v sent cluster-config without the device info for the remote on folder %v", deviceID, folder.Description())
			return errMissingRemoteInClusterConfig
		}
		if info.local.ID == protocol.EmptyDeviceID {
			l.Infof("Device %v sent cluster-config without the device info for us locally on folder %v", deviceID, folder.Description())
			return errMissingLocalInClusterConfig
		}
		ccDeviceInfos[folder.ID] = info
	}

	// Needs to happen outside of the fmut, as can cause CommitConfiguration
	if deviceCfg.AutoAcceptFolders {
		w, _ := m.cfg.Modify(func(cfg *config.Configuration) {
			changedFcfg := make(map[string]config.FolderConfiguration)
			haveFcfg := cfg.FolderMap()
			for _, folder := range cm.Folders {
				from, ok := haveFcfg[folder.ID]
				if to, changed := m.handleAutoAccepts(deviceID, folder, ccDeviceInfos[folder.ID], from, ok, cfg.Defaults.Folder.Path); changed {
					changedFcfg[folder.ID] = to
				}
			}
			if len(changedFcfg) == 0 {
				return
			}
			for i := range cfg.Folders {
				if fcfg, ok := changedFcfg[cfg.Folders[i].ID]; ok {
					cfg.Folders[i] = fcfg
					delete(changedFcfg, cfg.Folders[i].ID)
				}
			}
			for _, fcfg := range changedFcfg {
				cfg.Folders = append(cfg.Folders, fcfg)
			}
		})
		// Need to wait for the waiter, as this calls CommitConfiguration,
		// which sets up the folder and as we return from this call,
		// ClusterConfig starts poking at m.folderFiles and other things
		// that might not exist until the config is committed.
		w.Wait()
	}

	tempIndexFolders, paused, err := m.ccHandleFolders(cm.Folders, deviceCfg, ccDeviceInfos, indexSenderRegistry)
	if err != nil {
		return err
	}

	m.pmut.Lock()
	m.remotePausedFolders[deviceID] = paused
	m.pmut.Unlock()

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
		m.cfg.Modify(func(cfg *config.Configuration) {
			folders, devices, foldersDevices, introduced := m.handleIntroductions(deviceCfg, cm, cfg.FolderMap(), cfg.DeviceMap())
			folders, devices, deintroduced := m.handleDeintroductions(deviceCfg, foldersDevices, folders, devices)
			if !introduced && !deintroduced {
				return
			}
			cfg.Folders = make([]config.FolderConfiguration, 0, len(folders))
			for _, fcfg := range folders {
				cfg.Folders = append(cfg.Folders, fcfg)
			}
			cfg.Devices = make([]config.DeviceConfiguration, len(devices))
			for _, dcfg := range devices {
				cfg.Devices = append(cfg.Devices, dcfg)
			}
		})
	}

	return nil
}

func (m *model) ccHandleFolders(folders []protocol.Folder, deviceCfg config.DeviceConfiguration, ccDeviceInfos map[string]*indexSenderStartInfo, indexSenders *indexSenderRegistry) ([]string, map[string]struct{}, error) {
	var folderDevice config.FolderDeviceConfiguration
	tempIndexFolders := make([]string, 0, len(folders))
	paused := make(map[string]struct{}, len(folders))
	seenFolders := make(map[string]struct{}, len(folders))
	updatedPending := make([]updatedPendingFolder, 0, len(folders))
	deviceID := deviceCfg.DeviceID
	expiredPending, err := m.db.PendingFoldersForDevice(deviceID)
	if err != nil {
		l.Infof("Could not get pending folders for cleanup: %v", err)
	}
	of := db.ObservedFolder{Time: time.Now().Truncate(time.Second)}
	for _, folder := range folders {
		seenFolders[folder.ID] = struct{}{}

		cfg, ok := m.cfg.Folder(folder.ID)
		if ok {
			folderDevice, ok = cfg.Device(deviceID)
		}
		if !ok {
			indexSenders.remove(folder.ID)
			if deviceCfg.IgnoredFolder(folder.ID) {
				l.Infof("Ignoring folder %s from device %s since we are configured to", folder.Description(), deviceID)
				continue
			}
			delete(expiredPending, folder.ID)
			of.Label = folder.Label
			of.ReceiveEncrypted = len(ccDeviceInfos[folder.ID].local.EncryptionPasswordToken) > 0
			if err := m.db.AddOrUpdatePendingFolder(folder.ID, of, deviceID); err != nil {
				l.Warnf("Failed to persist pending folder entry to database: %v", err)
			}
			indexSenders.addPending(cfg, ccDeviceInfos[folder.ID])
			updatedPending = append(updatedPending, updatedPendingFolder{
				FolderID:         folder.ID,
				FolderLabel:      folder.Label,
				DeviceID:         deviceID,
				ReceiveEncrypted: of.ReceiveEncrypted,
			})
			// DEPRECATED: Only for backwards compatibility, should be removed.
			m.evLogger.Log(events.FolderRejected, map[string]string{
				"folder":      folder.ID,
				"folderLabel": folder.Label,
				"device":      deviceID.String(),
			})
			l.Infof("Unexpected folder %s sent from device %q; ensure that the folder exists and that this device is selected under \"Share With\" in the folder configuration.", folder.Description(), deviceID)
			continue
		}

		if folder.Paused {
			indexSenders.remove(folder.ID)
			paused[cfg.ID] = struct{}{}
			continue
		}

		if cfg.Paused {
			indexSenders.addPending(cfg, ccDeviceInfos[folder.ID])
			continue
		}

		if err := m.ccCheckEncryption(cfg, folderDevice, ccDeviceInfos[folder.ID], deviceCfg.Untrusted); err != nil {
			sameError := false
			if devs, ok := m.folderEncryptionFailures[folder.ID]; ok {
				sameError = devs[deviceID] == err
			} else {
				m.folderEncryptionFailures[folder.ID] = make(map[protocol.DeviceID]error)
			}
			m.folderEncryptionFailures[folder.ID][deviceID] = err
			msg := fmt.Sprintf("Failure checking encryption consistency with device %v for folder %v: %v", deviceID, cfg.Description(), err)
			if sameError {
				l.Debugln(msg)
			} else {
				l.Warnln(msg)
			}
			m.evLogger.Log(events.Failure, err.Error())
			return tempIndexFolders, paused, err
		}
		if devErrs, ok := m.folderEncryptionFailures[folder.ID]; ok {
			if len(devErrs) == 1 {
				delete(m.folderEncryptionFailures, folder.ID)
			} else {
				delete(m.folderEncryptionFailures[folder.ID], deviceID)
			}
		}

		// Handle indexes

		if !folder.DisableTempIndexes {
			tempIndexFolders = append(tempIndexFolders, folder.ID)
		}

		m.fmut.RLock()
		fs, ok := m.folderFiles[folder.ID]
		m.fmut.RUnlock()
		if !ok {
			// Shouldn't happen because !cfg.Paused, but might happen
			// if the folder is about to be unpaused, but not yet.
			l.Debugln("ccH: no fset", folder.ID)
			indexSenders.addPending(cfg, ccDeviceInfos[folder.ID])
			continue
		}

		indexSenders.add(cfg, fs, ccDeviceInfos[folder.ID])

		// We might already have files that we need to pull so let the
		// folder runner know that it should recheck the index data.
		m.fmut.RLock()
		if runner := m.folderRunners[folder.ID]; runner != nil {
			defer runner.SchedulePull()
		}
		m.fmut.RUnlock()
	}

	indexSenders.removeAllExcept(seenFolders)
	for folder := range expiredPending {
		m.db.RemovePendingFolderForDevice(folder, deviceID)
	}
	if len(updatedPending) > 0 || len(expiredPending) > 0 {
		expiredPendingList := make([]map[string]string, 0, len(expiredPending))
		for folderID := range expiredPending {
			expiredPendingList = append(expiredPendingList, map[string]string{
				"folderID": folderID,
				"deviceID": deviceID.String(),
			})
		}
		m.evLogger.Log(events.PendingFoldersChanged, map[string]interface{}{
			"added":   updatedPending,
			"removed": expiredPendingList,
		})
	}

	return tempIndexFolders, paused, nil
}

func (m *model) ccCheckEncryption(fcfg config.FolderConfiguration, folderDevice config.FolderDeviceConfiguration, ccDeviceInfos *indexSenderStartInfo, deviceUntrusted bool) error {
	hasTokenRemote := len(ccDeviceInfos.remote.EncryptionPasswordToken) > 0
	hasTokenLocal := len(ccDeviceInfos.local.EncryptionPasswordToken) > 0
	isEncryptedRemote := folderDevice.EncryptionPassword != ""
	isEncryptedLocal := fcfg.Type == config.FolderTypeReceiveEncrypted

	if !isEncryptedRemote && !isEncryptedLocal && deviceUntrusted {
		return errEncryptionNotEncryptedUntrusted
	}

	if !(hasTokenRemote || hasTokenLocal || isEncryptedRemote || isEncryptedLocal) {
		// Noone cares about encryption here
		return nil
	}

	if isEncryptedRemote && isEncryptedLocal {
		// Should never happen, but config racyness and be safe.
		return errEncryptionInvConfigLocal
	}

	if hasTokenRemote && hasTokenLocal {
		return errEncryptionInvConfigRemote
	}

	if !(hasTokenRemote || hasTokenLocal) {
		return errEncryptionNotEncryptedRemote
	}

	if !(isEncryptedRemote || isEncryptedLocal) {
		return errEncryptionNotEncryptedLocal
	}

	if isEncryptedRemote {
		passwordToken := protocol.PasswordToken(fcfg.ID, folderDevice.EncryptionPassword)
		match := false
		if hasTokenLocal {
			match = bytes.Equal(passwordToken, ccDeviceInfos.local.EncryptionPasswordToken)
		} else {
			// hasTokenRemote == true
			match = bytes.Equal(passwordToken, ccDeviceInfos.remote.EncryptionPasswordToken)
		}
		if !match {
			return errEncryptionPassword
		}
		return nil
	}

	// isEncryptedLocal == true

	var ccToken []byte
	if hasTokenLocal {
		ccToken = ccDeviceInfos.local.EncryptionPasswordToken
	} else {
		// hasTokenRemote == true
		ccToken = ccDeviceInfos.remote.EncryptionPasswordToken
	}
	m.fmut.RLock()
	token, ok := m.folderEncryptionPasswordTokens[fcfg.ID]
	m.fmut.RUnlock()
	if !ok {
		var err error
		token, err = readEncryptionToken(fcfg)
		if err != nil && !fs.IsNotExist(err) {
			return err
		}
		if err == nil {
			m.fmut.Lock()
			m.folderEncryptionPasswordTokens[fcfg.ID] = token
			m.fmut.Unlock()
		} else {
			if err := writeEncryptionToken(ccToken, fcfg); err != nil {
				return err
			}
			m.fmut.Lock()
			m.folderEncryptionPasswordTokens[fcfg.ID] = ccToken
			m.fmut.Unlock()
			// We can only announce ourselfs once we have the token,
			// thus we need to resend CCs now that we have it.
			m.sendClusterConfig(fcfg.DeviceIDs())
			return nil
		}
	}
	if !bytes.Equal(token, ccToken) {
		return errEncryptionPassword
	}
	return nil
}

func (m *model) sendClusterConfig(ids []protocol.DeviceID) {
	if len(ids) == 0 {
		return
	}
	ccConns := make([]protocol.Connection, 0, len(ids))
	m.pmut.RLock()
	for _, id := range ids {
		if conn, ok := m.conn[id]; ok {
			ccConns = append(ccConns, conn)
		}
	}
	m.pmut.RUnlock()
	// Generating cluster-configs acquires fmut -> must happen outside of pmut.
	for _, conn := range ccConns {
		cm, passwords, err := m.generateClusterConfig(conn.ID())
		if err != nil {
			if err != errEncryptionNeedToken {
				m.evLogger.Log(events.Failure, failureUnexpectedGenerateCCError)
				continue
			}
			go conn.Close(err)
			continue
		}
		conn.SetFolderPasswords(passwords)
		go conn.ClusterConfig(cm)
	}
}

// handleIntroductions handles adding devices/folders that are shared by an introducer device
func (m *model) handleIntroductions(introducerCfg config.DeviceConfiguration, cm protocol.ClusterConfig, folders map[string]config.FolderConfiguration, devices map[protocol.DeviceID]config.DeviceConfiguration) (map[string]config.FolderConfiguration, map[protocol.DeviceID]config.DeviceConfiguration, folderDeviceSet, bool) {
	changed := false

	foldersDevices := make(folderDeviceSet)

	for _, folder := range cm.Folders {
		// Adds devices which we do not have, but the introducer has
		// for the folders that we have in common. Also, shares folders
		// with devices that we have in common, yet are currently not sharing
		// the folder.

		fcfg, ok := folders[folder.ID]
		if !ok {
			// Don't have this folder, carry on.
			continue
		}

		folderChanged := false

		for _, device := range folder.Devices {
			// No need to share with self.
			if device.ID == m.id {
				continue
			}

			foldersDevices.set(device.ID, folder.ID)

			if _, ok := devices[device.ID]; !ok {
				// The device is currently unknown. Add it to the config.
				devices[device.ID] = m.introduceDevice(device, introducerCfg)
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
			folderChanged = true
		}

		if folderChanged {
			folders[fcfg.ID] = fcfg
			changed = true
		}
	}

	return folders, devices, foldersDevices, changed
}

// handleDeintroductions handles removals of devices/shares that are removed by an introducer device
func (m *model) handleDeintroductions(introducerCfg config.DeviceConfiguration, foldersDevices folderDeviceSet, folders map[string]config.FolderConfiguration, devices map[protocol.DeviceID]config.DeviceConfiguration) (map[string]config.FolderConfiguration, map[protocol.DeviceID]config.DeviceConfiguration, bool) {
	if introducerCfg.SkipIntroductionRemovals {
		return folders, devices, false
	}

	changed := false
	devicesNotIntroduced := make(map[protocol.DeviceID]struct{})

	// Check if we should unshare some folders, if the introducer has unshared them.
	for folderID, folderCfg := range folders {
		for k := 0; k < len(folderCfg.Devices); k++ {
			if folderCfg.Devices[k].IntroducedBy != introducerCfg.DeviceID {
				devicesNotIntroduced[folderCfg.Devices[k].DeviceID] = struct{}{}
				continue
			}
			if !foldersDevices.has(folderCfg.Devices[k].DeviceID, folderCfg.ID) {
				// We could not find that folder shared on the
				// introducer with the device that was introduced to us.
				// We should follow and unshare as well.
				l.Infof("Unsharing folder %s with %v as introducer %v no longer shares the folder with that device", folderCfg.Description(), folderCfg.Devices[k].DeviceID, folderCfg.Devices[k].IntroducedBy)
				folderCfg.Devices = append(folderCfg.Devices[:k], folderCfg.Devices[k+1:]...)
				folders[folderID] = folderCfg
				k--
				changed = true
			}
		}
	}

	// Check if we should remove some devices, if the introducer no longer
	// shares any folder with them. Yet do not remove if we share other
	// folders that haven't been introduced by the introducer.
	for deviceID, device := range devices {
		if device.IntroducedBy == introducerCfg.DeviceID {
			if !foldersDevices.hasDevice(deviceID) {
				if _, ok := devicesNotIntroduced[deviceID]; !ok {
					// The introducer no longer shares any folder with the
					// device, remove the device.
					l.Infof("Removing device %v as introducer %v no longer shares any folders with that device", deviceID, device.IntroducedBy)
					changed = true
					delete(devices, deviceID)
					continue
				}
				l.Infof("Would have removed %v as %v no longer shares any folders, yet there are other folders that are shared with this device that haven't been introduced by this introducer.", deviceID, device.IntroducedBy)
			}
		}
	}

	return folders, devices, changed
}

// handleAutoAccepts handles adding and sharing folders for devices that have
// AutoAcceptFolders set to true.
func (m *model) handleAutoAccepts(deviceID protocol.DeviceID, folder protocol.Folder, ccDeviceInfos *indexSenderStartInfo, cfg config.FolderConfiguration, haveCfg bool, defaultPath string) (config.FolderConfiguration, bool) {
	if !haveCfg {
		defaultPathFs := fs.NewFilesystem(fs.FilesystemTypeBasic, defaultPath)
		pathAlternatives := []string{
			fs.SanitizePath(folder.Label),
			fs.SanitizePath(folder.ID),
		}
		for _, path := range pathAlternatives {
			if _, err := defaultPathFs.Lstat(path); !fs.IsNotExist(err) {
				continue
			}

			fcfg := newFolderConfiguration(m.cfg, folder.ID, folder.Label, fs.FilesystemTypeBasic, filepath.Join(defaultPath, path))
			fcfg.Devices = append(fcfg.Devices, config.FolderDeviceConfiguration{
				DeviceID: deviceID,
			})

			if len(ccDeviceInfos.remote.EncryptionPasswordToken) > 0 || len(ccDeviceInfos.local.EncryptionPasswordToken) > 0 {
				fcfg.Type = config.FolderTypeReceiveEncrypted
			} else {
				ignores := m.cfg.DefaultIgnores()
				if err := m.SetIgnores(fcfg.ID, ignores.Lines); err != nil {
					l.Warnf("Failed to apply default ignores to auto-accepted folder %s at path %s: %v", folder.Description(), fcfg.Path, err)
				}
			}

			l.Infof("Auto-accepted %s folder %s at path %s", deviceID, folder.Description(), fcfg.Path)
			return fcfg, true
		}
		l.Infof("Failed to auto-accept folder %s from %s due to path conflict", folder.Description(), deviceID)
		return config.FolderConfiguration{}, false
	} else {
		for _, device := range cfg.DeviceIDs() {
			if device == deviceID {
				// Already shared nothing todo.
				return config.FolderConfiguration{}, false
			}
		}
		if cfg.Type == config.FolderTypeReceiveEncrypted {
			if len(ccDeviceInfos.remote.EncryptionPasswordToken) == 0 && len(ccDeviceInfos.local.EncryptionPasswordToken) == 0 {
				l.Infof("Failed to auto-accept device %s on existing folder %s as the remote wants to send us unencrypted data, but the folder type is receive-encrypted", folder.Description(), deviceID)
				return config.FolderConfiguration{}, false
			}
		} else {
			if len(ccDeviceInfos.remote.EncryptionPasswordToken) > 0 || len(ccDeviceInfos.local.EncryptionPasswordToken) > 0 {
				l.Infof("Failed to auto-accept device %s on existing folder %s as the remote wants to send us encrypted data, but the folder type is not receive-encrypted", folder.Description(), deviceID)
				return config.FolderConfiguration{}, false
			}
		}
		cfg.Devices = append(cfg.Devices, config.FolderDeviceConfiguration{
			DeviceID: deviceID,
		})
		l.Infof("Shared %s with %s due to auto-accept", folder.ID, deviceID)
		return cfg, true
	}
}

func (m *model) introduceDevice(device protocol.Device, introducerCfg config.DeviceConfiguration) config.DeviceConfiguration {
	addresses := []string{"dynamic"}
	for _, addr := range device.Addresses {
		if addr != "dynamic" {
			addresses = append(addresses, addr)
		}
	}

	l.Infof("Adding device %v to config (vouched for by introducer %v)", device.ID, introducerCfg.DeviceID)
	newDeviceCfg := m.cfg.DefaultDevice()
	newDeviceCfg.DeviceID = device.ID
	newDeviceCfg.Name = device.Name
	newDeviceCfg.Compression = introducerCfg.Compression
	newDeviceCfg.Addresses = addresses
	newDeviceCfg.CertName = device.CertName
	newDeviceCfg.IntroducedBy = introducerCfg.DeviceID

	// The introducers' introducers are also our introducers.
	if device.Introducer {
		l.Infof("Device %v is now also an introducer", device.ID)
		newDeviceCfg.Introducer = true
		newDeviceCfg.SkipIntroductionRemovals = device.SkipIntroductionRemovals
	}

	return newDeviceCfg
}

// Closed is called when a connection has been closed
func (m *model) Closed(device protocol.DeviceID, err error) {
	m.pmut.Lock()
	conn, ok := m.conn[device]
	if !ok {
		m.pmut.Unlock()
		return
	}

	delete(m.conn, device)
	delete(m.connRequestLimiters, device)
	delete(m.helloMessages, device)
	delete(m.deviceDownloads, device)
	delete(m.remotePausedFolders, device)
	closed := m.closed[device]
	delete(m.closed, device)
	delete(m.indexSenders, device)
	m.pmut.Unlock()

	m.progressEmitter.temporaryIndexUnsubscribe(conn)
	m.deviceDidClose(device, time.Since(conn.EstablishedAt()))

	l.Infof("Connection to %s at %s closed: %v", device, conn, err)
	m.evLogger.Log(events.DeviceDisconnected, map[string]string{
		"id":    device.String(),
		"error": err.Error(),
	})
	close(closed)
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
func (m *model) Request(deviceID protocol.DeviceID, folder, name string, blockNo, size int32, offset int64, hash []byte, weakHash uint32, fromTemporary bool) (out protocol.RequestResponse, err error) {
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
		return nil, protocol.ErrGeneric
	}

	if !folderCfg.SharedWith(deviceID) {
		l.Warnf("Request from %s for file %s in unshared folder %q", deviceID, name, folder)
		return nil, protocol.ErrGeneric
	}
	if folderCfg.Paused {
		l.Debugf("Request from %s for file %s in paused folder %q", deviceID, name, folder)
		return nil, protocol.ErrGeneric
	}

	// Make sure the path is valid and in canonical form
	if name, err = fs.Canonicalize(name); err != nil {
		l.Debugf("Request from %s in folder %q for invalid filename %s", deviceID, folder, name)
		return nil, protocol.ErrGeneric
	}

	if deviceID != protocol.LocalDeviceID {
		l.Debugf("%v REQ(in): %s: %q / %q o=%d s=%d t=%v", m, deviceID, folder, name, offset, size, fromTemporary)
	}

	if fs.IsInternal(name) {
		l.Debugf("%v REQ(in) for internal file: %s: %q / %q o=%d s=%d", m, deviceID, folder, name, offset, size)
		return nil, protocol.ErrInvalid
	}

	if folderIgnores.Match(name).IsIgnored() {
		l.Debugf("%v REQ(in) for ignored file: %s: %q / %q o=%d s=%d", m, deviceID, folder, name, offset, size)
		return nil, protocol.ErrInvalid
	}

	// Restrict parallel requests by connection/device

	m.pmut.RLock()
	limiter := m.connRequestLimiters[deviceID]
	m.pmut.RUnlock()

	// The requestResponse releases the bytes to the buffer pool and the
	// limiters when its Close method is called.
	res := newLimitedRequestResponse(int(size), limiter, m.globalRequestLimiter)

	defer func() {
		// Close it ourselves if it isn't returned due to an error
		if err != nil {
			res.Close()
		}
	}()

	// Grab the FS after limiting, as it causes I/O and we want to minimize
	// the race time between the symlink check and the read.

	folderFs := folderCfg.Filesystem()

	if err := osutil.TraversesSymlink(folderFs, filepath.Dir(name)); err != nil {
		l.Debugf("%v REQ(in) traversal check: %s - %s: %q / %q o=%d s=%d", m, err, deviceID, folder, name, offset, size)
		return nil, protocol.ErrNoSuchFile
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
		_, err := readOffsetIntoBuf(folderFs, tempFn, offset, res.data)
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

	n, err := readOffsetIntoBuf(folderFs, name, offset, res.data)
	if fs.IsNotExist(err) {
		l.Debugf("%v REQ(in) file doesn't exist: %s: %q / %q o=%d s=%d", m, deviceID, folder, name, offset, size)
		return nil, protocol.ErrNoSuchFile
	} else if err == io.EOF {
		// Read beyond end of file. This might indicate a problem, or it
		// might be a short block that gets padded when read for encrypted
		// folders. We ignore the error and let the hash validation in the
		// next step take care of it, by only hashing the part we actually
		// managed to read.
	} else if err != nil {
		l.Debugf("%v REQ(in) failed reading file (%v): %s: %q / %q o=%d s=%d", m, err, deviceID, folder, name, offset, size)
		return nil, protocol.ErrGeneric
	}

	if len(hash) > 0 && !scanner.Validate(res.data[:n], hash, weakHash) {
		m.recheckFile(deviceID, folder, name, offset, hash, weakHash)
		l.Debugf("%v REQ(in) failed validating data: %s: %q / %q o=%d s=%d", m, deviceID, folder, name, offset, size)
		return nil, protocol.ErrNoSuchFile
	}

	return res, nil
}

// newLimitedRequestResponse takes size bytes from the limiters in order,
// skipping nil limiters, then returns a requestResponse of the given size.
// When the requestResponse is closed the limiters are given back the bytes,
// in reverse order.
func newLimitedRequestResponse(size int, limiters ...*byteSemaphore) *requestResponse {
	for _, limiter := range limiters {
		if limiter != nil {
			limiter.take(size)
		}
	}

	res := newRequestResponse(size)

	go func() {
		res.Wait()
		for i := range limiters {
			limiter := limiters[len(limiters)-1-i]
			if limiter != nil {
				limiter.give(size)
			}
		}
	}()

	return res
}

func (m *model) recheckFile(deviceID protocol.DeviceID, folder, name string, offset int64, hash []byte, weakHash uint32) {
	cf, ok, err := m.CurrentFolderFile(folder, name)
	if err != nil {
		l.Debugf("%v recheckFile: %s: %q / %q: current file error: %v", m, deviceID, folder, name, err)
		return
	}
	if !ok {
		l.Debugf("%v recheckFile: %s: %q / %q: no current file", m, deviceID, folder, name)
		return
	}

	if cf.IsDeleted() || cf.IsInvalid() || cf.IsSymlink() || cf.IsDirectory() {
		l.Debugf("%v recheckFile: %s: %q / %q: not a regular file", m, deviceID, folder, name)
		return
	}

	blockIndex := int(offset / int64(cf.BlockSize()))
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
	if weakHash != 0 && block.WeakHash != weakHash {
		l.Debugf("%v recheckFile: %s: %q / %q i=%d: weak hash mismatch %v != %v", m, deviceID, folder, name, blockIndex, block.WeakHash, weakHash)
		return
	}

	// The hashes provided part of the request match what we expect to find according
	// to what we have in the database, yet the content we've read off the filesystem doesn't
	// Something is fishy, invalidate the file and rescan it.
	// The file will temporarily become invalid, which is ok as the content is messed up.
	m.fmut.RLock()
	runner, ok := m.folderRunners[folder]
	m.fmut.RUnlock()
	if !ok {
		l.Debugf("%v recheckFile: %s: %q / %q: Folder stopped before rescan could be scheduled", m, deviceID, folder, name)
		return
	}

	runner.ScheduleForceRescan(name)

	l.Debugf("%v recheckFile: %s: %q / %q", m, deviceID, folder, name)
}

func (m *model) CurrentFolderFile(folder string, file string) (protocol.FileInfo, bool, error) {
	m.fmut.RLock()
	fs, ok := m.folderFiles[folder]
	m.fmut.RUnlock()
	if !ok {
		return protocol.FileInfo{}, false, ErrFolderMissing
	}
	snap, err := fs.Snapshot()
	if err != nil {
		return protocol.FileInfo{}, false, err
	}
	f, ok := snap.Get(protocol.LocalDeviceID, file)
	snap.Release()
	return f, ok, nil
}

func (m *model) CurrentGlobalFile(folder string, file string) (protocol.FileInfo, bool, error) {
	m.fmut.RLock()
	fs, ok := m.folderFiles[folder]
	m.fmut.RUnlock()
	if !ok {
		return protocol.FileInfo{}, false, ErrFolderMissing
	}
	snap, err := fs.Snapshot()
	if err != nil {
		return protocol.FileInfo{}, false, err
	}
	f, ok := snap.GetGlobal(file)
	snap.Release()
	return f, ok, nil
}

// Connection returns the current connection for device, and a boolean whether a connection was found.
func (m *model) Connection(deviceID protocol.DeviceID) (protocol.Connection, bool) {
	m.pmut.RLock()
	cn, ok := m.conn[deviceID]
	m.pmut.RUnlock()
	if ok {
		m.deviceWasSeen(deviceID)
	}
	return cn, ok
}

// LoadIgnores loads or refreshes the ignore patterns from disk, if the
// folder is healthy, and returns the refreshed lines and patterns.
func (m *model) LoadIgnores(folder string) ([]string, []string, error) {
	m.fmut.RLock()
	cfg, cfgOk := m.folderCfgs[folder]
	ignores, ignoresOk := m.folderIgnores[folder]
	m.fmut.RUnlock()

	if !cfgOk {
		cfg, cfgOk = m.cfg.Folder(folder)
		if !cfgOk {
			return nil, nil, fmt.Errorf("folder %s does not exist", folder)
		}
	}

	if cfg.Type == config.FolderTypeReceiveEncrypted {
		return nil, nil, nil
	}

	// On creation a new folder with ignore patterns validly has no marker yet.
	if err := cfg.CheckPath(); err != nil && err != config.ErrMarkerMissing {
		return nil, nil, err
	}

	if !ignoresOk {
		ignores = ignore.New(cfg.Filesystem())
	}

	err := ignores.Load(".stignore")
	if fs.IsNotExist(err) {
		// Having no ignores is not an error.
		return nil, nil, nil
	}

	// Return lines and patterns, which may have some meaning even when err
	// != nil, depending on the specific error.
	return ignores.Lines(), ignores.Patterns(), err
}

// CurrentIgnores returns the currently loaded set of ignore patterns,
// whichever it may be. No attempt is made to load or refresh ignore
// patterns from disk.
func (m *model) CurrentIgnores(folder string) ([]string, []string, error) {
	m.fmut.RLock()
	_, cfgOk := m.folderCfgs[folder]
	ignores, ignoresOk := m.folderIgnores[folder]
	m.fmut.RUnlock()

	if !cfgOk {
		return nil, nil, fmt.Errorf("folder %s does not exist", folder)
	}

	if !ignoresOk {
		// Empty ignore patterns
		return []string{}, []string{}, nil
	}

	return ignores.Lines(), ignores.Patterns(), nil
}

func (m *model) SetIgnores(folder string, content []string) error {
	cfg, ok := m.cfg.Folder(folder)
	if !ok {
		return fmt.Errorf("folder %s does not exist", cfg.Description())
	}

	err := cfg.CheckPath()
	if err == config.ErrPathMissing {
		if err = cfg.CreateRoot(); err != nil {
			return errors.Wrap(err, "failed to create folder root")
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
func (m *model) OnHello(remoteID protocol.DeviceID, addr net.Addr, hello protocol.Hello) error {
	if m.cfg.IgnoredDevice(remoteID) {
		return errDeviceIgnored
	}

	cfg, ok := m.cfg.Device(remoteID)
	if !ok {
		if err := m.db.AddOrUpdatePendingDevice(remoteID, hello.DeviceName, addr.String()); err != nil {
			l.Warnf("Failed to persist pending device entry to database: %v", err)
		}
		m.evLogger.Log(events.PendingDevicesChanged, map[string][]interface{}{
			"added": {map[string]string{
				"deviceID": remoteID.String(),
				"name":     hello.DeviceName,
				"address":  addr.String(),
			}},
		})
		// DEPRECATED: Only for backwards compatibility, should be removed.
		m.evLogger.Log(events.DeviceRejected, map[string]string{
			"name":    hello.DeviceName,
			"device":  remoteID.String(),
			"address": addr.String(),
		})
		return errDeviceUnknown
	}

	if cfg.Paused {
		return errDevicePaused
	}

	if len(cfg.AllowedNetworks) > 0 && !connections.IsAllowedNetwork(addr.String(), cfg.AllowedNetworks) {
		// The connection is not from an allowed network.
		return errNetworkNotAllowed
	}

	if max := m.cfg.Options().ConnectionLimitMax; max > 0 && m.NumConnections() >= max {
		// We're not allowed to accept any more connections.
		return errConnLimitReached
	}

	return nil
}

// GetHello is called when we are about to connect to some remote device.
func (m *model) GetHello(id protocol.DeviceID) protocol.HelloIntf {
	name := ""
	if _, ok := m.cfg.Device(id); ok {
		// Set our name (from the config of our device ID) only if we already know about the other side device ID.
		if myCfg, ok := m.cfg.Device(m.id); ok {
			name = myCfg.Name
		}
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
func (m *model) AddConnection(conn protocol.Connection, hello protocol.Hello) {
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
		oldConn.Close(errReplacingConnection)
		<-closed
		m.pmut.Lock()
	}

	m.conn[deviceID] = conn
	closed := make(chan struct{})
	m.closed[deviceID] = closed
	m.deviceDownloads[deviceID] = newDeviceDownloadState()
	m.indexSenders[deviceID] = newIndexSenderRegistry(conn, closed, m.Supervisor, m.evLogger)
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

	m.evLogger.Log(events.DeviceConnected, event)

	l.Infof(`Device %s client is "%s %s" named "%s" at %s`, deviceID, hello.ClientName, hello.ClientVersion, hello.DeviceName, conn)

	conn.Start()
	m.pmut.Unlock()

	// Acquires fmut, so has to be done outside of pmut.
	cm, passwords, err := m.generateClusterConfig(deviceID)
	// We ignore errEncryptionNeedToken on a new connection, as the missing
	// token should be delivered in the cluster-config about to be received.
	if err != nil && err != errEncryptionNeedToken {
		m.evLogger.Log(events.Failure, failureUnexpectedGenerateCCError)
	}
	conn.SetFolderPasswords(passwords)
	conn.ClusterConfig(cm)

	if (device.Name == "" || m.cfg.Options().OverwriteRemoteDevNames) && hello.DeviceName != "" {
		m.cfg.Modify(func(cfg *config.Configuration) {
			for i := range cfg.Devices {
				if cfg.Devices[i].DeviceID == deviceID {
					if cfg.Devices[i].Name == "" || cfg.Options.OverwriteRemoteDevNames {
						cfg.Devices[i].Name = hello.DeviceName
					}
					return
				}
			}
		})
	}

	m.deviceWasSeen(deviceID)
}

func (m *model) DownloadProgress(device protocol.DeviceID, folder string, updates []protocol.FileDownloadProgressUpdate) error {
	m.fmut.RLock()
	cfg, ok := m.folderCfgs[folder]
	m.fmut.RUnlock()

	if !ok || cfg.DisableTempIndexes || !cfg.SharedWith(device) {
		return nil
	}

	m.pmut.RLock()
	downloads := m.deviceDownloads[device]
	m.pmut.RUnlock()
	downloads.Update(folder, updates)
	state := downloads.GetBlockCounts(folder)

	m.evLogger.Log(events.RemoteDownloadProgress, map[string]interface{}{
		"device": device.String(),
		"folder": folder,
		"state":  state,
	})

	return nil
}

func (m *model) deviceWasSeen(deviceID protocol.DeviceID) {
	m.fmut.RLock()
	sr, ok := m.deviceStatRefs[deviceID]
	m.fmut.RUnlock()
	if ok {
		_ = sr.WasSeen()
	}
}

func (m *model) deviceDidClose(deviceID protocol.DeviceID, duration time.Duration) {
	m.fmut.RLock()
	sr, ok := m.deviceStatRefs[deviceID]
	m.fmut.RUnlock()
	if ok {
		_ = sr.LastConnectionDuration(duration)
	}
}

func (m *model) requestGlobal(ctx context.Context, deviceID protocol.DeviceID, folder, name string, blockNo int, offset int64, size int, hash []byte, weakHash uint32, fromTemporary bool) ([]byte, error) {
	m.pmut.RLock()
	nc, ok := m.conn[deviceID]
	m.pmut.RUnlock()

	if !ok {
		return nil, fmt.Errorf("requestGlobal: no such device: %s", deviceID)
	}

	l.Debugf("%v REQ(out): %s: %q / %q b=%d o=%d s=%d h=%x wh=%x ft=%t", m, deviceID, folder, name, blockNo, offset, size, hash, weakHash, fromTemporary)

	return nc.Request(ctx, folder, name, blockNo, offset, size, hash, weakHash, fromTemporary)
}

func (m *model) ScanFolders() map[string]error {
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
			}
			wg.Done()
		}()
	}
	wg.Wait()
	return errors
}

func (m *model) ScanFolder(folder string) error {
	return m.ScanFolderSubdirs(folder, nil)
}

func (m *model) ScanFolderSubdirs(folder string, subs []string) error {
	m.fmut.RLock()
	err := m.checkFolderRunningLocked(folder)
	runner := m.folderRunners[folder]
	m.fmut.RUnlock()

	if err != nil {
		return err
	}

	return runner.Scan(subs)
}

func (m *model) DelayScan(folder string, next time.Duration) {
	m.fmut.RLock()
	runner, ok := m.folderRunners[folder]
	m.fmut.RUnlock()
	if !ok {
		return
	}
	runner.DelayScan(next)
}

// numHashers returns the number of hasher routines to use for a given folder,
// taking into account configuration and available CPU cores.
func (m *model) numHashers(folder string) int {
	m.fmut.RLock()
	folderCfg := m.folderCfgs[folder]
	numFolders := len(m.folderCfgs)
	m.fmut.RUnlock()

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

// generateClusterConfig returns a ClusterConfigMessage that is correct and the
// set of folder passwords for the given peer device
func (m *model) generateClusterConfig(device protocol.DeviceID) (protocol.ClusterConfig, map[string]string, error) {
	var message protocol.ClusterConfig

	m.fmut.RLock()
	defer m.fmut.RUnlock()

	folders := m.cfg.FolderList()
	passwords := make(map[string]string, len(folders))
	for _, folderCfg := range folders {
		if !folderCfg.SharedWith(device) {
			continue
		}

		var encryptionToken []byte
		var hasEncryptionToken bool
		if folderCfg.Type == config.FolderTypeReceiveEncrypted {
			if encryptionToken, hasEncryptionToken = m.folderEncryptionPasswordTokens[folderCfg.ID]; !hasEncryptionToken {
				// We haven't gotten a token yet and without one the other side
				// can't validate us - reset the connection to trigger a new
				// cluster-config and get the token.
				return message, nil, errEncryptionNeedToken
			}
		}

		protocolFolder := protocol.Folder{
			ID:                 folderCfg.ID,
			Label:              folderCfg.Label,
			ReadOnly:           folderCfg.Type == config.FolderTypeSendOnly,
			IgnorePermissions:  folderCfg.IgnorePerms,
			IgnoreDelete:       folderCfg.IgnoreDelete,
			DisableTempIndexes: folderCfg.DisableTempIndexes,
		}

		fs := m.folderFiles[folderCfg.ID]

		// Even if we aren't paused, if we haven't started the folder yet
		// pretend we are. Otherwise the remote might get confused about
		// the missing index info (and drop all the info). We will send
		// another cluster config once the folder is started.
		protocolFolder.Paused = folderCfg.Paused || fs == nil

		for _, folderDevice := range folderCfg.Devices {
			deviceCfg, _ := m.cfg.Device(folderDevice.DeviceID)

			protocolDevice := protocol.Device{
				ID:          deviceCfg.DeviceID,
				Name:        deviceCfg.Name,
				Addresses:   deviceCfg.Addresses,
				Compression: deviceCfg.Compression,
				CertName:    deviceCfg.CertName,
				Introducer:  deviceCfg.Introducer,
			}

			if deviceCfg.DeviceID == m.id && hasEncryptionToken {
				protocolDevice.EncryptionPasswordToken = encryptionToken
			} else if folderDevice.EncryptionPassword != "" {
				protocolDevice.EncryptionPasswordToken = protocol.PasswordToken(folderCfg.ID, folderDevice.EncryptionPassword)
				if folderDevice.DeviceID == device {
					passwords[folderCfg.ID] = folderDevice.EncryptionPassword
				}
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

	return message, passwords, nil
}

func (m *model) State(folder string) (string, time.Time, error) {
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

func (m *model) FolderErrors(folder string) ([]FileError, error) {
	m.fmut.RLock()
	err := m.checkFolderRunningLocked(folder)
	runner := m.folderRunners[folder]
	m.fmut.RUnlock()
	if err != nil {
		return nil, err
	}
	return runner.Errors(), nil
}

func (m *model) WatchError(folder string) error {
	m.fmut.RLock()
	err := m.checkFolderRunningLocked(folder)
	runner := m.folderRunners[folder]
	m.fmut.RUnlock()
	if err != nil {
		return nil // If the folder isn't running, there's no error to report.
	}
	return runner.WatchError()
}

func (m *model) Override(folder string) {
	// Grab the runner and the file set.

	m.fmut.RLock()
	runner, ok := m.folderRunners[folder]
	m.fmut.RUnlock()
	if !ok {
		return
	}

	// Run the override, taking updates as if they came from scanning.

	runner.Override()
}

func (m *model) Revert(folder string) {
	// Grab the runner and the file set.

	m.fmut.RLock()
	runner, ok := m.folderRunners[folder]
	m.fmut.RUnlock()
	if !ok {
		return
	}

	// Run the revert, taking updates as if they came from scanning.

	runner.Revert()
}

type TreeEntry struct {
	Name     string                `json:"name"`
	ModTime  time.Time             `json:"modTime"`
	Size     int64                 `json:"size"`
	Type     protocol.FileInfoType `json:"type"`
	Children []*TreeEntry          `json:"children,omitempty"`
}

func findByName(slice []*TreeEntry, name string) *TreeEntry {
	for _, child := range slice {
		if child.Name == name {
			return child
		}
	}
	return nil
}

func (m *model) GlobalDirectoryTree(folder, prefix string, levels int, dirsOnly bool) ([]*TreeEntry, error) {
	m.fmut.RLock()
	files, ok := m.folderFiles[folder]
	m.fmut.RUnlock()
	if !ok {
		return nil, ErrFolderMissing
	}

	root := &TreeEntry{
		Children: make([]*TreeEntry, 0),
	}
	sep := string(filepath.Separator)
	prefix = osutil.NativeFilename(prefix)

	if prefix != "" && !strings.HasSuffix(prefix, sep) {
		prefix = prefix + sep
	}

	snap, err := files.Snapshot()
	if err != nil {
		return nil, err
	}
	defer snap.Release()
	snap.WithPrefixedGlobalTruncated(prefix, func(fi protocol.FileIntf) bool {
		f := fi.(db.FileInfoTruncated)

		// Don't include the prefix itself.
		if f.IsInvalid() || f.IsDeleted() || strings.HasPrefix(prefix, f.Name) {
			return true
		}

		f.Name = strings.Replace(f.Name, prefix, "", 1)

		dir := filepath.Dir(f.Name)
		base := filepath.Base(f.Name)

		if levels > -1 && strings.Count(f.Name, sep) > levels {
			return true
		}

		parent := root
		if dir != "." {
			for _, path := range strings.Split(dir, sep) {
				child := findByName(parent.Children, path)
				if child == nil {
					err = fmt.Errorf("could not find child '%s' for path '%s' in parent '%s'", path, f.Name, parent.Name)
					return false
				}
				parent = child
			}
		}

		if dirsOnly && !f.IsDirectory() {
			return true
		}

		parent.Children = append(parent.Children, &TreeEntry{
			Name:    base,
			Type:    f.Type,
			ModTime: f.ModTime(),
			Size:    f.FileSize(),
		})

		return true
	})
	if err != nil {
		return nil, err
	}

	return root.Children, nil
}

func (m *model) GetFolderVersions(folder string) (map[string][]versioner.FileVersion, error) {
	m.fmut.RLock()
	err := m.checkFolderRunningLocked(folder)
	ver := m.folderVersioners[folder]
	m.fmut.RUnlock()
	if err != nil {
		return nil, err
	}
	if ver == nil {
		return nil, errNoVersioner
	}

	return ver.GetVersions()
}

func (m *model) RestoreFolderVersions(folder string, versions map[string]time.Time) (map[string]error, error) {
	m.fmut.RLock()
	err := m.checkFolderRunningLocked(folder)
	fcfg := m.folderCfgs[folder]
	ver := m.folderVersioners[folder]
	m.fmut.RUnlock()
	if err != nil {
		return nil, err
	}
	if ver == nil {
		return nil, errNoVersioner
	}

	restoreErrors := make(map[string]error)

	for file, version := range versions {
		if err := ver.Restore(file, version); err != nil {
			restoreErrors[file] = err
		}
	}

	// Trigger scan
	if !fcfg.FSWatcherEnabled {
		go func() { _ = m.ScanFolder(folder) }()
	}

	return restoreErrors, nil
}

func (m *model) Availability(folder string, file protocol.FileInfo, block protocol.BlockInfo) ([]Availability, error) {
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
		return nil, ErrFolderMissing
	}

	snap, err := fs.Snapshot()
	if err != nil {
		return nil, err
	}
	defer snap.Release()

	return m.availabilityInSnapshotPRlocked(cfg, snap, file, block), nil
}

func (m *model) availabilityInSnapshot(cfg config.FolderConfiguration, snap *db.Snapshot, file protocol.FileInfo, block protocol.BlockInfo) []Availability {
	m.pmut.RLock()
	defer m.pmut.RUnlock()
	return m.availabilityInSnapshotPRlocked(cfg, snap, file, block)
}

func (m *model) availabilityInSnapshotPRlocked(cfg config.FolderConfiguration, snap *db.Snapshot, file protocol.FileInfo, block protocol.BlockInfo) []Availability {
	var availabilities []Availability
	for _, device := range snap.Availability(file.Name) {
		if _, ok := m.remotePausedFolders[device]; !ok {
			continue
		}
		if _, ok := m.remotePausedFolders[device][cfg.ID]; ok {
			continue
		}
		_, ok := m.conn[device]
		if ok {
			availabilities = append(availabilities, Availability{ID: device, FromTemporary: false})
		}
	}

	for _, device := range cfg.Devices {
		if m.deviceDownloads[device.DeviceID].Has(cfg.ID, file.Name, file.Version, int(block.Offset/int64(file.BlockSize()))) {
			availabilities = append(availabilities, Availability{ID: device.DeviceID, FromTemporary: true})
		}
	}

	return availabilities
}

// BringToFront bumps the given files priority in the job queue.
func (m *model) BringToFront(folder, file string) {
	m.fmut.RLock()
	runner, ok := m.folderRunners[folder]
	m.fmut.RUnlock()

	if ok {
		runner.BringToFront(file)
	}
}

func (m *model) ResetFolder(folder string) {
	l.Infof("Cleaning data for folder %q", folder)
	db.DropFolder(m.db, folder)
}

func (m *model) String() string {
	return fmt.Sprintf("model@%p", m)
}

func (m *model) VerifyConfiguration(from, to config.Configuration) error {
	return nil
}

func (m *model) CommitConfiguration(from, to config.Configuration) bool {
	// TODO: This should not use reflect, and should take more care to try to handle stuff without restart.

	// Delay processing config changes until after the initial setup
	<-m.started

	// Go through the folder configs and figure out if we need to restart or not.

	// Tracks devices affected by any configuration change to resend ClusterConfig.
	clusterConfigDevices := make(deviceIDSet, len(from.Devices)+len(to.Devices))

	fromFolders := mapFolders(from.Folders)
	toFolders := mapFolders(to.Folders)
	for folderID, cfg := range toFolders {
		if _, ok := fromFolders[folderID]; !ok {
			// A folder was added.
			if cfg.Paused {
				l.Infoln("Paused folder", cfg.Description())
			} else {
				l.Infoln("Adding folder", cfg.Description())
				if err := m.newFolder(cfg, to.Options.CacheIgnoredFiles); err != nil {
					m.fatal(err)
					return true
				}
			}
			clusterConfigDevices.add(cfg.DeviceIDs())
		}
	}

	removedFolders := make(map[string]struct{})
	for folderID, fromCfg := range fromFolders {
		toCfg, ok := toFolders[folderID]
		if !ok {
			// The folder was removed.
			m.removeFolder(fromCfg)
			clusterConfigDevices.add(fromCfg.DeviceIDs())
			removedFolders[fromCfg.ID] = struct{}{}
			continue
		}

		if fromCfg.Paused && toCfg.Paused {
			continue
		}

		// This folder exists on both sides. Settings might have changed.
		// Check if anything differs that requires a restart.
		if !reflect.DeepEqual(fromCfg.RequiresRestartOnly(), toCfg.RequiresRestartOnly()) || from.Options.CacheIgnoredFiles != to.Options.CacheIgnoredFiles {
			if err := m.restartFolder(fromCfg, toCfg, to.Options.CacheIgnoredFiles); err != nil {
				m.fatal(err)
				return true
			}
			clusterConfigDevices.add(fromCfg.DeviceIDs())
			clusterConfigDevices.add(toCfg.DeviceIDs())
		}

		// Emit the folder pause/resume event
		if fromCfg.Paused != toCfg.Paused {
			eventType := events.FolderResumed
			if toCfg.Paused {
				eventType = events.FolderPaused
			}
			m.evLogger.Log(eventType, map[string]string{"id": toCfg.ID, "label": toCfg.Label})
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
	closeDevices := make([]protocol.DeviceID, 0, len(to.Devices))
	for deviceID, toCfg := range toDevices {
		fromCfg, ok := fromDevices[deviceID]
		if !ok {
			sr := stats.NewDeviceStatisticsReference(m.db, deviceID)
			m.fmut.Lock()
			m.deviceStatRefs[deviceID] = sr
			m.fmut.Unlock()
			continue
		}
		delete(fromDevices, deviceID)
		if fromCfg.Paused == toCfg.Paused {
			continue
		}

		// Ignored folder was removed, reconnect to retrigger the prompt.
		if !toCfg.Paused && len(fromCfg.IgnoredFolders) > len(toCfg.IgnoredFolders) {
			closeDevices = append(closeDevices, deviceID)
		}

		if toCfg.Paused {
			l.Infoln("Pausing", deviceID)
			closeDevices = append(closeDevices, deviceID)
			delete(clusterConfigDevices, deviceID)
			m.evLogger.Log(events.DevicePaused, map[string]string{"device": deviceID.String()})
		} else {
			m.evLogger.Log(events.DeviceResumed, map[string]string{"device": deviceID.String()})
		}
	}
	// Clean up after removed devices
	removedDevices := make([]protocol.DeviceID, 0, len(fromDevices))
	m.fmut.Lock()
	for deviceID := range fromDevices {
		delete(m.deviceStatRefs, deviceID)
		removedDevices = append(removedDevices, deviceID)
		delete(clusterConfigDevices, deviceID)
	}
	m.fmut.Unlock()

	m.pmut.RLock()
	for _, id := range closeDevices {
		if conn, ok := m.conn[id]; ok {
			go conn.Close(errDevicePaused)
		}
	}
	for _, id := range removedDevices {
		if conn, ok := m.conn[id]; ok {
			go conn.Close(errDeviceRemoved)
		}
	}
	m.pmut.RUnlock()
	// Generating cluster-configs acquires fmut -> must happen outside of pmut.
	m.sendClusterConfig(clusterConfigDevices.AsSlice())

	ignoredDevices := observedDeviceSet(to.IgnoredDevices)
	m.cleanPending(toDevices, toFolders, ignoredDevices, removedFolders)

	m.globalRequestLimiter.setCapacity(1024 * to.Options.MaxConcurrentIncomingRequestKiB())
	m.folderIOLimiter.setCapacity(to.Options.MaxFolderConcurrency())

	// Some options don't require restart as those components handle it fine
	// by themselves. Compare the options structs containing only the
	// attributes that require restart and act apprioriately.
	if !reflect.DeepEqual(from.Options.RequiresRestartOnly(), to.Options.RequiresRestartOnly()) {
		l.Debugln(m, "requires restart, options differ")
		return false
	}

	return true
}

func (m *model) cleanPending(existingDevices map[protocol.DeviceID]config.DeviceConfiguration, existingFolders map[string]config.FolderConfiguration, ignoredDevices deviceIDSet, removedFolders map[string]struct{}) {
	var removedPendingFolders []map[string]string
	pendingFolders, err := m.db.PendingFolders()
	if err != nil {
		l.Infof("Could not iterate through pending folder entries for cleanup: %v", err)
		// Continue with pending devices below, loop is skipped.
	}
	for folderID, pf := range pendingFolders {
		if _, ok := removedFolders[folderID]; ok {
			// Forget pending folder device associations for recently removed
			// folders as well, assuming the folder is no longer of interest
			// at all (but might become pending again).
			l.Debugf("Discarding pending removed folder %v from all devices", folderID)
			m.db.RemovePendingFolder(folderID)
			removedPendingFolders = append(removedPendingFolders, map[string]string{
				"folderID": folderID,
			})
			continue
		}
		for deviceID := range pf.OfferedBy {
			if dev, ok := existingDevices[deviceID]; !ok {
				l.Debugf("Discarding pending folder %v from unknown device %v", folderID, deviceID)
				goto removeFolderForDevice
			} else if dev.IgnoredFolder(folderID) {
				l.Debugf("Discarding now ignored pending folder %v for device %v", folderID, deviceID)
				goto removeFolderForDevice
			}
			if folderCfg, ok := existingFolders[folderID]; ok {
				if folderCfg.SharedWith(deviceID) {
					l.Debugf("Discarding now shared pending folder %v for device %v", folderID, deviceID)
					goto removeFolderForDevice
				}
			}
			continue
		removeFolderForDevice:
			m.db.RemovePendingFolderForDevice(folderID, deviceID)
			removedPendingFolders = append(removedPendingFolders, map[string]string{
				"folderID": folderID,
				"deviceID": deviceID.String(),
			})
		}
	}
	if len(removedPendingFolders) > 0 {
		m.evLogger.Log(events.PendingFoldersChanged, map[string]interface{}{
			"removed": removedPendingFolders,
		})
	}

	var removedPendingDevices []map[string]string
	pendingDevices, err := m.db.PendingDevices()
	if err != nil {
		l.Infof("Could not iterate through pending device entries for cleanup: %v", err)
		return
	}
	for deviceID := range pendingDevices {
		if _, ok := ignoredDevices[deviceID]; ok {
			l.Debugf("Discarding now ignored pending device %v", deviceID)
			goto removeDevice
		}
		if _, ok := existingDevices[deviceID]; ok {
			l.Debugf("Discarding now added pending device %v", deviceID)
			goto removeDevice
		}
		continue
	removeDevice:
		m.db.RemovePendingDevice(deviceID)
		removedPendingDevices = append(removedPendingDevices, map[string]string{
			"deviceID": deviceID.String(),
		})
	}
	if len(removedPendingDevices) > 0 {
		m.evLogger.Log(events.PendingDevicesChanged, map[string]interface{}{
			"removed": removedPendingDevices,
		})
	}
}

// checkFolderRunningLocked returns nil if the folder is up and running and a
// descriptive error if not.
// Need to hold (read) lock on m.fmut when calling this.
func (m *model) checkFolderRunningLocked(folder string) error {
	_, ok := m.folderRunners[folder]
	if ok {
		return nil
	}

	if cfg, ok := m.cfg.Folder(folder); !ok {
		return ErrFolderMissing
	} else if cfg.Paused {
		return ErrFolderPaused
	}

	return ErrFolderNotRunning
}

// PendingDevices lists unknown devices that tried to connect.
func (m *model) PendingDevices() (map[protocol.DeviceID]db.ObservedDevice, error) {
	return m.db.PendingDevices()
}

// PendingFolders lists folders that we don't yet share with the offering devices.  It
// returns the entries grouped by folder and filters for a given device unless the
// argument is specified as EmptyDeviceID.
func (m *model) PendingFolders(device protocol.DeviceID) (map[string]db.PendingFolder, error) {
	return m.db.PendingFoldersForDevice(device)
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

func observedDeviceSet(devices []config.ObservedDevice) deviceIDSet {
	res := make(deviceIDSet, len(devices))
	for _, dev := range devices {
		res[dev.ID] = struct{}{}
	}
	return res
}

func readOffsetIntoBuf(fs fs.Filesystem, file string, offset int64, buf []byte) (int, error) {
	fd, err := fs.Open(file)
	if err != nil {
		l.Debugln("readOffsetIntoBuf.Open", file, err)
		return 0, err
	}

	defer fd.Close()
	n, err := fd.ReadAt(buf, offset)
	if err != nil {
		l.Debugln("readOffsetIntoBuf.ReadAt", file, err)
	}
	return n, err
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
			UpdateType: protocol.FileDownloadProgressUpdateTypeForget,
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

func (b *fileInfoBatch) full() bool {
	return len(b.infos) >= maxBatchSizeFiles || b.size >= maxBatchSizeBytes
}

func (b *fileInfoBatch) flushIfFull() error {
	if b.full() {
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

type deviceIDSet map[protocol.DeviceID]struct{}

func (s deviceIDSet) add(ids []protocol.DeviceID) {
	for _, id := range ids {
		if _, ok := s[id]; !ok {
			s[id] = struct{}{}
		}
	}
}

func (s deviceIDSet) AsSlice() []protocol.DeviceID {
	ids := make([]protocol.DeviceID, 0, len(s))
	for id := range s {
		ids = append(ids, id)
	}
	return ids
}

func encryptionTokenPath(cfg config.FolderConfiguration) string {
	return filepath.Join(cfg.MarkerName, config.EncryptionTokenName)
}

type storedEncryptionToken struct {
	FolderID string
	Token    []byte
}

func readEncryptionToken(cfg config.FolderConfiguration) ([]byte, error) {
	fd, err := cfg.Filesystem().Open(encryptionTokenPath(cfg))
	if err != nil {
		return nil, err
	}
	defer fd.Close()
	var stored storedEncryptionToken
	if err := json.NewDecoder(fd).Decode(&stored); err != nil {
		return nil, err
	}
	return stored.Token, nil
}

func writeEncryptionToken(token []byte, cfg config.FolderConfiguration) error {
	tokenName := encryptionTokenPath(cfg)
	fd, err := cfg.Filesystem().OpenFile(tokenName, fs.OptReadWrite|fs.OptCreate, 0666)
	if err != nil {
		return err
	}
	defer fd.Close()
	return json.NewEncoder(fd).Encode(storedEncryptionToken{
		FolderID: cfg.ID,
		Token:    token,
	})
}

func newFolderConfiguration(w config.Wrapper, id, label string, fsType fs.FilesystemType, path string) config.FolderConfiguration {
	fcfg := w.DefaultFolder()
	fcfg.ID = id
	fcfg.Label = label
	fcfg.FilesystemType = fsType
	fcfg.Path = path
	return fcfg
}

type updatedPendingFolder struct {
	FolderID         string            `json:"folderID"`
	FolderLabel      string            `json:"folderLabel"`
	DeviceID         protocol.DeviceID `json:"deviceID"`
	ReceiveEncrypted bool              `json:"receiveEncrypted"`
}
