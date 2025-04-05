// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:generate -command counterfeiter go run github.com/maxbrunsfeld/counterfeiter/v6
//go:generate counterfeiter -o mocks/model.go --fake-name Model . Model

package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	stdsync "sync"
	"sync/atomic"
	"time"

	"github.com/thejerf/suture/v4"

	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/connections"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/scanner"
	"github.com/syncthing/syncthing/lib/semaphore"
	"github.com/syncthing/syncthing/lib/stats"
	"github.com/syncthing/syncthing/lib/svcutil"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/ur/contract"
	"github.com/syncthing/syncthing/lib/versioner"
)

type service interface {
	suture.Service
	BringToFront(string)
	Override()
	Revert()
	DelayScan(d time.Duration)
	ScheduleScan()
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

	ResetFolder(folder string) error
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
	NeedFolderFiles(folder string, page, perpage int) ([]protocol.FileInfo, []protocol.FileInfo, []protocol.FileInfo, error)
	RemoteNeedFolderFiles(folder string, device protocol.DeviceID, page, perpage int) ([]protocol.FileInfo, error)
	LocalChangedFolderFiles(folder string, page, perpage int) ([]protocol.FileInfo, error)
	FolderProgressBytesCompleted(folder string) int64

	CurrentFolderFile(folder string, file string) (protocol.FileInfo, bool, error)
	CurrentGlobalFile(folder string, file string) (protocol.FileInfo, bool, error)
	GetMtimeMapping(folder string, file string) (fs.MtimeMapping, error)
	Availability(folder string, file protocol.FileInfo, block protocol.BlockInfo) ([]Availability, error)

	Completion(device protocol.DeviceID, folder string) (FolderCompletion, error)
	ConnectionStats() map[string]interface{}
	DeviceStatistics() (map[protocol.DeviceID]stats.DeviceStatistics, error)
	FolderStatistics() (map[string]stats.FolderStatistics, error)
	UsageReportingStats(report *contract.Report, version int, preview bool)
	ConnectedTo(remoteID protocol.DeviceID) bool

	PendingDevices() (map[protocol.DeviceID]db.ObservedDevice, error)
	PendingFolders(device protocol.DeviceID) (map[string]db.PendingFolder, error)
	DismissPendingDevice(device protocol.DeviceID) error
	DismissPendingFolder(device protocol.DeviceID, folder string) error

	GlobalDirectoryTree(folder, prefix string, levels int, dirsOnly bool) ([]*TreeEntry, error)

	RequestGlobal(ctx context.Context, deviceID protocol.DeviceID, folder, name string, blockNo int, offset int64, size int, hash []byte, weakHash uint32, fromTemporary bool) ([]byte, error)
}

type model struct {
	*suture.Supervisor

	// constructor parameters
	cfg            config.Wrapper
	id             protocol.DeviceID
	db             *db.Lowlevel
	protectedFiles []string
	evLogger       events.Logger

	// constant or concurrency safe fields
	finder          *db.BlockFinder
	progressEmitter *ProgressEmitter
	shortID         protocol.ShortID
	// globalRequestLimiter limits the amount of data in concurrent incoming
	// requests
	globalRequestLimiter *semaphore.Semaphore
	// folderIOLimiter limits the number of concurrent I/O heavy operations,
	// such as scans and pulls.
	folderIOLimiter *semaphore.Semaphore
	fatalChan       chan error
	started         chan struct{}
	keyGen          *protocol.KeyGenerator
	promotionTimer  *time.Timer

	// fields protected by mut
	mut                            sync.RWMutex
	folderCfgs                     map[string]config.FolderConfiguration                  // folder -> cfg
	folderFiles                    map[string]*db.FileSet                                 // folder -> files
	deviceStatRefs                 map[protocol.DeviceID]*stats.DeviceStatisticsReference // deviceID -> statsRef
	folderIgnores                  map[string]*ignore.Matcher                             // folder -> matcher object
	folderRunners                  *serviceMap[string, service]                           // folder -> puller or scanner
	folderRestartMuts              syncMutexMap                                           // folder -> restart mutex
	folderVersioners               map[string]versioner.Versioner                         // folder -> versioner (may be nil)
	folderEncryptionPasswordTokens map[string][]byte                                      // folder -> encryption token (may be missing, and only for encryption type folders)
	folderEncryptionFailures       map[string]map[protocol.DeviceID]error                 // folder -> device -> error regarding encryption consistency (may be missing)
	connections                    map[string]protocol.Connection                         // connection ID -> connection
	deviceConnIDs                  map[protocol.DeviceID][]string                         // device -> connection IDs (invariant: if the key exists, the value is len >= 1, with the primary connection at the start of the slice)
	promotedConnID                 map[protocol.DeviceID]string                           // device -> latest promoted connection ID
	connRequestLimiters            map[protocol.DeviceID]*semaphore.Semaphore
	closed                         map[string]chan struct{} // connection ID -> closed channel
	helloMessages                  map[protocol.DeviceID]protocol.Hello
	deviceDownloads                map[protocol.DeviceID]*deviceDownloadState
	remoteFolderStates             map[protocol.DeviceID]map[string]remoteFolderState // deviceID -> folders
	indexHandlers                  *serviceMap[protocol.DeviceID, *indexHandlerRegistry]

	// for testing only
	foldersRunning atomic.Int32
}

var _ config.Verifier = &model{}

type folderFactory func(*model, *db.FileSet, *ignore.Matcher, config.FolderConfiguration, versioner.Versioner, events.Logger, *semaphore.Semaphore) service

var folderFactories = make(map[config.FolderType]folderFactory)

var (
	errDeviceUnknown    = errors.New("unknown device")
	errDevicePaused     = errors.New("device is paused")
	ErrFolderPaused     = errors.New("folder is paused")
	ErrFolderNotRunning = errors.New("folder is not running")
	ErrFolderMissing    = errors.New("no such folder")
	errNoVersioner      = errors.New("folder has no versioner")
	// errors about why a connection is closed
	errStopped                            = errors.New("Syncthing is being stopped")
	errEncryptionInvConfigLocal           = errors.New("can't encrypt outgoing data because local data is encrypted (folder-type receive-encrypted)")
	errEncryptionInvConfigRemote          = errors.New("remote has encrypted data and encrypts that data for us - this is impossible")
	errEncryptionNotEncryptedLocal        = errors.New("remote expects to exchange encrypted data, but is configured for plain data")
	errEncryptionPlainForReceiveEncrypted = errors.New("remote expects to exchange plain data, but is configured to be encrypted")
	errEncryptionPlainForRemoteEncrypted  = errors.New("remote expects to exchange plain data, but local data is encrypted (folder-type receive-encrypted)")
	errEncryptionNotEncryptedUntrusted    = errors.New("device is untrusted, but configured to receive plain data")
	errEncryptionPassword                 = errors.New("different encryption passwords used")
	errEncryptionTokenRead                = errors.New("failed to read encryption token")
	errEncryptionTokenWrite               = errors.New("failed to write encryption token")
	errMissingRemoteInClusterConfig       = errors.New("remote device missing in cluster config")
	errMissingLocalInClusterConfig        = errors.New("local device missing in cluster config")
)

