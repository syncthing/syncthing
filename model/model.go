package model

/*

Locking
=======

The model has read and write locks. These must be acquired as appropriate by
public methods. To prevent deadlock situations, private methods should never
acquire locks, but document what locks they require.

*/

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path"
	"sync"
	"time"

	"github.com/calmh/syncthing/buffers"
	"github.com/calmh/syncthing/protocol"
)

type Model struct {
	sync.RWMutex
	dir string

	global    map[string]File // the latest version of each file as it exists in the cluster
	local     map[string]File // the files we currently have locally on disk
	remote    map[string]map[string]File
	protoConn map[string]Connection
	rawConn   map[string]io.Closer
	fq        FileQueue // queue for files to fetch
	dq        chan File // queue for files to delete

	updatedLocal int64 // timestamp of last update to local
	updateGlobal int64 // timestamp of last update to remote

	lastIdxBcast        time.Time
	lastIdxBcastRequest time.Time

	rwRunning bool
	delete    bool

	trace map[string]bool

	fileLastChanged   map[string]time.Time
	fileWasSuppressed map[string]int

	parallellRequests int
	limitRequestRate  chan struct{}
}

type Connection interface {
	ID() string
	Index([]protocol.FileInfo)
	Request(name string, offset int64, size uint32, hash []byte) ([]byte, error)
	Statistics() protocol.Statistics
}

const (
	idxBcastHoldtime = 15 * time.Second  // Wait at least this long after the last index modification
	idxBcastMaxDelay = 120 * time.Second // Unless we've already waited this long

	minFileHoldTimeS = 60  // Never allow file changes more often than this
	maxFileHoldTimeS = 600 // Always allow file changes at least this often
)

var (
	ErrNoSuchFile = errors.New("no such file")
	ErrInvalid    = errors.New("file is invalid")
)

// NewModel creates and starts a new model. The model starts in read-only mode,
// where it sends index information to connected peers and responds to requests
// for file data without altering the local repository in any way.
func NewModel(dir string) *Model {
	m := &Model{
		dir:               dir,
		global:            make(map[string]File),
		local:             make(map[string]File),
		remote:            make(map[string]map[string]File),
		protoConn:         make(map[string]Connection),
		rawConn:           make(map[string]io.Closer),
		lastIdxBcast:      time.Now(),
		trace:             make(map[string]bool),
		fileLastChanged:   make(map[string]time.Time),
		fileWasSuppressed: make(map[string]int),
		dq:                make(chan File),
	}

	go m.broadcastIndexLoop()
	return m
}

func (m *Model) LimitRate(kbps int) {
	m.limitRequestRate = make(chan struct{}, kbps)
	n := kbps/10 + 1
	go func() {
		for {
			time.Sleep(100 * time.Millisecond)
			for i := 0; i < n; i++ {
				select {
				case m.limitRequestRate <- struct{}{}:
				}
			}
		}
	}()
}

// Trace enables trace logging of the given facility. This is a debugging function; grep for m.trace.
func (m *Model) Trace(t string) {
	m.Lock()
	defer m.Unlock()
	m.trace[t] = true
}

// StartRW starts read/write processing on the current model. When in
// read/write mode the model will attempt to keep in sync with the cluster by
// pulling needed files from peer nodes.
func (m *Model) StartRW(del bool, threads int) {
	m.Lock()
	defer m.Unlock()

	if m.rwRunning {
		panic("starting started model")
	}

	m.rwRunning = true
	m.delete = del
	m.parallellRequests = threads

	go m.cleanTempFiles()
	if del {
		go m.deleteFiles()
	}
}

// Generation returns an opaque integer that is guaranteed to increment on
// every change to the local repository or global model.
func (m *Model) Generation() int64 {
	m.RLock()
	defer m.RUnlock()

	return m.updatedLocal + m.updateGlobal
}

type ConnectionInfo struct {
	protocol.Statistics
	Address string
}

// ConnectionStats returns a map with connection statistics for each connected node.
func (m *Model) ConnectionStats() map[string]ConnectionInfo {
	type remoteAddrer interface {
		RemoteAddr() net.Addr
	}

	m.RLock()
	defer m.RUnlock()

	var res = make(map[string]ConnectionInfo)
	for node, conn := range m.protoConn {
		ci := ConnectionInfo{
			Statistics: conn.Statistics(),
		}
		if nc, ok := m.rawConn[node].(remoteAddrer); ok {
			ci.Address = nc.RemoteAddr().String()
		}
		res[node] = ci
	}
	return res
}

