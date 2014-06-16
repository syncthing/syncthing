// Copyright (C) 2014 Jakob Borg and other contributors. All rights reserved.
// Use of this source code is governed by an MIT-style license that can be
// found in the LICENSE file.

package model

import (
	"compress/gzip"
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/calmh/syncthing/buffers"
	"github.com/calmh/syncthing/cid"
	"github.com/calmh/syncthing/config"
	"github.com/calmh/syncthing/files"
	"github.com/calmh/syncthing/lamport"
	"github.com/calmh/syncthing/osutil"
	"github.com/calmh/syncthing/protocol"
	"github.com/calmh/syncthing/scanner"
)

type repoState int

const (
	RepoIdle repoState = iota
	RepoScanning
	RepoSyncing
	RepoCleaning
)

// Somewhat arbitrary amount of bytes that we choose to let represent the size
// of an unsynchronized directory entry or a deleted file. We need it to be
// larger than zero so that it's visible that there is some amount of bytes to
// transfer to bring the systems into synchronization.
const zeroEntrySize = 128

type Model struct {
	indexDir string
	cfg      *config.Configuration

	clientName    string
	clientVersion string

	repoCfgs   map[string]config.RepositoryConfiguration // repo -> cfg
	repoFiles  map[string]*files.Set                     // repo -> files
	repoNodes  map[string][]string                       // repo -> nodeIDs
	nodeRepos  map[string][]string                       // nodeID -> repos
	suppressor map[string]*suppressor                    // repo -> suppressor
	rmut       sync.RWMutex                              // protects the above

	repoState map[string]repoState // repo -> state
	smut      sync.RWMutex

	cm *cid.Map

	protoConn map[string]protocol.Connection
	rawConn   map[string]io.Closer
	nodeVer   map[string]string
	pmut      sync.RWMutex // protects protoConn and rawConn

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
func NewModel(indexDir string, cfg *config.Configuration, clientName, clientVersion string) *Model {
	m := &Model{
		indexDir:      indexDir,
		cfg:           cfg,
		clientName:    clientName,
		clientVersion: clientVersion,
		repoCfgs:      make(map[string]config.RepositoryConfiguration),
		repoFiles:     make(map[string]*files.Set),
		repoNodes:     make(map[string][]string),
		nodeRepos:     make(map[string][]string),
		repoState:     make(map[string]repoState),
		suppressor:    make(map[string]*suppressor),
		cm:            cid.NewMap(),
		protoConn:     make(map[string]protocol.Connection),
		rawConn:       make(map[string]io.Closer),
		nodeVer:       make(map[string]string),
		sup:           suppressor{threshold: int64(cfg.Options.MaxChangeKbps)},
	}

	go m.broadcastIndexLoop()
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
	Completion    int
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

		var tot int64
		var have int64

		for _, repo := range m.nodeRepos[node] {
			for _, f := range m.repoFiles[repo].Global() {
				if !protocol.IsDeleted(f.Flags) {
					size := f.Size
					if protocol.IsDirectory(f.Flags) {
						size = zeroEntrySize
					}
					tot += size
					have += size
				}
			}

			for _, f := range m.repoFiles[repo].Need(m.cm.Get(node)) {
				if !protocol.IsDeleted(f.Flags) {
					size := f.Size
					if protocol.IsDirectory(f.Flags) {
						size = zeroEntrySize
					}
					have -= size
				}
			}
		}

		ci.Completion = 100
		if tot != 0 {
			ci.Completion = int(100 * have / tot)
		}

		res[node] = ci
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

func sizeOf(fs []scanner.File) (files, deleted int, bytes int64) {
	for _, f := range fs {
		if !protocol.IsDeleted(f.Flags) {
			files++
			if !protocol.IsDirectory(f.Flags) {
				bytes += f.Size
			} else {
				bytes += zeroEntrySize
			}
		} else {
			deleted++
			bytes += zeroEntrySize
		}
	}
	return
}

// GlobalSize returns the number of files, deleted files and total bytes for all
// files in the global model.
func (m *Model) GlobalSize(repo string) (files, deleted int, bytes int64) {
	m.rmut.RLock()
	defer m.rmut.RUnlock()
	if rf, ok := m.repoFiles[repo]; ok {
		return sizeOf(rf.Global())
	}
	return 0, 0, 0
}

// LocalSize returns the number of files, deleted files and total bytes for all
// files in the local repository.
func (m *Model) LocalSize(repo string) (files, deleted int, bytes int64) {
	m.rmut.RLock()
	defer m.rmut.RUnlock()
	if rf, ok := m.repoFiles[repo]; ok {
		return sizeOf(rf.Have(cid.LocalID))
	}
	return 0, 0, 0
}

// NeedSize returns the number and total size of currently needed files.
func (m *Model) NeedSize(repo string) (files int, bytes int64) {
	f, d, b := sizeOf(m.NeedFilesRepo(repo))
	return f + d, b
}

// NeedFiles returns the list of currently needed files and the total size.
func (m *Model) NeedFilesRepo(repo string) []scanner.File {
	m.rmut.RLock()
	defer m.rmut.RUnlock()
	if rf, ok := m.repoFiles[repo]; ok {
		f := rf.Need(cid.LocalID)
		if r := m.repoCfgs[repo].FileRanker(); r != nil {
			files.SortBy(r).Sort(f)
		}
		return f
	}
	return nil
}

// Index is called when a new node is connected and we receive their full index.
// Implements the protocol.Model interface.
func (m *Model) Index(nodeID string, repo string, fs []protocol.FileInfo) {
	if debug {
		l.Debugf("IDX(in): %s %q: %d files", nodeID, repo, len(fs))
	}

	if !m.repoSharedWith(repo, nodeID) {
		l.Warnf("Unexpected repository ID %q sent from node %q; ensure that the repository exists and that this node is selected under \"Share With\" in the repository configuration.", repo, nodeID)
		return
	}

	var files = make([]scanner.File, len(fs))
	for i := range fs {
		f := fs[i]
		lamport.Default.Tick(f.Version)
		if debug {
			var flagComment string
			if protocol.IsDeleted(f.Flags) {
				flagComment = " (deleted)"
			}
			l.Debugf("IDX(in): %s %q/%q m=%d f=%o%s v=%d (%d blocks)", nodeID, repo, f.Name, f.Modified, f.Flags, flagComment, f.Version, len(f.Blocks))
		}
		files[i] = fileFromFileInfo(f)
	}

	id := m.cm.Get(nodeID)
	m.rmut.RLock()
	if r, ok := m.repoFiles[repo]; ok {
		r.Replace(id, files)
	} else {
		l.Fatalf("Index for nonexistant repo %q", repo)
	}
	m.rmut.RUnlock()
}

// IndexUpdate is called for incremental updates to connected nodes' indexes.
// Implements the protocol.Model interface.
func (m *Model) IndexUpdate(nodeID string, repo string, fs []protocol.FileInfo) {
	if debug {
		l.Debugf("IDXUP(in): %s / %q: %d files", nodeID, repo, len(fs))
	}

	if !m.repoSharedWith(repo, nodeID) {
		l.Warnf("Unexpected repository ID %q sent from node %q; ensure that the repository exists and that this node is selected under \"Share With\" in the repository configuration.", repo, nodeID)
		return
	}

	var files = make([]scanner.File, len(fs))
	for i := range fs {
		f := fs[i]
		lamport.Default.Tick(f.Version)
		if debug {
			var flagComment string
			if protocol.IsDeleted(f.Flags) {
				flagComment = " (deleted)"
			}
			l.Debugf("IDXUP(in): %s %q/%q m=%d f=%o%s v=%d (%d blocks)", nodeID, repo, f.Name, f.Modified, f.Flags, flagComment, f.Version, len(f.Blocks))
		}
		files[i] = fileFromFileInfo(f)
	}

	id := m.cm.Get(nodeID)
	m.rmut.RLock()
	if r, ok := m.repoFiles[repo]; ok {
		r.Update(id, files)
	} else {
		l.Fatalf("IndexUpdate for nonexistant repo %q", repo)
	}
	m.rmut.RUnlock()
}

func (m *Model) repoSharedWith(repo, nodeID string) bool {
	m.rmut.RLock()
	defer m.rmut.RUnlock()
	for _, nrepo := range m.nodeRepos[nodeID] {
		if nrepo == repo {
			return true
		}
	}
	return false
}

func (m *Model) ClusterConfig(nodeID string, config protocol.ClusterConfigMessage) {
	compErr := compareClusterConfig(m.clusterConfig(nodeID), config)
	if debug {
		l.Debugf("ClusterConfig: %s: %#v", nodeID, config)
		l.Debugf("  ... compare: %s: %v", nodeID, compErr)
	}

	if compErr != nil {
		l.Warnf("%s: %v", nodeID, compErr)
		m.Close(nodeID, compErr)
	}

	m.pmut.Lock()
	if config.ClientName == "syncthing" {
		m.nodeVer[nodeID] = config.ClientVersion
	} else {
		m.nodeVer[nodeID] = config.ClientName + " " + config.ClientVersion
	}
	m.pmut.Unlock()
}

// Close removes the peer from the model and closes the underlying connection if possible.
// Implements the protocol.Model interface.
func (m *Model) Close(node string, err error) {
	if debug {
		l.Debugf("%s: %v", node, err)
	}

	if err != io.EOF {
		l.Warnf("Connection to %s closed: %v", node, err)
	} else if _, ok := err.(ClusterConfigMismatch); ok {
		l.Warnf("Connection to %s closed: %v", node, err)
	}

	cid := m.cm.Get(node)
	m.rmut.RLock()
	for _, repo := range m.nodeRepos[node] {
		m.repoFiles[repo].Replace(cid, nil)
	}
	m.rmut.RUnlock()
	m.cm.Clear(node)

	m.pmut.Lock()
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
func (m *Model) Request(nodeID, repo, name string, offset int64, size int) ([]byte, error) {
	// Verify that the requested file exists in the local model.
	m.rmut.RLock()
	r, ok := m.repoFiles[repo]
	m.rmut.RUnlock()

	if !ok {
		l.Warnf("Request from %s for file %s in nonexistent repo %q", nodeID, name, repo)
		return nil, ErrNoSuchFile
	}

	lf := r.Get(cid.LocalID, name)
	if lf.Suppressed || protocol.IsDeleted(lf.Flags) {
		if debug {
			l.Debugf("REQ(in): %s: %q / %q o=%d s=%d; invalid: %v", nodeID, repo, name, offset, size, lf)
		}
		return nil, ErrInvalid
	}

	if offset > lf.Size {
		if debug {
			l.Debugf("REQ(in; nonexistent): %s: %q o=%d s=%d", nodeID, name, offset, size)
		}
		return nil, ErrNoSuchFile
	}

	if debug && nodeID != "<local>" {
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

	buf := buffers.Get(int(size))
	_, err = fd.ReadAt(buf, offset)
	if err != nil {
		return nil, err
	}

	return buf, nil
}

// ReplaceLocal replaces the local repository index with the given list of files.
func (m *Model) ReplaceLocal(repo string, fs []scanner.File) {
	m.rmut.RLock()
	m.repoFiles[repo].ReplaceWithDelete(cid.LocalID, fs)
	m.rmut.RUnlock()
}

func (m *Model) SeedLocal(repo string, fs []protocol.FileInfo) {
	var sfs = make([]scanner.File, len(fs))
	for i := 0; i < len(fs); i++ {
		lamport.Default.Tick(fs[i].Version)
		sfs[i] = fileFromFileInfo(fs[i])
		sfs[i].Suppressed = false // we might have saved an index with files that were suppressed; the should not be on startup
	}

	m.rmut.RLock()
	m.repoFiles[repo].Replace(cid.LocalID, sfs)
	m.rmut.RUnlock()
}

func (m *Model) CurrentRepoFile(repo string, file string) scanner.File {
	m.rmut.RLock()
	f := m.repoFiles[repo].Get(cid.LocalID, file)
	m.rmut.RUnlock()
	return f
}

func (m *Model) CurrentGlobalFile(repo string, file string) scanner.File {
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
func (cf cFiler) CurrentFile(file string) scanner.File {
	return cf.m.CurrentRepoFile(cf.r, file)
}

// ConnectedTo returns true if we are connected to the named node.
func (m *Model) ConnectedTo(nodeID string) bool {
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
	m.pmut.Unlock()

	cm := m.clusterConfig(nodeID)
	protoConn.ClusterConfig(cm)

	var idxToSend = make(map[string][]protocol.FileInfo)

	m.rmut.RLock()
	for _, repo := range m.nodeRepos[nodeID] {
		idxToSend[repo] = m.protocolIndex(repo)
	}
	m.rmut.RUnlock()

	go func() {
		for repo, idx := range idxToSend {
			if debug {
				l.Debugf("IDX(out/initial): %s: %q: %d files", nodeID, repo, len(idx))
			}
			protoConn.Index(repo, idx)
		}
	}()
}

// protocolIndex returns the current local index in protocol data types.
func (m *Model) protocolIndex(repo string) []protocol.FileInfo {
	var index []protocol.FileInfo

	fs := m.repoFiles[repo].Have(cid.LocalID)

	for _, f := range fs {
		mf := fileInfoFromFile(f)
		if debug {
			var flagComment string
			if protocol.IsDeleted(mf.Flags) {
				flagComment = " (deleted)"
			}
			l.Debugf("IDX(out): %q/%q m=%d f=%o%s v=%d (%d blocks)", repo, mf.Name, mf.Modified, mf.Flags, flagComment, mf.Version, len(mf.Blocks))
		}
		index = append(index, mf)
	}

	return index
}

func (m *Model) updateLocal(repo string, f scanner.File) {
	m.rmut.RLock()
	m.repoFiles[repo].Update(cid.LocalID, []scanner.File{f})
	m.rmut.RUnlock()
}

func (m *Model) requestGlobal(nodeID, repo, name string, offset int64, size int, hash []byte) ([]byte, error) {
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

func (m *Model) broadcastIndexLoop() {
	var lastChange = map[string]uint64{}
	for {
		time.Sleep(5 * time.Second)

		m.pmut.RLock()
		m.rmut.RLock()

		var indexWg sync.WaitGroup
		for repo, fs := range m.repoFiles {
			repo := repo

			c := fs.Changes(cid.LocalID)
			if c == lastChange[repo] {
				continue
			}
			lastChange[repo] = c

			idx := m.protocolIndex(repo)
			indexWg.Add(1)
			go func() {
				err := m.saveIndex(repo, m.indexDir, idx)
				if err != nil {
					l.Infof("Saving index for %q: %v", repo, err)
				}
				indexWg.Done()
			}()

			for _, nodeID := range m.repoNodes[repo] {
				nodeID := nodeID
				if conn, ok := m.protoConn[nodeID]; ok {
					indexWg.Add(1)
					if debug {
						l.Debugf("IDX(out/loop): %s: %d files", nodeID, len(idx))
					}
					go func() {
						conn.Index(repo, idx)
						indexWg.Done()
					}()
				}
			}
		}

		m.rmut.RUnlock()
		m.pmut.RUnlock()

		indexWg.Wait()
	}
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
	m.repoFiles[cfg.ID] = files.NewSet()
	m.suppressor[cfg.ID] = &suppressor{threshold: int64(m.cfg.Options.MaxChangeKbps)}

	m.repoNodes[cfg.ID] = make([]string, len(cfg.Nodes))
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
	w := &scanner.Walker{
		Dir:          m.repoCfgs[repo].Directory,
		IgnoreFile:   ".stignore",
		BlockSize:    scanner.StandardBlockSize,
		TempNamer:    defTempNamer,
		Suppressor:   m.suppressor[repo],
		CurrentFiler: cFiler{m, repo},
		IgnorePerms:  m.repoCfgs[repo].IgnorePerms,
	}
	m.rmut.RUnlock()
	m.setState(repo, RepoScanning)
	fs, _, err := w.Walk()
	if err != nil {
		return err
	}
	m.ReplaceLocal(repo, fs)
	m.setState(repo, RepoIdle)
	return nil
}

func (m *Model) SaveIndexes(dir string) {
	m.rmut.RLock()
	for repo := range m.repoCfgs {
		fs := m.protocolIndex(repo)
		err := m.saveIndex(repo, dir, fs)
		if err != nil {
			l.Infof("Saving index for %q: %v", repo, err)
		}
	}
	m.rmut.RUnlock()
}

func (m *Model) LoadIndexes(dir string) {
	m.rmut.RLock()
	for repo := range m.repoCfgs {
		fs := m.loadIndex(repo, dir)
		m.SeedLocal(repo, fs)
	}
	m.rmut.RUnlock()
}

func (m *Model) saveIndex(repo string, dir string, fs []protocol.FileInfo) error {
	id := fmt.Sprintf("%x", sha1.Sum([]byte(m.repoCfgs[repo].Directory)))
	name := id + ".idx.gz"
	name = filepath.Join(dir, name)
	tmp := fmt.Sprintf("%s.tmp.%d", name, time.Now().UnixNano())
	idxf, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)

	gzw := gzip.NewWriter(idxf)

	n, err := protocol.IndexMessage{
		Repository: repo,
		Files:      fs,
	}.EncodeXDR(gzw)
	if err != nil {
		gzw.Close()
		idxf.Close()
		return err
	}

	err = gzw.Close()
	if err != nil {
		return err
	}

	err = idxf.Close()
	if err != nil {
		return err
	}

	if debug {
		l.Debugln("wrote index,", n, "bytes uncompressed")
	}

	return osutil.Rename(tmp, name)
}

func (m *Model) loadIndex(repo string, dir string) []protocol.FileInfo {
	id := fmt.Sprintf("%x", sha1.Sum([]byte(m.repoCfgs[repo].Directory)))
	name := id + ".idx.gz"
	name = filepath.Join(dir, name)

	idxf, err := os.Open(name)
	if err != nil {
		return nil
	}
	defer idxf.Close()

	gzr, err := gzip.NewReader(idxf)
	if err != nil {
		return nil
	}
	defer gzr.Close()

	var im protocol.IndexMessage
	err = im.DecodeXDR(gzr)
	if err != nil || im.Repository != repo {
		return nil
	}

	return im.Files
}

// clusterConfig returns a ClusterConfigMessage that is correct for the given peer node
func (m *Model) clusterConfig(node string) protocol.ClusterConfigMessage {
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
				ID:    node,
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
	m.repoState[repo] = state
	m.smut.Unlock()
}

func (m *Model) State(repo string) string {
	m.smut.RLock()
	state := m.repoState[repo]
	m.smut.RUnlock()
	switch state {
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

func (m *Model) Override(repo string) {
	fs := m.NeedFilesRepo(repo)

	m.rmut.Lock()
	r := m.repoFiles[repo]
	for i := range fs {
		f := &fs[i]
		h := r.Get(cid.LocalID, f.Name)
		if h.Name != f.Name {
			// We are missing the file
			f.Flags |= protocol.FlagDeleted
			f.Blocks = nil
		} else {
			// We have the file, replace with our version
			*f = h
		}
		f.Version = lamport.Default.Tick(f.Version)
	}
	m.rmut.Unlock()

	r.Update(cid.LocalID, fs)
}
