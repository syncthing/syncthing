// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"encoding/binary"
)

const (
	keyPrefixLen   = 1
	keyFolderLen   = 4 // indexed
	keyDeviceLen   = 4 // indexed
	keySequenceLen = 8
	keyHashLen     = 32

	maxInt64 int64 = 1<<63 - 1
)

const (
	// KeyTypeDevice <int32 folder ID> <int32 device ID> <file name> = FileInfo
	KeyTypeDevice = 0

	// KeyTypeGlobal <int32 folder ID> <file name> = VersionList
	KeyTypeGlobal = 1

	// KeyTypeBlock <int32 folder ID> <32 bytes hash> <§file name> = int32 (block index)
	KeyTypeBlock = 2

	// KeyTypeDeviceStatistic <device ID as string> <some string> = some value
	KeyTypeDeviceStatistic = 3

	// KeyTypeFolderStatistic <folder ID as string> <some string> = some value
	KeyTypeFolderStatistic = 4

	// KeyTypeVirtualMtime <int32 folder ID> <file name> = dbMtime
	KeyTypeVirtualMtime = 5

	// KeyTypeFolderIdx <int32 id> = string value
	KeyTypeFolderIdx = 6

	// KeyTypeDeviceIdx <int32 id> = string value
	KeyTypeDeviceIdx = 7

	// KeyTypeIndexID <int32 device ID> <int32 folder ID> = protocol.IndexID
	KeyTypeIndexID = 8

	// KeyTypeFolderMeta <int32 folder ID> = CountsSet
	KeyTypeFolderMeta = 9

	// KeyTypeMiscData <some string> = some value
	KeyTypeMiscData = 10

	// KeyTypeSequence <int32 folder ID> <int64 sequence number> = KeyTypeDevice key
	KeyTypeSequence = 11

	// KeyTypeNeed <int32 folder ID> <file name> = <nothing>
	KeyTypeNeed = 12
)

type keyer interface {
	// device file key stuff
	GenerateDeviceFileKey(key, folder, device, name []byte) deviceFileKey
	NameFromDeviceFileKey(key []byte) []byte
	DeviceFromDeviceFileKey(key []byte) ([]byte, bool)
	FolderFromDeviceFileKey(key []byte) ([]byte, bool)

	// global version key stuff
	GenerateGlobalVersionKey(key, folder, name []byte) globalVersionKey
	NameFromGlobalVersionKey(key []byte) []byte
	FolderFromGlobalVersionKey(key []byte) ([]byte, bool)

	// block map key stuff (former BlockMap)
	GenerateBlockMapKey(key, folder, hash, name []byte) blockMapKey
	NameFromBlockMapKey(key []byte) []byte

	// file need index
	GenerateNeedFileKey(key, folder, name []byte) needFileKey

	// file sequence index
	GenerateSequenceKey(key, folder []byte, seq int64) sequenceKey
	SequenceFromSequenceKey(key []byte) int64

	// index IDs
	GenerateIndexIDKey(key, device, folder []byte) indexIDKey
	DeviceFromIndexIDKey(key []byte) ([]byte, bool)

	// Mtimes
	GenerateMtimesKey(key, folder []byte) mtimesKey

	// Folder metadata
	GenerateFolderMetaKey(key, folder []byte) folderMetaKey
}

// defaultKeyer implements our key scheme. It needs folder and device
// indexes.
type defaultKeyer struct {
	folderIdx *smallIndex
	deviceIdx *smallIndex
}

func newDefaultKeyer(folderIdx, deviceIdx *smallIndex) defaultKeyer {
	return defaultKeyer{
		folderIdx: folderIdx,
		deviceIdx: deviceIdx,
	}
}

type deviceFileKey []byte

func (k deviceFileKey) WithoutNameAndDevice() []byte {
	return k[:keyPrefixLen+keyFolderLen]
}

func (k defaultKeyer) GenerateDeviceFileKey(key, folder, device, name []byte) deviceFileKey {
	key = resize(key, keyPrefixLen+keyFolderLen+keyDeviceLen+len(name))
	key[0] = KeyTypeDevice
	binary.BigEndian.PutUint32(key[keyPrefixLen:], k.folderIdx.ID(folder))
	binary.BigEndian.PutUint32(key[keyPrefixLen+keyFolderLen:], k.deviceIdx.ID(device))
	copy(key[keyPrefixLen+keyFolderLen+keyDeviceLen:], name)
	return key
}

func (k defaultKeyer) NameFromDeviceFileKey(key []byte) []byte {
	return key[keyPrefixLen+keyFolderLen+keyDeviceLen:]
}

func (k defaultKeyer) DeviceFromDeviceFileKey(key []byte) ([]byte, bool) {
	return k.deviceIdx.Val(binary.BigEndian.Uint32(key[keyPrefixLen+keyFolderLen:]))
}