// LocalSize returns the number of files, deleted files and total bytes for all
// files in the global model.
func (m *Model) GlobalSize() (files, deleted, bytes int) {
	m.RLock()
	defer m.RUnlock()

	for _, f := range m.global {
		if f.Flags&protocol.FlagDeleted == 0 {
			files++
			bytes += f.Size()
		} else {
			deleted++
		}
	}
	return
}

// LocalSize returns the number of files, deleted files and total bytes for all
// files in the local repository.
func (m *Model) LocalSize() (files, deleted, bytes int) {
	m.RLock()
	defer m.RUnlock()

	for _, f := range m.local {
		if f.Flags&protocol.FlagDeleted == 0 {
			files++
			bytes += f.Size()
		} else {
			deleted++
		}
	}
	return
}

// InSyncSize returns the number and total byte size of the local files that
// are in sync with the global model.
func (m *Model) InSyncSize() (files, bytes int) {
	m.RLock()
	defer m.RUnlock()

	for n, f := range m.local {
		if gf, ok := m.global[n]; ok && f.Equals(gf) {
			files++
			bytes += f.Size()
		}
	}
	return
}

// NeedFiles returns the list of currently needed files and the total size.
func (m *Model) NeedFiles() (files []File, bytes int) {
	m.RLock()
	defer m.RUnlock()

	for _, n := range m.fq.QueuedFiles() {
		f := m.global[n]
		files = append(files, f)
		bytes += f.Size()
	}
	return
}

// Index is called when a new node is connected and we receive their full index.
// Implements the protocol.Model interface.
func (m *Model) Index(nodeID string, fs []protocol.FileInfo) {
	m.Lock()
	defer m.Unlock()

	if m.trace["net"] {
		log.Printf("NET IDX(in): %s: %d files", nodeID, len(fs))
	}

	repo := make(map[string]File)
	for _, f := range fs {
		m.indexUpdate(repo, f)
	}
	m.remote[nodeID] = repo

	m.recomputeGlobal()
	m.recomputeNeed()
}

// IndexUpdate is called for incremental updates to connected nodes' indexes.
// Implements the protocol.Model interface.
func (m *Model) IndexUpdate(nodeID string, fs []protocol.FileInfo) {
	m.Lock()
	defer m.Unlock()

	if m.trace["net"] {
		log.Printf("NET IDXUP(in): %s: %d files", nodeID, len(fs))
	}

	repo, ok := m.remote[nodeID]
	if !ok {
		log.Printf("WARNING: Index update from node %s that does not have an index", nodeID)
		return
	}

	for _, f := range fs {
		m.indexUpdate(repo, f)
	}

	m.recomputeGlobal()
	m.recomputeNeed()
}

func (m *Model) indexUpdate(repo map[string]File, f protocol.FileInfo) {
	if m.trace["idx"] {
		var flagComment string
		if f.Flags&protocol.FlagDeleted != 0 {
			flagComment = " (deleted)"
		}
		log.Printf("IDX(in): %q m=%d f=%o%s v=%d (%d blocks)", f.Name, f.Modified, f.Flags, flagComment, f.Version, len(f.Blocks))
	}

	if extraFlags := f.Flags &^ (protocol.FlagInvalid | protocol.FlagDeleted | 0xfff); extraFlags != 0 {
		log.Printf("WARNING: IDX(in): Unknown flags 0x%x in index record %+v", extraFlags, f)
		return
	}

	repo[f.Name] = fileFromFileInfo(f)
}

// Close removes the peer from the model and closes the underlyign connection if possible.
// Implements the protocol.Model interface.
func (m *Model) Close(node string, err error) {
	m.Lock()
	defer m.Unlock()

	conn, ok := m.rawConn[node]
	if ok {
		conn.Close()
	}

	delete(m.remote, node)
	delete(m.protoConn, node)
	delete(m.rawConn, node)
	m.fq.RemoveAvailable(node)

	m.recomputeGlobal()
	m.recomputeNeed()
}

