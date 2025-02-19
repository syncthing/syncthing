package sqlite

import (
	"iter"
	"sync"

	"github.com/syncthing/syncthing/lib/config"
	olddb "github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/protocol"
)

// FolderDB is a thin wrapper around DB that sticks closer to the older
// FileSet API by being folder-centric. It also provides a layer of locking
// to lessen transaction contention on the actual database.
type FolderDB struct {
	db     *DB
	folder string
	mut    sync.RWMutex
}

func NewFolderDB(db *DB, folder string) *FolderDB {
	return &FolderDB{db: db, folder: folder}
}

func (f *FolderDB) Update(device protocol.DeviceID, fs []protocol.FileInfo) error {
	f.mut.Lock()
	defer f.mut.Unlock()
	return f.db.Update(f.folder, device, fs)
}

func (f *FolderDB) Drop(device protocol.DeviceID, names []string) error {
	f.mut.Lock()
	defer f.mut.Unlock()
	return f.db.Drop(f.folder, device, names)
}

func (f *FolderDB) DropNames(names []string) error {
	f.mut.Lock()
	defer f.mut.Unlock()
	return f.db.DropNames(f.folder, protocol.LocalDeviceID, names)
}

func (f *FolderDB) Local(device protocol.DeviceID, file string) (protocol.FileInfo, bool, error) {
	f.mut.RLock()
	defer f.mut.RUnlock()
	return f.db.Local(f.folder, device, file)
}

func (f *FolderDB) Global(file string) (protocol.FileInfo, bool, error) {
	f.mut.RLock()
	defer f.mut.RUnlock()
	return f.db.Global(f.folder, file)
}

func (f *FolderDB) AllGlobalPrefix(prefix string) iter.Seq2[protocol.FileInfo, error] {
	f.mut.RLock()
	defer f.mut.RUnlock()
	return f.db.AllGlobalPrefix(f.folder, prefix)
}

func (f *FolderDB) Sequence(device protocol.DeviceID) int64 {
	f.mut.RLock()
	defer f.mut.RUnlock()
	return f.db.Sequence(f.folder, device)
}

func (f *FolderDB) AllNeededNames(device protocol.DeviceID, order config.PullOrder, limit int) iter.Seq2[string, error] {
	f.mut.RLock()
	defer f.mut.RUnlock()
	return f.db.AllNeededNames(f.folder, device, order, limit)
}

func (f *FolderDB) Availability(file string) ([]protocol.DeviceID, error) {
	f.mut.RLock()
	defer f.mut.RUnlock()
	return f.db.Availability(f.folder, file)
}

func (f *FolderDB) AllLocal(device protocol.DeviceID) iter.Seq2[*protocol.FileInfo, error] {
	f.mut.RLock()
	defer f.mut.RUnlock()
	return f.db.AllLocal(f.folder, device)
}

func (f *FolderDB) AllLocalSequenced(device protocol.DeviceID, startSeq int64) iter.Seq2[*protocol.FileInfo, error] {
	f.mut.RLock()
	defer f.mut.RUnlock()
	return f.db.AllLocalSequenced(f.folder, device, startSeq)
}

func (f *FolderDB) AllLocalPrefixed(device protocol.DeviceID, prefix string) iter.Seq2[*protocol.FileInfo, error] {
	f.mut.RLock()
	defer f.mut.RUnlock()
	return f.db.AllLocalPrefixed(f.folder, device, prefix)
}

func (f *FolderDB) AllForBlocksHash(h []byte) iter.Seq2[*protocol.FileInfo, error] {
	return f.db.AllForBlocksHash(f.folder, h)
}

func (f *FolderDB) LocalSize(device protocol.DeviceID) olddb.Counts {
	f.mut.RLock()
	defer f.mut.RUnlock()
	return f.db.LocalSize(f.folder, device)
}

func (f *FolderDB) GlobalSize() olddb.Counts {
	f.mut.RLock()
	defer f.mut.RUnlock()
	return f.db.GlobalSize(f.folder)
}

func (f *FolderDB) NeedSize(device protocol.DeviceID) olddb.Counts {
	f.mut.RLock()
	defer f.mut.RUnlock()
	return f.db.NeedSize(f.folder, device)
}

func (f *FolderDB) ReceiveOnlySize() olddb.Counts {
	f.mut.RLock()
	defer f.mut.RUnlock()
	return f.db.ReceiveOnlySize(f.folder)
}

func (f *FolderDB) IndexID(device protocol.DeviceID) (protocol.IndexID, error) {
	f.mut.RLock()
	defer f.mut.RUnlock()
	return f.db.IndexID(f.folder, device)
}
