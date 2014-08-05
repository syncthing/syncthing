// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package model

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/syncthing/syncthing/config"
	"github.com/syncthing/syncthing/events"
	"github.com/syncthing/syncthing/files"
	"github.com/syncthing/syncthing/lamport"
	"github.com/syncthing/syncthing/protocol"
	"github.com/syncthing/syncthing/scanner"
	"github.com/syndtr/goleveldb/leveldb"
)

type repoState int

const (
	RepoIdle repoState = iota
	RepoScanning
	RepoSyncing
	RepoCleaning
)

func (s repoState) String() string {
	switch s {
	case RepoIdle:
		return "idle"
	case RepoScanning:
		return "scanning"
	case RepoCleaning:
		return "cleaning"
	case RepoSyncing:
		return "syncing"
	default:
		return "unknown"
	}
}

// Somewhat arbitrary amount of bytes that we choose to let represent the size
// of an unsynchronized directory entry or a deleted file. We need it to be
// larger than zero so that it's visible that there is some amount of bytes to
// transfer to bring the systems into synchronization.
const zeroEntrySize = 128

// How many files to send in each Index/IndexUpdate message.
const indexBatchSize = 1000

type Model struct {
	indexDir string
	cfg      *config.Configuration
	db       *leveldb.DB

	clientName    string
	clientVersion string

	repoCfgs   map[string]config.RepositoryConfiguration // repo -> cfg
	repoFiles  map[string]*files.Set                     // repo -> files
	repoNodes  map[string][]protocol.NodeID              // repo -> nodeIDs
	nodeRepos  map[protocol.NodeID][]string              // nodeID -> repos
	suppressor map[string]*suppressor                    // repo -> suppressor
	rmut       sync.RWMutex                              // protects the above

	repoState        map[string]repoState // repo -> state
	repoStateChanged map[string]time.Time // repo -> time when state changed
	smut             sync.RWMutex

	protoConn map[protocol.NodeID]protocol.Connection
	rawConn   map[protocol.NodeID]io.Closer
	nodeVer   map[protocol.NodeID]string
	pmut      sync.RWMutex // protects protoConn and rawConn

	sentLocalVer map[protocol.NodeID]map[string]uint64
	slMut        sync.Mutex

	sup suppressor

	addedRepo bool
	started   bool
}

var (
	ErrNoSuchFile = errors.New("no such file")
	ErrInvalid    = errors.New("file is invalid")
)

// NewModel creates and starts a new model. The model starts in read-only mode,
// where it sends index information to connected peers and responds to requests
// for file data without altering the local repository in any way.
func NewModel(indexDir string, cfg *config.Configuration, clientName, clientVersion string, db *leveldb.DB) *Model {
	m := &Model{
		indexDir:         indexDir,
		cfg:              cfg,
		db:               db,
		clientName:       clientName,
		clientVersion:    clientVersion,
		repoCfgs:         make(map[string]config.RepositoryConfiguration),
		repoFiles:        make(map[string]*files.Set),
		repoNodes:        make(map[string][]protocol.NodeID),
		nodeRepos:        make(map[protocol.NodeID][]string),
		repoState:        make(map[string]repoState),
		repoStateChanged: make(map[string]time.Time),
		suppressor:       make(map[string]*suppressor),
		protoConn:        make(map[protocol.NodeID]protocol.Connection),
		rawConn:          make(map[protocol.NodeID]io.Closer),
		nodeVer:          make(map[protocol.NodeID]string),
		sentLocalVer:     make(map[protocol.NodeID]map[string]uint64),
		sup:              suppressor{threshold: int64(cfg.Options.MaxChangeKbps)},
	}

	var timeout = 20 * 60 // seconds
	if t := os.Getenv("STDEADLOCKTIMEOUT"); len(t) > 0 {
		it, err := strconv.Atoi(t)
		if err == nil {
			timeout = it
		}
	}
	deadlockDetect(&m.rmut, time.Duration(timeout)*time.Second)
	deadlockDetect(&m.smut, time.Duration(timeout)*time.Second)
	deadlockDetect(&m.pmut, time.Duration(timeout)*time.Second)
	return m
}