// Request returns the specified data segment by reading it from local disk.
// Implements the protocol.Model interface.
func (m *Model) Request(nodeID, name string, offset int64, size uint32, hash []byte) ([]byte, error) {
	// Verify that the requested file exists in the local and global model.
	m.RLock()
	lf, localOk := m.local[name]
	_, globalOk := m.global[name]
	m.RUnlock()
	if !localOk || !globalOk {
		log.Printf("SECURITY (nonexistent file) REQ(in): %s: %q o=%d s=%d h=%x", nodeID, name, offset, size, hash)
		return nil, ErrNoSuchFile
	}
	if lf.Flags&protocol.FlagInvalid != 0 {
		return nil, ErrInvalid
	}

	if m.trace["net"] && nodeID != "<local>" {
		log.Printf("NET REQ(in): %s: %q o=%d s=%d h=%x", nodeID, name, offset, size, hash)
	}
	fn := path.Join(m.dir, name)
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

	if m.limitRequestRate != nil {
		for s := 0; s < len(buf); s += 1024 {
			<-m.limitRequestRate
		}
	}

	return buf, nil
}

// ReplaceLocal replaces the local repository index with the given list of files.
// Change suppression is applied to files changing too often.
func (m *Model) ReplaceLocal(fs []File) {
	m.Lock()
	defer m.Unlock()

	var updated bool
	var newLocal = make(map[string]File)

	for _, f := range fs {
		newLocal[f.Name] = f
		if ef := m.local[f.Name]; !ef.Equals(f) {
			updated = true
		}
	}

	if m.markDeletedLocals(newLocal) {
		updated = true
	}

	if len(newLocal) != len(m.local) {
		updated = true
	}

	if updated {
		m.local = newLocal
		m.recomputeGlobal()
		m.recomputeNeed()
		m.updatedLocal = time.Now().Unix()
		m.lastIdxBcastRequest = time.Now()
	}
}

// SeedLocal replaces the local repository index with the given list of files,
// in protocol data types. Does not track deletes, should only be used to seed
// the local index from a cache file at startup.
func (m *Model) SeedLocal(fs []protocol.FileInfo) {
	m.Lock()
	defer m.Unlock()

	m.local = make(map[string]File)
	for _, f := range fs {
		m.local[f.Name] = fileFromFileInfo(f)
	}

	m.recomputeGlobal()
	m.recomputeNeed()
}

// ConnectedTo returns true if we are connected to the named node.
func (m *Model) ConnectedTo(nodeID string) bool {
	m.RLock()
	defer m.RUnlock()
	_, ok := m.protoConn[nodeID]
	return ok
}

// ProtocolIndex returns the current local index in protocol data types.
func (m *Model) ProtocolIndex() []protocol.FileInfo {
	m.RLock()
	defer m.RUnlock()
	return m.protocolIndex()
}

// RepoID returns a unique ID representing the current repository location.
func (m *Model) RepoID() string {
	return fmt.Sprintf("%x", sha1.Sum([]byte(m.dir)))
}

// AddConnection adds a new peer connection to the model. An initial index will
// be sent to the connected peer, thereafter index updates whenever the local
// repository changes.
func (m *Model) AddConnection(rawConn io.Closer, protoConn Connection) {
	nodeID := protoConn.ID()
	m.Lock()
	m.protoConn[nodeID] = protoConn
	m.rawConn[nodeID] = rawConn
	m.Unlock()

	m.RLock()
	idx := m.protocolIndex()
	m.RUnlock()

	go func() {
		protoConn.Index(idx)
	}()

	if m.rwRunning {
		for i := 0; i < m.parallellRequests; i++ {
			i := i
			go func() {
				if m.trace["pull"] {
					log.Println("PULL: Starting", nodeID, i)
				}
				for {
					m.RLock()
					if _, ok := m.protoConn[nodeID]; !ok {
						if m.trace["pull"] {
							log.Println("PULL: Exiting", nodeID, i)
						}
						m.RUnlock()
						return
					}
					m.RUnlock()

					qb, ok := m.fq.Get(nodeID)
					if ok {
						if m.trace["pull"] {
							log.Println("PULL: Request", nodeID, i, qb.name, qb.block.Offset)
						}
						data, _ := protoConn.Request(qb.name, qb.block.Offset, qb.block.Size, qb.block.Hash)
						m.fq.Done(qb.name, qb.block.Offset, data)
					} else {
						time.Sleep(1 * time.Second)
					}
				}
			}()
		}
	}
}