// NewModel creates and starts a new model. The model starts in read-only mode,
// where it sends index information to connected peers and responds to requests
// for file data without altering the local folder in any way.
func NewModel(cfg config.Wrapper, id protocol.DeviceID, ldb *db.Lowlevel, protectedFiles []string, evLogger events.Logger, keyGen *protocol.KeyGenerator) Model {
	spec := svcutil.SpecWithDebugLogger(l)
	m := &model{
		Supervisor: suture.New("model", spec),

		// constructor parameters
		cfg:            cfg,
		id:             id,
		db:             ldb,
		protectedFiles: protectedFiles,
		evLogger:       evLogger,

		// constant or concurrency safe fields
		finder:               db.NewBlockFinder(ldb),
		progressEmitter:      NewProgressEmitter(cfg, evLogger),
		shortID:              id.Short(),
		globalRequestLimiter: semaphore.New(1024 * cfg.Options().MaxConcurrentIncomingRequestKiB()),
		folderIOLimiter:      semaphore.New(cfg.Options().MaxFolderConcurrency()),
		fatalChan:            make(chan error),
		started:              make(chan struct{}),
		keyGen:               keyGen,
		promotionTimer:       time.NewTimer(0),

		// fields protected by mut
		mut:                            sync.NewRWMutex(),
		folderCfgs:                     make(map[string]config.FolderConfiguration),
		folderFiles:                    make(map[string]*db.FileSet),
		deviceStatRefs:                 make(map[protocol.DeviceID]*stats.DeviceStatisticsReference),
		folderIgnores:                  make(map[string]*ignore.Matcher),
		folderRunners:                  newServiceMap[string, service](evLogger),
		folderVersioners:               make(map[string]versioner.Versioner),
		folderEncryptionPasswordTokens: make(map[string][]byte),
		folderEncryptionFailures:       make(map[string]map[protocol.DeviceID]error),
		connections:                    make(map[string]protocol.Connection),
		deviceConnIDs:                  make(map[protocol.DeviceID][]string),
		promotedConnID:                 make(map[protocol.DeviceID]string),
		connRequestLimiters:            make(map[protocol.DeviceID]*semaphore.Semaphore),
		closed:                         make(map[string]chan struct{}),
		helloMessages:                  make(map[protocol.DeviceID]protocol.Hello),
		deviceDownloads:                make(map[protocol.DeviceID]*deviceDownloadState),
		remoteFolderStates:             make(map[protocol.DeviceID]map[string]remoteFolderState),
		indexHandlers:                  newServiceMap[protocol.DeviceID, *indexHandlerRegistry](evLogger),
	}
	for devID, cfg := range cfg.Devices() {
		m.deviceStatRefs[devID] = stats.NewDeviceStatisticsReference(m.db, devID)
		m.setConnRequestLimitersLocked(cfg)
	}
	m.Add(m.folderRunners)
	m.Add(m.progressEmitter)
	m.Add(m.indexHandlers)
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

	for {
		select {
		case <-ctx.Done():
			l.Debugln(m, "context closed, stopping", ctx.Err())
			return ctx.Err()
		case err := <-m.fatalChan:
			l.Debugln(m, "fatal error, stopping", err)
			return svcutil.AsFatalErr(err, svcutil.ExitError)
		case <-m.promotionTimer.C:
			l.Debugln("promotion timer fired")
			m.promoteConnections()
		}
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
	m.mut.RLock()
	closed := make([]chan struct{}, 0, len(m.connections))
	for connID, conn := range m.connections {
		closed = append(closed, m.closed[connID])
		go conn.Close(errStopped)
	}
	m.mut.RUnlock()
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

// Need to hold lock on m.mut when calling this.
func (m *model) addAndStartFolderLocked(cfg config.FolderConfiguration, fset *db.FileSet, cacheIgnoredFiles bool) {
	ignores := ignore.New(cfg.Filesystem(nil), ignore.WithCache(cacheIgnoredFiles))
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

	_, ok := m.folderRunners.Get(cfg.ID)
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
			l.Warnln("Failed to create folder root directory:", err)
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
	ffs := cfg.Filesystem(nil)
	_ = ffs.Hide(config.DefaultMarkerName)
	_ = ffs.Hide(versioner.DefaultPath)
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

	m.warnAboutOverwritingProtectedFiles(cfg, ignores)

	p := folderFactory(m, fset, ignores, cfg, ver, m.evLogger, m.folderIOLimiter)
	m.folderRunners.Add(folder, p)

	l.Infof("Ready to synchronize %s (%s)", cfg.Description(), cfg.Type)
}

func (m *model) warnAboutOverwritingProtectedFiles(cfg config.FolderConfiguration, ignores *ignore.Matcher) {
	if cfg.Type == config.FolderTypeSendOnly {
		return
	}

	// This is a bit of a hack.
	ffs := cfg.Filesystem(nil)
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
	m.mut.RLock()
	wait := m.folderRunners.StopAndWaitChan(cfg.ID, 0)
	m.mut.RUnlock()
	<-wait

	m.mut.Lock()

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
		if err := cfg.RemoveMarker(); err != nil && !errors.Is(err, os.ErrNotExist) {
			moved := config.DefaultMarkerName + time.Now().Format(".removed-20060102-150405")
			fs := cfg.Filesystem(nil)
			_ = fs.Rename(config.DefaultMarkerName, moved)
		}
	}

	m.cleanupFolderLocked(cfg)
	m.indexHandlers.Each(func(_ protocol.DeviceID, r *indexHandlerRegistry) error {
		r.Remove(cfg.ID)
		return nil
	})

	m.mut.Unlock()

	// Remove it from the database
	db.DropFolder(m.db, cfg.ID)
}

// Need to hold lock on m.mut when calling this.
func (m *model) cleanupFolderLocked(cfg config.FolderConfiguration) {
	// clear up our config maps
	m.folderRunners.Remove(cfg.ID)
	delete(m.folderCfgs, cfg.ID)
	delete(m.folderFiles, cfg.ID)
	delete(m.folderIgnores, cfg.ID)
	delete(m.folderVersioners, cfg.ID)
	delete(m.folderEncryptionPasswordTokens, cfg.ID)
	delete(m.folderEncryptionFailures, cfg.ID)
}

func (m *model) restartFolder(from, to config.FolderConfiguration, cacheIgnoredFiles bool) error {
	if to.ID == "" {
		panic("bug: cannot restart empty folder ID")
	}
	if to.ID != from.ID {
		l.Warnf("bug: folder restart cannot change ID %q -> %q", from.ID, to.ID)
		panic("bug: folder restart cannot change ID")
	}
	folder := to.ID

	// This mutex protects the entirety of the restart operation, preventing
	// there from being more than one folder restart operation in progress
	// at any given time. The usual locking stuff doesn't cover this,
	// because those locks are released while we are waiting for the folder
	// to shut down (and must be so because the folder might need them as
	// part of its operations before shutting down).
	restartMut := m.folderRestartMuts.Get(folder)
	restartMut.Lock()
	defer restartMut.Unlock()

	m.mut.RLock()
	wait := m.folderRunners.StopAndWaitChan(from.ID, 0)
	m.mut.RUnlock()
	<-wait

	m.mut.Lock()
	defer m.mut.Unlock()

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
			fset, err = db.NewFileSet(folder, m.db)
			if err != nil {
				return fmt.Errorf("restarting %v: %w", to.Description(), err)
			}
		}
		m.addAndStartFolderLocked(to, fset, cacheIgnoredFiles)
	}

	runner, _ := m.folderRunners.Get(to.ID)
	m.indexHandlers.Each(func(_ protocol.DeviceID, r *indexHandlerRegistry) error {
		r.RegisterFolderState(to, fset, runner)
		return nil
	})

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
	m.mut.Lock()
	defer m.mut.Unlock()

	// Creating the fileset can take a long time (metadata calculation), but
	// nevertheless should happen inside the lock (same as when restarting
	// a folder).
	fset, err := db.NewFileSet(cfg.ID, m.db)
	if err != nil {
		return fmt.Errorf("adding %v: %w", cfg.Description(), err)
	}

	m.addAndStartFolderLocked(cfg, fset, cacheIgnoredFiles)

	// Cluster configs might be received and processed before reaching this
	// point, i.e. before the folder is started. If that's the case, start
	// index senders here.
	m.indexHandlers.Each(func(_ protocol.DeviceID, r *indexHandlerRegistry) error {
		runner, _ := m.folderRunners.Get(cfg.ID)
		r.RegisterFolderState(cfg, fset, runner)
		return nil
	})

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
		m.mut.RLock()
		for _, conn := range m.connections {
			report.TransportStats[conn.Transport()]++
		}
		m.mut.RUnlock()

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

type ConnectionStats struct {
	protocol.Statistics // Total for primary + secondaries

	Connected     bool   `json:"connected"`
	Paused        bool   `json:"paused"`
	ClientVersion string `json:"clientVersion"`

	Address string `json:"address"` // mirror values from Primary, for compatibility with <1.24.0
	Type    string `json:"type"`    // mirror values from Primary, for compatibility with <1.24.0
	IsLocal bool   `json:"isLocal"` // mirror values from Primary, for compatibility with <1.24.0
	Crypto  string `json:"crypto"`  // mirror values from Primary, for compatibility with <1.24.0

	Primary   ConnectionInfo   `json:"primary,omitempty"`
	Secondary []ConnectionInfo `json:"secondary,omitempty"`
}

type ConnectionInfo struct {
	protocol.Statistics
	Address string `json:"address"`
	Type    string `json:"type"`
	IsLocal bool   `json:"isLocal"`
	Crypto  string `json:"crypto"`
}

// ConnectionStats returns a map with connection statistics for each device.
func (m *model) ConnectionStats() map[string]interface{} {
	m.mut.RLock()
	defer m.mut.RUnlock()

	res := make(map[string]interface{})
	devs := m.cfg.Devices()
	conns := make(map[string]ConnectionStats, len(devs))
	for device, deviceCfg := range devs {
		if device == m.id {
			continue
		}
		hello := m.helloMessages[device]
		versionString := hello.ClientVersion
		if hello.ClientName != "syncthing" {
			versionString = hello.ClientName + " " + hello.ClientVersion
		}
		connIDs, ok := m.deviceConnIDs[device]
		cs := ConnectionStats{
			Connected:     ok,
			Paused:        deviceCfg.Paused,
			ClientVersion: strings.TrimSpace(versionString),
		}
		if ok {
			conn := m.connections[connIDs[0]]

			cs.Primary.Type = conn.Type()
			cs.Primary.IsLocal = conn.IsLocal()
			cs.Primary.Crypto = conn.Crypto()
			cs.Primary.Statistics = conn.Statistics()
			cs.Primary.Address = conn.RemoteAddr().String()

			cs.Type = cs.Primary.Type
			cs.IsLocal = cs.Primary.IsLocal
			cs.Crypto = cs.Primary.Crypto
			cs.Address = cs.Primary.Address
			cs.Statistics = cs.Primary.Statistics

			for _, connID := range connIDs[1:] {
				conn = m.connections[connID]
				sec := ConnectionInfo{
					Statistics: conn.Statistics(),
					Address:    conn.RemoteAddr().String(),
					Type:       conn.Type(),
					IsLocal:    conn.IsLocal(),
					Crypto:     conn.Crypto(),
				}
				if sec.At.After(cs.At) {
					cs.At = sec.At
				}
				if sec.StartedAt.Before(cs.StartedAt) {
					cs.StartedAt = sec.StartedAt
				}
				cs.InBytesTotal += sec.InBytesTotal
				cs.OutBytesTotal += sec.OutBytesTotal
				cs.Secondary = append(cs.Secondary, sec)
			}
		}

		conns[device.String()] = cs
	}

	res["connections"] = conns

	in, out := protocol.TotalInOut()
	res["total"] = map[string]interface{}{
		"at":            time.Now().Truncate(time.Second),
		"inBytesTotal":  in,
		"outBytesTotal": out,
	}

	return res
}

// DeviceStatistics returns statistics about each device
func (m *model) DeviceStatistics() (map[protocol.DeviceID]stats.DeviceStatistics, error) {
	m.mut.RLock()
	defer m.mut.RUnlock()
	res := make(map[protocol.DeviceID]stats.DeviceStatistics, len(m.deviceStatRefs))
	for id, sr := range m.deviceStatRefs {
		stats, err := sr.GetStatistics()
		if err != nil {
			return nil, err
		}
		if len(m.deviceConnIDs[id]) > 0 {
			// If a device is currently connected, we can see them right
			// now.
			stats.LastSeen = time.Now().Truncate(time.Second)
		}
		res[id] = stats
	}
	return res, nil
}

