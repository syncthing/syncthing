package sqlite

import (
	"iter"

	"github.com/syncthing/syncthing/lib/protocol"
)

type FolderDB struct {
	db     *DB
	folder string
}

func NewFolderDB(db *DB, folder string) *FolderDB {
	return &FolderDB{db: db, folder: folder}
}

func (f *FolderDB) Update(device protocol.DeviceID, fs []protocol.FileInfo) error {
	return f.db.Update(f.folder, device, fs)
}

func (f *FolderDB) Drop(device protocol.DeviceID) error {
	return f.db.Drop(f.folder, device)
}

func (f *FolderDB) DropNames(names []string) error {
	return f.db.DropNames(f.folder, protocol.LocalDeviceID, names)
}

func (f *FolderDB) Local(device protocol.DeviceID, file string) (*protocol.FileInfo, bool, error) {
	return f.db.Local(f.folder, device, file)
}

func (f *FolderDB) Global(file string) (*protocol.FileInfo, bool, error) {
	return f.db.Global(f.folder, file)
}

func (f *FolderDB) AllNeededNames(device protocol.DeviceID) ([]string, error) {
	return f.db.AllNeededNames(f.folder, device)
}

func (f *FolderDB) AllLocal(device protocol.DeviceID) iter.Seq2[*protocol.FileInfo, error] {
	return f.db.AllLocal(f.folder, device)
}

func (f *FolderDB) AllLocalSequenced(device protocol.DeviceID, startSeq int64) iter.Seq2[*protocol.FileInfo, error] {
	return f.db.AllLocalSequenced(f.folder, device, startSeq)
}

func (f *FolderDB) AllLocalPrefixed(device protocol.DeviceID, prefix string) iter.Seq2[*protocol.FileInfo, error] {
	return f.db.AllLocalPrefixed(f.folder, device, prefix)
}