func (m *Model) shouldSuppressChange(name string) bool {
	sup := shouldSuppressChange(m.fileLastChanged[name], m.fileWasSuppressed[name])
	if sup {
		m.fileWasSuppressed[name]++
	} else {
		m.fileWasSuppressed[name] = 0
		m.fileLastChanged[name] = time.Now()
	}
	return sup
}

func shouldSuppressChange(lastChange time.Time, numChanges int) bool {
	sinceLast := time.Since(lastChange)
	if sinceLast > maxFileHoldTimeS*time.Second {
		return false
	}
	if sinceLast < time.Duration((numChanges+2)*minFileHoldTimeS)*time.Second {
		return true
	}
	return false
}

// protocolIndex returns the current local index in protocol data types.
// Must be called with the read lock held.
func (m *Model) protocolIndex() []protocol.FileInfo {
	var index []protocol.FileInfo
	for _, f := range m.local {
		mf := fileInfoFromFile(f)
		if m.trace["idx"] {
			var flagComment string
			if mf.Flags&protocol.FlagDeleted != 0 {
				flagComment = " (deleted)"
			}
			log.Printf("IDX(out): %q m=%d f=%o%s v=%d (%d blocks)", mf.Name, mf.Modified, mf.Flags, flagComment, mf.Version, len(mf.Blocks))
		}
		index = append(index, mf)
	}
	return index
}

func (m *Model) requestGlobal(nodeID, name string, offset int64, size uint32, hash []byte) ([]byte, error) {
	m.RLock()
	nc, ok := m.protoConn[nodeID]
	m.RUnlock()
	if !ok {
		return nil, fmt.Errorf("requestGlobal: no such node: %s", nodeID)
	}

	if m.trace["net"] {
		log.Printf("NET REQ(out): %s: %q o=%d s=%d h=%x", nodeID, name, offset, size, hash)
	}

	return nc.Request(name, offset, size, hash)
}

func (m *Model) broadcastIndexLoop() {
	for {
		m.RLock()
		bcastRequested := m.lastIdxBcastRequest.After(m.lastIdxBcast)
		holdtimeExceeded := time.Since(m.lastIdxBcastRequest) > idxBcastHoldtime
		m.RUnlock()

		maxDelayExceeded := time.Since(m.lastIdxBcast) > idxBcastMaxDelay
		if bcastRequested && (holdtimeExceeded || maxDelayExceeded) {
			m.Lock()
			var indexWg sync.WaitGroup
			indexWg.Add(len(m.protoConn))
			idx := m.protocolIndex()
			m.lastIdxBcast = time.Now()
			for _, node := range m.protoConn {
				node := node
				if m.trace["net"] {
					log.Printf("NET IDX(out/loop): %s: %d files", node.ID(), len(idx))
				}
				go func() {
					node.Index(idx)
					indexWg.Done()
				}()
			}
			m.Unlock()
			indexWg.Wait()
		}
		time.Sleep(idxBcastHoldtime)
	}
}

// markDeletedLocals sets the deleted flag on files that have gone missing locally.
// Must be called with the write lock held.
func (m *Model) markDeletedLocals(newLocal map[string]File) bool {
	// For every file in the existing local table, check if they are also
	// present in the new local table. If they are not, check that we already
	// had the newest version available according to the global table and if so
	// note the file as having been deleted.
	var updated bool
	for n, f := range m.local {
		if _, ok := newLocal[n]; !ok {
			if gf := m.global[n]; !gf.NewerThan(f) {
				if f.Flags&protocol.FlagDeleted == 0 {
					f.Flags = protocol.FlagDeleted
					f.Version++
					f.Blocks = nil
					updated = true
				}
				newLocal[n] = f
			}
		}
	}
	return updated
}

func (m *Model) updateLocalLocked(f File) {
	m.Lock()
	m.updateLocal(f)
	m.Unlock()
}

func (m *Model) updateLocal(f File) {
	if ef, ok := m.local[f.Name]; !ok || !ef.Equals(f) {
		m.local[f.Name] = f
		m.recomputeGlobal()
		m.recomputeNeed()
		m.updatedLocal = time.Now().Unix()
		m.lastIdxBcastRequest = time.Now()
	}
}

