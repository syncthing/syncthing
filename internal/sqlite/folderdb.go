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

func (db *FolderDB) Update(device protocol.DeviceID, fs []protocol.FileInfo) error {
	return db.db.Update(db.folder, device, fs)
}

func (db *FolderDB) Drop(device protocol.DeviceID) error {
	return db.db.Drop(db.folder, device)
}

func (db *FolderDB) Local(device protocol.DeviceID, file string) (*protocol.FileInfo, bool, error) {
	return db.db.Local(db.folder, device, file)
}

func (db *FolderDB) Global(file string) (*protocol.FileInfo, bool, error) {
	return db.db.Global(db.folder, file)
}

func (db *FolderDB) AllNeededNames(device protocol.DeviceID) ([]string, error) {
	return db.db.AllNeededNames(db.folder, device)
}

func (db *FolderDB) AllLocal(device protocol.DeviceID) iter.Seq2[*protocol.FileInfo, error] {
	return db.db.AllLocal(db.folder, device)
}

func (db *FolderDB) AllLocalSequenced(device protocol.DeviceID, startSeq int64) iter.Seq2[*protocol.FileInfo, error] {
	return db.db.AllLocalSequenced(db.folder, device, startSeq)
}

func (db *FolderDB) AllLocalPrefixed(device protocol.DeviceID, prefix string) iter.Seq2[*protocol.FileInfo, error] {
	return db.db.AllLocalPrefixed(db.folder, device, prefix)
}
