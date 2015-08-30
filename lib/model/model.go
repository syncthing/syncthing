// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"bufio"
	"crypto/tls"
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
	"time"

	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/scanner"
	"github.com/syncthing/syncthing/lib/stats"
	"github.com/syncthing/syncthing/lib/symlinks"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/versioner"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/thejerf/suture"
)

// How many files to send in each Index/IndexUpdate message.
const (
	indexTargetSize        = 250 * 1024 // Aim for making index messages no larger than 250 KiB (uncompressed)
	indexPerFileSize       = 250        // Each FileInfo is approximately this big, in bytes, excluding BlockInfos
	indexPerBlockSize      = 40         // Each BlockInfo is approximately this big
	indexBatchSize         = 1000       // Either way, don't include more files than this
	reqValidationTime      = time.Hour  // How long to cache validation entries for Request messages
	reqValidationCacheSize = 1000       // How many entries to aim for in the validation cache size
)

type service interface {
	Serve()
	Stop()
	Jobs() ([]string, []string) // In progress, Queued
	BringToFront(string)
	DelayScan(d time.Duration)
	IndexUpdated() // Remote index was updated notification
	Scan(subs []string) error

	setState(state folderState)
	setError(err error)
	clearError()
	getState() (folderState, time.Time, error)
}

type Model struct {
	*suture.Supervisor

	cfg               *config.Wrapper
	db                *leveldb.DB
	finder            *db.BlockFinder
	progressEmitter   *ProgressEmitter
	id                protocol.DeviceID
	shortID           uint64
	cacheIgnoredFiles bool

	deviceName    string
	clientName    string
	clientVersion string

	folderCfgs     map[string]config.FolderConfiguration                  // folder -> cfg
	folderFiles    map[string]*db.FileSet                                 // folder -> files
	folderDevices  map[string][]protocol.DeviceID                         // folder -> deviceIDs
	deviceFolders  map[protocol.DeviceID][]string                         // deviceID -> folders
	deviceStatRefs map[protocol.DeviceID]*stats.DeviceStatisticsReference // deviceID -> statsRef
	folderIgnores  map[string]*ignore.Matcher                             // folder -> matcher object
	folderRunners  map[string]service                                     // folder -> puller or scanner
	folderStatRefs map[string]*stats.FolderStatisticsReference            // folder -> statsRef
	fmut           sync.RWMutex                                           // protects the above

	conn         map[protocol.DeviceID]Connection
	deviceVer    map[protocol.DeviceID]string
	devicePaused map[protocol.DeviceID]bool
	pmut         sync.RWMutex // protects the above

	reqValidationCache map[string]time.Time // folder / file name => time when confirmed to exist
	rvmut              sync.RWMutex         // protects reqValidationCache
}

var (
	symlinkWarning = stdsync.Once{}
)

// NewModel creates and starts a new model. The model starts in read-only mode,
// where it sends index information to connected peers and responds to requests
// for file data without altering the local folder in any way.
func NewModel(cfg *config.Wrapper, id protocol.DeviceID, deviceName, clientName, clientVersion string, ldb *leveldb.DB) *Model {
	m := &Model{
		Supervisor: suture.New("model", suture.Spec{
			Log: func(line string) {
				if debug {
					l.Debugln(line)
				}
			},
		}),
		cfg:                cfg,
		db:                 ldb,
		finder:             db.NewBlockFinder(ldb, cfg),
		progressEmitter:    NewProgressEmitter(cfg),
		id:                 id,
		shortID:            id.Short(),
		cacheIgnoredFiles:  cfg.Options().CacheIgnoredFiles,
		deviceName:         deviceName,
		clientName:         clientName,
		clientVersion:      clientVersion,
		folderCfgs:         make(map[string]config.FolderConfiguration),
		folderFiles:        make(map[string]*db.FileSet),
		folderDevices:      make(map[string][]protocol.DeviceID),
		deviceFolders:      make(map[protocol.DeviceID][]string),
		deviceStatRefs:     make(map[protocol.DeviceID]*stats.DeviceStatisticsReference),
		folderIgnores:      make(map[string]*ignore.Matcher),
		folderRunners:      make(map[string]service),
		folderStatRefs:     make(map[string]*stats.FolderStatisticsReference),
		conn:               make(map[protocol.DeviceID]Connection),
		deviceVer:          make(map[protocol.DeviceID]string),
		devicePaused:       make(map[protocol.DeviceID]bool),
		reqValidationCache: make(map[string]time.Time),

		fmut:  sync.NewRWMutex(),
		pmut:  sync.NewRWMutex(),
		rvmut: sync.NewRWMutex(),
	}
	if cfg.Options().ProgressUpdateIntervalS > -1 {
		go m.progressEmitter.Serve()
	}

	return m
}

// StartDeadlockDetector starts a deadlock detector on the models locks which
// causes panics in case the locks cannot be acquired in the given timeout
// period.
func (m *Model) StartDeadlockDetector(timeout time.Duration) {
	l.Infof("Starting deadlock detector with %v timeout", timeout)
	deadlockDetect(m.fmut, timeout)
	deadlockDetect(m.pmut, timeout)
}

// StartFolderRW starts read/write processing on the current model. When in
// read/write mode the model will attempt to keep in sync with the cluster by
// pulling needed files from peer devices.
func (m *Model) StartFolderRW(folder string) {
	m.fmut.Lock()
	cfg, ok := m.folderCfgs[folder]
	if !ok {
		panic("cannot start nonexistent folder " + folder)
	}

	_, ok = m.folderRunners[folder]
	if ok {
		panic("cannot start already running folder " + folder)
	}
	p := newRWFolder(m, m.shortID, cfg)
	m.folderRunners[folder] = p
	m.fmut.Unlock()

	if len(cfg.Versioning.Type) > 0 {
		factory, ok := versioner.Factories[cfg.Versioning.Type]
		if !ok {
			l.Fatalf("Requested versioning type %q that does not exist", cfg.Versioning.Type)
		}

		versioner := factory(folder, cfg.Path(), cfg.Versioning.Params)
		if service, ok := versioner.(suture.Service); ok {
			// The versioner implements the suture.Service interface, so
			// expects to be run in the background in addition to being called
			// when files are going to be archived.
			m.Add(service)
		}
		p.versioner = versioner
	}

	m.Add(p)

	l.Okln("Ready to synchronize", folder, "(read-write)")
}