// Must be called with the write lock held.
func (m *Model) recomputeGlobal() {
	var newGlobal = make(map[string]File)

	for n, f := range m.local {
		newGlobal[n] = f
	}

	var highestMod int64
	for nodeID, fs := range m.remote {
		for n, nf := range fs {
			if lf, ok := newGlobal[n]; !ok || nf.NewerThan(lf) {
				newGlobal[n] = nf
				m.fq.SetAvailable(n, nodeID)
				if nf.Modified > highestMod {
					highestMod = nf.Modified
				}
			} else if lf.Equals(nf) {
				m.fq.AddAvailable(n, nodeID)
			}
		}
	}

	// Figure out if anything actually changed

	var updated bool
	if highestMod > m.updateGlobal || len(newGlobal) != len(m.global) {
		updated = true
	} else {
		for n, f0 := range newGlobal {
			if f1, ok := m.global[n]; !ok || !f0.Equals(f1) {
				updated = true
				break
			}
		}
	}

	if updated {
		m.updateGlobal = time.Now().Unix()
		m.global = newGlobal
	}
}

// Must be called with the write lock held.
func (m *Model) recomputeNeed() {
	for n, gf := range m.global {
		if m.fq.Queued(n) {
			continue
		}
		lf, ok := m.local[n]
		if !ok || gf.NewerThan(lf) {
			if gf.Flags&protocol.FlagInvalid != 0 {
				// Never attempt to sync invalid files
				continue
			}
			if gf.Flags&protocol.FlagDeleted != 0 && !m.delete {
				// Don't want to delete files, so forget this need
				continue
			}
			if gf.Flags&protocol.FlagDeleted != 0 && !ok {
				// Don't have the file, so don't need to delete it
				continue
			}
			if m.trace["need"] {
				log.Printf("NEED: lf:%v gf:%v", lf, gf)
			}

			if gf.Flags&protocol.FlagDeleted != 0 {
				m.dq <- gf
			} else {
				local, remote := BlockDiff(lf.Blocks, gf.Blocks)
				fm := fileMonitor{
					name:        n,
					path:        path.Clean(path.Join(m.dir, n)),
					global:      gf,
					model:       m,
					localBlocks: local,
				}
				m.fq.Add(n, remote, &fm)
			}
		}
	}
}

func (m *Model) WhoHas(name string) []string {
	m.RLock()
	defer m.RUnlock()
	return m.whoHas(name)
}

// Must be called with the read lock held.
func (m *Model) whoHas(name string) []string {
	var remote []string

	gf := m.global[name]
	for node, files := range m.remote {
		if file, ok := files[name]; ok && file.Equals(gf) {
			remote = append(remote, node)
		}
	}

	return remote
}

func (m *Model) deleteFiles() {
	for file := range m.dq {
		if m.trace["file"] {
			log.Println("FILE: Delete", file.Name)
		}
		path := path.Clean(path.Join(m.dir, file.Name))
		err := os.Remove(path)
		if err != nil {
			log.Printf("WARNING: %s: %v", file.Name, err)
		}
		m.updateLocalLocked(file)
	}
}

func fileFromFileInfo(f protocol.FileInfo) File {
	var blocks = make([]Block, len(f.Blocks))
	var offset int64
	for i, b := range f.Blocks {
		blocks[i] = Block{
			Offset: offset,
			Size:   b.Size,
			Hash:   b.Hash,
		}
		offset += int64(b.Size)
	}
	return File{
		Name:     f.Name,
		Flags:    f.Flags,
		Modified: f.Modified,
		Version:  f.Version,
		Blocks:   blocks,
	}
}

func fileInfoFromFile(f File) protocol.FileInfo {
	var blocks = make([]protocol.BlockInfo, len(f.Blocks))
	for i, b := range f.Blocks {
		blocks[i] = protocol.BlockInfo{
			Size: b.Size,
			Hash: b.Hash,
		}
	}
	return protocol.FileInfo{
		Name:     f.Name,
		Flags:    f.Flags,
		Modified: f.Modified,
		Version:  f.Version,
		Blocks:   blocks,
	}
}
