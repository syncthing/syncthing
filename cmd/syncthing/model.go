package main

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
	"github.com/calmh/syncthing/files"
	"github.com/calmh/syncthing/lamport"
	"github.com/calmh/syncthing/protocol"
	"github.com/calmh/syncthing/scanner"
)

type Model struct {
	repoDirs  map[string]string     // repo -> dir
	repoFiles map[string]*files.Set // repo -> files
	repoNodes map[string][]string   // repo -> nodeIDs
	nodeRepos map[string][]string   // nodeID -> repos
	rmut      sync.RWMutex          // protects the above

	cm *cid.Map

	protoConn map[string]protocol.Connection
	rawConn   map[string]io.Closer
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
func NewModel(maxChangeBw int) *Model {
	m := &Model{
		repoDirs:  make(map[string]string),
		repoFiles: make(map[string]*files.Set),
		repoNodes: make(map[string][]string),
		nodeRepos: make(map[string][]string),
		cm:        cid.NewMap(),
		protoConn: make(map[string]protocol.Connection),
		rawConn:   make(map[string]io.Closer),
		sup:       suppressor{threshold: int64(maxChangeBw)},
	}

	go m.broadcastIndexLoop()
	return m
}

// StartRW starts read/write processing on the current model. When in
// read/write mode the model will attempt to keep in sync with the cluster by
// pulling needed files from peer nodes.
func (m *Model) StartRepoRW(repo string, threads int) {
	m.rmut.Lock()
	defer m.rmut.Unlock()

	if dir, ok := m.repoDirs[repo]; !ok {
		panic("cannot start without repo")
	} else {
		newPuller(repo, dir, m, threads)
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
	ClientID      string
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
			ClientID:      conn.Option("clientId"),
			ClientVersion: conn.Option("clientVersion"),
		}
		if nc, ok := m.rawConn[node].(remoteAddrer); ok {
			ci.Address = nc.RemoteAddr().String()
		}

		var tot int64
		var have int64

		for _, repo := range m.nodeRepos[node] {
			for _, f := range m.repoFiles[repo].Global() {
				if f.Flags&protocol.FlagDeleted == 0 {
					tot += f.Size
					have += f.Size
				}
			}

			for _, f := range m.repoFiles[repo].Need(m.cm.Get(node)) {
				if f.Flags&protocol.FlagDeleted == 0 {
					have -= f.Size
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

	return res
}

func sizeOf(fs []scanner.File) (files, deleted int, bytes int64) {
	for _, f := range fs {
		if f.Flags&protocol.FlagDeleted == 0 {
			files++
			bytes += f.Size
		} else {
			deleted++
		}
	}
	return
}

// GlobalSize returns the number of files, deleted files and total bytes for all
// files in the global model.
func (m *Model) GlobalSize() (files, deleted int, bytes int64) {
	m.rmut.RLock()
	var fs []scanner.File
	for _, rf := range m.repoFiles {
		fs = append(fs, rf.Global()...)
	}
	m.rmut.RUnlock()
	return sizeOf(fs)
}

// LocalSize returns the number of files, deleted files and total bytes for all
// files in the local repository.
func (m *Model) LocalSize() (files, deleted int, bytes int64) {
	m.rmut.RLock()
	var fs []scanner.File
	for _, rf := range m.repoFiles {
		fs = append(fs, rf.Have(cid.LocalID)...)
	}
	m.rmut.RUnlock()
	return sizeOf(fs)
}

// InSyncSize returns the number and total byte size of the local files that
// are in sync with the global model.
func (m *Model) InSyncSize() (files int, bytes int64) {
	var gf []scanner.File
	var nf []scanner.File

	m.rmut.RLock()
	for _, rf := range m.repoFiles {
		gf = append(gf, rf.Global()...)
		nf = append(nf, rf.Need(cid.LocalID)...)
	}
	m.rmut.RUnlock()

	gn, _, gb := sizeOf(gf)
	nn, _, nb := sizeOf(nf)

	return gn - nn, gb - nb
}

// NeedFiles returns the list of currently needed files and the total size.
func (m *Model) NeedFiles() ([]scanner.File, int64) {
	var nf []scanner.File
	m.rmut.RLock()
	for _, rf := range m.repoFiles {
		nf = append(nf, rf.Need(cid.LocalID)...)
	}
	m.rmut.RUnlock()

	var bytes int64
	for _, f := range nf {
		bytes += f.Size
	}

	return nf, bytes
}

// NeedFiles returns the list of currently needed files and the total size.
func (m *Model) NeedFilesRepo(repo string) []scanner.File {
	m.rmut.RLock()
	nf := m.repoFiles[repo].Need(cid.LocalID)
	m.rmut.RUnlock()
	return nf
}

// Index is called when a new node is connected and we receive their full index.
// Implements the protocol.Model interface.
func (m *Model) Index(nodeID string, repo string, fs []protocol.FileInfo) {
	if debugNet {
		dlog.Printf("IDX(in): %s / %q: %d files", nodeID, repo, len(fs))
	}

	var files = make([]scanner.File, len(fs))
	for i := range fs {
		lamport.Default.Tick(fs[i].Version)
		files[i] = fileFromFileInfo(fs[i])
	}

	id := m.cm.Get(nodeID)
	m.rmut.RLock()
	if r, ok := m.repoFiles[repo]; ok {
		r.Replace(id, files)
	} else {
		warnf("Index from %s for nonexistant repo %q; dropping", nodeID, repo)
	}
	m.rmut.RUnlock()
}

// IndexUpdate is called for incremental updates to connected nodes' indexes.
// Implements the protocol.Model interface.
func (m *Model) IndexUpdate(nodeID string, repo string, fs []protocol.FileInfo) {
	if debugNet {
		dlog.Printf("IDXUP(in): %s / %q: %d files", nodeID, repo, len(fs))
	}

	var files = make([]scanner.File, len(fs))
	for i := range fs {
		lamport.Default.Tick(fs[i].Version)
		files[i] = fileFromFileInfo(fs[i])
	}

	id := m.cm.Get(nodeID)
	m.rmut.RLock()
	if r, ok := m.repoFiles[repo]; ok {
		r.Update(id, files)
	} else {
		warnf("Index update from %s for nonexistant repo %q; dropping", nodeID, repo)
	}
	m.rmut.RUnlock()
}

// Close removes the peer from the model and closes the underlying connection if possible.
// Implements the protocol.Model interface.
func (m *Model) Close(node string, err error) {
	if debugNet {
		dlog.Printf("%s: %v", node, err)
	}
	if err == protocol.ErrClusterHash {
		warnf("Connection to %s closed due to mismatched cluster hash. Ensure that the configured cluster members are identical on both nodes.", node)
	} else if err != io.EOF {
		warnf("Connection to %s closed: %v", node, err)
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
		warnf("Request from %s for file %s in nonexistent repo %q", nodeID, name, repo)
		return nil, ErrNoSuchFile
	}

	lf := r.Get(cid.LocalID, name)
	if offset > lf.Size {
		warnf("SECURITY (nonexistent file) REQ(in): %s: %q o=%d s=%d", nodeID, name, offset, size)
		return nil, ErrNoSuchFile
	}

	if lf.Suppressed {
		return nil, ErrInvalid
	}

	if debugNet && nodeID != "<local>" {
		dlog.Printf("REQ(in): %s: %q / %q o=%d s=%d", nodeID, repo, name, offset, size)
	}
	m.rmut.RLock()
	fn := filepath.Join(m.repoDirs[repo], name)
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

	go func() {
		m.rmut.RLock()
		repos := m.nodeRepos[nodeID]
		m.rmut.RUnlock()
		for _, repo := range repos {
			idx := m.ProtocolIndex(repo)
			if debugNet {
				dlog.Printf("IDX(out/initial): %s: %q: %d files", nodeID, repo, len(idx))
			}
			protoConn.Index(repo, idx)
		}
	}()
}

// ProtocolIndex returns the current local index in protocol data types.
// Must be called with the read lock held.
func (m *Model) ProtocolIndex(repo string) []protocol.FileInfo {
	var index []protocol.FileInfo

	m.rmut.RLock()
	fs := m.repoFiles[repo].Have(cid.LocalID)
	m.rmut.RUnlock()

	for _, f := range fs {
		mf := fileInfoFromFile(f)
		if debugIdx {
			var flagComment string
			if mf.Flags&protocol.FlagDeleted != 0 {
				flagComment = " (deleted)"
			}
			dlog.Printf("IDX(out): %q/%q m=%d f=%o%s v=%d (%d blocks)", repo, mf.Name, mf.Modified, mf.Flags, flagComment, mf.Version, len(mf.Blocks))
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

	if debugNet {
		dlog.Printf("REQ(out): %s: %q / %q o=%d s=%d h=%x", nodeID, repo, name, offset, size, hash)
	}

	return nc.Request(repo, name, offset, size)
}

func (m *Model) broadcastIndexLoop() {
	var lastChange = map[string]uint64{}
	for {
		time.Sleep(5 * time.Second)

		m.pmut.RLock()
		m.rmut.RLock()

		for repo, fs := range m.repoFiles {
			c := fs.Changes(cid.LocalID)
			if c == lastChange[repo] {
				continue
			}
			lastChange[repo] = c

			idx := m.ProtocolIndex(repo)
			m.saveIndex(repo, confDir, idx)

			var indexWg sync.WaitGroup
			for _, nodeID := range m.repoNodes[repo] {
				if conn, ok := m.protoConn[nodeID]; ok {
					indexWg.Add(1)
					if debugNet {
						dlog.Printf("IDX(out/loop): %s: %d files", nodeID, len(idx))
					}
					go func() {
						conn.Index(repo, idx)
						indexWg.Done()
					}()
				}
			}

			indexWg.Wait()
		}

		m.rmut.RUnlock()
		m.pmut.RUnlock()
	}
}

func (m *Model) AddRepo(id, dir string, nodes []NodeConfiguration) {
	if m.started {
		panic("cannot add repo to started model")
	}
	if len(id) == 0 {
		panic("cannot add empty repo id")
	}

	m.rmut.Lock()
	m.repoDirs[id] = dir
	m.repoFiles[id] = files.NewSet()

	m.repoNodes[id] = make([]string, len(nodes))
	for i, node := range nodes {
		m.repoNodes[id][i] = node.NodeID
		m.nodeRepos[node.NodeID] = append(m.nodeRepos[node.NodeID], id)
	}

	m.addedRepo = true
	m.rmut.Unlock()
}

func (m *Model) ScanRepos() {
	m.rmut.RLock()
	for repo := range m.repoDirs {
		m.ScanRepo(repo)
	}
	m.rmut.RUnlock()
}

func (m *Model) ScanRepo(repo string) {
	sup := &suppressor{threshold: int64(cfg.Options.MaxChangeKbps)}
	w := &scanner.Walker{
		Dir:          m.repoDirs[repo],
		IgnoreFile:   ".stignore",
		BlockSize:    BlockSize,
		TempNamer:    defTempNamer,
		Suppressor:   sup,
		CurrentFiler: cFiler{m, repo},
	}
	fs, _ := w.Walk()
	m.ReplaceLocal(repo, fs)
}

func (m *Model) SaveIndexes(dir string) {
	m.rmut.RLock()
	for repo := range m.repoDirs {
		fs := m.ProtocolIndex(repo)
		m.saveIndex(repo, dir, fs)
	}
	m.rmut.RUnlock()
}

func (m *Model) LoadIndexes(dir string) {
	m.rmut.RLock()
	for repo := range m.repoDirs {
		fs := m.loadIndex(repo, dir)
		m.SeedLocal(repo, fs)
	}
	m.rmut.RUnlock()
}

func (m *Model) saveIndex(repo string, dir string, fs []protocol.FileInfo) {
	id := fmt.Sprintf("%x", sha1.Sum([]byte(m.repoDirs[repo])))
	name := id + ".idx.gz"
	name = filepath.Join(dir, name)

	idxf, err := os.Create(name + ".tmp")
	if err != nil {
		return
	}

	gzw := gzip.NewWriter(idxf)

	protocol.IndexMessage{
		Repository: repo,
		Files:      fs,
	}.EncodeXDR(gzw)
	gzw.Close()
	idxf.Close()

	Rename(name+".tmp", name)
}

func (m *Model) loadIndex(repo string, dir string) []protocol.FileInfo {
	id := fmt.Sprintf("%x", sha1.Sum([]byte(m.repoDirs[repo])))
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

func fileFromFileInfo(f protocol.FileInfo) scanner.File {
	var blocks = make([]scanner.Block, len(f.Blocks))
	var offset int64
	for i, b := range f.Blocks {
		blocks[i] = scanner.Block{
			Offset: offset,
			Size:   b.Size,
			Hash:   b.Hash,
		}
		offset += int64(b.Size)
	}
	return scanner.File{
		// Name is with native separator and normalization
		Name:       filepath.FromSlash(f.Name),
		Size:       offset,
		Flags:      f.Flags &^ protocol.FlagInvalid,
		Modified:   f.Modified,
		Version:    f.Version,
		Blocks:     blocks,
		Suppressed: f.Flags&protocol.FlagInvalid != 0,
	}
}

func fileInfoFromFile(f scanner.File) protocol.FileInfo {
	var blocks = make([]protocol.BlockInfo, len(f.Blocks))
	for i, b := range f.Blocks {
		blocks[i] = protocol.BlockInfo{
			Size: b.Size,
			Hash: b.Hash,
		}
	}
	pf := protocol.FileInfo{
		Name:     filepath.ToSlash(f.Name),
		Flags:    f.Flags,
		Modified: f.Modified,
		Version:  f.Version,
		Blocks:   blocks,
	}
	if f.Suppressed {
		pf.Flags |= protocol.FlagInvalid
	}
	return pf
}
