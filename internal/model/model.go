// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

package model

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/syncthing/syncthing/internal/config"
	"github.com/syncthing/syncthing/internal/events"
	"github.com/syncthing/syncthing/internal/files"
	"github.com/syncthing/syncthing/internal/ignore"
	"github.com/syncthing/syncthing/internal/lamport"
	"github.com/syncthing/syncthing/internal/osutil"
	"github.com/syncthing/syncthing/internal/protocol"
	"github.com/syncthing/syncthing/internal/scanner"
	"github.com/syncthing/syncthing/internal/stats"
	"github.com/syncthing/syncthing/internal/versioner"
	"github.com/syndtr/goleveldb/leveldb"
)

type folderState int

const (
	FolderIdle folderState = iota
	FolderScanning
	FolderSyncing
	FolderCleaning
)

func (s folderState) String() string {
	switch s {
	case FolderIdle:
		return "idle"
	case FolderScanning:
		return "scanning"
	case FolderCleaning:
		return "cleaning"
	case FolderSyncing:
		return "syncing"
	default:
		return "unknown"
	}
}

// How many files to send in each Index/IndexUpdate message.
const (
	indexTargetSize   = 250 * 1024 // Aim for making index messages no larger than 250 KiB (uncompressed)
	indexPerFileSize  = 250        // Each FileInfo is approximately this big, in bytes, excluding BlockInfos
	IndexPerBlockSize = 40         // Each BlockInfo is approximately this big
	indexBatchSize    = 1000       // Either way, don't include more files than this
)

type service interface {
	Serve()
	Stop()
}

type Model struct {
	cfg    *config.ConfigWrapper
	db     *leveldb.DB
	finder *files.BlockFinder

	deviceName    string
	clientName    string
	clientVersion string

	folderCfgs     map[string]config.FolderConfiguration                  // folder -> cfg
	folderFiles    map[string]*files.Set                                  // folder -> files
	folderDevices  map[string][]protocol.DeviceID                         // folder -> deviceIDs
	deviceFolders  map[protocol.DeviceID][]string                         // deviceID -> folders
	deviceStatRefs map[protocol.DeviceID]*stats.DeviceStatisticsReference // deviceID -> statsRef
	folderIgnores  map[string]*ignore.Matcher                             // folder -> matcher object
	folderRunners  map[string]service                                     // folder -> puller or scanner
	fmut           sync.RWMutex                                           // protects the above

	folderState        map[string]folderState // folder -> state
	folderStateChanged map[string]time.Time   // folder -> time when state changed
	smut               sync.RWMutex

	protoConn map[protocol.DeviceID]protocol.Connection
	rawConn   map[protocol.DeviceID]io.Closer
	deviceVer map[protocol.DeviceID]string
	pmut      sync.RWMutex // protects protoConn and rawConn

	addedFolder bool
	started     bool
}

var (
	ErrNoSuchFile = errors.New("no such file")
	ErrInvalid    = errors.New("file is invalid")
)

// NewModel creates and starts a new model. The model starts in read-only mode,
// where it sends index information to connected peers and responds to requests
// for file data without altering the local folder in any way.
func NewModel(cfg *config.ConfigWrapper, deviceName, clientName, clientVersion string, db *leveldb.DB) *Model {
	m := &Model{
		cfg:                cfg,
		db:                 db,
		deviceName:         deviceName,
		clientName:         clientName,
		clientVersion:      clientVersion,
		folderCfgs:         make(map[string]config.FolderConfiguration),
		folderFiles:        make(map[string]*files.Set),
		folderDevices:      make(map[string][]protocol.DeviceID),
		deviceFolders:      make(map[protocol.DeviceID][]string),
		deviceStatRefs:     make(map[protocol.DeviceID]*stats.DeviceStatisticsReference),
		folderIgnores:      make(map[string]*ignore.Matcher),
		folderRunners:      make(map[string]service),
		folderState:        make(map[string]folderState),
		folderStateChanged: make(map[string]time.Time),
		protoConn:          make(map[protocol.DeviceID]protocol.Connection),
		rawConn:            make(map[protocol.DeviceID]io.Closer),
		deviceVer:          make(map[protocol.DeviceID]string),
		finder:             files.NewBlockFinder(db, cfg),
	}

	var timeout = 20 * 60 // seconds
	if t := os.Getenv("STDEADLOCKTIMEOUT"); len(t) > 0 {
		it, err := strconv.Atoi(t)
		if err == nil {
			timeout = it
		}
	}
	deadlockDetect(&m.fmut, time.Duration(timeout)*time.Second)
	deadlockDetect(&m.smut, time.Duration(timeout)*time.Second)
	deadlockDetect(&m.pmut, time.Duration(timeout)*time.Second)
	return m
}

