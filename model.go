package main

/*

Locking
=======

The model has read and write locks. These must be acquired as appropriate by
public methods. To prevent deadlock situations, private methods should never
acquire locks, but document what locks they require.

*/

import (
	"errors"
	"fmt"
	"os"
	"path"
	"sync"
	"time"

	"github.com/calmh/syncthing/buffers"
	"github.com/calmh/syncthing/protocol"
)

type Model struct {
	sync.RWMutex
	dir     string
	updated int64
	global  map[string]File // the latest version of each file as it exists in the cluster
	local   map[string]File // the files we currently have locally on disk
	remote  map[string]map[string]File
	need    map[string]bool // the files we need to update
	nodes   map[string]*protocol.Connection

	lastIdxBcast        time.Time
	lastIdxBcastRequest time.Time
}

var (
	errNoSuchNode = errors.New("no such node")
)

const (
	FlagDeleted = 1 << 12

	idxBcastHoldtime = 15 * time.Second  // Wait at least this long after the last index modification
	idxBcastMaxDelay = 120 * time.Second // Unless we've already waited this long
)

func NewModel(dir string) *Model {
	m := &Model{
		dir:          dir,
		global:       make(map[string]File),
		local:        make(map[string]File),
		remote:       make(map[string]map[string]File),
		need:         make(map[string]bool),
		nodes:        make(map[string]*protocol.Connection),
		lastIdxBcast: time.Now(),
	}

	go m.printStats()
	go m.broadcastIndexLoop()
	return m
}

func (m *Model) Start() {
	go m.puller()
}

func (m *Model) printStats() {
	for {
		time.Sleep(60 * time.Second)
		m.RLock()
		for node, conn := range m.nodes {
			stats := conn.Statistics()
			if (stats.InBytesPerSec > 0 || stats.OutBytesPerSec > 0) && stats.Latency > 0 {
				infof("%s: %sB/s in, %sB/s out, %0.02f ms", node, toSI(stats.InBytesPerSec), toSI(stats.OutBytesPerSec), stats.Latency.Seconds()*1000)
			} else if stats.InBytesPerSec > 0 || stats.OutBytesPerSec > 0 {
				infof("%s: %sB/s in, %sB/s out", node, toSI(stats.InBytesPerSec), toSI(stats.OutBytesPerSec))
			}
		}
		m.RUnlock()
	}
}