// StartFolderRO starts read only processing on the current model. When in
// read only mode the model will announce files to the cluster but not pull in
// any external changes.
func (m *Model) StartFolderRO(folder string) {
	m.fmut.Lock()
	cfg, ok := m.folderCfgs[folder]
	if !ok {
		panic("cannot start nonexistent folder " + folder)
	}

	_, ok = m.folderRunners[folder]
	if ok {
		panic("cannot start already running folder " + folder)
	}
	s := newROFolder(m, folder, time.Duration(cfg.RescanIntervalS)*time.Second)
	m.folderRunners[folder] = s
	m.fmut.Unlock()

	m.Add(s)

	l.Okln("Ready to synchronize", folder, "(read only; no external updates accepted)")
}

type ConnectionInfo struct {
	protocol.Statistics
	Connected     bool
	Paused        bool
	Address       string
	ClientVersion string
	Type          ConnectionType
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
		"type":          info.Type.String(),
	})
}

// ConnectionStats returns a map with connection statistics for each connected device.
func (m *Model) ConnectionStats() map[string]interface{} {
	type remoteAddrer interface {
		RemoteAddr() net.Addr
	}

	m.pmut.RLock()
	m.fmut.RLock()

	res := make(map[string]interface{})
	devs := m.cfg.Devices()
	conns := make(map[string]ConnectionInfo, len(devs))
	for device := range devs {
		ci := ConnectionInfo{
			ClientVersion: m.deviceVer[device],
			Paused:        m.devicePaused[device],
		}
		if conn, ok := m.conn[device]; ok {
			ci.Type = conn.Type
			ci.Connected = ok
			ci.Statistics = conn.Statistics()
			if addr := conn.RemoteAddr(); addr != nil {
				ci.Address = addr.String()
			}
		}

		conns[device.String()] = ci
	}

	res["connections"] = conns

	m.fmut.RUnlock()
	m.pmut.RUnlock()

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
	var res = make(map[string]stats.DeviceStatistics)
	for id := range m.cfg.Devices() {
		res[id.String()] = m.deviceStatRef(id).GetStatistics()
	}
	return res
}

// FolderStatistics returns statistics about each folder
func (m *Model) FolderStatistics() map[string]stats.FolderStatistics {
	var res = make(map[string]stats.FolderStatistics)
	for id := range m.cfg.Folders() {
		res[id] = m.folderStatRef(id).GetStatistics()
	}
	return res
}

// Completion returns the completion status, in percent, for the given device
// and folder.
func (m *Model) Completion(device protocol.DeviceID, folder string) float64 {
	var tot int64

	m.fmut.RLock()
	rf, ok := m.folderFiles[folder]
	m.fmut.RUnlock()
	if !ok {
		return 0 // Folder doesn't exist, so we hardly have any of it
	}

	rf.WithGlobalTruncated(func(f db.FileIntf) bool {
		if !f.IsDeleted() {
			tot += f.Size()
		}
		return true
	})

	if tot == 0 {
		return 100 // Folder is empty, so we have all of it
	}

	var need int64
	rf.WithNeedTruncated(device, func(f db.FileIntf) bool {
		if !f.IsDeleted() {
			need += f.Size()
		}
		return true
	})

	res := 100 * (1 - float64(need)/float64(tot))
	if debug {
		l.Debugf("%v Completion(%s, %q): %f (%d / %d)", m, device, folder, res, need, tot)
	}

	return res
}

func sizeOf(fs []protocol.FileInfo) (files, deleted int, bytes int64) {
	for _, f := range fs {
		fs, de, by := sizeOfFile(f)
		files += fs
		deleted += de
		bytes += by
	}
	return
}

func sizeOfFile(f db.FileIntf) (files, deleted int, bytes int64) {
	if !f.IsDeleted() {
		files++
	} else {
		deleted++
	}
	bytes += f.Size()
	return
}

// GlobalSize returns the number of files, deleted files and total bytes for all
// files in the global model.
func (m *Model) GlobalSize(folder string) (nfiles, deleted int, bytes int64) {
	m.fmut.RLock()
	defer m.fmut.RUnlock()
	if rf, ok := m.folderFiles[folder]; ok {
		rf.WithGlobalTruncated(func(f db.FileIntf) bool {
			fs, de, by := sizeOfFile(f)
			nfiles += fs
			deleted += de
			bytes += by
			return true
		})
	}
	return
}

// LocalSize returns the number of files, deleted files and total bytes for all
// files in the local folder.
func (m *Model) LocalSize(folder string) (nfiles, deleted int, bytes int64) {
	m.fmut.RLock()
	defer m.fmut.RUnlock()
	if rf, ok := m.folderFiles[folder]; ok {
		rf.WithHaveTruncated(protocol.LocalDeviceID, func(f db.FileIntf) bool {
			if f.IsInvalid() {
				return true
			}
			fs, de, by := sizeOfFile(f)
			nfiles += fs
			deleted += de
			bytes += by
			return true
		})
	}
	return
}