// StartRW starts read/write processing on the current model. When in
// read/write mode the model will attempt to keep in sync with the cluster by
// pulling needed files from peer nodes.
func (m *Model) StartRepoRW(repo string, threads int) {
	m.rmut.RLock()
	defer m.rmut.RUnlock()

	if cfg, ok := m.repoCfgs[repo]; !ok {
		panic("cannot start without repo")
	} else {
		newPuller(cfg, m, threads, m.cfg)
	}
}

// StartRO starts read only processing on the current model. When in
// read only mode the model will announce files to the cluster but not
// pull in any external changes.
func (m *Model) StartRepoRO(repo string) {
	m.StartRepoRW(repo, 0) // zero threads => read only
}

type ConnectionInfo struct {
	protocol.Statistics
	Address       string
	ClientVersion string
}

// ConnectionStats returns a map with connection statistics for each connected node.
func (m *Model) ConnectionStats() map[string]ConnectionInfo {
	type remoteAddrer interface {
		RemoteAddr() net.Addr
	}

	m.pmut.RLock()
	m.rmut.RLock()

	var res = make(map[string]ConnectionInfo)
	for node, conn := range m.protoConn {
		ci := ConnectionInfo{
			Statistics:    conn.Statistics(),
			ClientVersion: m.nodeVer[node],
		}
		if nc, ok := m.rawConn[node].(remoteAddrer); ok {
			ci.Address = nc.RemoteAddr().String()
		}

		res[node.String()] = ci
	}

	m.rmut.RUnlock()
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

// Returns the completion status, in percent, for the given node and repo.
func (m *Model) Completion(node protocol.NodeID, repo string) float64 {
	var tot int64

	m.rmut.RLock()
	rf, ok := m.repoFiles[repo]
	m.rmut.RUnlock()
	if !ok {
		return 0 // Repo doesn't exist, so we hardly have any of it
	}

	rf.WithGlobal(func(f protocol.FileInfo) bool {
		if !protocol.IsDeleted(f.Flags) {
			var size int64
			if protocol.IsDirectory(f.Flags) {
				size = zeroEntrySize
			} else {
				size = f.Size()
			}
			tot += size
		}
		return true
	})

	if tot == 0 {
		return 100 // Repo is empty, so we have all of it
	}

	var need int64
	rf.WithNeed(node, func(f protocol.FileInfo) bool {
		if !protocol.IsDeleted(f.Flags) {
			var size int64
			if protocol.IsDirectory(f.Flags) {
				size = zeroEntrySize
			} else {
				size = f.Size()
			}
			need += size
		}
		return true
	})

	return 100 * (1 - float64(need)/float64(tot))
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

func sizeOfFile(f protocol.FileInfo) (files, deleted int, bytes int64) {
	if !protocol.IsDeleted(f.Flags) {
		files++
		if !protocol.IsDirectory(f.Flags) {
			bytes += f.Size()
		} else {
			bytes += zeroEntrySize
		}
	} else {
		deleted++
		bytes += zeroEntrySize
	}
	return
}

// GlobalSize returns the number of files, deleted files and total bytes for all
// files in the global model.
func (m *Model) GlobalSize(repo string) (files, deleted int, bytes int64) {
	m.rmut.RLock()
	defer m.rmut.RUnlock()
	if rf, ok := m.repoFiles[repo]; ok {
		rf.WithGlobal(func(f protocol.FileInfo) bool {
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
// files in the local repository.
func (m *Model) LocalSize(repo string) (files, deleted int, bytes int64) {
	m.rmut.RLock()
	defer m.rmut.RUnlock()
	if rf, ok := m.repoFiles[repo]; ok {
		rf.WithHave(protocol.LocalNodeID, func(f protocol.FileInfo) bool {
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
func (m *Model) NeedSize(repo string) (files int, bytes int64) {
	m.rmut.RLock()
	defer m.rmut.RUnlock()
	if rf, ok := m.repoFiles[repo]; ok {
		rf.WithNeed(protocol.LocalNodeID, func(f protocol.FileInfo) bool {
			fs, de, by := sizeOfFile(f)
			files += fs + de
			bytes += by
			return true
		})
	}
	return
}

// NeedFiles returns the list of currently needed files
func (m *Model) NeedFilesRepo(repo string) []protocol.FileInfo {
	m.rmut.RLock()
	defer m.rmut.RUnlock()
	if rf, ok := m.repoFiles[repo]; ok {
		fs := make([]protocol.FileInfo, 0, indexBatchSize)
		rf.WithNeed(protocol.LocalNodeID, func(f protocol.FileInfo) bool {
			fs = append(fs, f)
			return len(fs) < indexBatchSize
		})
		return fs
	}
	return nil
}

// Index is called when a new node is connected and we receive their full index.
// Implements the protocol.Model interface.
func (m *Model) Index(nodeID protocol.NodeID, repo string, fs []protocol.FileInfo) {
	if debug {
		l.Debugf("IDX(in): %s %q: %d files", nodeID, repo, len(fs))
	}

	if !m.repoSharedWith(repo, nodeID) {
		l.Warnf("Unexpected repository ID %q sent from node %q; ensure that the repository exists and that this node is selected under \"Share With\" in the repository configuration.", repo, nodeID)
		return
	}

	for i := range fs {
		lamport.Default.Tick(fs[i].Version)
	}

	m.rmut.RLock()
	r, ok := m.repoFiles[repo]
	m.rmut.RUnlock()
	if ok {
		r.Replace(nodeID, fs)
	} else {
		l.Fatalf("Index for nonexistant repo %q", repo)
	}

	events.Default.Log(events.RemoteIndexUpdated, map[string]interface{}{
		"node":    nodeID.String(),
		"repo":    repo,
		"items":   len(fs),
		"version": r.LocalVersion(nodeID),
	})
}

// IndexUpdate is called for incremental updates to connected nodes' indexes.
// Implements the protocol.Model interface.
func (m *Model) IndexUpdate(nodeID protocol.NodeID, repo string, fs []protocol.FileInfo) {
	if debug {
		l.Debugf("IDXUP(in): %s / %q: %d files", nodeID, repo, len(fs))
	}

	if !m.repoSharedWith(repo, nodeID) {
		l.Infof("Update for unexpected repository ID %q sent from node %q; ensure that the repository exists and that this node is selected under \"Share With\" in the repository configuration.", repo, nodeID)
		return
	}

	for i := range fs {
		lamport.Default.Tick(fs[i].Version)
	}

	m.rmut.RLock()
	r, ok := m.repoFiles[repo]
	m.rmut.RUnlock()
	if ok {
		r.Update(nodeID, fs)
	} else {
		l.Fatalf("IndexUpdate for nonexistant repo %q", repo)
	}

	events.Default.Log(events.RemoteIndexUpdated, map[string]interface{}{
		"node":    nodeID.String(),
		"repo":    repo,
		"items":   len(fs),
		"version": r.LocalVersion(nodeID),
	})
}

func (m *Model) repoSharedWith(repo string, nodeID protocol.NodeID) bool {
	m.rmut.RLock()
	defer m.rmut.RUnlock()
	for _, nrepo := range m.nodeRepos[nodeID] {
		if nrepo == repo {
			return true
		}
	}
	return false
}

func (m *Model) ClusterConfig(nodeID protocol.NodeID, config protocol.ClusterConfigMessage) {
	m.pmut.Lock()
	if config.ClientName == "syncthing" {
		m.nodeVer[nodeID] = config.ClientVersion
	} else {
		m.nodeVer[nodeID] = config.ClientName + " " + config.ClientVersion
	}
	m.pmut.Unlock()

	l.Infof(`Node %s client is "%s %s"`, nodeID, config.ClientName, config.ClientVersion)
}

// Close removes the peer from the model and closes the underlying connection if possible.
// Implements the protocol.Model interface.
func (m *Model) Close(node protocol.NodeID, err error) {
	l.Infof("Connection to %s closed: %v", node, err)
	events.Default.Log(events.NodeDisconnected, map[string]string{
		"id":    node.String(),
		"error": err.Error(),
	})

	m.pmut.Lock()
	m.rmut.RLock()
	for _, repo := range m.nodeRepos[node] {
		m.repoFiles[repo].Replace(node, nil)
	}
	m.rmut.RUnlock()

	conn, ok := m.rawConn[node]
	if ok {
		conn.Close()
	}
	delete(m.protoConn, node)
	delete(m.rawConn, node)
	delete(m.nodeVer, node)
	m.pmut.Unlock()
}

// Request returns the specified data segment by reading it from local disk.
// Implements the protocol.Model interface.
func (m *Model) Request(nodeID protocol.NodeID, repo, name string, offset int64, size int) ([]byte, error) {
	// Verify that the requested file exists in the local model.
	m.rmut.RLock()
	r, ok := m.repoFiles[repo]
	m.rmut.RUnlock()

	if !ok {
		l.Warnf("Request from %s for file %s in nonexistent repo %q", nodeID, name, repo)
		return nil, ErrNoSuchFile
	}

	lf := r.Get(protocol.LocalNodeID, name)
	if protocol.IsInvalid(lf.Flags) || protocol.IsDeleted(lf.Flags) {
		if debug {
			l.Debugf("REQ(in): %s: %q / %q o=%d s=%d; invalid: %v", nodeID, repo, name, offset, size, lf)
		}
		return nil, ErrInvalid
	}

	if offset > lf.Size() {
		if debug {
			l.Debugf("REQ(in; nonexistent): %s: %q o=%d s=%d", nodeID, name, offset, size)
		}
		return nil, ErrNoSuchFile
	}

	if debug && nodeID != protocol.LocalNodeID {
		l.Debugf("REQ(in): %s: %q / %q o=%d s=%d", nodeID, repo, name, offset, size)
	}
	m.rmut.RLock()
	fn := filepath.Join(m.repoCfgs[repo].Directory, name)
	m.rmut.RUnlock()
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

// ReplaceLocal replaces the local repository index with the given list of files.
func (m *Model) ReplaceLocal(repo string, fs []protocol.FileInfo) {
	m.rmut.RLock()
	m.repoFiles[repo].ReplaceWithDelete(protocol.LocalNodeID, fs)
	m.rmut.RUnlock()
}

func (m *Model) CurrentRepoFile(repo string, file string) protocol.FileInfo {
	m.rmut.RLock()
	f := m.repoFiles[repo].Get(protocol.LocalNodeID, file)
	m.rmut.RUnlock()
	return f
}

func (m *Model) CurrentGlobalFile(repo string, file string) protocol.FileInfo {
	m.rmut.RLock()
	f := m.repoFiles[repo].GetGlobal(file)
	m.rmut.RUnlock()
	return f
}

type cFiler struct {
	m *Model
	r string
}

// Implements scanner.CurrentFiler
func (cf cFiler) CurrentFile(file string) protocol.FileInfo {
	return cf.m.CurrentRepoFile(cf.r, file)
}

// ConnectedTo returns true if we are connected to the named node.
func (m *Model) ConnectedTo(nodeID protocol.NodeID) bool {
	m.pmut.RLock()
	_, ok := m.protoConn[nodeID]
	m.pmut.RUnlock()
	return ok
}

// AddConnection adds a new peer connection to the model. An initial index will
// be sent to the connected peer, thereafter index updates whenever the local
// repository changes.
func (m *Model) AddConnection(rawConn io.Closer, protoConn protocol.Connection) {
	nodeID := protoConn.ID()

	m.pmut.Lock()
	if _, ok := m.protoConn[nodeID]; ok {
		panic("add existing node")
	}
	m.protoConn[nodeID] = protoConn
	if _, ok := m.rawConn[nodeID]; ok {
		panic("add existing node")
	}
	m.rawConn[nodeID] = rawConn

	cm := m.clusterConfig(nodeID)
	protoConn.ClusterConfig(cm)

	m.rmut.RLock()
	for _, repo := range m.nodeRepos[nodeID] {
		fs := m.repoFiles[repo]
		go sendIndexes(protoConn, repo, fs)
	}
	m.rmut.RUnlock()
	m.pmut.Unlock()
}

func sendIndexes(conn protocol.Connection, repo string, fs *files.Set) {
	nodeID := conn.ID()
	name := conn.Name()
	var err error

	if debug {
		l.Debugf("sendIndexes for %s-%s@/%q starting", nodeID, name, repo)
	}

	defer func() {
		if debug {
			l.Debugf("sendIndexes for %s-%s@/%q exiting: %v", nodeID, name, repo, err)
		}
	}()

	minLocalVer, err := sendIndexTo(true, 0, conn, repo, fs)

	for err == nil {
		time.Sleep(5 * time.Second)
		if fs.LocalVersion(protocol.LocalNodeID) <= minLocalVer {
			continue
		}

		minLocalVer, err = sendIndexTo(false, minLocalVer, conn, repo, fs)
	}
}

func sendIndexTo(initial bool, minLocalVer uint64, conn protocol.Connection, repo string, fs *files.Set) (uint64, error) {
	nodeID := conn.ID()
	name := conn.Name()
	batch := make([]protocol.FileInfo, 0, indexBatchSize)
	maxLocalVer := uint64(0)
	var err error

	fs.WithHave(protocol.LocalNodeID, func(f protocol.FileInfo) bool {
		if f.LocalVersion <= minLocalVer {
			return true
		}

		if f.LocalVersion > maxLocalVer {
			maxLocalVer = f.LocalVersion
		}

		if len(batch) == indexBatchSize {
			if initial {
				if err = conn.Index(repo, batch); err != nil {
					return false
				}
				if debug {
					l.Debugf("sendIndexes for %s-%s/%q: %d files (initial index)", nodeID, name, repo, len(batch))
				}
				initial = false
			} else {
				if err = conn.IndexUpdate(repo, batch); err != nil {
					return false
				}
				if debug {
					l.Debugf("sendIndexes for %s-%s/%q: %d files (batched update)", nodeID, name, repo, len(batch))
				}
			}

			batch = make([]protocol.FileInfo, 0, indexBatchSize)
		}

		batch = append(batch, f)
		return true
	})

	if initial && err == nil {
		err = conn.Index(repo, batch)
		if debug && err == nil {
			l.Debugf("sendIndexes for %s-%s/%q: %d files (small initial index)", nodeID, name, repo, len(batch))
		}
	} else if len(batch) > 0 && err == nil {
		err = conn.IndexUpdate(repo, batch)
		if debug && err == nil {
			l.Debugf("sendIndexes for %s-%s/%q: %d files (last batch)", nodeID, name, repo, len(batch))
		}
	}

	return maxLocalVer, err
}

func (m *Model) updateLocal(repo string, f protocol.FileInfo) {
	f.LocalVersion = 0
	m.rmut.RLock()
	m.repoFiles[repo].Update(protocol.LocalNodeID, []protocol.FileInfo{f})
	m.rmut.RUnlock()
	events.Default.Log(events.LocalIndexUpdated, map[string]interface{}{
		"repo":     repo,
		"name":     f.Name,
		"modified": time.Unix(f.Modified, 0),
		"flags":    fmt.Sprintf("0%o", f.Flags),
		"size":     f.Size(),
	})
}

func (m *Model) requestGlobal(nodeID protocol.NodeID, repo, name string, offset int64, size int, hash []byte) ([]byte, error) {
	m.pmut.RLock()
	nc, ok := m.protoConn[nodeID]
	m.pmut.RUnlock()

	if !ok {
		return nil, fmt.Errorf("requestGlobal: no such node: %s", nodeID)
	}

	if debug {
		l.Debugf("REQ(out): %s: %q / %q o=%d s=%d h=%x", nodeID, repo, name, offset, size, hash)
	}

	return nc.Request(repo, name, offset, size)
}

func (m *Model) AddRepo(cfg config.RepositoryConfiguration) {
	if m.started {
		panic("cannot add repo to started model")
	}
	if len(cfg.ID) == 0 {
		panic("cannot add empty repo id")
	}

	m.rmut.Lock()
	m.repoCfgs[cfg.ID] = cfg
	m.repoFiles[cfg.ID] = files.NewSet(cfg.ID, m.db)
	m.suppressor[cfg.ID] = &suppressor{threshold: int64(m.cfg.Options.MaxChangeKbps)}

	m.repoNodes[cfg.ID] = make([]protocol.NodeID, len(cfg.Nodes))
	for i, node := range cfg.Nodes {
		m.repoNodes[cfg.ID][i] = node.NodeID
		m.nodeRepos[node.NodeID] = append(m.nodeRepos[node.NodeID], cfg.ID)
	}

	m.addedRepo = true
	m.rmut.Unlock()
}

func (m *Model) ScanRepos() {
	m.rmut.RLock()
	var repos = make([]string, 0, len(m.repoCfgs))
	for repo := range m.repoCfgs {
		repos = append(repos, repo)
	}
	m.rmut.RUnlock()

	var wg sync.WaitGroup
	wg.Add(len(repos))
	for _, repo := range repos {
		repo := repo
		go func() {
			err := m.ScanRepo(repo)
			if err != nil {
				invalidateRepo(m.cfg, repo, err)
			}
			wg.Done()
		}()
	}
	wg.Wait()
}

func (m *Model) CleanRepos() {
	m.rmut.RLock()
	var dirs = make([]string, 0, len(m.repoCfgs))
	for _, cfg := range m.repoCfgs {
		dirs = append(dirs, cfg.Directory)
	}
	m.rmut.RUnlock()

	var wg sync.WaitGroup
	wg.Add(len(dirs))
	for _, dir := range dirs {
		w := &scanner.Walker{
			Dir:       dir,
			TempNamer: defTempNamer,
		}
		go func() {
			w.CleanTempFiles()
			wg.Done()
		}()
	}
	wg.Wait()
}

func (m *Model) ScanRepo(repo string) error {
	m.rmut.RLock()
	fs := m.repoFiles[repo]
	dir := m.repoCfgs[repo].Directory

	w := &scanner.Walker{
		Dir:          dir,
		IgnoreFile:   ".stignore",
		BlockSize:    scanner.StandardBlockSize,
		TempNamer:    defTempNamer,
		Suppressor:   m.suppressor[repo],
		CurrentFiler: cFiler{m, repo},
		IgnorePerms:  m.repoCfgs[repo].IgnorePerms,
	}
	m.rmut.RUnlock()

	m.setState(repo, RepoScanning)
	fchan, _, err := w.Walk()

	if err != nil {
		return err
	}
	batchSize := 100
	batch := make([]protocol.FileInfo, 0, 00)
	for f := range fchan {
		events.Default.Log(events.LocalIndexUpdated, map[string]interface{}{
			"repo":     repo,
			"name":     f.Name,
			"modified": time.Unix(f.Modified, 0),
			"flags":    fmt.Sprintf("0%o", f.Flags),
			"size":     f.Size(),
		})
		if len(batch) == batchSize {
			fs.Update(protocol.LocalNodeID, batch)
			batch = batch[:0]
		}
		batch = append(batch, f)
	}
	if len(batch) > 0 {
		fs.Update(protocol.LocalNodeID, batch)
	}

	batch = batch[:0]
	fs.WithHave(protocol.LocalNodeID, func(f protocol.FileInfo) bool {
		if !protocol.IsDeleted(f.Flags) {
			if len(batch) == batchSize {
				fs.Update(protocol.LocalNodeID, batch)
				batch = batch[:0]
			}
			if _, err := os.Stat(filepath.Join(dir, f.Name)); err != nil && os.IsNotExist(err) {
				// File has been deleted
				f.Blocks = nil
				f.Flags |= protocol.FlagDeleted
				f.Version = lamport.Default.Tick(f.Version)
				f.LocalVersion = 0
				events.Default.Log(events.LocalIndexUpdated, map[string]interface{}{
					"repo":     repo,
					"name":     f.Name,
					"modified": time.Unix(f.Modified, 0),
					"flags":    fmt.Sprintf("0%o", f.Flags),
					"size":     f.Size(),
				})
				batch = append(batch, f)
			}
		}
		return true
	})
	if len(batch) > 0 {
		fs.Update(protocol.LocalNodeID, batch)
	}

	m.setState(repo, RepoIdle)
	return nil
}

// clusterConfig returns a ClusterConfigMessage that is correct for the given peer node
func (m *Model) clusterConfig(node protocol.NodeID) protocol.ClusterConfigMessage {
	cm := protocol.ClusterConfigMessage{
		ClientName:    m.clientName,
		ClientVersion: m.clientVersion,
	}

	m.rmut.RLock()
	for _, repo := range m.nodeRepos[node] {
		cr := protocol.Repository{
			ID: repo,
		}
		for _, node := range m.repoNodes[repo] {
			// TODO: Set read only bit when relevant
			cr.Nodes = append(cr.Nodes, protocol.Node{
				ID:    node[:],
				Flags: protocol.FlagShareTrusted,
			})
		}
		cm.Repositories = append(cm.Repositories, cr)
	}
	m.rmut.RUnlock()

	return cm
}

func (m *Model) setState(repo string, state repoState) {
	m.smut.Lock()
	oldState := m.repoState[repo]
	changed, ok := m.repoStateChanged[repo]
	if state != oldState {
		m.repoState[repo] = state
		m.repoStateChanged[repo] = time.Now()
		eventData := map[string]interface{}{
			"repo": repo,
			"to":   state.String(),
		}
		if ok {
			eventData["duration"] = time.Since(changed).Seconds()
			eventData["from"] = oldState.String()
		}
		events.Default.Log(events.StateChanged, eventData)
	}
	m.smut.Unlock()
}

func (m *Model) State(repo string) (string, time.Time) {
	m.smut.RLock()
	state := m.repoState[repo]
	changed := m.repoStateChanged[repo]
	m.smut.RUnlock()
	return state.String(), changed
}

func (m *Model) Override(repo string) {
	m.rmut.RLock()
	fs := m.repoFiles[repo]
	m.rmut.RUnlock()

	batch := make([]protocol.FileInfo, 0, indexBatchSize)
	fs.WithNeed(protocol.LocalNodeID, func(need protocol.FileInfo) bool {
		if len(batch) == indexBatchSize {
			fs.Update(protocol.LocalNodeID, batch)
			batch = batch[:0]
		}

		have := fs.Get(protocol.LocalNodeID, need.Name)
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
		fs.Update(protocol.LocalNodeID, batch)
	}
}

// Version returns the change version for the given repository. This is
// guaranteed to increment if the contents of the local or global repository
// has changed.
func (m *Model) LocalVersion(repo string) uint64 {
	m.rmut.Lock()
	defer m.rmut.Unlock()

	fs, ok := m.repoFiles[repo]
	if !ok {
		return 0
	}

	ver := fs.LocalVersion(protocol.LocalNodeID)
	for _, n := range m.repoNodes[repo] {
		ver += fs.LocalVersion(n)
	}

	return ver
}