func toSI(n int) string {
	if n > 1<<30 {
		return fmt.Sprintf("%.02f G", float64(n)/(1<<30))
	}
	if n > 1<<20 {
		return fmt.Sprintf("%.02f M", float64(n)/(1<<20))
	}
	if n > 1<<10 {
		return fmt.Sprintf("%.01f K", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d ", n)
}

func (m *Model) Index(nodeID string, fs []protocol.FileInfo) {
	m.Lock()
	defer m.Unlock()

	if opts.Debug.TraceNet {
		debugf("NET IDX(in): %s: %d files", nodeID, len(fs))
	}

	m.remote[nodeID] = make(map[string]File)
	for _, f := range fs {
		if f.Flags&FlagDeleted != 0 && !opts.Delete {
			// Files marked as deleted do not even enter the model
			continue
		}
		m.remote[nodeID][f.Name] = fileFromProtocol(f)
	}

	m.recomputeGlobal()
	m.recomputeNeed()
}

func (m *Model) IndexUpdate(nodeID string, fs []protocol.FileInfo) {
	m.Lock()
	defer m.Unlock()

	if opts.Debug.TraceNet {
		debugf("NET IDXUP(in): %s: %d files", nodeID, len(fs))
	}

	repo, ok := m.remote[nodeID]
	if !ok {
		return
	}

	for _, f := range fs {
		if f.Flags&FlagDeleted != 0 && !opts.Delete {
			// Files marked as deleted do not even enter the model
			continue
		}
		repo[f.Name] = fileFromProtocol(f)
	}

	m.recomputeGlobal()
	m.recomputeNeed()
}

func (m *Model) SeedIndex(fs []protocol.FileInfo) {
	m.Lock()
	defer m.Unlock()

	m.local = make(map[string]File)
	for _, f := range fs {
		m.local[f.Name] = fileFromProtocol(f)
	}

	m.recomputeGlobal()
	m.recomputeNeed()
}

func (m *Model) Close(node string) {
	m.Lock()
	defer m.Unlock()

	if opts.Debug.TraceNet {
		debugf("NET CLOSE: %s", node)
	}

	delete(m.remote, node)
	delete(m.nodes, node)

	m.recomputeGlobal()
	m.recomputeNeed()
}

func (m *Model) Request(nodeID, name string, offset uint64, size uint32, hash []byte) ([]byte, error) {
	if opts.Debug.TraceNet && nodeID != "<local>" {
		debugf("NET REQ(in): %s: %q o=%d s=%d h=%x", nodeID, name, offset, size, hash)
	}
	fn := path.Join(m.dir, name)
	fd, err := os.Open(fn) // XXX: Inefficient, should cache fd?
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	buf := buffers.Get(int(size))
	_, err = fd.ReadAt(buf, int64(offset))
	if err != nil {
		return nil, err
	}

	return buf, nil
}

func (m *Model) RequestGlobal(nodeID, name string, offset uint64, size uint32, hash []byte) ([]byte, error) {
	m.RLock()
	nc, ok := m.nodes[nodeID]
	m.RUnlock()
	if !ok {
		return nil, errNoSuchNode
	}

	if opts.Debug.TraceNet {
		debugf("NET REQ(out): %s: %q o=%d s=%d h=%x", nodeID, name, offset, size, hash)
	}

	return nc.Request(name, offset, size, hash)
}

func (m *Model) ReplaceLocal(fs []File) {
	m.Lock()
	defer m.Unlock()

	var updated bool
	var newLocal = make(map[string]File)

	for _, f := range fs {
		newLocal[f.Name] = f
		if ef := m.local[f.Name]; ef.Modified != f.Modified {
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
		m.updated = time.Now().Unix()
		m.lastIdxBcastRequest = time.Now()
	}
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
			indexWg.Add(len(m.nodes))
			idx := m.protocolIndex()
			m.lastIdxBcast = time.Now()
			for _, node := range m.nodes {
				node := node
				if opts.Debug.TraceNet {
					debugf("NET IDX(out): %s: %d files", node.ID, len(idx))
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
			if gf := m.global[n]; gf.Modified <= f.Modified {
				if f.Flags&FlagDeleted == 0 {
					f.Flags = FlagDeleted
					f.Modified = f.Modified + 1
					f.Blocks = nil
					updated = true
				}
				newLocal[n] = f
			}
		}
	}
	return updated
}

func (m *Model) UpdateLocal(f File) {
	m.Lock()
	defer m.Unlock()

	if ef, ok := m.local[f.Name]; !ok || ef.Modified != f.Modified {
		m.local[f.Name] = f
		m.recomputeGlobal()
		m.recomputeNeed()
		m.updated = time.Now().Unix()
		m.lastIdxBcastRequest = time.Now()
	}
}

func (m *Model) Dir() string {
	m.RLock()
	defer m.RUnlock()
	return m.dir
}

func (m *Model) HaveFiles() []File {
	m.RLock()
	defer m.RUnlock()
	var files []File
	for _, file := range m.local {
		files = append(files, file)
	}
	return files
}

func (m *Model) LocalFile(name string) (File, bool) {
	m.RLock()
	defer m.RUnlock()
	f, ok := m.local[name]
	return f, ok
}

func (m *Model) GlobalFile(name string) (File, bool) {
	m.RLock()
	defer m.RUnlock()
	f, ok := m.global[name]
	return f, ok
}

// Must be called with the write lock held.
func (m *Model) recomputeGlobal() {
	var newGlobal = make(map[string]File)

	for n, f := range m.local {
		newGlobal[n] = f
	}

	for _, fs := range m.remote {
		for n, f := range fs {
			if cf, ok := newGlobal[n]; !ok || cf.Modified < f.Modified {
				newGlobal[n] = f
			}
		}
	}

	m.global = newGlobal
}

// Must be called with the write lock held.
func (m *Model) recomputeNeed() {
	m.need = make(map[string]bool)
	for n, f := range m.global {
		hf, ok := m.local[n]
		if !ok || f.Modified > hf.Modified {
			m.need[n] = true
		}
	}
}

// Must be called with the read lock held.
func (m *Model) whoHas(name string) []string {
	var remote []string

	gf := m.global[name]
	for node, files := range m.remote {
		if file, ok := files[name]; ok && file.Modified == gf.Modified {
			remote = append(remote, node)
		}
	}

	return remote
}

func (m *Model) ConnectedTo(nodeID string) bool {
	m.RLock()
	defer m.RUnlock()
	_, ok := m.nodes[nodeID]
	return ok
}

func (m *Model) ProtocolIndex() []protocol.FileInfo {
	m.RLock()
	defer m.RUnlock()
	return m.protocolIndex()
}

// Must be called with the read lock held.
func (m *Model) protocolIndex() []protocol.FileInfo {
	var index []protocol.FileInfo
	for _, f := range m.local {
		mf := protocol.FileInfo{
			Name:     f.Name,
			Flags:    f.Flags,
			Modified: int64(f.Modified),
		}
		for _, b := range f.Blocks {
			mf.Blocks = append(mf.Blocks, protocol.BlockInfo{
				Length: b.Length,
				Hash:   b.Hash,
			})
		}
		if opts.Debug.TraceIdx {
			var flagComment string
			if mf.Flags&FlagDeleted != 0 {
				flagComment = " (deleted)"
			}
			debugf("IDX: %q m=%d f=%o%s (%d blocks)", mf.Name, mf.Modified, mf.Flags, flagComment, len(mf.Blocks))
		}
		index = append(index, mf)
	}
	return index
}

func (m *Model) AddNode(node *protocol.Connection) {
	m.Lock()
	m.nodes[node.ID] = node
	m.Unlock()
	m.RLock()
	idx := m.protocolIndex()
	m.RUnlock()

	if opts.Debug.TraceNet {
		debugf("NET IDX(out): %s: %d files", node.ID, len(idx))
	}

	// Sending the index might take a while if we have many files and a slow
	// uplink. Return from AddNode in the meantime.
	go node.Index(idx)
}

func fileFromProtocol(f protocol.FileInfo) File {
	var blocks []Block
	var offset uint64
	for _, b := range f.Blocks {
		blocks = append(blocks, Block{
			Offset: offset,
			Length: b.Length,
			Hash:   b.Hash,
		})
		offset += uint64(b.Length)
	}
	return File{
		Name:     f.Name,
		Flags:    f.Flags,
		Modified: int64(f.Modified),
		Blocks:   blocks,
	}
}