// FolderStatistics returns statistics about each folder
func (m *model) FolderStatistics() (map[string]stats.FolderStatistics, error) {
	res := make(map[string]stats.FolderStatistics)
	m.mut.RLock()
	defer m.mut.RUnlock()
	err := m.folderRunners.Each(func(id string, runner service) error {
		stats, err := runner.GetStatistics()
		if err != nil {
			return err
		}
		res[id] = stats
		return nil
	})
	if err != nil {
		return nil, err
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
	RemoteState   remoteFolderState
}

func newFolderCompletion(global, need db.Counts, sequence int64, state remoteFolderState) FolderCompletion {
	comp := FolderCompletion{
		GlobalBytes: global.Bytes,
		NeedBytes:   need.Bytes,
		GlobalItems: global.Files + global.Directories + global.Symlinks,
		NeedItems:   need.Files + need.Directories + need.Symlinks,
		NeedDeletes: need.Deleted,
		Sequence:    sequence,
		RemoteState: state,
	}
	comp.setCompletionPct()
	return comp
}

func (comp *FolderCompletion) add(other FolderCompletion) {
	comp.GlobalBytes += other.GlobalBytes
	comp.NeedBytes += other.NeedBytes
	comp.GlobalItems += other.GlobalItems
	comp.NeedItems += other.NeedItems
	comp.NeedDeletes += other.NeedDeletes
	comp.setCompletionPct()
}

func (comp *FolderCompletion) setCompletionPct() {
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

// Map returns the members as a map, e.g. used in api to serialize as JSON.
func (comp *FolderCompletion) Map() map[string]interface{} {
	return map[string]interface{}{
		"completion":  comp.CompletionPct,
		"globalBytes": comp.GlobalBytes,
		"needBytes":   comp.NeedBytes,
		"globalItems": comp.GlobalItems,
		"needItems":   comp.NeedItems,
		"needDeletes": comp.NeedDeletes,
		"sequence":    comp.Sequence,
		"remoteState": comp.RemoteState,
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
		if fcfg.Paused {
			continue
		}
		if device == protocol.LocalDeviceID || fcfg.SharedWith(device) {
			folderComp, err := m.folderCompletion(device, fcfg.ID)
			if errors.Is(err, ErrFolderPaused) {
				continue
			} else if err != nil {
				return FolderCompletion{}, err
			}
			comp.add(folderComp)
		}
	}
	return comp, nil
}

func (m *model) folderCompletion(device protocol.DeviceID, folder string) (FolderCompletion, error) {
	m.mut.RLock()
	err := m.checkFolderRunningRLocked(folder)
	rf := m.folderFiles[folder]
	m.mut.RUnlock()
	if err != nil {
		return FolderCompletion{}, err
	}

	snap, err := rf.Snapshot()
	if err != nil {
		return FolderCompletion{}, err
	}
	defer snap.Release()

	m.mut.RLock()
	state := m.remoteFolderStates[device][folder]
	downloaded := m.deviceDownloads[device].BytesDownloaded(folder)
	m.mut.RUnlock()

	need := snap.NeedSize(device)
	need.Bytes -= downloaded
	// This might be more than it really is, because some blocks can be of a smaller size.
	if need.Bytes < 0 {
		need.Bytes = 0
	}

	comp := newFolderCompletion(snap.GlobalSize(), need, snap.Sequence(device), state)

	l.Debugf("%v Completion(%s, %q): %v", m, device, folder, comp.Map())
	return comp, nil
}

// DBSnapshot returns a snapshot of the database content relevant to the given folder.
func (m *model) DBSnapshot(folder string) (*db.Snapshot, error) {
	m.mut.RLock()
	err := m.checkFolderRunningRLocked(folder)
	rf := m.folderFiles[folder]
	m.mut.RUnlock()
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
func (m *model) NeedFolderFiles(folder string, page, perpage int) ([]protocol.FileInfo, []protocol.FileInfo, []protocol.FileInfo, error) {
	m.mut.RLock()
	rf, rfOk := m.folderFiles[folder]
	runner, runnerOk := m.folderRunners.Get(folder)
	cfg := m.folderCfgs[folder]
	m.mut.RUnlock()

	if !rfOk {
		return nil, nil, nil, ErrFolderMissing
	}

	snap, err := rf.Snapshot()
	if err != nil {
		return nil, nil, nil, err
	}
	defer snap.Release()
	var progress, queued, rest []protocol.FileInfo
	var seen map[string]struct{}

	p := newPager(page, perpage)

	if runnerOk {
		progressNames, queuedNames, skipped := runner.Jobs(page, perpage)

		progress = make([]protocol.FileInfo, len(progressNames))
		queued = make([]protocol.FileInfo, len(queuedNames))
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

	rest = make([]protocol.FileInfo, 0, perpage)
	snap.WithNeedTruncated(protocol.LocalDeviceID, func(f protocol.FileInfo) bool {
		if cfg.IgnoreDelete && f.IsDeleted() {
			return true
		}

		if p.skip() {
			return true
		}
		if _, ok := seen[f.Name]; !ok {
			rest = append(rest, f)
			p.get--
		}
		return p.get > 0
	})

	return progress, queued, rest, nil
}

// RemoteNeedFolderFiles returns paginated list of currently needed files for a
// remote device to become synced with a folder.
func (m *model) RemoteNeedFolderFiles(folder string, device protocol.DeviceID, page, perpage int) ([]protocol.FileInfo, error) {
	m.mut.RLock()
	rf, ok := m.folderFiles[folder]
	m.mut.RUnlock()

	if !ok {
		return nil, ErrFolderMissing
	}

	snap, err := rf.Snapshot()
	if err != nil {
		return nil, err
	}
	defer snap.Release()

	files := make([]protocol.FileInfo, 0, perpage)
	p := newPager(page, perpage)
	snap.WithNeedTruncated(device, func(f protocol.FileInfo) bool {
		if p.skip() {
			return true
		}
		files = append(files, f)
		return !p.done()
	})
	return files, nil
}

func (m *model) LocalChangedFolderFiles(folder string, page, perpage int) ([]protocol.FileInfo, error) {
	m.mut.RLock()
	rf, ok := m.folderFiles[folder]
	m.mut.RUnlock()

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
	files := make([]protocol.FileInfo, 0, perpage)

	snap.WithHaveTruncated(protocol.LocalDeviceID, func(f protocol.FileInfo) bool {
		if !f.IsReceiveOnlyChanged() {
			return true
		}
		if p.skip() {
			return true
		}
		files = append(files, f)
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
func (m *model) Index(conn protocol.Connection, idx *protocol.Index) error {
	return m.handleIndex(conn, idx.Folder, idx.Files, false, 0, idx.LastSequence)
}

// IndexUpdate is called for incremental updates to connected devices' indexes.
// Implements the protocol.Model interface.
func (m *model) IndexUpdate(conn protocol.Connection, idxUp *protocol.IndexUpdate) error {
	return m.handleIndex(conn, idxUp.Folder, idxUp.Files, true, idxUp.PrevSequence, idxUp.LastSequence)
}

func (m *model) handleIndex(conn protocol.Connection, folder string, fs []protocol.FileInfo, update bool, prevSequence, lastSequence int64) error {
	op := "Index"
	if update {
		op += " update"
	}

	deviceID := conn.DeviceID()
	l.Debugf("%v (in): %s / %q: %d files", op, deviceID, folder, len(fs))

	if cfg, ok := m.cfg.Folder(folder); !ok || !cfg.SharedWith(deviceID) {
		l.Warnf("%v for unexpected folder ID %q sent from device %q; ensure that the folder exists and that this device is selected under \"Share With\" in the folder configuration.", op, folder, deviceID)
		return fmt.Errorf("%s: %w", folder, ErrFolderMissing)
	} else if cfg.Paused {
		l.Debugf("%v for paused folder (ID %q) sent from device %q.", op, folder, deviceID)
		return fmt.Errorf("%s: %w", folder, ErrFolderPaused)
	}

	m.mut.RLock()
	indexHandler, ok := m.getIndexHandlerRLocked(conn)
	m.mut.RUnlock()
	if !ok {
		// This should be impossible, as an index handler is registered when
		// we send a cluster config, and that is what triggers index
		// sending.
		m.evLogger.Log(events.Failure, "index sender does not exist for connection on which indexes were received")
		l.Debugf("%v for folder (ID %q) sent from device %q: missing index handler", op, folder, deviceID)
		return fmt.Errorf("%s: %w", folder, ErrFolderNotRunning)
	}

	return indexHandler.ReceiveIndex(folder, fs, update, op, prevSequence, lastSequence)
}

type clusterConfigDeviceInfo struct {
	local, remote protocol.Device
}

type ClusterConfigReceivedEventData struct {
	Device protocol.DeviceID `json:"device"`
}

func (m *model) ClusterConfig(conn protocol.Connection, cm *protocol.ClusterConfig) error {
	deviceID := conn.DeviceID()

	if cm.Secondary {
		// No handling of secondary connection ClusterConfigs; they merely
		// indicate the connection is ready to start.
		l.Debugf("Skipping secondary ClusterConfig from %v at %s", deviceID.Short(), conn)
		return nil
	}

	// Check the peer device's announced folders against our own. Emits events
	// for folders that we don't expect (unknown or not shared).
	// Also, collect a list of folders we do share, and if he's interested in
	// temporary indexes, subscribe the connection.

	l.Debugf("Handling ClusterConfig from %v at %s", deviceID.Short(), conn)
	indexHandlerRegistry := m.ensureIndexHandler(conn)

	deviceCfg, ok := m.cfg.Device(deviceID)
	if !ok {
		l.Debugf("Device %s disappeared from config while processing cluster-config", deviceID.Short())
		return errDeviceUnknown
	}

	// Assemble the device information from the connected device about
	// themselves and us for all folders.
	ccDeviceInfos := make(map[string]*clusterConfigDeviceInfo, len(cm.Folders))
	for _, folder := range cm.Folders {
		info := &clusterConfigDeviceInfo{}
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
			l.Infof("Device %v sent cluster-config without the device info for the remote on folder %v", deviceID.Short(), folder.Description())
			return errMissingRemoteInClusterConfig
		}
		if info.local.ID == protocol.EmptyDeviceID {
			l.Infof("Device %v sent cluster-config without the device info for us locally on folder %v", deviceID.Short(), folder.Description())
			return errMissingLocalInClusterConfig
		}
		ccDeviceInfos[folder.ID] = info
	}

	for _, info := range ccDeviceInfos {
		if deviceCfg.Introducer && info.local.Introducer {
			l.Warnf("Remote %v is an introducer to us, and we are to them - only one should be introducer to the other, see https://docs.syncthing.net/users/introducer.html", deviceCfg.Description())
		}
		break
	}

	// Needs to happen outside of the mut, as can cause CommitConfiguration
	if deviceCfg.AutoAcceptFolders {
		w, _ := m.cfg.Modify(func(cfg *config.Configuration) {
			changedFcfg := make(map[string]config.FolderConfiguration)
			haveFcfg := cfg.FolderMap()
			for _, folder := range cm.Folders {
				from, ok := haveFcfg[folder.ID]
				if to, changed := m.handleAutoAccepts(deviceID, folder, ccDeviceInfos[folder.ID], from, ok, cfg.Defaults.Folder); changed {
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

	tempIndexFolders, states, err := m.ccHandleFolders(cm.Folders, deviceCfg, ccDeviceInfos, indexHandlerRegistry)
	if err != nil {
		return err
	}

	m.mut.Lock()
	m.remoteFolderStates[deviceID] = states
	m.mut.Unlock()

	m.evLogger.Log(events.ClusterConfigReceived, ClusterConfigReceivedEventData{
		Device: deviceID,
	})

	if len(tempIndexFolders) > 0 {
		var connOK bool
		var conn protocol.Connection
		m.mut.RLock()
		if connIDs, connIDOK := m.deviceConnIDs[deviceID]; connIDOK {
			conn, connOK = m.connections[connIDs[0]]
		}
		m.mut.RUnlock()
		// In case we've got ClusterConfig, and the connection disappeared
		// from infront of our nose.
		if connOK {
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
			cfg.Devices = make([]config.DeviceConfiguration, 0, len(devices))
			for _, dcfg := range devices {
				cfg.Devices = append(cfg.Devices, dcfg)
			}
		})
	}

	return nil
}

func (m *model) ensureIndexHandler(conn protocol.Connection) *indexHandlerRegistry {
	deviceID := conn.DeviceID()
	connID := conn.ConnectionID()

	m.mut.Lock()
	defer m.mut.Unlock()

	indexHandlerRegistry, ok := m.indexHandlers.Get(deviceID)
	if ok && indexHandlerRegistry.conn.ConnectionID() == connID {
		// This is an existing and proper index handler for this connection.
		return indexHandlerRegistry
	}

	if ok {
		// A handler exists, but it's for another connection than the one we
		// now got a ClusterConfig on. This should be unusual as it means
		// the other side has decided to start using a new primary
		// connection but we haven't seen it close yet. Ideally it will
		// close shortly by itself...
		l.Infof("Abandoning old index handler for %s (%s) in favour of %s", deviceID.Short(), indexHandlerRegistry.conn.ConnectionID(), connID)
		m.indexHandlers.RemoveAndWait(deviceID, 0)
	}

	// Create a new index handler for this device.
	indexHandlerRegistry = newIndexHandlerRegistry(conn, m.deviceDownloads[deviceID], m.evLogger)
	for id, fcfg := range m.folderCfgs {
		l.Debugln("Registering folder", id, "for", deviceID.Short())
		runner, _ := m.folderRunners.Get(id)
		indexHandlerRegistry.RegisterFolderState(fcfg, m.folderFiles[id], runner)
	}
	m.indexHandlers.Add(deviceID, indexHandlerRegistry)

	return indexHandlerRegistry
}

func (m *model) getIndexHandlerRLocked(conn protocol.Connection) (*indexHandlerRegistry, bool) {
	// Reads from index handlers, which requires the mutex to be read locked

	deviceID := conn.DeviceID()
	connID := conn.ConnectionID()

	indexHandlerRegistry, ok := m.indexHandlers.Get(deviceID)
	if ok && indexHandlerRegistry.conn.ConnectionID() == connID {
		// This is an existing and proper index handler for this connection.
		return indexHandlerRegistry, true
	}

	// There is no index handler, or it's not registered for this connection.
	return nil, false
}

func (m *model) ccHandleFolders(folders []protocol.Folder, deviceCfg config.DeviceConfiguration, ccDeviceInfos map[string]*clusterConfigDeviceInfo, indexHandlers *indexHandlerRegistry) ([]string, map[string]remoteFolderState, error) {
	var folderDevice config.FolderDeviceConfiguration
	tempIndexFolders := make([]string, 0, len(folders))
	seenFolders := make(map[string]remoteFolderState, len(folders))
	updatedPending := make([]updatedPendingFolder, 0, len(folders))
	deviceID := deviceCfg.DeviceID
	expiredPending, err := m.db.PendingFoldersForDevice(deviceID)
	if err != nil {
		l.Infof("Could not get pending folders for cleanup: %v", err)
	}
	of := db.ObservedFolder{Time: time.Now().Truncate(time.Second)}
	for _, folder := range folders {
		seenFolders[folder.ID] = remoteFolderValid

		cfg, ok := m.cfg.Folder(folder.ID)
		if ok {
			folderDevice, ok = cfg.Device(deviceID)
		}
		if !ok {
			indexHandlers.Remove(folder.ID)
			if deviceCfg.IgnoredFolder(folder.ID) {
				l.Infof("Ignoring folder %s from device %s since it is in the list of ignored folders", folder.Description(), deviceID)
				continue
			}
			delete(expiredPending, folder.ID)
			of.Label = folder.Label
			of.ReceiveEncrypted = len(ccDeviceInfos[folder.ID].local.EncryptionPasswordToken) > 0
			of.RemoteEncrypted = len(ccDeviceInfos[folder.ID].remote.EncryptionPasswordToken) > 0
			if err := m.db.AddOrUpdatePendingFolder(folder.ID, of, deviceID); err != nil {
				l.Warnf("Failed to persist pending folder entry to database: %v", err)
			}
			if !folder.Paused {
				indexHandlers.AddIndexInfo(folder.ID, ccDeviceInfos[folder.ID])
			}
			updatedPending = append(updatedPending, updatedPendingFolder{
				FolderID:         folder.ID,
				FolderLabel:      folder.Label,
				DeviceID:         deviceID,
				ReceiveEncrypted: of.ReceiveEncrypted,
				RemoteEncrypted:  of.RemoteEncrypted,
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
			indexHandlers.Remove(folder.ID)
			seenFolders[cfg.ID] = remoteFolderPaused
			continue
		}

		if cfg.Paused {
			indexHandlers.AddIndexInfo(folder.ID, ccDeviceInfos[folder.ID])
			continue
		}

		if err := m.ccCheckEncryption(cfg, folderDevice, ccDeviceInfos[folder.ID], deviceCfg.Untrusted); err != nil {
			sameError := false
			m.mut.Lock()
			if devs, ok := m.folderEncryptionFailures[folder.ID]; ok {
				sameError = devs[deviceID] == err
			} else {
				m.folderEncryptionFailures[folder.ID] = make(map[protocol.DeviceID]error)
			}
			m.folderEncryptionFailures[folder.ID][deviceID] = err
			m.mut.Unlock()
			msg := fmt.Sprintf("Failure checking encryption consistency with device %v for folder %v: %v", deviceID, cfg.Description(), err)
			if sameError {
				l.Debugln(msg)
			} else {
				if rerr, ok := err.(*redactedError); ok {
					err = rerr.redacted
				}
				m.evLogger.Log(events.Failure, err.Error())
				l.Warnln(msg)
			}
			return tempIndexFolders, seenFolders, err
		}
		m.mut.Lock()
		if devErrs, ok := m.folderEncryptionFailures[folder.ID]; ok {
			if len(devErrs) == 1 {
				delete(m.folderEncryptionFailures, folder.ID)
			} else {
				delete(m.folderEncryptionFailures[folder.ID], deviceID)
			}
		}
		m.mut.Unlock()

		// Handle indexes

		if !folder.DisableTempIndexes {
			tempIndexFolders = append(tempIndexFolders, folder.ID)
		}

		indexHandlers.AddIndexInfo(folder.ID, ccDeviceInfos[folder.ID])
	}

	indexHandlers.RemoveAllExcept(seenFolders)

	// Explicitly mark folders we offer, but the remote has not accepted
	for folderID, cfg := range m.cfg.Folders() {
		if _, seen := seenFolders[folderID]; !seen && cfg.SharedWith(deviceID) {
			l.Debugf("Remote device %v has not accepted sharing folder %s", deviceID.Short(), cfg.Description())
			seenFolders[folderID] = remoteFolderNotSharing
		}
	}

	expiredPendingList := make([]map[string]string, 0, len(expiredPending))
	for folder := range expiredPending {
		if err = m.db.RemovePendingFolderForDevice(folder, deviceID); err != nil {
			msg := "Failed to remove pending folder-device entry"
			l.Warnf("%v (%v, %v): %v", msg, folder, deviceID, err)
			m.evLogger.Log(events.Failure, msg)
			continue
		}
		expiredPendingList = append(expiredPendingList, map[string]string{
			"folderID": folder,
			"deviceID": deviceID.String(),
		})
	}
	if len(updatedPending) > 0 || len(expiredPendingList) > 0 {
		m.evLogger.Log(events.PendingFoldersChanged, map[string]interface{}{
			"added":   updatedPending,
			"removed": expiredPendingList,
		})
	}

	return tempIndexFolders, seenFolders, nil
}

func (m *model) ccCheckEncryption(fcfg config.FolderConfiguration, folderDevice config.FolderDeviceConfiguration, ccDeviceInfos *clusterConfigDeviceInfo, deviceUntrusted bool) error {
	hasTokenRemote := len(ccDeviceInfos.remote.EncryptionPasswordToken) > 0
	hasTokenLocal := len(ccDeviceInfos.local.EncryptionPasswordToken) > 0
	isEncryptedRemote := folderDevice.EncryptionPassword != ""
	isEncryptedLocal := fcfg.Type == config.FolderTypeReceiveEncrypted

	if !isEncryptedRemote && !isEncryptedLocal && deviceUntrusted {
		return errEncryptionNotEncryptedUntrusted
	}

	if !(hasTokenRemote || hasTokenLocal || isEncryptedRemote || isEncryptedLocal) {
		// No one cares about encryption here
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
		if isEncryptedRemote {
			return errEncryptionPlainForRemoteEncrypted
		} else {
			return errEncryptionPlainForReceiveEncrypted
		}
	}

	if !(isEncryptedRemote || isEncryptedLocal) {
		return errEncryptionNotEncryptedLocal
	}

	if isEncryptedRemote {
		passwordToken := protocol.PasswordToken(m.keyGen, fcfg.ID, folderDevice.EncryptionPassword)
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
	m.mut.RLock()
	token, ok := m.folderEncryptionPasswordTokens[fcfg.ID]
	m.mut.RUnlock()
	if !ok {
		var err error
		token, err = readEncryptionToken(fcfg)
		if err != nil && !fs.IsNotExist(err) {
			if rerr, ok := redactPathError(err); ok {
				return rerr
			}
			return &redactedError{
				error:    err,
				redacted: errEncryptionTokenRead,
			}
		}
		if err == nil {
			m.mut.Lock()
			m.folderEncryptionPasswordTokens[fcfg.ID] = token
			m.mut.Unlock()
		} else {
			if err := writeEncryptionToken(ccToken, fcfg); err != nil {
				if rerr, ok := redactPathError(err); ok {
					return rerr
				} else {
					return &redactedError{
						error:    err,
						redacted: errEncryptionTokenWrite,
					}
				}
			}
			m.mut.Lock()
			m.folderEncryptionPasswordTokens[fcfg.ID] = ccToken
			m.mut.Unlock()
			// We can only announce ourselves once we have the token,
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
	m.mut.RLock()
	for _, id := range ids {
		if connIDs, ok := m.deviceConnIDs[id]; ok {
			ccConns = append(ccConns, m.connections[connIDs[0]])
		}
	}
	m.mut.RUnlock()
	// Generating cluster-configs acquires the mutex.
	for _, conn := range ccConns {
		cm, passwords := m.generateClusterConfig(conn.DeviceID())
		conn.SetFolderPasswords(passwords)
		go conn.ClusterConfig(cm)
	}
}

// handleIntroductions handles adding devices/folders that are shared by an introducer device
func (m *model) handleIntroductions(introducerCfg config.DeviceConfiguration, cm *protocol.ClusterConfig, folders map[string]config.FolderConfiguration, devices map[protocol.DeviceID]config.DeviceConfiguration) (map[string]config.FolderConfiguration, map[protocol.DeviceID]config.DeviceConfiguration, folderDeviceSet, bool) {
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

			if fcfg.Type != config.FolderTypeReceiveEncrypted && device.EncryptionPasswordToken != nil {
				l.Infof("Cannot share folder %s with %v because the introducer %v encrypts data, which requires a password", folder.Description(), device.ID, introducerCfg.DeviceID)
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
func (*model) handleDeintroductions(introducerCfg config.DeviceConfiguration, foldersDevices folderDeviceSet, folders map[string]config.FolderConfiguration, devices map[protocol.DeviceID]config.DeviceConfiguration) (map[string]config.FolderConfiguration, map[protocol.DeviceID]config.DeviceConfiguration, bool) {
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
func (m *model) handleAutoAccepts(deviceID protocol.DeviceID, folder protocol.Folder, ccDeviceInfos *clusterConfigDeviceInfo, cfg config.FolderConfiguration, haveCfg bool, defaultFolderCfg config.FolderConfiguration) (config.FolderConfiguration, bool) {
	if !haveCfg {
		defaultPathFs := fs.NewFilesystem(defaultFolderCfg.FilesystemType.ToFS(), defaultFolderCfg.Path)
		var pathAlternatives []string
		if alt := fs.SanitizePath(folder.Label); alt != "" {
			pathAlternatives = append(pathAlternatives, alt)
		}
		if alt := fs.SanitizePath(folder.ID); alt != "" {
			pathAlternatives = append(pathAlternatives, alt)
		}
		if len(pathAlternatives) == 0 {
			l.Infof("Failed to auto-accept folder %s from %s due to lack of path alternatives", folder.Description(), deviceID)
			return config.FolderConfiguration{}, false
		}
		for _, path := range pathAlternatives {
			// Make sure the folder path doesn't already exist.
			if _, err := defaultPathFs.Lstat(path); !fs.IsNotExist(err) {
				continue
			}

			// Attempt to create it to make sure it does, now.
			fullPath := filepath.Join(defaultFolderCfg.Path, path)
			if err := defaultPathFs.MkdirAll(path, 0o700); err != nil {
				l.Warnf("Failed to create path for auto-accepted folder %s at path %s: %v", folder.Description(), fullPath, err)
				continue
			}

			fcfg := newFolderConfiguration(m.cfg, folder.ID, folder.Label, defaultFolderCfg.FilesystemType, fullPath)
			fcfg.Devices = append(fcfg.Devices, config.FolderDeviceConfiguration{
				DeviceID: deviceID,
			})

			if len(ccDeviceInfos.remote.EncryptionPasswordToken) > 0 || len(ccDeviceInfos.local.EncryptionPasswordToken) > 0 {
				fcfg.Type = config.FolderTypeReceiveEncrypted
				// Override the user-configured defaults, as normally done by the GUI
				fcfg.FSWatcherEnabled = false
				if fcfg.RescanIntervalS != 0 {
					minRescanInterval := 3600 * 24
					if fcfg.RescanIntervalS < minRescanInterval {
						fcfg.RescanIntervalS = minRescanInterval
					}
				}
				fcfg.Versioning.Reset()
				// Other necessary settings are ensured by FolderConfiguration itself
			} else {
				ignores := m.cfg.DefaultIgnores()
				if err := m.setIgnores(fcfg, ignores.Lines); err != nil {
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
func (m *model) Closed(conn protocol.Connection, err error) {
	connID := conn.ConnectionID()
	deviceID := conn.DeviceID()

	m.mut.Lock()
	conn, ok := m.connections[connID]
	if !ok {
		m.mut.Unlock()
		return
	}

	closed := m.closed[connID]
	delete(m.closed, connID)
	delete(m.connections, connID)

	removedIsPrimary := m.promotedConnID[deviceID] == connID
	remainingConns := without(m.deviceConnIDs[deviceID], connID)
	var wait <-chan error
	if removedIsPrimary {
		m.progressEmitter.temporaryIndexUnsubscribe(conn)
		if idxh, ok := m.indexHandlers.Get(deviceID); ok && idxh.conn.ConnectionID() == connID {
			wait = m.indexHandlers.RemoveAndWaitChan(deviceID, 0)
		}
		m.scheduleConnectionPromotion()
	}
	if len(remainingConns) == 0 {
		// All device connections closed
		delete(m.deviceConnIDs, deviceID)
		delete(m.promotedConnID, deviceID)
		delete(m.connRequestLimiters, deviceID)
		delete(m.helloMessages, deviceID)
		delete(m.remoteFolderStates, deviceID)
		delete(m.deviceDownloads, deviceID)
	} else {
		// Some connections remain
		m.deviceConnIDs[deviceID] = remainingConns
	}

	m.mut.Unlock()
	if wait != nil {
		<-wait
	}

	m.mut.RLock()
	m.deviceDidCloseRLocked(deviceID, time.Since(conn.EstablishedAt()))
	m.mut.RUnlock()

	k := map[bool]string{false: "secondary", true: "primary"}[removedIsPrimary]
	l.Infof("Lost %s connection to %s at %s: %v (%d remain)", k, deviceID.Short(), conn, err, len(remainingConns))

	if len(remainingConns) == 0 {
		l.Infof("Connection to %s at %s closed: %v", deviceID.Short(), conn, err)
		m.evLogger.Log(events.DeviceDisconnected, map[string]string{
			"id":    deviceID.String(),
			"error": err.Error(),
		})
	}
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
func (m *model) Request(conn protocol.Connection, req *protocol.Request) (out protocol.RequestResponse, err error) {
	if req.Size < 0 || req.Offset < 0 {
		return nil, protocol.ErrInvalid
	}

	deviceID := conn.DeviceID()

	m.mut.RLock()
	folderCfg, ok := m.folderCfgs[req.Folder]
	folderIgnores := m.folderIgnores[req.Folder]
	m.mut.RUnlock()
	if !ok {
		// The folder might be already unpaused in the config, but not yet
		// in the model.
		l.Debugf("Request from %s for file %s in unstarted folder %q", deviceID.Short(), req.Name, req.Folder)
		return nil, protocol.ErrGeneric
	}

	if !folderCfg.SharedWith(deviceID) {
		l.Warnf("Request from %s for file %s in unshared folder %q", deviceID.Short(), req.Name, req.Folder)
		return nil, protocol.ErrGeneric
	}
	if folderCfg.Paused {
		l.Debugf("Request from %s for file %s in paused folder %q", deviceID.Short(), req.Name, req.Folder)
		return nil, protocol.ErrGeneric
	}

	// Make sure the path is valid and in canonical form
	if name, err := fs.Canonicalize(req.Name); err != nil {
		l.Debugf("Request from %s in folder %q for invalid filename %s", deviceID.Short(), req.Folder, req.Name)
		return nil, protocol.ErrGeneric
	} else {
		req.Name = name
	}

	if deviceID != protocol.LocalDeviceID {
		l.Debugf("%v REQ(in): %s: %q / %q o=%d s=%d t=%v", m, deviceID.Short(), req.Folder, req.Name, req.Offset, req.Size, req.FromTemporary)
	}

	if fs.IsInternal(req.Name) {
		l.Debugf("%v REQ(in) for internal file: %s: %q / %q o=%d s=%d", m, deviceID.Short(), req.Folder, req.Name, req.Offset, req.Size)
		return nil, protocol.ErrInvalid
	}

	if folderIgnores.Match(req.Name).IsIgnored() {
		l.Debugf("%v REQ(in) for ignored file: %s: %q / %q o=%d s=%d", m, deviceID.Short(), req.Folder, req.Name, req.Offset, req.Size)
		return nil, protocol.ErrInvalid
	}

	// Restrict parallel requests by connection/device

	m.mut.RLock()
	limiter := m.connRequestLimiters[deviceID]
	m.mut.RUnlock()

	// The requestResponse releases the bytes to the buffer pool and the
	// limiters when its Close method is called.
	res := newLimitedRequestResponse(int(req.Size), limiter, m.globalRequestLimiter)

	defer func() {
		// Close it ourselves if it isn't returned due to an error
		if err != nil {
			res.Close()
		}
	}()

	// Grab the FS after limiting, as it causes I/O and we want to minimize
	// the race time between the symlink check and the read.

	folderFs := folderCfg.Filesystem(nil)

	if err := osutil.TraversesSymlink(folderFs, filepath.Dir(req.Name)); err != nil {
		l.Debugf("%v REQ(in) traversal check: %s - %s: %q / %q o=%d s=%d", m, err, deviceID.Short(), req.Folder, req.Name, req.Offset, req.Size)
		return nil, protocol.ErrNoSuchFile
	}

	// Only check temp files if the flag is set, and if we are set to advertise
	// the temp indexes.
	if req.FromTemporary && !folderCfg.DisableTempIndexes {
		tempFn := fs.TempName(req.Name)

		if info, err := folderFs.Lstat(tempFn); err != nil || !info.IsRegular() {
			// Reject reads for anything that doesn't exist or is something
			// other than a regular file.
			l.Debugf("%v REQ(in) failed stating temp file (%v): %s: %q / %q o=%d s=%d", m, err, deviceID.Short(), req.Folder, req.Name, req.Offset, req.Size)
			return nil, protocol.ErrNoSuchFile
		}
		_, err := readOffsetIntoBuf(folderFs, tempFn, req.Offset, res.data)
		if err == nil && scanner.Validate(res.data, req.Hash, req.WeakHash) {
			return res, nil
		}
		// Fall through to reading from a non-temp file, just in case the temp
		// file has finished downloading.
	}

	if info, err := folderFs.Lstat(req.Name); err != nil || !info.IsRegular() {
		// Reject reads for anything that doesn't exist or is something
		// other than a regular file.
		l.Debugf("%v REQ(in) failed stating file (%v): %s: %q / %q o=%d s=%d", m, err, deviceID.Short(), req.Folder, req.Name, req.Offset, req.Size)
		return nil, protocol.ErrNoSuchFile
	}

	n, err := readOffsetIntoBuf(folderFs, req.Name, req.Offset, res.data)
	if fs.IsNotExist(err) {
		l.Debugf("%v REQ(in) file doesn't exist: %s: %q / %q o=%d s=%d", m, deviceID.Short(), req.Folder, req.Name, req.Offset, req.Size)
		return nil, protocol.ErrNoSuchFile
	} else if err == io.EOF {
		// Read beyond end of file. This might indicate a problem, or it
		// might be a short block that gets padded when read for encrypted
		// folders. We ignore the error and let the hash validation in the
		// next step take care of it, by only hashing the part we actually
		// managed to read.
	} else if err != nil {
		l.Debugf("%v REQ(in) failed reading file (%v): %s: %q / %q o=%d s=%d", m, err, deviceID.Short(), req.Folder, req.Name, req.Offset, req.Size)
		return nil, protocol.ErrGeneric
	}

	if folderCfg.Type != config.FolderTypeReceiveEncrypted && len(req.Hash) > 0 && !scanner.Validate(res.data[:n], req.Hash, req.WeakHash) {
		m.recheckFile(deviceID, req.Folder, req.Name, req.Offset, req.Hash, req.WeakHash)
		l.Debugf("%v REQ(in) failed validating data: %s: %q / %q o=%d s=%d", m, deviceID.Short(), req.Folder, req.Name, req.Offset, req.Size)
		return nil, protocol.ErrNoSuchFile
	}

	return res, nil
}

// newLimitedRequestResponse takes size bytes from the limiters in order,
// skipping nil limiters, then returns a requestResponse of the given size.
// When the requestResponse is closed the limiters are given back the bytes,
// in reverse order.
func newLimitedRequestResponse(size int, limiters ...*semaphore.Semaphore) *requestResponse {
	multi := semaphore.MultiSemaphore(limiters)
	multi.Take(size)

	res := newRequestResponse(size)

	go func() {
		res.Wait()
		multi.Give(size)
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
	m.mut.RLock()
	runner, ok := m.folderRunners.Get(folder)
	m.mut.RUnlock()
	if !ok {
		l.Debugf("%v recheckFile: %s: %q / %q: Folder stopped before rescan could be scheduled", m, deviceID, folder, name)
		return
	}

	runner.ScheduleForceRescan(name)

	l.Debugf("%v recheckFile: %s: %q / %q", m, deviceID, folder, name)
}

func (m *model) CurrentFolderFile(folder string, file string) (protocol.FileInfo, bool, error) {
	m.mut.RLock()
	fs, ok := m.folderFiles[folder]
	m.mut.RUnlock()
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
	m.mut.RLock()
	ffs, ok := m.folderFiles[folder]
	m.mut.RUnlock()
	if !ok {
		return protocol.FileInfo{}, false, ErrFolderMissing
	}
	snap, err := ffs.Snapshot()
	if err != nil {
		return protocol.FileInfo{}, false, err
	}
	f, ok := snap.GetGlobal(file)
	snap.Release()
	return f, ok, nil
}

func (m *model) GetMtimeMapping(folder string, file string) (fs.MtimeMapping, error) {
	m.mut.RLock()
	ffs, ok := m.folderFiles[folder]
	fcfg := m.folderCfgs[folder]
	m.mut.RUnlock()
	if !ok {
		return fs.MtimeMapping{}, ErrFolderMissing
	}
	return fs.GetMtimeMapping(fcfg.Filesystem(ffs), file)
}

// Connection returns if we are connected to the given device.
func (m *model) ConnectedTo(deviceID protocol.DeviceID) bool {
	m.mut.RLock()
	_, ok := m.deviceConnIDs[deviceID]
	m.mut.RUnlock()
	return ok
}

// LoadIgnores loads or refreshes the ignore patterns from disk, if the
// folder is healthy, and returns the refreshed lines and patterns.
func (m *model) LoadIgnores(folder string) ([]string, []string, error) {
	m.mut.RLock()
	cfg, cfgOk := m.folderCfgs[folder]
	ignores, ignoresOk := m.folderIgnores[folder]
	m.mut.RUnlock()

	if !cfgOk {
		cfg, cfgOk = m.cfg.Folder(folder)
		if !cfgOk {
			return nil, nil, fmt.Errorf("folder %s does not exist", folder)
		}
	}

	if cfg.Type == config.FolderTypeReceiveEncrypted {
		return nil, nil, nil
	}

	if !ignoresOk {
		ignores = ignore.New(cfg.Filesystem(nil))
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
	m.mut.RLock()
	_, cfgOk := m.folderCfgs[folder]
	ignores, ignoresOk := m.folderIgnores[folder]
	m.mut.RUnlock()

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
	return m.setIgnores(cfg, content)
}

func (m *model) setIgnores(cfg config.FolderConfiguration, content []string) error {
	err := cfg.CheckPath()
	if err == config.ErrPathMissing {
		if err = cfg.CreateRoot(); err != nil {
			return fmt.Errorf("failed to create folder root: %w", err)
		}
		err = cfg.CheckPath()
	}
	if err != nil && err != config.ErrMarkerMissing {
		return err
	}

	if err := ignore.WriteIgnores(cfg.Filesystem(nil), ".stignore", content); err != nil {
		l.Warnln("Saving .stignore:", err)
		return err
	}

	m.mut.RLock()
	runner, ok := m.folderRunners.Get(cfg.ID)
	m.mut.RUnlock()
	if ok {
		runner.ScheduleScan()
	}
	return nil
}

// OnHello is called when an device connects to us.
// This allows us to extract some information from the Hello message
// and add it to a list of known devices ahead of any checks.
func (m *model) OnHello(remoteID protocol.DeviceID, addr net.Addr, hello protocol.Hello) error {
	if _, ok := m.cfg.Device(remoteID); !ok {
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
	return nil
}

// AddConnection adds a new peer connection to the model. An initial index will
// be sent to the connected peer, thereafter index updates whenever the local
// folder changes.
func (m *model) AddConnection(conn protocol.Connection, hello protocol.Hello) {
	deviceID := conn.DeviceID()
	deviceCfg, ok := m.cfg.Device(deviceID)
	if !ok {
		l.Infoln("Trying to add connection to unknown device")
		return
	}

	connID := conn.ConnectionID()
	closed := make(chan struct{})

	m.mut.Lock()

	m.connections[connID] = conn
	m.closed[connID] = closed
	m.helloMessages[deviceID] = hello
	m.deviceConnIDs[deviceID] = append(m.deviceConnIDs[deviceID], connID)
	if m.deviceDownloads[deviceID] == nil {
		m.deviceDownloads[deviceID] = newDeviceDownloadState()
	}

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

	if len(m.deviceConnIDs[deviceID]) == 1 {
		l.Infof(`Device %s client is "%s %s" named "%s" at %s`, deviceID.Short(), hello.ClientName, hello.ClientVersion, hello.DeviceName, conn)
	} else {
		l.Infof(`Additional connection (+%d) for device %s at %s`, len(m.deviceConnIDs[deviceID])-1, deviceID.Short(), conn)
	}

	m.mut.Unlock()

	if (deviceCfg.Name == "" || m.cfg.Options().OverwriteRemoteDevNames) && hello.DeviceName != "" {
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
	m.scheduleConnectionPromotion()
}

func (m *model) scheduleConnectionPromotion() {
	// Keeps deferring to prevent multiple executions in quick succession,
	// e.g. if multiple connections to a single device are closed.
	m.promotionTimer.Reset(time.Second)
}

// promoteConnections checks for devices that have connections, but where
// the primary connection hasn't started index handlers etc. yet, and
// promotes the primary connection to be the index handling one. This should
// be called after adding new connections, and after closing a primary
// device connection.
func (m *model) promoteConnections() {
	m.mut.Lock()
	defer m.mut.Unlock()

	for deviceID, connIDs := range m.deviceConnIDs {
		cm, passwords := m.generateClusterConfigRLocked(deviceID)
		if m.promotedConnID[deviceID] != connIDs[0] {
			// The previously promoted connection is not the current
			// primary; we should promote the primary connection to be the
			// index handling one. We do this by sending a ClusterConfig on
			// it, which will cause the other side to start sending us index
			// messages there. (On our side, we manage index handlers based
			// on where we get ClusterConfigs from the peer.)
			conn := m.connections[connIDs[0]]
			l.Debugf("Promoting connection to %s at %s", deviceID.Short(), conn)
			if conn.Statistics().StartedAt.IsZero() {
				conn.SetFolderPasswords(passwords)
				conn.Start()
			}
			conn.ClusterConfig(cm)
			m.promotedConnID[deviceID] = connIDs[0]
		}

		// Make sure any other new connections also get started, and that
		// they get a secondary-marked ClusterConfig.
		for _, connID := range connIDs[1:] {
			conn := m.connections[connID]
			if conn.Statistics().StartedAt.IsZero() {
				conn.SetFolderPasswords(passwords)
				conn.Start()
				conn.ClusterConfig(&protocol.ClusterConfig{Secondary: true})
			}
		}
	}
}

func (m *model) DownloadProgress(conn protocol.Connection, p *protocol.DownloadProgress) error {
	deviceID := conn.DeviceID()

	m.mut.RLock()
	cfg, ok := m.folderCfgs[p.Folder]
	m.mut.RUnlock()

	if !ok || cfg.DisableTempIndexes || !cfg.SharedWith(deviceID) {
		return nil
	}

	m.mut.RLock()
	downloads := m.deviceDownloads[deviceID]
	m.mut.RUnlock()
	downloads.Update(p.Folder, p.Updates)
	state := downloads.GetBlockCounts(p.Folder)

	m.evLogger.Log(events.RemoteDownloadProgress, map[string]interface{}{
		"device": deviceID.String(),
		"folder": p.Folder,
		"state":  state,
	})

	return nil
}

func (m *model) deviceWasSeen(deviceID protocol.DeviceID) {
	m.mut.RLock()
	sr, ok := m.deviceStatRefs[deviceID]
	m.mut.RUnlock()
	if ok {
		_ = sr.WasSeen()
	}
}

func (m *model) deviceDidCloseRLocked(deviceID protocol.DeviceID, duration time.Duration) {
	if sr, ok := m.deviceStatRefs[deviceID]; ok {
		_ = sr.LastConnectionDuration(duration)
		_ = sr.WasSeen()
	}
}

func (m *model) RequestGlobal(ctx context.Context, deviceID protocol.DeviceID, folder, name string, blockNo int, offset int64, size int, hash []byte, weakHash uint32, fromTemporary bool) ([]byte, error) {
	conn, connOK := m.requestConnectionForDevice(deviceID)
	if !connOK {
		return nil, fmt.Errorf("requestGlobal: no connection to device: %s", deviceID.Short())
	}

	l.Debugf("%v REQ(out): %s (%s): %q / %q b=%d o=%d s=%d h=%x wh=%x ft=%t", m, deviceID.Short(), conn, folder, name, blockNo, offset, size, hash, weakHash, fromTemporary)
	return conn.Request(ctx, &protocol.Request{Folder: folder, Name: name, BlockNo: blockNo, Offset: offset, Size: size, Hash: hash, WeakHash: weakHash, FromTemporary: fromTemporary})
}

// requestConnectionForDevice returns a connection to the given device, to
// be used for sending a request. If there is only one device connection,
// this is the one to use. If there are multiple then we avoid the first
// ("primary") connection, which is dedicated to index data, and pick a
// random one of the others.
func (m *model) requestConnectionForDevice(deviceID protocol.DeviceID) (protocol.Connection, bool) {
	m.mut.RLock()
	defer m.mut.RUnlock()

	connIDs, ok := m.deviceConnIDs[deviceID]
	if !ok {
		return nil, false
	}

	// If there is an entry in deviceConns, it always contains at least one
	// connection.
	connID := connIDs[0]
	if len(connIDs) > 1 {
		// Pick a random connection of the non-primary ones
		idx := rand.Intn(len(connIDs)-1) + 1
		connID = connIDs[idx]
	}

	conn, connOK := m.connections[connID]
	return conn, connOK
}

func (m *model) ScanFolders() map[string]error {
	m.mut.RLock()
	folders := make([]string, 0, len(m.folderCfgs))
	for folder := range m.folderCfgs {
		folders = append(folders, folder)
	}
	m.mut.RUnlock()

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
	m.mut.RLock()
	err := m.checkFolderRunningRLocked(folder)
	runner, _ := m.folderRunners.Get(folder)
	m.mut.RUnlock()

	if err != nil {
		return err
	}

	return runner.Scan(subs)
}

func (m *model) DelayScan(folder string, next time.Duration) {
	m.mut.RLock()
	runner, ok := m.folderRunners.Get(folder)
	m.mut.RUnlock()
	if !ok {
		return
	}
	runner.DelayScan(next)
}

// numHashers returns the number of hasher routines to use for a given folder,
// taking into account configuration and available CPU cores.
func (m *model) numHashers(folder string) int {
	m.mut.RLock()
	folderCfg := m.folderCfgs[folder]
	numFolders := max(1, len(m.folderCfgs))
	m.mut.RUnlock()

	if folderCfg.Hashers > 0 {
		// Specific value set in the config, use that.
		return folderCfg.Hashers
	}

	if build.IsWindows || build.IsDarwin || build.IsIOS || build.IsAndroid {
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
func (m *model) generateClusterConfig(device protocol.DeviceID) (*protocol.ClusterConfig, map[string]string) {
	m.mut.RLock()
	defer m.mut.RUnlock()
	return m.generateClusterConfigRLocked(device)
}

func (m *model) generateClusterConfigRLocked(device protocol.DeviceID) (*protocol.ClusterConfig, map[string]string) {
	message := &protocol.ClusterConfig{}
	folders := m.cfg.FolderList()
	passwords := make(map[string]string, len(folders))
	for _, folderCfg := range folders {
		if !folderCfg.SharedWith(device) {
			continue
		}

		encryptionToken, hasEncryptionToken := m.folderEncryptionPasswordTokens[folderCfg.ID]
		if folderCfg.Type == config.FolderTypeReceiveEncrypted && !hasEncryptionToken {
			// We haven't gotten a token for us yet and without one the other
			// side can't validate us - pretend we don't have the folder yet.
			continue
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
				Compression: deviceCfg.Compression.ToProtocol(),
				CertName:    deviceCfg.CertName,
				Introducer:  deviceCfg.Introducer,
			}

			if deviceCfg.DeviceID == m.id && hasEncryptionToken {
				protocolDevice.EncryptionPasswordToken = encryptionToken
			} else if folderDevice.EncryptionPassword != "" {
				protocolDevice.EncryptionPasswordToken = protocol.PasswordToken(m.keyGen, folderCfg.ID, folderDevice.EncryptionPassword)
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

	return message, passwords
}

func (m *model) State(folder string) (string, time.Time, error) {
	m.mut.RLock()
	runner, ok := m.folderRunners.Get(folder)
	m.mut.RUnlock()
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
	m.mut.RLock()
	err := m.checkFolderRunningRLocked(folder)
	runner, _ := m.folderRunners.Get(folder)
	m.mut.RUnlock()
	if err != nil {
		return nil, err
	}
	return runner.Errors(), nil
}

func (m *model) WatchError(folder string) error {
	m.mut.RLock()
	err := m.checkFolderRunningRLocked(folder)
	runner, _ := m.folderRunners.Get(folder)
	m.mut.RUnlock()
	if err != nil {
		return nil // If the folder isn't running, there's no error to report.
	}
	return runner.WatchError()
}

func (m *model) Override(folder string) {
	// Grab the runner and the file set.

	m.mut.RLock()
	runner, ok := m.folderRunners.Get(folder)
	m.mut.RUnlock()
	if !ok {
		return
	}

	// Run the override, taking updates as if they came from scanning.

	runner.Override()
}

func (m *model) Revert(folder string) {
	// Grab the runner and the file set.

	m.mut.RLock()
	runner, ok := m.folderRunners.Get(folder)
	m.mut.RUnlock()
	if !ok {
		return
	}

	// Run the revert, taking updates as if they came from scanning.

	runner.Revert()
}

type TreeEntry struct {
	Name     string       `json:"name"`
	ModTime  time.Time    `json:"modTime"`
	Size     int64        `json:"size"`
	Type     string       `json:"type"`
	Children []*TreeEntry `json:"children,omitempty"`
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
	m.mut.RLock()
	files, ok := m.folderFiles[folder]
	m.mut.RUnlock()
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
	snap.WithPrefixedGlobalTruncated(prefix, func(f protocol.FileInfo) bool {
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
			Type:    f.Type.String(),
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
	m.mut.RLock()
	err := m.checkFolderRunningRLocked(folder)
	ver := m.folderVersioners[folder]
	m.mut.RUnlock()
	if err != nil {
		return nil, err
	}
	if ver == nil {
		return nil, errNoVersioner
	}

	return ver.GetVersions()
}

func (m *model) RestoreFolderVersions(folder string, versions map[string]time.Time) (map[string]error, error) {
	m.mut.RLock()
	err := m.checkFolderRunningRLocked(folder)
	fcfg := m.folderCfgs[folder]
	ver := m.folderVersioners[folder]
	m.mut.RUnlock()
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
	m.mut.RLock()
	defer m.mut.RUnlock()

	fs, ok := m.folderFiles[folder]
	cfg := m.folderCfgs[folder]

	if !ok {
		return nil, ErrFolderMissing
	}

	snap, err := fs.Snapshot()
	if err != nil {
		return nil, err
	}
	defer snap.Release()

	return m.blockAvailabilityRLocked(cfg, snap, file, block), nil
}

func (m *model) blockAvailability(cfg config.FolderConfiguration, snap *db.Snapshot, file protocol.FileInfo, block protocol.BlockInfo) []Availability {
	m.mut.RLock()
	defer m.mut.RUnlock()
	return m.blockAvailabilityRLocked(cfg, snap, file, block)
}

func (m *model) blockAvailabilityRLocked(cfg config.FolderConfiguration, snap *db.Snapshot, file protocol.FileInfo, block protocol.BlockInfo) []Availability {
	var candidates []Availability

	candidates = append(candidates, m.fileAvailabilityRLocked(cfg, snap, file)...)
	candidates = append(candidates, m.blockAvailabilityFromTemporaryRLocked(cfg, file, block)...)

	return candidates
}

func (m *model) fileAvailability(cfg config.FolderConfiguration, snap *db.Snapshot, file protocol.FileInfo) []Availability {
	m.mut.RLock()
	defer m.mut.RUnlock()
	return m.fileAvailabilityRLocked(cfg, snap, file)
}

func (m *model) fileAvailabilityRLocked(cfg config.FolderConfiguration, snap *db.Snapshot, file protocol.FileInfo) []Availability {
	var availabilities []Availability
	for _, device := range snap.Availability(file.Name) {
		if _, ok := m.remoteFolderStates[device]; !ok {
			continue
		}
		if state := m.remoteFolderStates[device][cfg.ID]; state != remoteFolderValid {
			continue
		}
		_, ok := m.deviceConnIDs[device]
		if ok {
			availabilities = append(availabilities, Availability{ID: device, FromTemporary: false})
		}
	}
	return availabilities
}

func (m *model) blockAvailabilityFromTemporaryRLocked(cfg config.FolderConfiguration, file protocol.FileInfo, block protocol.BlockInfo) []Availability {
	var availabilities []Availability
	for _, device := range cfg.Devices {
		if m.deviceDownloads[device.DeviceID].Has(cfg.ID, file.Name, file.Version, int(block.Offset/int64(file.BlockSize()))) {
			availabilities = append(availabilities, Availability{ID: device.DeviceID, FromTemporary: true})
		}
	}
	return availabilities
}

// BringToFront bumps the given files priority in the job queue.
func (m *model) BringToFront(folder, file string) {
	m.mut.RLock()
	runner, ok := m.folderRunners.Get(folder)
	m.mut.RUnlock()

	if ok {
		runner.BringToFront(file)
	}
}

func (m *model) ResetFolder(folder string) error {
	m.mut.RLock()
	defer m.mut.RUnlock()
	_, ok := m.folderRunners.Get(folder)
	if ok {
		return errors.New("folder must be paused when resetting")
	}
	l.Infof("Cleaning metadata for reset folder %q", folder)
	db.DropFolder(m.db, folder)
	return nil
}

func (m *model) String() string {
	return fmt.Sprintf("model@%p", m)
}

func (*model) VerifyConfiguration(from, to config.Configuration) error {
	toFolders := to.FolderMap()
	for _, from := range from.Folders {
		to, ok := toFolders[from.ID]
		if ok && from.Type != to.Type && (from.Type == config.FolderTypeReceiveEncrypted || to.Type == config.FolderTypeReceiveEncrypted) {
			return errors.New("folder type must not be changed from/to receive-encrypted")
		}
	}

	// Verify that any requested versioning is possible to construct, or we
	// will panic later when starting the folder.
	for _, to := range to.Folders {
		if to.Versioning.Type != "" {
			if _, err := versioner.New(to); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *model) CommitConfiguration(from, to config.Configuration) bool {
	// TODO: This should not use reflect, and should take more care to try to handle stuff without restart.

	// Delay processing config changes until after the initial setup
	<-m.started

	// Go through the folder configs and figure out if we need to restart or not.

	// Tracks devices affected by any configuration change to resend ClusterConfig.
	clusterConfigDevices := make(deviceIDSet, len(from.Devices)+len(to.Devices))
	closeDevices := make([]protocol.DeviceID, 0, len(to.Devices))

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
			if toCfg.Type != config.FolderTypeReceiveEncrypted {
				clusterConfigDevices.add(toCfg.DeviceIDs())
			} else {
				// If we don't have the encryption token yet, we need to drop
				// the connection to make the remote re-send the cluster-config
				// and with it the token.
				m.mut.RLock()
				_, ok := m.folderEncryptionPasswordTokens[toCfg.ID]
				m.mut.RUnlock()
				if !ok {
					closeDevices = append(closeDevices, toCfg.DeviceIDs()...)
				} else {
					clusterConfigDevices.add(toCfg.DeviceIDs())
				}
			}
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

	// Pausing a device, unpausing is handled by the connection service.
	fromDevices := from.DeviceMap()
	toDevices := to.DeviceMap()
	for deviceID, toCfg := range toDevices {
		fromCfg, ok := fromDevices[deviceID]
		if !ok {
			sr := stats.NewDeviceStatisticsReference(m.db, deviceID)
			m.mut.Lock()
			m.deviceStatRefs[deviceID] = sr
			m.mut.Unlock()
			continue
		}
		delete(fromDevices, deviceID)
		if fromCfg.Paused == toCfg.Paused {
			continue
		}

		if toCfg.Paused {
			l.Infoln("Pausing", deviceID)
			closeDevices = append(closeDevices, deviceID)
			m.evLogger.Log(events.DevicePaused, map[string]string{"device": deviceID.String()})
		} else {
			// Ignored folder was removed, reconnect to retrigger the prompt.
			if len(fromCfg.IgnoredFolders) > len(toCfg.IgnoredFolders) {
				closeDevices = append(closeDevices, deviceID)
			}

			l.Infoln("Resuming", deviceID)
			m.evLogger.Log(events.DeviceResumed, map[string]string{"device": deviceID.String()})
		}

		if toCfg.MaxRequestKiB != fromCfg.MaxRequestKiB {
			m.mut.Lock()
			m.setConnRequestLimitersLocked(toCfg)
			m.mut.Unlock()
		}
	}

	// Clean up after removed devices
	removedDevices := make([]protocol.DeviceID, 0, len(fromDevices))
	m.mut.Lock()
	for deviceID := range fromDevices {
		delete(m.deviceStatRefs, deviceID)
		removedDevices = append(removedDevices, deviceID)
		delete(clusterConfigDevices, deviceID)
	}
	m.mut.Unlock()

	m.mut.RLock()
	for _, id := range closeDevices {
		delete(clusterConfigDevices, id)
		if conns, ok := m.deviceConnIDs[id]; ok {
			for _, connID := range conns {
				go m.connections[connID].Close(errDevicePaused)
			}
		}
	}
	for _, id := range removedDevices {
		delete(clusterConfigDevices, id)
		if conns, ok := m.deviceConnIDs[id]; ok {
			for _, connID := range conns {
				go m.connections[connID].Close(errDevicePaused)
			}
		}
	}
	m.mut.RUnlock()
	// Generating cluster-configs acquires the mutex.
	m.sendClusterConfig(clusterConfigDevices.AsSlice())

	ignoredDevices := observedDeviceSet(to.IgnoredDevices)
	m.cleanPending(toDevices, toFolders, ignoredDevices, removedFolders)

	m.globalRequestLimiter.SetCapacity(1024 * to.Options.MaxConcurrentIncomingRequestKiB())
	m.folderIOLimiter.SetCapacity(to.Options.MaxFolderConcurrency())

	// Some options don't require restart as those components handle it fine
	// by themselves. Compare the options structs containing only the
	// attributes that require restart and act apprioriately.
	if !reflect.DeepEqual(from.Options.RequiresRestartOnly(), to.Options.RequiresRestartOnly()) {
		l.Debugln(m, "requires restart, options differ")
		return false
	}

	return true
}

func (m *model) setConnRequestLimitersLocked(cfg config.DeviceConfiguration) {
	// Touches connRequestLimiters which is protected by the mutex.
	// 0: default, <0: no limiting
	switch {
	case cfg.MaxRequestKiB > 0:
		m.connRequestLimiters[cfg.DeviceID] = semaphore.New(1024 * cfg.MaxRequestKiB)
	case cfg.MaxRequestKiB == 0:
		m.connRequestLimiters[cfg.DeviceID] = semaphore.New(1024 * defaultPullerPendingKiB)
	}
}

func (m *model) cleanPending(existingDevices map[protocol.DeviceID]config.DeviceConfiguration, existingFolders map[string]config.FolderConfiguration, ignoredDevices deviceIDSet, removedFolders map[string]struct{}) {
	var removedPendingFolders []map[string]string
	pendingFolders, err := m.db.PendingFolders()
	if err != nil {
		msg := "Could not iterate through pending folder entries for cleanup"
		l.Warnf("%v: %v", msg, err)
		m.evLogger.Log(events.Failure, msg)
		// Continue with pending devices below, loop is skipped.
	}
	for folderID, pf := range pendingFolders {
		if _, ok := removedFolders[folderID]; ok {
			// Forget pending folder device associations for recently removed
			// folders as well, assuming the folder is no longer of interest
			// at all (but might become pending again).
			l.Debugf("Discarding pending removed folder %v from all devices", folderID)
			if err := m.db.RemovePendingFolder(folderID); err != nil {
				msg := "Failed to remove pending folder entry"
				l.Warnf("%v (%v): %v", msg, folderID, err)
				m.evLogger.Log(events.Failure, msg)
			} else {
				removedPendingFolders = append(removedPendingFolders, map[string]string{
					"folderID": folderID,
				})
			}
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
			if err := m.db.RemovePendingFolderForDevice(folderID, deviceID); err != nil {
				msg := "Failed to remove pending folder-device entry"
				l.Warnf("%v (%v, %v): %v", msg, folderID, deviceID, err)
				m.evLogger.Log(events.Failure, msg)
				continue
			}
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
		msg := "Could not iterate through pending device entries for cleanup"
		l.Warnf("%v: %v", msg, err)
		m.evLogger.Log(events.Failure, msg)
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
		if err := m.db.RemovePendingDevice(deviceID); err != nil {
			msg := "Failed to remove pending device entry"
			l.Warnf("%v: %v", msg, err)
			m.evLogger.Log(events.Failure, msg)
			continue
		}
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

// checkFolderRunningRLocked returns nil if the folder is up and running and a
// descriptive error if not.
// Need to hold (read) lock on m.mut when calling this.
func (m *model) checkFolderRunningRLocked(folder string) error {
	_, ok := m.folderRunners.Get(folder)
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

// DismissPendingDevices removes the record of a specific pending device.
func (m *model) DismissPendingDevice(device protocol.DeviceID) error {
	l.Debugf("Discarding pending device %v", device)
	err := m.db.RemovePendingDevice(device)
	if err != nil {
		return err
	}
	removedPendingDevices := []map[string]string{
		{"deviceID": device.String()},
	}
	m.evLogger.Log(events.PendingDevicesChanged, map[string]interface{}{
		"removed": removedPendingDevices,
	})
	return nil
}

// DismissPendingFolders removes records of pending folders.  Either a specific folder /
// device combination, or all matching a specific folder ID if the device argument is
// specified as EmptyDeviceID.
func (m *model) DismissPendingFolder(device protocol.DeviceID, folder string) error {
	var removedPendingFolders []map[string]string
	if device == protocol.EmptyDeviceID {
		l.Debugf("Discarding pending removed folder %s from all devices", folder)
		err := m.db.RemovePendingFolder(folder)
		if err != nil {
			return err
		}
		removedPendingFolders = []map[string]string{
			{"folderID": folder},
		}
	} else {
		l.Debugf("Discarding pending folder %s from device %v", folder, device)
		err := m.db.RemovePendingFolderForDevice(folder, device)
		if err != nil {
			return err
		}
		removedPendingFolders = []map[string]string{
			{
				"folderID": folder,
				"deviceID": device.String(),
			},
		}
	}
	if len(removedPendingFolders) > 0 {
		m.evLogger.Log(events.PendingFoldersChanged, map[string]interface{}{
			"removed": removedPendingFolders,
		})
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
	fd, err := cfg.Filesystem(nil).Open(encryptionTokenPath(cfg))
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
	fd, err := cfg.Filesystem(nil).OpenFile(tokenName, fs.OptReadWrite|fs.OptCreate, 0o666)
	if err != nil {
		return err
	}
	defer fd.Close()
	return json.NewEncoder(fd).Encode(storedEncryptionToken{
		FolderID: cfg.ID,
		Token:    token,
	})
}

func newFolderConfiguration(w config.Wrapper, id, label string, fsType config.FilesystemType, path string) config.FolderConfiguration {
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
	RemoteEncrypted  bool              `json:"remoteEncrypted"`
}

// redactPathError checks if the error is actually a os.PathError, and if yes
// returns a redactedError with the path removed.
func redactPathError(err error) (error, bool) {
	perr, ok := err.(*os.PathError)
	if !ok {
		return nil, false
	}
	return &redactedError{
		error:    err,
		redacted: fmt.Errorf("%v: %w", perr.Op, perr.Err),
	}, true
}

type redactedError struct {
	error
	redacted error
}

func without[E comparable, S ~[]E](s S, e E) S {
	for i, x := range s {
		if x == e {
			return append(s[:i], s[i+1:]...)
		}
	}
	return s
}