// StartRW starts read/write processing on the current model. When in
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
	p := &Puller{
		folder:        folder,
		dir:           cfg.Path,
		scanIntv:      time.Duration(cfg.RescanIntervalS) * time.Second,
		model:         m,
		ignorePerms:   cfg.IgnorePerms,
		lenientMtimes: cfg.LenientMtimes,
	}
	m.folderRunners[folder] = p
	m.fmut.Unlock()

	if len(cfg.Versioning.Type) > 0 {
		factory, ok := versioner.Factories[cfg.Versioning.Type]
		if !ok {
			l.Fatalf("Requested versioning type %q that does not exist", cfg.Versioning.Type)
		}
		p.versioner = factory(folder, cfg.Path, cfg.Versioning.Params)
	}

	if cfg.LenientMtimes {
		l.Infof("Folder %q is running with LenientMtimes workaround. Syncing may not work properly.", folder)
	}

	go p.Serve()
}

// StartRO starts read only processing on the current model. When in
// read only mode the model will announce files to the cluster but not
// pull in any external changes.
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
	s := &Scanner{
		folder: folder,
		intv:   time.Duration(cfg.RescanIntervalS) * time.Second,
		model:  m,
	}
	m.folderRunners[folder] = s
	m.fmut.Unlock()

	go s.Serve()
}

type ConnectionInfo struct {
	protocol.Statistics
	Address       string
	ClientVersion string
}

