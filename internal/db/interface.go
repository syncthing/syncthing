package db // import "github.com/syncthing/syncthing/internal/db/sqlite"

import (
	"iter"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
)

type DB interface {
	AllForBlocksHash(folder string, h []byte) iter.Seq2[protocol.FileInfo, error]
	AllForBlocksHashAnyFolder(errptr *error, h []byte) iter.Seq2[string, protocol.FileInfo]
	AllGlobal(folder string) iter.Seq2[protocol.FileInfo, error]
	AllGlobalPrefix(folder string, prefix string) iter.Seq2[protocol.FileInfo, error]
	AllLocal(folder string, device protocol.DeviceID) iter.Seq2[protocol.FileInfo, error]
	AllLocalPrefixed(folder string, device protocol.DeviceID, prefix string) iter.Seq2[protocol.FileInfo, error]
	AllLocalSequenced(folder string, device protocol.DeviceID, startSeq int64) iter.Seq2[protocol.FileInfo, error]
	AllNeededNames(folder string, device protocol.DeviceID, order config.PullOrder, limit int) iter.Seq2[string, error]
	Availability(folder, file string) ([]protocol.DeviceID, error)
	Blocks(hash []byte) iter.Seq2[BlockMapEntry, error]
	Close() error
	DevicesForFolder(folder string) ([]protocol.DeviceID, error)
	DropAllFiles(folder string, device protocol.DeviceID) error
	DropDevice(device protocol.DeviceID) error
	DropFilesNamed(folder string, device protocol.DeviceID, names []string) error
	DropFolder(folder string) error
	DropIndexIDs() error
	Folders() ([]string, error)
	Global(folder string, file string) (protocol.FileInfo, bool, error)
	GlobalSize(folder string) Counts
	IndexID(folder string, device protocol.DeviceID) (protocol.IndexID, error)
	KV() KV
	Local(folder string, device protocol.DeviceID, file string) (protocol.FileInfo, bool, error)
	LocalSize(folder string, device protocol.DeviceID) Counts
	NeedSize(folder string, device protocol.DeviceID) Counts
	ReceiveOnlySize(folder string) Counts
	Sequence(folder string, device protocol.DeviceID) (int64, error)
	SetIndexID(folder string, device protocol.DeviceID, id protocol.IndexID) error
	Update(folder string, device protocol.DeviceID, fs []protocol.FileInfo) error
}

type KV interface {
	Get(key string) ([]byte, error)
	Put(key string, val []byte) error
	Delete(key string) error
	Prefix(prefix string) iter.Seq2[KVEntry, error]
}

type KVEntry struct {
	Key   string
	Value []byte
}

type BlockMapEntry struct {
	BlocklistHash []byte `db:"blocklist_hash"`
	Index         int    `db:"idx"`
	Offset        int64
	Size          int
}
