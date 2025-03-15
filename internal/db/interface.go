package db // import "github.com/syncthing/syncthing/internal/db/sqlite"

import (
	"iter"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/thejerf/suture/v4"
)

type DB interface {
	suture.Service

	// Basics
	Update(folder string, device protocol.DeviceID, fs []protocol.FileInfo) error
	Close() error

	// Single files
	GetDeviceFile(folder string, device protocol.DeviceID, file string) (protocol.FileInfo, bool, error)
	GetGlobalAvailability(folder, file string) ([]protocol.DeviceID, error)
	GetGlobalFile(folder string, file string) (protocol.FileInfo, bool, error)

	// File iterators
	AllGlobalFiles(folder string) (iter.Seq[FileMetadata], func() error)
	AllGlobalFilesPrefix(folder string, prefix string) (iter.Seq[FileMetadata], func() error)
	AllLocalBlocksWithHash(hash []byte) (iter.Seq[BlockMapEntry], func() error)
	AllLocalFiles(folder string, device protocol.DeviceID) (iter.Seq[protocol.FileInfo], func() error)
	AllLocalFilesBySequence(folder string, device protocol.DeviceID, startSeq int64, limit int) (iter.Seq[protocol.FileInfo], func() error)
	AllLocalFilesWithPrefix(folder string, device protocol.DeviceID, prefix string) (iter.Seq[protocol.FileInfo], func() error)
	AllLocalFilesWithBlocksHash(folder string, h []byte) (iter.Seq[protocol.FileInfo], func() error)
	AllLocalFilesWithBlocksHashAnyFolder(h []byte) (iter.Seq2[string, protocol.FileInfo], func() error)
	AllNeededGlobalFiles(folder string, device protocol.DeviceID, order config.PullOrder, limit, offset int) (iter.Seq[protocol.FileInfo], func() error)

	// Cleanup
	DropAllFiles(folder string, device protocol.DeviceID) error
	DropDevice(device protocol.DeviceID) error
	DropFilesNamed(folder string, device protocol.DeviceID, names []string) error
	DropFolder(folder string) error

	// Various metadata
	GetDeviceSequence(folder string, device protocol.DeviceID) (int64, error)
	ListFolders() ([]string, error)
	ListDevicesForFolder(folder string) ([]protocol.DeviceID, error)

	// Counts
	CountGlobal(folder string) (Counts, error)
	CountLocal(folder string, device protocol.DeviceID) (Counts, error)
	CountNeed(folder string, device protocol.DeviceID) (Counts, error)
	CountReceiveOnlyChanged(folder string) (Counts, error)

	// Index IDs
	IndexIDDropAll() error
	IndexIDGet(folder string, device protocol.DeviceID) (protocol.IndexID, error)
	IndexIDSet(folder string, device protocol.DeviceID, id protocol.IndexID) error

	// MtimeFS
	MtimeDelete(folder, name string) error
	MtimeGet(folder, name string) (ondisk, virtual time.Time)
	MtimePut(folder, name string, ondisk, virtual time.Time) error

	// Generic KV
	KVDelete(key string) error
	KVGet(key string) ([]byte, error)
	KVPrefix(prefix string) (iter.Seq[KeyValue], func() error)
	KVPut(key string, val []byte) error
}

type BlockMapEntry struct {
	BlocklistHash []byte
	BlockIndex    int
	Offset        int64
	Size          int
}

type KeyValue struct {
	Key   string
	Value []byte
}

type FileMetadata struct {
	Sequence      int64
	Name          string
	Type          protocol.FileInfoType
	ModifiedNanos int64
	Size          int64
	Deleted       bool
	Invalid       bool
	LocalFlags    int
}

func (m *FileMetadata) Modified() time.Time {
	return time.Unix(0, m.ModifiedNanos)
}