// ConnectionStats returns a map with connection statistics for each connected device.
func (m *Model) ConnectionStats() map[string]ConnectionInfo {
	type remoteAddrer interface {
		RemoteAddr() net.Addr
	}

	m.pmut.RLock()
	m.fmut.RLock()

	var res = make(map[string]ConnectionInfo)
	for device, conn := range m.protoConn {
		ci := ConnectionInfo{
			Statistics:    conn.Statistics(),
			ClientVersion: m.deviceVer[device],
		}
		if nc, ok := m.rawConn[device].(remoteAddrer); ok {
			ci.Address = nc.RemoteAddr().String()
		}

		res[device.String()] = ci
	}

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

// Returns statistics about each device
func (m *Model) DeviceStatistics() map[string]stats.DeviceStatistics {
	var res = make(map[string]stats.DeviceStatistics)
	for id := range m.cfg.Devices() {
		res[id.String()] = m.deviceStatRef(id).GetStatistics()
	}
	return res
}

// Returns the completion status, in percent, for the given device and folder.
func (m *Model) Completion(device protocol.DeviceID, folder string) float64 {
	defer m.leveldbPanicWorkaround()

	var tot int64

	m.fmut.RLock()
	rf, ok := m.folderFiles[folder]
	m.fmut.RUnlock()
	if !ok {
		return 0 // Folder doesn't exist, so we hardly have any of it
	}

	rf.WithGlobalTruncated(func(f protocol.FileIntf) bool {
		if !f.IsDeleted() {
			tot += f.Size()
		}
		return true
	})

	if tot == 0 {
		return 100 // Folder is empty, so we have all of it
	}

	var need int64
	rf.WithNeedTruncated(device, func(f protocol.FileIntf) bool {
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

func sizeOfFile(f protocol.FileIntf) (files, deleted int, bytes int64) {
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
func (m *Model) GlobalSize(folder string) (files, deleted int, bytes int64) {
	defer m.leveldbPanicWorkaround()

	m.fmut.RLock()
	defer m.fmut.RUnlock()
	if rf, ok := m.folderFiles[folder]; ok {
		rf.WithGlobalTruncated(func(f protocol.FileIntf) bool {
			fs, de, by := sizeOfFile(f)
			files += fs
			deleted += de
			bytes += by
			return true
		})
	}
	return
}

// LocalSize returns the number of files, deleted files and total bytes for all
// files in the local folder.
func (m *Model) LocalSize(folder string) (files, deleted int, bytes int64) {
	defer m.leveldbPanicWorkaround()

	m.fmut.RLock()
	defer m.fmut.RUnlock()
	if rf, ok := m.folderFiles[folder]; ok {
		rf.WithHaveTruncated(protocol.LocalDeviceID, func(f protocol.FileIntf) bool {
			if f.IsInvalid() {
				return true
			}
			fs, de, by := sizeOfFile(f)
			files += fs
			deleted += de
			bytes += by
			return true
		})
	}
	return
}

// NeedSize returns the number and total size of currently needed files.
func (m *Model) NeedSize(folder string) (files int, bytes int64) {
	defer m.leveldbPanicWorkaround()

	m.fmut.RLock()
	defer m.fmut.RUnlock()
	if rf, ok := m.folderFiles[folder]; ok {
		rf.WithNeedTruncated(protocol.LocalDeviceID, func(f protocol.FileIntf) bool {
			fs, de, by := sizeOfFile(f)
			files += fs + de
			bytes += by
			return true
		})
	}
	if debug {
		l.Debugf("%v NeedSize(%q): %d %d", m, folder, files, bytes)
	}
	return
}

// NeedFiles returns the list of currently needed files, stopping at maxFiles
// files or maxBlocks blocks. Limits <= 0 are ignored.
func (m *Model) NeedFolderFilesLimited(folder string, maxFiles, maxBlocks int) []protocol.FileInfo {
	defer m.leveldbPanicWorkaround()

	m.fmut.RLock()
	defer m.fmut.RUnlock()
	nblocks := 0
	if rf, ok := m.folderFiles[folder]; ok {
		fs := make([]protocol.FileInfo, 0, maxFiles)
		rf.WithNeed(protocol.LocalDeviceID, func(f protocol.FileIntf) bool {
			fi := f.(protocol.FileInfo)
			fs = append(fs, fi)
			nblocks += len(fi.Blocks)
			return (maxFiles <= 0 || len(fs) < maxFiles) && (maxBlocks <= 0 || nblocks < maxBlocks)
		})
		return fs
	}
	return nil
}

// Index is called when a new device is connected and we receive their full index.
// Implements the protocol.Model interface.
func (m *Model) Index(deviceID protocol.DeviceID, folder string, fs []protocol.FileInfo) {
	if debug {
		l.Debugf("IDX(in): %s %q: %d files", deviceID, folder, len(fs))
	}

	if !m.folderSharedWith(folder, deviceID) {
		events.Default.Log(events.FolderRejected, map[string]string{
			"folder": folder,
			"device": deviceID.String(),
		})
		l.Warnf("Unexpected folder ID %q sent from device %q; ensure that the folder exists and that this device is selected under \"Share With\" in the folder configuration.", folder, deviceID)
		return
	}

	m.fmut.RLock()
	files, ok := m.folderFiles[folder]
	ignores, _ := m.folderIgnores[folder]
	m.fmut.RUnlock()

	if !ok {
		l.Fatalf("Index for nonexistant folder %q", folder)
	}

	for i := 0; i < len(fs); {
		lamport.Default.Tick(fs[i].Version)
		if ignores != nil && ignores.Match(fs[i].Name) {
			if debug {
				l.Debugln("dropping update for ignored", fs[i])
			}
			fs[i] = fs[len(fs)-1]
			fs = fs[:len(fs)-1]
		} else {
			i++
		}
	}

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
func (m *Model) IndexUpdate(deviceID protocol.DeviceID, folder string, fs []protocol.FileInfo) {
	if debug {
		l.Debugf("%v IDXUP(in): %s / %q: %d files", m, deviceID, folder, len(fs))
	}

	if !m.folderSharedWith(folder, deviceID) {
		l.Infof("Update for unexpected folder ID %q sent from device %q; ensure that the folder exists and that this device is selected under \"Share With\" in the folder configuration.", folder, deviceID)
		return
	}

	m.fmut.RLock()
	files, ok := m.folderFiles[folder]
	ignores, _ := m.folderIgnores[folder]
	m.fmut.RUnlock()

	if !ok {
		l.Fatalf("IndexUpdate for nonexistant folder %q", folder)
	}

	for i := 0; i < len(fs); {
		lamport.Default.Tick(fs[i].Version)
		if ignores != nil && ignores.Match(fs[i].Name) {
			if debug {
				l.Debugln("dropping update for ignored", fs[i])
			}
			fs[i] = fs[len(fs)-1]
			fs = fs[:len(fs)-1]
		} else {
			i++
		}
	}

	files.Update(deviceID, fs)

	events.Default.Log(events.RemoteIndexUpdated, map[string]interface{}{
		"device":  deviceID.String(),
		"folder":  folder,
		"items":   len(fs),
		"version": files.LocalVersion(deviceID),
	})
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
	m.pmut.Unlock()

	l.Infof(`Device %s client is "%s %s"`, deviceID, cm.ClientName, cm.ClientVersion)

	if name := cm.GetOption("name"); name != "" {
		l.Infof("Device %s name is %q", deviceID, name)
		device, ok := m.cfg.Devices()[deviceID]
		if ok && device.Name == "" {
			device.Name = name
			m.cfg.SetDevice(device)
		}
	}

	if m.cfg.Devices()[deviceID].Introducer {
		// This device is an introducer. Go through the announced lists of folders
		// and devices and add what we are missing.

		var changed bool
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
						Compression: true,
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

		if changed {
			m.cfg.Save()
		}
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

	conn, ok := m.rawConn[device]
	if ok {
		if conn, ok := conn.(*tls.Conn); ok {
			// If the underlying connection is a *tls.Conn, Close() does more
			// than it says on the tin. Specifically, it sends a TLS alert
			// message, which might block forever if the connection is dead
			// and we don't have a deadline site.
			conn.SetWriteDeadline(time.Now().Add(250 * time.Millisecond))
		}
		conn.Close()
	}
	delete(m.protoConn, device)
	delete(m.rawConn, device)
	delete(m.deviceVer, device)
	m.pmut.Unlock()
}

// Request returns the specified data segment by reading it from local disk.
// Implements the protocol.Model interface.
func (m *Model) Request(deviceID protocol.DeviceID, folder, name string, offset int64, size int) ([]byte, error) {
	// Verify that the requested file exists in the local model.
	m.fmut.RLock()
	r, ok := m.folderFiles[folder]
	m.fmut.RUnlock()

	if !ok {
		l.Warnf("Request from %s for file %s in nonexistent folder %q", deviceID, name, folder)
		return nil, ErrNoSuchFile
	}

	lf := r.Get(protocol.LocalDeviceID, name)
	if protocol.IsInvalid(lf.Flags) || protocol.IsDeleted(lf.Flags) {
		if debug {
			l.Debugf("%v REQ(in): %s: %q / %q o=%d s=%d; invalid: %v", m, deviceID, folder, name, offset, size, lf)
		}
		return nil, ErrInvalid
	}

	if offset > lf.Size() {
		if debug {
			l.Debugf("%v REQ(in; nonexistent): %s: %q o=%d s=%d", m, deviceID, name, offset, size)
		}
		return nil, ErrNoSuchFile
	}

	if debug && deviceID != protocol.LocalDeviceID {
		l.Debugf("%v REQ(in): %s: %q / %q o=%d s=%d", m, deviceID, folder, name, offset, size)
	}
	m.fmut.RLock()
	fn := filepath.Join(m.folderCfgs[folder].Path, name)
	m.fmut.RUnlock()
	fd, err := os.Open(fn) // XXX: Inefficient, should cache fd?
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	buf := make([]byte, size)
	_, err = fd.ReadAt(buf, offset)
	if err != nil {
		return nil, err
	}

	return buf, nil
}

// ReplaceLocal replaces the local folder index with the given list of files.
func (m *Model) ReplaceLocal(folder string, fs []protocol.FileInfo) {
	m.fmut.RLock()
	m.folderFiles[folder].ReplaceWithDelete(protocol.LocalDeviceID, fs)
	m.fmut.RUnlock()
}

func (m *Model) CurrentFolderFile(folder string, file string) protocol.FileInfo {
	m.fmut.RLock()
	f := m.folderFiles[folder].Get(protocol.LocalDeviceID, file)
	m.fmut.RUnlock()
	return f
}

func (m *Model) CurrentGlobalFile(folder string, file string) protocol.FileInfo {
	m.fmut.RLock()
	f := m.folderFiles[folder].GetGlobal(file)
	m.fmut.RUnlock()
	return f
}

type cFiler struct {
	m *Model
	r string
}

// Implements scanner.CurrentFiler
func (cf cFiler) CurrentFile(file string) protocol.FileInfo {
	return cf.m.CurrentFolderFile(cf.r, file)
}

// ConnectedTo returns true if we are connected to the named device.
func (m *Model) ConnectedTo(deviceID protocol.DeviceID) bool {
	m.pmut.RLock()
	_, ok := m.protoConn[deviceID]
	m.pmut.RUnlock()
	if ok {
		m.deviceWasSeen(deviceID)
	}
	return ok
}

func (m *Model) GetIgnores(folder string) ([]string, error) {
	var lines []string

	cfg, ok := m.folderCfgs[folder]
	if !ok {
		return lines, fmt.Errorf("Folder %s does not exist", folder)
	}

	m.fmut.Lock()
	defer m.fmut.Unlock()

	fd, err := os.Open(filepath.Join(cfg.Path, ".stignore"))
	if err != nil {
		if os.IsNotExist(err) {
			return lines, nil
		}
		l.Warnln("Loading .stignore:", err)
		return lines, err
	}
	defer fd.Close()

	scanner := bufio.NewScanner(fd)
	for scanner.Scan() {
		lines = append(lines, strings.TrimSpace(scanner.Text()))
	}

	return lines, nil
}

func (m *Model) SetIgnores(folder string, content []string) error {
	cfg, ok := m.folderCfgs[folder]
	if !ok {
		return fmt.Errorf("Folder %s does not exist", folder)
	}

	fd, err := ioutil.TempFile(cfg.Path, ".syncthing.stignore-"+folder)
	if err != nil {
		l.Warnln("Saving .stignore:", err)
		return err
	}
	defer os.Remove(fd.Name())

	for _, line := range content {
		_, err = fmt.Fprintln(fd, line)
		if err != nil {
			l.Warnln("Saving .stignore:", err)
			return err
		}
	}

	err = fd.Close()
	if err != nil {
		l.Warnln("Saving .stignore:", err)
		return err
	}

	file := filepath.Join(cfg.Path, ".stignore")
	err = osutil.Rename(fd.Name(), file)
	if err != nil {
		l.Warnln("Saving .stignore:", err)
		return err
	}

	return m.ScanFolder(folder)
}

// AddConnection adds a new peer connection to the model. An initial index will
// be sent to the connected peer, thereafter index updates whenever the local
// folder changes.
func (m *Model) AddConnection(rawConn io.Closer, protoConn protocol.Connection) {
	deviceID := protoConn.ID()

	m.pmut.Lock()
	if _, ok := m.protoConn[deviceID]; ok {
		panic("add existing device")
	}
	m.protoConn[deviceID] = protoConn
	if _, ok := m.rawConn[deviceID]; ok {
		panic("add existing device")
	}
	m.rawConn[deviceID] = rawConn

	cm := m.clusterConfig(deviceID)
	protoConn.ClusterConfig(cm)

	m.fmut.RLock()
	for _, folder := range m.deviceFolders[deviceID] {
		fs := m.folderFiles[folder]
		go sendIndexes(protoConn, folder, fs, m.folderIgnores[folder])
	}
	m.fmut.RUnlock()
	m.pmut.Unlock()

	m.deviceWasSeen(deviceID)
}

func (m *Model) deviceStatRef(deviceID protocol.DeviceID) *stats.DeviceStatisticsReference {
	m.fmut.Lock()
	defer m.fmut.Unlock()

	if sr, ok := m.deviceStatRefs[deviceID]; ok {
		return sr
	} else {
		sr = stats.NewDeviceStatisticsReference(m.db, deviceID)
		m.deviceStatRefs[deviceID] = sr
		return sr
	}
}

func (m *Model) deviceWasSeen(deviceID protocol.DeviceID) {
	m.deviceStatRef(deviceID).WasSeen()
}

func sendIndexes(conn protocol.Connection, folder string, fs *files.Set, ignores *ignore.Matcher) {
	deviceID := conn.ID()
	name := conn.Name()
	var err error

	if debug {
		l.Debugf("sendIndexes for %s-%s/%q starting", deviceID, name, folder)
	}

	minLocalVer, err := sendIndexTo(true, 0, conn, folder, fs, ignores)

	for err == nil {
		time.Sleep(5 * time.Second)
		if fs.LocalVersion(protocol.LocalDeviceID) <= minLocalVer {
			continue
		}

		minLocalVer, err = sendIndexTo(false, minLocalVer, conn, folder, fs, ignores)
	}

	if debug {
		l.Debugf("sendIndexes for %s-%s/%q exiting: %v", deviceID, name, folder, err)
	}
}

func sendIndexTo(initial bool, minLocalVer uint64, conn protocol.Connection, folder string, fs *files.Set, ignores *ignore.Matcher) (uint64, error) {
	deviceID := conn.ID()
	name := conn.Name()
	batch := make([]protocol.FileInfo, 0, indexBatchSize)
	currentBatchSize := 0
	maxLocalVer := uint64(0)
	var err error

	fs.WithHave(protocol.LocalDeviceID, func(fi protocol.FileIntf) bool {
		f := fi.(protocol.FileInfo)
		if f.LocalVersion <= minLocalVer {
			return true
		}

		if f.LocalVersion > maxLocalVer {
			maxLocalVer = f.LocalVersion
		}

		if ignores != nil && ignores.Match(f.Name) {
			if debug {
				l.Debugln("not sending update for ignored", f)
			}
			return true
		}

		if len(batch) == indexBatchSize || currentBatchSize > indexTargetSize {
			if initial {
				if err = conn.Index(folder, batch); err != nil {
					return false
				}
				if debug {
					l.Debugf("sendIndexes for %s-%s/%q: %d files (<%d bytes) (initial index)", deviceID, name, folder, len(batch), currentBatchSize)
				}
				initial = false
			} else {
				if err = conn.IndexUpdate(folder, batch); err != nil {
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
		currentBatchSize += indexPerFileSize + len(f.Blocks)*IndexPerBlockSize
		return true
	})

	if initial && err == nil {
		err = conn.Index(folder, batch)
		if debug && err == nil {
			l.Debugf("sendIndexes for %s-%s/%q: %d files (small initial index)", deviceID, name, folder, len(batch))
		}
	} else if len(batch) > 0 && err == nil {
		err = conn.IndexUpdate(folder, batch)
		if debug && err == nil {
			l.Debugf("sendIndexes for %s-%s/%q: %d files (last batch)", deviceID, name, folder, len(batch))
		}
	}

	return maxLocalVer, err
}

func (m *Model) updateLocal(folder string, f protocol.FileInfo) {
	f.LocalVersion = 0
	m.fmut.RLock()
	m.folderFiles[folder].Update(protocol.LocalDeviceID, []protocol.FileInfo{f})
	m.fmut.RUnlock()
	events.Default.Log(events.LocalIndexUpdated, map[string]interface{}{
		"folder":   folder,
		"name":     f.Name,
		"modified": time.Unix(f.Modified, 0),
		"flags":    fmt.Sprintf("0%o", f.Flags),
		"size":     f.Size(),
	})
}

func (m *Model) requestGlobal(deviceID protocol.DeviceID, folder, name string, offset int64, size int, hash []byte) ([]byte, error) {
	m.pmut.RLock()
	nc, ok := m.protoConn[deviceID]
	m.pmut.RUnlock()

	if !ok {
		return nil, fmt.Errorf("requestGlobal: no such device: %s", deviceID)
	}

	if debug {
		l.Debugf("%v REQ(out): %s: %q / %q o=%d s=%d h=%x", m, deviceID, folder, name, offset, size, hash)
	}

	return nc.Request(folder, name, offset, size)
}

func (m *Model) AddFolder(cfg config.FolderConfiguration) {
	if m.started {
		panic("cannot add folder to started model")
	}
	if len(cfg.ID) == 0 {
		panic("cannot add empty folder id")
	}

	m.fmut.Lock()
	m.folderCfgs[cfg.ID] = cfg
	m.folderFiles[cfg.ID] = files.NewSet(cfg.ID, m.db)

	m.folderDevices[cfg.ID] = make([]protocol.DeviceID, len(cfg.Devices))
	for i, device := range cfg.Devices {
		m.folderDevices[cfg.ID][i] = device.DeviceID
		m.deviceFolders[device.DeviceID] = append(m.deviceFolders[device.DeviceID], cfg.ID)
	}

	m.addedFolder = true
	m.fmut.Unlock()
}

func (m *Model) ScanFolders() {
	m.fmut.RLock()
	var folders = make([]string, 0, len(m.folderCfgs))
	for folder := range m.folderCfgs {
		folders = append(folders, folder)
	}
	m.fmut.RUnlock()

	var wg sync.WaitGroup
	wg.Add(len(folders))
	for _, folder := range folders {
		folder := folder
		go func() {
			err := m.ScanFolder(folder)
			if err != nil {
				m.cfg.InvalidateFolder(folder, err.Error())
			}
			wg.Done()
		}()
	}
	wg.Wait()
}

func (m *Model) ScanFolder(folder string) error {
	return m.ScanFolderSub(folder, "")
}

func (m *Model) ScanFolderSub(folder, sub string) error {
	if p := filepath.Clean(filepath.Join(folder, sub)); !strings.HasPrefix(p, folder) {
		return errors.New("invalid subpath")
	}

	m.fmut.RLock()
	fs, ok := m.folderFiles[folder]
	dir := m.folderCfgs[folder].Path

	ignores, _ := ignore.Load(filepath.Join(dir, ".stignore"), m.cfg.Options().CacheIgnoredFiles)
	m.folderIgnores[folder] = ignores

	w := &scanner.Walker{
		Dir:          dir,
		Sub:          sub,
		Matcher:      ignores,
		BlockSize:    protocol.BlockSize,
		TempNamer:    defTempNamer,
		CurrentFiler: cFiler{m, folder},
		IgnorePerms:  m.folderCfgs[folder].IgnorePerms,
	}
	m.fmut.RUnlock()
	if !ok {
		return errors.New("no such folder")
	}

	m.setState(folder, FolderScanning)
	fchan, err := w.Walk()

	if err != nil {
		return err
	}
	batchSize := 100
	batch := make([]protocol.FileInfo, 0, 00)
	for f := range fchan {
		events.Default.Log(events.LocalIndexUpdated, map[string]interface{}{
			"folder":   folder,
			"name":     f.Name,
			"modified": time.Unix(f.Modified, 0),
			"flags":    fmt.Sprintf("0%o", f.Flags),
			"size":     f.Size(),
		})
		if len(batch) == batchSize {
			fs.Update(protocol.LocalDeviceID, batch)
			batch = batch[:0]
		}
		batch = append(batch, f)
	}
	if len(batch) > 0 {
		fs.Update(protocol.LocalDeviceID, batch)
	}

	batch = batch[:0]
	// TODO: We should limit the Have scanning to start at sub
	seenPrefix := false
	fs.WithHaveTruncated(protocol.LocalDeviceID, func(fi protocol.FileIntf) bool {
		f := fi.(protocol.FileInfoTruncated)
		if !strings.HasPrefix(f.Name, sub) {
			// Return true so that we keep iterating, until we get to the part
			// of the tree we are interested in. Then return false so we stop
			// iterating when we've passed the end of the subtree.
			return !seenPrefix
		}

		seenPrefix = true
		if !protocol.IsDeleted(f.Flags) {
			if f.IsInvalid() {
				return true
			}

			if len(batch) == batchSize {
				fs.Update(protocol.LocalDeviceID, batch)
				batch = batch[:0]
			}

			if ignores != nil && ignores.Match(f.Name) {
				// File has been ignored. Set invalid bit.
				l.Debugln("setting invalid bit on ignored", f)
				nf := protocol.FileInfo{
					Name:     f.Name,
					Flags:    f.Flags | protocol.FlagInvalid,
					Modified: f.Modified,
					Version:  f.Version, // The file is still the same, so don't bump version
				}
				events.Default.Log(events.LocalIndexUpdated, map[string]interface{}{
					"folder":   folder,
					"name":     f.Name,
					"modified": time.Unix(f.Modified, 0),
					"flags":    fmt.Sprintf("0%o", f.Flags),
					"size":     f.Size(),
				})
				batch = append(batch, nf)
			} else if _, err := os.Stat(filepath.Join(dir, f.Name)); err != nil && os.IsNotExist(err) {
				// File has been deleted
				nf := protocol.FileInfo{
					Name:     f.Name,
					Flags:    f.Flags | protocol.FlagDeleted,
					Modified: f.Modified,
					Version:  lamport.Default.Tick(f.Version),
				}
				events.Default.Log(events.LocalIndexUpdated, map[string]interface{}{
					"folder":   folder,
					"name":     f.Name,
					"modified": time.Unix(f.Modified, 0),
					"flags":    fmt.Sprintf("0%o", f.Flags),
					"size":     f.Size(),
				})
				batch = append(batch, nf)
			}
		}
		return true
	})
	if len(batch) > 0 {
		fs.Update(protocol.LocalDeviceID, batch)
	}

	m.setState(folder, FolderIdle)
	return nil
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

func (m *Model) setState(folder string, state folderState) {
	m.smut.Lock()
	oldState := m.folderState[folder]
	changed, ok := m.folderStateChanged[folder]
	if state != oldState {
		m.folderState[folder] = state
		m.folderStateChanged[folder] = time.Now()
		eventData := map[string]interface{}{
			"folder": folder,
			"to":     state.String(),
		}
		if ok {
			eventData["duration"] = time.Since(changed).Seconds()
			eventData["from"] = oldState.String()
		}
		events.Default.Log(events.StateChanged, eventData)
	}
	m.smut.Unlock()
}

func (m *Model) State(folder string) (string, time.Time) {
	m.smut.RLock()
	state := m.folderState[folder]
	changed := m.folderStateChanged[folder]
	m.smut.RUnlock()
	return state.String(), changed
}

func (m *Model) Override(folder string) {
	m.fmut.RLock()
	fs := m.folderFiles[folder]
	m.fmut.RUnlock()

	m.setState(folder, FolderScanning)
	batch := make([]protocol.FileInfo, 0, indexBatchSize)
	fs.WithNeed(protocol.LocalDeviceID, func(fi protocol.FileIntf) bool {
		need := fi.(protocol.FileInfo)
		if len(batch) == indexBatchSize {
			fs.Update(protocol.LocalDeviceID, batch)
			batch = batch[:0]
		}

		have := fs.Get(protocol.LocalDeviceID, need.Name)
		if have.Name != need.Name {
			// We are missing the file
			need.Flags |= protocol.FlagDeleted
			need.Blocks = nil
		} else {
			// We have the file, replace with our version
			need = have
		}
		need.Version = lamport.Default.Tick(need.Version)
		need.LocalVersion = 0
		batch = append(batch, need)
		return true
	})
	if len(batch) > 0 {
		fs.Update(protocol.LocalDeviceID, batch)
	}
	m.setState(folder, FolderIdle)
}

// CurrentLocalVersion returns the change version for the given folder.
// This is guaranteed to increment if the contents of the local folder has
// changed.
func (m *Model) CurrentLocalVersion(folder string) uint64 {
	m.fmut.Lock()
	defer m.fmut.Unlock()

	fs, ok := m.folderFiles[folder]
	if !ok {
		// The folder might not exist, since this can be called with a user
		// specified folder name from the REST interface.
		return 0
	}

	return fs.LocalVersion(protocol.LocalDeviceID)
}

// RemoteLocalVersion returns the change version for the given folder, as
// sent by remote peers. This is guaranteed to increment if the contents of
// the remote or global folder has changed.
func (m *Model) RemoteLocalVersion(folder string) uint64 {
	m.fmut.Lock()
	defer m.fmut.Unlock()

	fs, ok := m.folderFiles[folder]
	if !ok {
		panic("bug: LocalVersion called for nonexistent folder " + folder)
	}

	var ver uint64
	for _, n := range m.folderDevices[folder] {
		ver += fs.LocalVersion(n)
	}

	return ver
}

func (m *Model) availability(folder string, file string) []protocol.DeviceID {
	m.fmut.Lock()
	defer m.fmut.Unlock()

	fs, ok := m.folderFiles[folder]
	if !ok {
		return nil
	}

	return fs.Availability(file)
}

func (m *Model) String() string {
	return fmt.Sprintf("model@%p", m)
}

func (m *Model) leveldbPanicWorkaround() {
	// When an inconsistency is detected in leveldb we panic(). This is
	// appropriate because it should never happen, but currently it does for
	// some reason. However it only seems to trigger in the asynchronous full-
	// database scans that happen due to REST and usage-reporting calls. In
	// those places we defer to this workaround to catch the panic instead of
	// taking down syncthing.

	// This is just a band-aid and should be removed as soon as we have found
	// a real root cause.

	if pnc := recover(); pnc != nil {
		if err, ok := pnc.(error); ok && strings.Contains(err.Error(), "leveldb") {
			l.Infoln("recovered:", err)
		} else {
			// Any non-leveldb error is genuine and should continue panicing.
			panic(err)
		}
	}
}