func (k defaultKeyer) FolderFromDeviceFileKey(key []byte) ([]byte, bool) {
	return k.folderIdx.Val(binary.BigEndian.Uint32(key[keyPrefixLen:]))
}

type globalVersionKey []byte

func (k globalVersionKey) WithoutName() []byte {
	return k[:keyPrefixLen+keyFolderLen]
}

func (k defaultKeyer) GenerateGlobalVersionKey(key, folder, name []byte) globalVersionKey {
	key = resize(key, keyPrefixLen+keyFolderLen+len(name))
	key[0] = KeyTypeGlobal
	binary.BigEndian.PutUint32(key[keyPrefixLen:], k.folderIdx.ID(folder))
	copy(key[keyPrefixLen+keyFolderLen:], name)
	return key
}

func (k defaultKeyer) NameFromGlobalVersionKey(key []byte) []byte {
	return key[keyPrefixLen+keyFolderLen:]
}

func (k defaultKeyer) FolderFromGlobalVersionKey(key []byte) ([]byte, bool) {
	return k.folderIdx.Val(binary.BigEndian.Uint32(key[keyPrefixLen:]))
}

type blockMapKey []byte

func (k defaultKeyer) GenerateBlockMapKey(key, folder, hash, name []byte) blockMapKey {
	key = resize(key, keyPrefixLen+keyFolderLen+keyHashLen+len(name))
	key[0] = KeyTypeBlock
	binary.BigEndian.PutUint32(key[keyPrefixLen:], k.folderIdx.ID(folder))
	copy(key[keyPrefixLen+keyFolderLen:], hash)
	copy(key[keyPrefixLen+keyFolderLen+keyHashLen:], name)
	return key
}

func (k defaultKeyer) NameFromBlockMapKey(key []byte) []byte {
	return key[keyPrefixLen+keyFolderLen+keyHashLen:]
}

func (k blockMapKey) WithoutHashAndName() []byte {
	return k[:keyPrefixLen+keyFolderLen]
}

type needFileKey []byte

func (k needFileKey) WithoutName() []byte {
	return k[:keyPrefixLen+keyFolderLen]
}

func (k defaultKeyer) GenerateNeedFileKey(key, folder, name []byte) needFileKey {
	key = resize(key, keyPrefixLen+keyFolderLen+len(name))
	key[0] = KeyTypeNeed
	binary.BigEndian.PutUint32(key[keyPrefixLen:], k.folderIdx.ID(folder))
	copy(key[keyPrefixLen+keyFolderLen:], name)
	return key
}

type sequenceKey []byte

func (k sequenceKey) WithoutSequence() []byte {
	return k[:keyPrefixLen+keyFolderLen]
}

func (k defaultKeyer) GenerateSequenceKey(key, folder []byte, seq int64) sequenceKey {
	key = resize(key, keyPrefixLen+keyFolderLen+keySequenceLen)
	key[0] = KeyTypeSequence
	binary.BigEndian.PutUint32(key[keyPrefixLen:], k.folderIdx.ID(folder))
	binary.BigEndian.PutUint64(key[keyPrefixLen+keyFolderLen:], uint64(seq))
	return key
}

func (k defaultKeyer) SequenceFromSequenceKey(key []byte) int64 {
	return int64(binary.BigEndian.Uint64(key[keyPrefixLen+keyFolderLen:]))
}

type indexIDKey []byte

func (k defaultKeyer) GenerateIndexIDKey(key, device, folder []byte) indexIDKey {
	key = resize(key, keyPrefixLen+keyDeviceLen+keyFolderLen)
	key[0] = KeyTypeIndexID
	binary.BigEndian.PutUint32(key[keyPrefixLen:], k.deviceIdx.ID(device))
	binary.BigEndian.PutUint32(key[keyPrefixLen+keyDeviceLen:], k.folderIdx.ID(folder))
	return key
}

func (k defaultKeyer) DeviceFromIndexIDKey(key []byte) ([]byte, bool) {
	return k.deviceIdx.Val(binary.BigEndian.Uint32(key[keyPrefixLen:]))
}

type mtimesKey []byte

func (k defaultKeyer) GenerateMtimesKey(key, folder []byte) mtimesKey {
	key = resize(key, keyPrefixLen+keyFolderLen)
	key[0] = KeyTypeVirtualMtime
	binary.BigEndian.PutUint32(key[keyPrefixLen:], k.folderIdx.ID(folder))
	return key
}

type folderMetaKey []byte

func (k defaultKeyer) GenerateFolderMetaKey(key, folder []byte) folderMetaKey {
	key = resize(key, keyPrefixLen+keyFolderLen)
	key[0] = KeyTypeFolderMeta
	binary.BigEndian.PutUint32(key[keyPrefixLen:], k.folderIdx.ID(folder))
	return key
}

// resize returns a byte slice of the specified size, reusing bs if possible
func resize(bs []byte, size int) []byte {
	if cap(bs) < size {
		return make([]byte, size)
	}
	return bs[:size]
}