// NeedSize returns the number and total size of currently needed files.
func (m *Model) NeedSize(folder string) (nfiles int, bytes int64) {
	m.fmut.RLock()
	defer m.fmut.RUnlock()
	if rf, ok := m.folderFiles[folder]; ok {
		rf.WithNeedTruncated(protocol.LocalDeviceID, func(f db.FileIntf) bool {
			fs, de, by := sizeOfFile(f)
			nfiles += fs + de
			bytes += by
			return true
		})
	}
	bytes -= m.progressEmitter.BytesCompleted(folder)
	if debug {
		l.Debugf("%v NeedSize(%q): %d %d", m, folder, nfiles, bytes)
	}
	return
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
	rf.WithNeedTruncated(protocol.LocalDeviceID, func(f db.FileIntf) bool {
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
func (m *Model) Index(deviceID protocol.DeviceID, folder string, fs []protocol.FileInfo, flags uint32, options []protocol.Option) {
	if flags != 0 {
		l.Warnln("protocol error: unknown flags 0x%x in Index message", flags)
		return
	}

	if debug {
		l.Debugf("IDX(in): %s %q: %d files", deviceID, folder, len(fs))
	}

	if !m.folderSharedWith(folder, deviceID) {
		events.Default.Log(events.FolderRejected, map[string]string{
			"folder": folder,
			"device": deviceID.String(),
		})
		l.Infof("Unexpected folder ID %q sent from device %q; ensure that the folder exists and that this device is selected under \"Share With\" in the folder configuration.", folder, deviceID)
		return
	}

	m.fmut.RLock()
	cfg := m.folderCfgs[folder]
	files, ok := m.folderFiles[folder]
	runner := m.folderRunners[folder]
	m.fmut.RUnlock()

	if runner != nil {
		// Runner may legitimately not be set if this is the "cleanup" Index
		// message at startup.
		defer runner.IndexUpdated()
	}

	if !ok {
		l.Fatalf("Index for nonexistant folder %q", folder)
	}

	fs = filterIndex(folder, fs, cfg.IgnoreDelete)
	files.Replace(deviceID, fs)

	events.Default.Log(events.RemoteIndexUpdated, map[string]interface{}{
		"device":  deviceID.String(),
		"folder":  folder,
		"items":   len(fs),
		"version": files.LocalVersion(deviceID),
	})
}

// IndexUpdate is called for incremental updates to connected devices' indexes.
// Implements the protocol.Model interface.
func (m *Model) IndexUpdate(deviceID protocol.DeviceID, folder string, fs []protocol.FileInfo, flags uint32, options []protocol.Option) {
	if flags != 0 {
		l.Warnln("protocol error: unknown flags 0x%x in IndexUpdate message", flags)
		return
	}

	if debug {
		l.Debugf("%v IDXUP(in): %s / %q: %d files", m, deviceID, folder, len(fs))
	}

	if !m.folderSharedWith(folder, deviceID) {
		l.Infof("Update for unexpected folder ID %q sent from device %q; ensure that the folder exists and that this device is selected under \"Share With\" in the folder configuration.", folder, deviceID)
		return
	}

	m.fmut.RLock()
	files := m.folderFiles[folder]
	cfg := m.folderCfgs[folder]
	runner, ok := m.folderRunners[folder]
	m.fmut.RUnlock()

	if !ok {
		l.Fatalf("IndexUpdate for nonexistant folder %q", folder)
	}

	fs = filterIndex(folder, fs, cfg.IgnoreDelete)
	files.Update(deviceID, fs)

	events.Default.Log(events.RemoteIndexUpdated, map[string]interface{}{
		"device":  deviceID.String(),
		"folder":  folder,
		"items":   len(fs),
		"version": files.LocalVersion(deviceID),
	})

	runner.IndexUpdated()
}

func (m *Model) folderSharedWith(folder string, deviceID protocol.DeviceID) bool {
	m.fmut.RLock()
	defer m.fmut.RUnlock()
	for _, nfolder := range m.deviceFolders[deviceID] {
		if nfolder == folder {
			return true
		}
	}
	return false
}

func (m *Model) ClusterConfig(deviceID protocol.DeviceID, cm protocol.ClusterConfigMessage) {
	m.pmut.Lock()
	if cm.ClientName == "syncthing" {
		m.deviceVer[deviceID] = cm.ClientVersion
	} else {
		m.deviceVer[deviceID] = cm.ClientName + " " + cm.ClientVersion
	}

	event := map[string]string{
		"id":            deviceID.String(),
		"clientName":    cm.ClientName,
		"clientVersion": cm.ClientVersion,
	}

	if conn, ok := m.conn[deviceID]; ok {
		event["type"] = conn.Type.String()
		addr := conn.RemoteAddr()
		if addr != nil {
			event["addr"] = addr.String()
		}
	}

	m.pmut.Unlock()

	events.Default.Log(events.DeviceConnected, event)

	l.Infof(`Device %s client is "%s %s"`, deviceID, cm.ClientName, cm.ClientVersion)

	var changed bool

	if name := cm.GetOption("name"); name != "" {
		l.Infof("Device %s name is %q", deviceID, name)
		device, ok := m.cfg.Devices()[deviceID]
		if ok && device.Name == "" {
			device.Name = name
			m.cfg.SetDevice(device)
			changed = true
		}
	}

	if m.cfg.Devices()[deviceID].Introducer {
		// This device is an introducer. Go through the announced lists of folders
		// and devices and add what we are missing.

		for _, folder := range cm.Folders {
			// If we don't have this folder yet, skip it. Ideally, we'd
			// offer up something in the GUI to create the folder, but for the
			// moment we only handle folders that we already have.
			if _, ok := m.folderDevices[folder.ID]; !ok {
				continue
			}

		nextDevice:
			for _, device := range folder.Devices {
				var id protocol.DeviceID
				copy(id[:], device.ID)

				if _, ok := m.cfg.Devices()[id]; !ok {
					// The device is currently unknown. Add it to the config.

					l.Infof("Adding device %v to config (vouched for by introducer %v)", id, deviceID)
					newDeviceCfg := config.DeviceConfiguration{
						DeviceID:    id,
						Compression: m.cfg.Devices()[deviceID].Compression,
						Addresses:   []string{"dynamic"},
					}

					// The introducers' introducers are also our introducers.
					if device.Flags&protocol.FlagIntroducer != 0 {
						l.Infof("Device %v is now also an introducer", id)
						newDeviceCfg.Introducer = true
					}

					m.cfg.SetDevice(newDeviceCfg)
					changed = true
				}

				for _, er := range m.deviceFolders[id] {
					if er == folder.ID {
						// We already share the folder with this device, so
						// nothing to do.
						continue nextDevice
					}
				}

				// We don't yet share this folder with this device. Add the device
				// to sharing list of the folder.

				l.Infof("Adding device %v to share %q (vouched for by introducer %v)", id, folder.ID, deviceID)

				m.deviceFolders[id] = append(m.deviceFolders[id], folder.ID)
				m.folderDevices[folder.ID] = append(m.folderDevices[folder.ID], id)

				folderCfg := m.cfg.Folders()[folder.ID]
				folderCfg.Devices = append(folderCfg.Devices, config.FolderDeviceConfiguration{
					DeviceID: id,
				})
				m.cfg.SetFolder(folderCfg)

				changed = true
			}
		}
	}

	if changed {
		m.cfg.Save()
	}
}

// Close removes the peer from the model and closes the underlying connection if possible.
// Implements the protocol.Model interface.
func (m *Model) Close(device protocol.DeviceID, err error) {
	l.Infof("Connection to %s closed: %v", device, err)
	events.Default.Log(events.DeviceDisconnected, map[string]string{
		"id":    device.String(),
		"error": err.Error(),
	})

	m.pmut.Lock()
	m.fmut.RLock()
	for _, folder := range m.deviceFolders[device] {
		m.folderFiles[folder].Replace(device, nil)
	}
	m.fmut.RUnlock()

	conn, ok := m.conn[device]
	if ok {
		closeRawConn(conn)
	}
	delete(m.conn, device)
	delete(m.deviceVer, device)
	m.pmut.Unlock()
}

// Request returns the specified data segment by reading it from local disk.
// Implements the protocol.Model interface.
func (m *Model) Request(deviceID protocol.DeviceID, folder, name string, offset int64, hash []byte, flags uint32, options []protocol.Option, buf []byte) error {
	if offset < 0 {
		return protocol.ErrNoSuchFile
	}

	if !m.folderSharedWith(folder, deviceID) {
		l.Warnf("Request from %s for file %s in unshared folder %q", deviceID, name, folder)
		return protocol.ErrNoSuchFile
	}

	if flags != 0 {
		// We don't currently support or expect any flags.
		return fmt.Errorf("protocol error: unknown flags 0x%x in Request message", flags)
	}

	// Verify that the requested file exists in the local model. We only need
	// to validate this file if we haven't done so recently, so we keep a
	// cache of successfull results. "Recently" can be quite a long time, as
	// we remove validation cache entries when we detect local changes. If
	// we're out of sync here and the file actually doesn't exist any more, or
	// has shrunk or something, then we'll anyway get a read error that we
	// pass on to the other side.

	m.rvmut.RLock()
	validated := m.reqValidationCache[folder+"/"+name]
	m.rvmut.RUnlock()

	if time.Since(validated) > reqValidationTime {
		m.fmut.RLock()
		folderFiles, ok := m.folderFiles[folder]
		m.fmut.RUnlock()

		if !ok {
			l.Warnf("Request from %s for file %s in nonexistent folder %q", deviceID, name, folder)
			return protocol.ErrNoSuchFile
		}

		// This call is really expensive for large files, as we load the full
		// block list which may be megabytes and megabytes of data to allocate
		// space for, read, and deserialize.
		lf, ok := folderFiles.Get(protocol.LocalDeviceID, name)
		if !ok {
			return protocol.ErrNoSuchFile
		}

		if lf.IsInvalid() || lf.IsDeleted() {
			if debug {
				l.Debugf("%v REQ(in): %s: %q / %q o=%d s=%d; invalid: %v", m, deviceID, folder, name, offset, len(buf), lf)
			}
			return protocol.ErrInvalid
		}

		if offset > lf.Size() {
			if debug {
				l.Debugf("%v REQ(in; nonexistent): %s: %q o=%d s=%d", m, deviceID, name, offset, len(buf))
			}
			return protocol.ErrNoSuchFile
		}

		m.rvmut.Lock()
		m.reqValidationCache[folder+"/"+name] = time.Now()
		if len(m.reqValidationCache) > reqValidationCacheSize {
			// Don't let the cache grow infinitely
			for name, validated := range m.reqValidationCache {
				if time.Since(validated) > time.Minute {
					delete(m.reqValidationCache, name)
				}
			}

			if len(m.reqValidationCache) > reqValidationCacheSize*9/10 {
				// The first clean didn't help much, we're still over 90%
				// full; we may have synced a lot of files lately. Prune the
				// cache more aggressively by removing every other item so we
				// don't get stuck doing useless cache cleaning.
				i := 0
				for name := range m.reqValidationCache {
					if i%2 == 0 {
						delete(m.reqValidationCache, name)
					}
					i++
				}
			}
		}
		m.rvmut.Unlock()
	}

	if debug && deviceID != protocol.LocalDeviceID {
		l.Debugf("%v REQ(in): %s: %q / %q o=%d s=%d", m, deviceID, folder, name, offset, len(buf))
	}
	m.fmut.RLock()
	fn := filepath.Join(m.folderCfgs[folder].Path(), name)
	m.fmut.RUnlock()

	var reader io.ReaderAt
	var err error
	if info, err := os.Lstat(fn); err == nil && info.Mode()&os.ModeSymlink != 0 {
		target, _, err := symlinks.Read(fn)
		if err != nil {
			return err
		}
		reader = strings.NewReader(target)
	} else {
		// Cannot easily cache fd's because we might need to delete the file
		// at any moment.
		reader, err = os.Open(fn)
		if err != nil {
			return err
		}

		defer reader.(*os.File).Close()
	}

	_, err = reader.ReadAt(buf, offset)
	if err != nil {
		return err
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
	f, ok := fs.Get(protocol.LocalDeviceID, file)
	return f, ok
}

func (m *Model) CurrentGlobalFile(folder string, file string) (protocol.FileInfo, bool) {
	m.fmut.RLock()
	fs, ok := m.folderFiles[folder]
	m.fmut.RUnlock()
	if !ok {
		return protocol.FileInfo{}, false
	}
	f, ok := fs.GetGlobal(file)
	return f, ok
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
	var lines []string

	m.fmut.RLock()
	cfg, ok := m.folderCfgs[folder]
	m.fmut.RUnlock()
	if !ok {
		return lines, nil, fmt.Errorf("Folder %s does not exist", folder)
	}

	fd, err := os.Open(filepath.Join(cfg.Path(), ".stignore"))
	if err != nil {
		if os.IsNotExist(err) {
			return lines, nil, nil
		}
		l.Warnln("Loading .stignore:", err)
		return lines, nil, err
	}
	defer fd.Close()

	scanner := bufio.NewScanner(fd)
	for scanner.Scan() {
		lines = append(lines, strings.TrimSpace(scanner.Text()))
	}

	m.fmut.RLock()
	patterns := m.folderIgnores[folder].Patterns()
	m.fmut.RUnlock()

	return lines, patterns, nil
}

func (m *Model) SetIgnores(folder string, content []string) error {
	cfg, ok := m.folderCfgs[folder]
	if !ok {
		return fmt.Errorf("Folder %s does not exist", folder)
	}

	path := filepath.Join(cfg.Path(), ".stignore")

	fd, err := osutil.CreateAtomic(path, 0644)
	if err != nil {
		l.Warnln("Saving .stignore:", err)
		return err
	}

	for _, line := range content {
		fmt.Fprintln(fd, line)
	}

	if err := fd.Close(); err != nil {
		l.Warnln("Saving .stignore:", err)
		return err
	}
	osutil.HideFile(path)

	return m.ScanFolder(folder)
}

// AddConnection adds a new peer connection to the model. An initial index will
// be sent to the connected peer, thereafter index updates whenever the local
// folder changes.
func (m *Model) AddConnection(conn Connection) {
	deviceID := conn.ID()

	m.pmut.Lock()
	if _, ok := m.conn[deviceID]; ok {
		panic("add existing device")
	}
	m.conn[deviceID] = conn

	conn.Start()

	cm := m.clusterConfig(deviceID)
	conn.ClusterConfig(cm)

	m.fmut.RLock()
	for _, folder := range m.deviceFolders[deviceID] {
		fs := m.folderFiles[folder]
		go sendIndexes(conn, folder, fs, m.folderIgnores[folder])
	}
	m.fmut.RUnlock()
	m.pmut.Unlock()

	m.deviceWasSeen(deviceID)
}

func (m *Model) PauseDevice(device protocol.DeviceID) {
	m.pmut.Lock()
	m.devicePaused[device] = true
	_, ok := m.conn[device]
	m.pmut.Unlock()
	if ok {
		m.Close(device, errors.New("device paused"))
	}
	events.Default.Log(events.DevicePaused, map[string]string{"device": device.String()})
}

func (m *Model) ResumeDevice(device protocol.DeviceID) {
	m.pmut.Lock()
	m.devicePaused[device] = false
	m.pmut.Unlock()
	events.Default.Log(events.DeviceResumed, map[string]string{"device": device.String()})
}

func (m *Model) IsPaused(device protocol.DeviceID) bool {
	m.pmut.Lock()
	paused := m.devicePaused[device]
	m.pmut.Unlock()
	return paused
}

func (m *Model) deviceStatRef(deviceID protocol.DeviceID) *stats.DeviceStatisticsReference {
	m.fmut.Lock()
	defer m.fmut.Unlock()

	if sr, ok := m.deviceStatRefs[deviceID]; ok {
		return sr
	}

	sr := stats.NewDeviceStatisticsReference(m.db, deviceID)
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
	m.folderStatRef(folder).ReceivedFile(file)
}

func sendIndexes(conn protocol.Connection, folder string, fs *db.FileSet, ignores *ignore.Matcher) {
	deviceID := conn.ID()
	name := conn.Name()
	var err error

	if debug {
		l.Debugf("sendIndexes for %s-%s/%q starting", deviceID, name, folder)
	}

	minLocalVer, err := sendIndexTo(true, 0, conn, folder, fs, ignores)

	sub := events.Default.Subscribe(events.LocalIndexUpdated)
	defer events.Default.Unsubscribe(sub)

	for err == nil {
		// While we have sent a localVersion at least equal to the one
		// currently in the database, wait for the local index to update. The
		// local index may update for other folders than the one we are
		// sending for.
		if fs.LocalVersion(protocol.LocalDeviceID) <= minLocalVer {
			sub.Poll(time.Minute)
			continue
		}

		minLocalVer, err = sendIndexTo(false, minLocalVer, conn, folder, fs, ignores)

		// Wait a short amount of time before entering the next loop. If there
		// are continous changes happening to the local index, this gives us
		// time to batch them up a little.
		time.Sleep(250 * time.Millisecond)
	}

	if debug {
		l.Debugf("sendIndexes for %s-%s/%q exiting: %v", deviceID, name, folder, err)
	}
}

func sendIndexTo(initial bool, minLocalVer int64, conn protocol.Connection, folder string, fs *db.FileSet, ignores *ignore.Matcher) (int64, error) {
	deviceID := conn.ID()
	name := conn.Name()
	batch := make([]protocol.FileInfo, 0, indexBatchSize)
	currentBatchSize := 0
	maxLocalVer := int64(0)
	var err error

	fs.WithHave(protocol.LocalDeviceID, func(fi db.FileIntf) bool {
		f := fi.(protocol.FileInfo)
		if f.LocalVersion <= minLocalVer {
			return true
		}

		if f.LocalVersion > maxLocalVer {
			maxLocalVer = f.LocalVersion
		}

		if ignores.Match(f.Name) || symlinkInvalid(folder, f) {
			if debug {
				l.Debugln("not sending update for ignored/unsupported symlink", f)
			}
			return true
		}

		if len(batch) == indexBatchSize || currentBatchSize > indexTargetSize {
			if initial {
				if err = conn.Index(folder, batch, 0, nil); err != nil {
					return false
				}
				if debug {
					l.Debugf("sendIndexes for %s-%s/%q: %d files (<%d bytes) (initial index)", deviceID, name, folder, len(batch), currentBatchSize)
				}
				initial = false
			} else {
				if err = conn.IndexUpdate(folder, batch, 0, nil); err != nil {
					return false
				}
				if debug {
					l.Debugf("sendIndexes for %s-%s/%q: %d files (<%d bytes) (batched update)", deviceID, name, folder, len(batch), currentBatchSize)
				}
			}

			batch = make([]protocol.FileInfo, 0, indexBatchSize)
			currentBatchSize = 0
		}

		batch = append(batch, f)
		currentBatchSize += indexPerFileSize + len(f.Blocks)*indexPerBlockSize
		return true
	})

	if initial && err == nil {
		err = conn.Index(folder, batch, 0, nil)
		if debug && err == nil {
			l.Debugf("sendIndexes for %s-%s/%q: %d files (small initial index)", deviceID, name, folder, len(batch))
		}
	} else if len(batch) > 0 && err == nil {
		err = conn.IndexUpdate(folder, batch, 0, nil)
		if debug && err == nil {
			l.Debugf("sendIndexes for %s-%s/%q: %d files (last batch)", deviceID, name, folder, len(batch))
		}
	}

	return maxLocalVer, err
}

func (m *Model) updateLocals(folder string, fs []protocol.FileInfo) {
	m.fmut.RLock()
	files := m.folderFiles[folder]
	m.fmut.RUnlock()
	files.Update(protocol.LocalDeviceID, fs)

	m.rvmut.Lock()
	for _, f := range fs {
		delete(m.reqValidationCache, folder+"/"+f.Name)
	}
	m.rvmut.Unlock()

	events.Default.Log(events.LocalIndexUpdated, map[string]interface{}{
		"folder":  folder,
		"items":   len(fs),
		"version": files.LocalVersion(protocol.LocalDeviceID),
	})
}

func (m *Model) requestGlobal(deviceID protocol.DeviceID, folder, name string, offset int64, size int, hash []byte, flags uint32, options []protocol.Option) ([]byte, error) {
	m.pmut.RLock()
	nc, ok := m.conn[deviceID]
	m.pmut.RUnlock()

	if !ok {
		return nil, fmt.Errorf("requestGlobal: no such device: %s", deviceID)
	}

	if debug {
		l.Debugf("%v REQ(out): %s: %q / %q o=%d s=%d h=%x f=%x op=%s", m, deviceID, folder, name, offset, size, hash, flags, options)
	}

	return nc.Request(folder, name, offset, size, hash, flags, options)
}

func (m *Model) AddFolder(cfg config.FolderConfiguration) {
	if len(cfg.ID) == 0 {
		panic("cannot add empty folder id")
	}

	m.fmut.Lock()
	m.folderCfgs[cfg.ID] = cfg
	m.folderFiles[cfg.ID] = db.NewFileSet(cfg.ID, m.db)

	m.folderDevices[cfg.ID] = make([]protocol.DeviceID, len(cfg.Devices))
	for i, device := range cfg.Devices {
		m.folderDevices[cfg.ID][i] = device.DeviceID
		m.deviceFolders[device.DeviceID] = append(m.deviceFolders[device.DeviceID], cfg.ID)
	}

	ignores := ignore.New(m.cacheIgnoredFiles)
	_ = ignores.Load(filepath.Join(cfg.Path(), ".stignore")) // Ignore error, there might not be an .stignore
	m.folderIgnores[cfg.ID] = ignores

	m.fmut.Unlock()
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
	return m.ScanFolderSubs(folder, nil)
}

func (m *Model) ScanFolderSubs(folder string, subs []string) error {
	m.fmut.Lock()
	runner, ok := m.folderRunners[folder]
	m.fmut.Unlock()

	// Folders are added to folderRunners only when they are started. We can't
	// scan them before they have started, so that's what we need to check for
	// here.
	if !ok {
		return errors.New("no such folder")
	}

	return runner.Scan(subs)
}

func (m *Model) internalScanFolderSubs(folder string, subs []string) error {
	for i, sub := range subs {
		sub = osutil.NativeFilename(sub)
		if p := filepath.Clean(filepath.Join(folder, sub)); !strings.HasPrefix(p, folder) {
			return errors.New("invalid subpath")
		}
		subs[i] = sub
	}

	m.fmut.Lock()
	fs := m.folderFiles[folder]
	folderCfg := m.folderCfgs[folder]
	ignores := m.folderIgnores[folder]
	runner, ok := m.folderRunners[folder]
	m.fmut.Unlock()

	// Folders are added to folderRunners only when they are started. We can't
	// scan them before they have started, so that's what we need to check for
	// here.
	if !ok {
		return errors.New("no such folder")
	}

	if err := m.CheckFolderHealth(folder); err != nil {
		return err
	}

	_ = ignores.Load(filepath.Join(folderCfg.Path(), ".stignore")) // Ignore error, there might not be an .stignore

	// Required to make sure that we start indexing at a directory we're already
	// aware off.
	var unifySubs []string
nextSub:
	for _, sub := range subs {
		for sub != "" {
			parent := filepath.Dir(sub)
			if parent == "." || parent == string(filepath.Separator) {
				parent = ""
			}
			if _, ok = fs.Get(protocol.LocalDeviceID, parent); ok {
				break
			}
			sub = parent
		}
		for _, us := range unifySubs {
			if strings.HasPrefix(sub, us) {
				continue nextSub
			}
		}
		unifySubs = append(unifySubs, sub)
	}
	subs = unifySubs

	w := &scanner.Walker{
		Folder:                folderCfg.ID,
		Dir:                   folderCfg.Path(),
		Subs:                  subs,
		Matcher:               ignores,
		BlockSize:             protocol.BlockSize,
		TempNamer:             defTempNamer,
		TempLifetime:          time.Duration(m.cfg.Options().KeepTemporariesH) * time.Hour,
		CurrentFiler:          cFiler{m, folder},
		MtimeRepo:             db.NewVirtualMtimeRepo(m.db, folderCfg.ID),
		IgnorePerms:           folderCfg.IgnorePerms,
		AutoNormalize:         folderCfg.AutoNormalize,
		Hashers:               m.numHashers(folder),
		ShortID:               m.shortID,
		ProgressTickIntervalS: folderCfg.ScanProgressIntervalS,
	}

	runner.setState(FolderScanning)

	fchan, err := w.Walk()
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

	batchSizeFiles := 100
	batchSizeBlocks := 2048 // about 256 MB

	batch := make([]protocol.FileInfo, 0, batchSizeFiles)
	blocksHandled := 0

	for f := range fchan {
		if len(batch) == batchSizeFiles || blocksHandled > batchSizeBlocks {
			if err := m.CheckFolderHealth(folder); err != nil {
				l.Infof("Stopping folder %s mid-scan due to folder error: %s", folder, err)
				return err
			}
			m.updateLocals(folder, batch)
			batch = batch[:0]
			blocksHandled = 0
		}
		batch = append(batch, f)
		blocksHandled += len(f.Blocks)
	}

	if err := m.CheckFolderHealth(folder); err != nil {
		l.Infof("Stopping folder %s mid-scan due to folder error: %s", folder, err)
		return err
	} else if len(batch) > 0 {
		m.updateLocals(folder, batch)
	}

	batch = batch[:0]
	// TODO: We should limit the Have scanning to start at sub
	seenPrefix := false
	var iterError error
	fs.WithHaveTruncated(protocol.LocalDeviceID, func(fi db.FileIntf) bool {
		f := fi.(db.FileInfoTruncated)
		hasPrefix := len(subs) == 0
		for _, sub := range subs {
			if strings.HasPrefix(f.Name, sub) {
				hasPrefix = true
				break
			}
		}
		// Return true so that we keep iterating, until we get to the part
		// of the tree we are interested in. Then return false so we stop
		// iterating when we've passed the end of the subtree.
		if !hasPrefix {
			return !seenPrefix
		}

		seenPrefix = true
		if !f.IsDeleted() {
			if f.IsInvalid() {
				return true
			}

			if len(batch) == batchSizeFiles {
				if err := m.CheckFolderHealth(folder); err != nil {
					iterError = err
					return false
				}
				m.updateLocals(folder, batch)
				batch = batch[:0]
			}

			if ignores.Match(f.Name) || symlinkInvalid(folder, f) {
				// File has been ignored or an unsupported symlink. Set invalid bit.
				if debug {
					l.Debugln("setting invalid bit on ignored", f)
				}
				nf := protocol.FileInfo{
					Name:     f.Name,
					Flags:    f.Flags | protocol.FlagInvalid,
					Modified: f.Modified,
					Version:  f.Version, // The file is still the same, so don't bump version
				}
				batch = append(batch, nf)
			} else if _, err := osutil.Lstat(filepath.Join(folderCfg.Path(), f.Name)); err != nil {
				// File has been deleted.

				// We don't specifically verify that the error is
				// os.IsNotExist because there is a corner case when a
				// directory is suddenly transformed into a file. When that
				// happens, files that were in the directory (that is now a
				// file) are deleted but will return a confusing error ("not a
				// directory") when we try to Lstat() them.

				nf := protocol.FileInfo{
					Name:     f.Name,
					Flags:    f.Flags | protocol.FlagDeleted,
					Modified: f.Modified,
					Version:  f.Version.Update(m.shortID),
				}
				batch = append(batch, nf)
			}
		}
		return true
	})

	if iterError != nil {
		l.Infof("Stopping folder %s mid-scan due to folder error: %s", folder, iterError)
		return iterError
	}

	if err := m.CheckFolderHealth(folder); err != nil {
		l.Infof("Stopping folder %s mid-scan due to folder error: %s", folder, err)
		return err
	} else if len(batch) > 0 {
		m.updateLocals(folder, batch)
	}

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

	if perFolder := runtime.GOMAXPROCS(-1) / numFolders; perFolder > 0 {
		// We have CPUs to spare, divide them per folder.
		return perFolder
	}

	return 1
}

// clusterConfig returns a ClusterConfigMessage that is correct for the given peer device
func (m *Model) clusterConfig(device protocol.DeviceID) protocol.ClusterConfigMessage {
	cm := protocol.ClusterConfigMessage{
		ClientName:    m.clientName,
		ClientVersion: m.clientVersion,
		Options: []protocol.Option{
			{
				Key:   "name",
				Value: m.deviceName,
			},
		},
	}

	m.fmut.RLock()
	for _, folder := range m.deviceFolders[device] {
		cr := protocol.Folder{
			ID: folder,
		}
		for _, device := range m.folderDevices[folder] {
			// DeviceID is a value type, but with an underlying array. Copy it
			// so we don't grab aliases to the same array later on in device[:]
			device := device
			// TODO: Set read only bit when relevant
			cn := protocol.Device{
				ID:    device[:],
				Flags: protocol.FlagShareTrusted,
			}
			if deviceCfg := m.cfg.Devices()[device]; deviceCfg.Introducer {
				cn.Flags |= protocol.FlagIntroducer
			}
			cr.Devices = append(cr.Devices, cn)
		}
		cm.Folders = append(cm.Folders, cr)
	}
	m.fmut.RUnlock()

	return cm
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
	batch := make([]protocol.FileInfo, 0, indexBatchSize)
	fs.WithNeed(protocol.LocalDeviceID, func(fi db.FileIntf) bool {
		need := fi.(protocol.FileInfo)
		if len(batch) == indexBatchSize {
			m.updateLocals(folder, batch)
			batch = batch[:0]
		}

		have, ok := fs.Get(protocol.LocalDeviceID, need.Name)
		if !ok || have.Name != need.Name {
			// We are missing the file
			need.Flags |= protocol.FlagDeleted
			need.Blocks = nil
			need.Version = need.Version.Update(m.shortID)
		} else {
			// We have the file, replace with our version
			have.Version = have.Version.Merge(need.Version).Update(m.shortID)
			need = have
		}
		need.LocalVersion = 0
		batch = append(batch, need)
		return true
	})
	if len(batch) > 0 {
		m.updateLocals(folder, batch)
	}
	runner.setState(FolderIdle)
}

// CurrentLocalVersion returns the change version for the given folder.
// This is guaranteed to increment if the contents of the local folder has
// changed.
func (m *Model) CurrentLocalVersion(folder string) (int64, bool) {
	m.fmut.RLock()
	fs, ok := m.folderFiles[folder]
	m.fmut.RUnlock()
	if !ok {
		// The folder might not exist, since this can be called with a user
		// specified folder name from the REST interface.
		return 0, false
	}

	return fs.LocalVersion(protocol.LocalDeviceID), true
}

// RemoteLocalVersion returns the change version for the given folder, as
// sent by remote peers. This is guaranteed to increment if the contents of
// the remote or global folder has changed.
func (m *Model) RemoteLocalVersion(folder string) (int64, bool) {
	m.fmut.RLock()
	defer m.fmut.RUnlock()

	fs, ok := m.folderFiles[folder]
	if !ok {
		// The folder might not exist, since this can be called with a user
		// specified folder name from the REST interface.
		return 0, false
	}

	var ver int64
	for _, n := range m.folderDevices[folder] {
		ver += fs.LocalVersion(n)
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
				time.Unix(f.Modified, 0), f.Size(),
			}
		}

		return true
	})

	return output
}

func (m *Model) Availability(folder, file string) []protocol.DeviceID {
	// Acquire this lock first, as the value returned from foldersFiles can
	// get heavily modified on Close()
	m.pmut.RLock()
	defer m.pmut.RUnlock()

	m.fmut.RLock()
	fs, ok := m.folderFiles[folder]
	m.fmut.RUnlock()
	if !ok {
		return nil
	}

	availableDevices := []protocol.DeviceID{}
	for _, device := range fs.Availability(file) {
		_, ok := m.conn[device]
		if ok {
			availableDevices = append(availableDevices, device)
		}
	}
	return availableDevices
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
	if minFree := float64(m.cfg.Options().MinHomeDiskFreePct); minFree > 0 {
		if free, err := osutil.DiskFreePercentage(m.cfg.ConfigPath()); err == nil && free < minFree {
			return errors.New("home disk is out of space")
		}
	}

	folder, ok := m.cfg.Folders()[id]
	if !ok {
		return errors.New("folder does not exist")
	}

	fi, err := os.Stat(folder.Path())
	if v, ok := m.CurrentLocalVersion(id); ok && v > 0 {
		// Safety check. If the cached index contains files but the
		// folder doesn't exist, we have a problem. We would assume
		// that all files have been deleted which might not be the case,
		// so mark it as invalid instead.
		if err != nil || !fi.IsDir() {
			err = errors.New("folder path missing")
		} else if !folder.HasMarker() {
			err = errors.New("folder marker missing")
		} else if free, errDfp := osutil.DiskFreePercentage(folder.Path()); errDfp == nil && free < float64(folder.MinDiskFreePct) {
			err = errors.New("out of disk space")
		}
	} else if os.IsNotExist(err) {
		// If we don't have any files in the index, and the directory
		// doesn't exist, try creating it.
		err = osutil.MkdirAll(folder.Path(), 0700)
		if err == nil {
			err = folder.CreateMarker()
		}
	} else if !folder.HasMarker() {
		// If we don't have any files in the index, and the path does exist
		// but the marker is not there, create it.
		err = folder.CreateMarker()
	}

	m.fmut.RLock()
	runner, runnerExists := m.folderRunners[folder.ID]
	m.fmut.RUnlock()

	var oldErr error
	if runnerExists {
		_, _, oldErr = runner.getState()
	}

	if err != nil {
		if oldErr != nil && oldErr.Error() != err.Error() {
			l.Infof("Folder %q error changed: %q -> %q", folder.ID, oldErr, err)
		} else if oldErr == nil {
			l.Warnf("Stopping folder %q - %v", folder.ID, err)
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

	return err
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
			if debug {
				l.Debugln(m, "adding folder", folderID)
			}
			m.AddFolder(cfg)
			if cfg.ReadOnly {
				m.StartFolderRO(folderID)
			} else {
				m.StartFolderRW(folderID)
			}

			// Drop connections to all devices that can now share the new
			// folder.
			m.pmut.Lock()
			for _, dev := range cfg.DeviceIDs() {
				if conn, ok := m.conn[dev]; ok {
					closeRawConn(conn)
				}
			}
			m.pmut.Unlock()
		}
	}

	for folderID, fromCfg := range fromFolders {
		toCfg, ok := toFolders[folderID]
		if !ok {
			// A folder was removed. Requires restart.
			if debug {
				l.Debugln(m, "requires restart, removing folder", folderID)
			}
			return false
		}

		// This folder exists on both sides. Compare the device lists, as we
		// can handle adding a device (but not currently removing one).

		fromDevs := mapDevices(fromCfg.DeviceIDs())
		toDevs := mapDevices(toCfg.DeviceIDs())
		for dev := range fromDevs {
			if _, ok := toDevs[dev]; !ok {
				// A device was removed. Requires restart.
				if debug {
					l.Debugln(m, "requires restart, removing device", dev, "from folder", folderID)
				}
				return false
			}
		}

		for dev := range toDevs {
			if _, ok := fromDevs[dev]; !ok {
				// A device was added. Handle it!

				m.fmut.Lock()
				m.pmut.Lock()

				m.folderCfgs[folderID] = toCfg
				m.folderDevices[folderID] = append(m.folderDevices[folderID], dev)
				m.deviceFolders[dev] = append(m.deviceFolders[dev], folderID)

				// If we already have a connection to this device, we should
				// disconnect it so that we start sharing the folder with it.
				// We close the underlying connection and let the normal error
				// handling kick in to clean up and reconnect.
				if conn, ok := m.conn[dev]; ok {
					closeRawConn(conn)
				}

				m.pmut.Unlock()
				m.fmut.Unlock()
			}
		}

		// Check if anything else differs, apart from the device list.
		fromCfg.Devices = nil
		toCfg.Devices = nil
		if !reflect.DeepEqual(fromCfg, toCfg) {
			if debug {
				l.Debugln(m, "requires restart, folder", folderID, "configuration differs")
			}
			return false
		}
	}

	// Removing a device requres restart
	toDevs := mapDeviceCfgs(from.Devices)
	for _, dev := range from.Devices {
		if _, ok := toDevs[dev.DeviceID]; !ok {
			if debug {
				l.Debugln(m, "requires restart, device", dev.DeviceID, "was removed")
			}
			return false
		}
	}

	// All of the generic options require restart
	if !reflect.DeepEqual(from.Options, to.Options) {
		if debug {
			l.Debugln(m, "requires restart, options differ")
		}
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

// mapDeviceCfgs returns a map of device ID to nothing for the given slice of
// device configurations.
func mapDeviceCfgs(devices []config.DeviceConfiguration) map[protocol.DeviceID]struct{} {
	m := make(map[protocol.DeviceID]struct{}, len(devices))
	for _, dev := range devices {
		m[dev.DeviceID] = struct{}{}
	}
	return m
}

func filterIndex(folder string, fs []protocol.FileInfo, dropDeletes bool) []protocol.FileInfo {
	for i := 0; i < len(fs); {
		if fs[i].Flags&^protocol.FlagsAll != 0 {
			if debug {
				l.Debugln("dropping update for file with unknown bits set", fs[i])
			}
			fs[i] = fs[len(fs)-1]
			fs = fs[:len(fs)-1]
		} else if fs[i].IsDeleted() && dropDeletes {
			if debug {
				l.Debugln("dropping update for undesired delete", fs[i])
			}
			fs[i] = fs[len(fs)-1]
			fs = fs[:len(fs)-1]
		} else if symlinkInvalid(folder, fs[i]) {
			if debug {
				l.Debugln("dropping update for unsupported symlink", fs[i])
			}
			fs[i] = fs[len(fs)-1]
			fs = fs[:len(fs)-1]
		} else {
			i++
		}
	}
	return fs
}

func symlinkInvalid(folder string, fi db.FileIntf) bool {
	if !symlinks.Supported && fi.IsSymlink() && !fi.IsInvalid() && !fi.IsDeleted() {
		symlinkWarning.Do(func() {
			l.Warnln("Symlinks are disabled, unsupported or require Administrator privileges. This might cause your folder to appear out of sync.")
		})

		// Need to type switch for the concrete type to be able to access fields...
		var name string
		switch fi := fi.(type) {
		case protocol.FileInfo:
			name = fi.Name
		case db.FileInfoTruncated:
			name = fi.Name
		}
		l.Infoln("Unsupported symlink", name, "in folder", folder)
		return true
	}
	return false
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
