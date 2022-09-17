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
	KeyTypeDevice byte = 0

	// KeyTypeGlobal <int32 folder ID> <file name> = VersionList
	KeyTypeGlobal byte = 1

	// KeyTypeBlock <int32 folder ID> <32 bytes hash> <§file name> = int32 (block index)
	KeyTypeBlock byte = 2

	// KeyTypeDeviceStatistic <device ID as string> <some string> = some value
	KeyTypeDeviceStatistic byte = 3

	// KeyTypeFolderStatistic <folder ID as string> <some string> = some value
	KeyTypeFolderStatistic byte = 4

	// KeyTypeVirtualMtime <int32 folder ID> <file name> = mtimeMapping
	KeyTypeVirtualMtime byte = 5

	// KeyTypeFolderIdx <int32 id> = string value
	KeyTypeFolderIdx byte = 6

	// KeyTypeDeviceIdx <int32 id> = string value
	KeyTypeDeviceIdx byte = 7

	// KeyTypeIndexID <int32 device ID> <int32 folder ID> = protocol.IndexID
	KeyTypeIndexID byte = 8

	// KeyTypeFolderMeta <int32 folder ID> = CountsSet
	KeyTypeFolderMeta byte = 9

	// KeyTypeMiscData <some string> = some value
	KeyTypeMiscData byte = 10

	// KeyTypeSequence <int32 folder ID> <int64 sequence number> = KeyTypeDevice key
	KeyTypeSequence byte = 11

	// KeyTypeNeed <int32 folder ID> <file name> = <nothing>
	KeyTypeNeed byte = 12

	// KeyTypeBlockList <block list hash> = BlockList
	KeyTypeBlockList byte = 13

	// KeyTypeBlockListMap <int32 folder ID> <block list hash> <file name> = <nothing>
	KeyTypeBlockListMap byte = 14

	// KeyTypeVersion <version hash> = Vector
	KeyTypeVersion byte = 15

	// KeyTypePendingFolder <int32 device ID> <folder ID as string> = ObservedFolder
	KeyTypePendingFolder byte = 16

	// KeyTypePendingDevice <device ID in wire format> = ObservedDevice
	KeyTypePendingDevice byte = 17
)

type keyer interface {
	// device file key stuff
	GenerateDeviceFileKey(key, folder, device, name []byte) (deviceFileKey, error)
	NameFromDeviceFileKey(key []byte) []byte
	DeviceFromDeviceFileKey(key []byte) ([]byte, bool)
	FolderFromDeviceFileKey(key []byte) ([]byte, bool)

	// global version key stuff
	GenerateGlobalVersionKey(key, folder, name []byte) (globalVersionKey, error)
	NameFromGlobalVersionKey(key []byte) []byte

	// block map key stuff (former BlockMap)
	GenerateBlockMapKey(key, folder, hash, name []byte) (blockMapKey, error)
	NameFromBlockMapKey(key []byte) []byte
	GenerateBlockListMapKey(key, folder, hash, name []byte) (blockListMapKey, error)
	NameFromBlockListMapKey(key []byte) []byte

	// file need index
	GenerateNeedFileKey(key, folder, name []byte) (needFileKey, error)

	// file sequence index
	GenerateSequenceKey(key, folder []byte, seq int64) (sequenceKey, error)
	SequenceFromSequenceKey(key []byte) int64

	// index IDs
	GenerateIndexIDKey(key, device, folder []byte) (indexIDKey, error)
	FolderFromIndexIDKey(key []byte) ([]byte, bool)

	// Mtimes
	GenerateMtimesKey(key, folder []byte) (mtimesKey, error)

	// Folder metadata
	GenerateFolderMetaKey(key, folder []byte) (folderMetaKey, error)

	// Block lists
	GenerateBlockListKey(key []byte, hash []byte) blockListKey

	// Version vectors
	GenerateVersionKey(key []byte, hash []byte) versionKey

	// Pending (unshared) folders and devices
	GeneratePendingFolderKey(key, device, folder []byte) (pendingFolderKey, error)
	FolderFromPendingFolderKey(key []byte) []byte
	DeviceFromPendingFolderKey(key []byte) ([]byte, bool)

	GeneratePendingDeviceKey(key, device []byte) pendingDeviceKey
	DeviceFromPendingDeviceKey(key []byte) []byte
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

func (k deviceFileKey) WithoutName() []byte {
	return k[:keyPrefixLen+keyFolderLen+keyDeviceLen]
}

func (k defaultKeyer) GenerateDeviceFileKey(key, folder, device, name []byte) (deviceFileKey, error) {
	folderID, err := k.folderIdx.ID(folder)
	if err != nil {
		return nil, err
	}
	deviceID, err := k.deviceIdx.ID(device)
	if err != nil {
		return nil, err
	}
	key = resize(key, keyPrefixLen+keyFolderLen+keyDeviceLen+len(name))
	key[0] = KeyTypeDevice
	binary.BigEndian.PutUint32(key[keyPrefixLen:], folderID)
	binary.BigEndian.PutUint32(key[keyPrefixLen+keyFolderLen:], deviceID)
	copy(key[keyPrefixLen+keyFolderLen+keyDeviceLen:], name)
	return key, nil
}

func (defaultKeyer) NameFromDeviceFileKey(key []byte) []byte {
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

func (k defaultKeyer) GenerateGlobalVersionKey(key, folder, name []byte) (globalVersionKey, error) {
	folderID, err := k.folderIdx.ID(folder)
	if err != nil {
		return nil, err
	}
	key = resize(key, keyPrefixLen+keyFolderLen+len(name))
	key[0] = KeyTypeGlobal
	binary.BigEndian.PutUint32(key[keyPrefixLen:], folderID)
	copy(key[keyPrefixLen+keyFolderLen:], name)
	return key, nil
}

func (defaultKeyer) NameFromGlobalVersionKey(key []byte) []byte {
	return key[keyPrefixLen+keyFolderLen:]
}

type blockMapKey []byte

func (k defaultKeyer) GenerateBlockMapKey(key, folder, hash, name []byte) (blockMapKey, error) {
	folderID, err := k.folderIdx.ID(folder)
	if err != nil {
		return nil, err
	}
	key = resize(key, keyPrefixLen+keyFolderLen+keyHashLen+len(name))
	key[0] = KeyTypeBlock
	binary.BigEndian.PutUint32(key[keyPrefixLen:], folderID)
	copy(key[keyPrefixLen+keyFolderLen:], hash)
	copy(key[keyPrefixLen+keyFolderLen+keyHashLen:], name)
	return key, nil
}

func (defaultKeyer) NameFromBlockMapKey(key []byte) []byte {
	return key[keyPrefixLen+keyFolderLen+keyHashLen:]
}

func (k blockMapKey) WithoutHashAndName() []byte {
	return k[:keyPrefixLen+keyFolderLen]
}

type blockListMapKey []byte

func (k defaultKeyer) GenerateBlockListMapKey(key, folder, hash, name []byte) (blockListMapKey, error) {
	folderID, err := k.folderIdx.ID(folder)
	if err != nil {
		return nil, err
	}
	key = resize(key, keyPrefixLen+keyFolderLen+keyHashLen+len(name))
	key[0] = KeyTypeBlockListMap
	binary.BigEndian.PutUint32(key[keyPrefixLen:], folderID)
	copy(key[keyPrefixLen+keyFolderLen:], hash)
	copy(key[keyPrefixLen+keyFolderLen+keyHashLen:], name)
	return key, nil
}

func (defaultKeyer) NameFromBlockListMapKey(key []byte) []byte {
	return key[keyPrefixLen+keyFolderLen+keyHashLen:]
}

func (k blockListMapKey) WithoutHashAndName() []byte {
	return k[:keyPrefixLen+keyFolderLen]
}

type needFileKey []byte

func (k needFileKey) WithoutName() []byte {
	return k[:keyPrefixLen+keyFolderLen]
}

func (k defaultKeyer) GenerateNeedFileKey(key, folder, name []byte) (needFileKey, error) {
	folderID, err := k.folderIdx.ID(folder)
	if err != nil {
		return nil, err
	}
	key = resize(key, keyPrefixLen+keyFolderLen+len(name))
	key[0] = KeyTypeNeed
	binary.BigEndian.PutUint32(key[keyPrefixLen:], folderID)
	copy(key[keyPrefixLen+keyFolderLen:], name)
	return key, nil
}

type sequenceKey []byte

func (k sequenceKey) WithoutSequence() []byte {
	return k[:keyPrefixLen+keyFolderLen]
}

func (k defaultKeyer) GenerateSequenceKey(key, folder []byte, seq int64) (sequenceKey, error) {
	folderID, err := k.folderIdx.ID(folder)
	if err != nil {
		return nil, err
	}
	key = resize(key, keyPrefixLen+keyFolderLen+keySequenceLen)
	key[0] = KeyTypeSequence
	binary.BigEndian.PutUint32(key[keyPrefixLen:], folderID)
	binary.BigEndian.PutUint64(key[keyPrefixLen+keyFolderLen:], uint64(seq))
	return key, nil
}

func (defaultKeyer) SequenceFromSequenceKey(key []byte) int64 {
	return int64(binary.BigEndian.Uint64(key[keyPrefixLen+keyFolderLen:]))
}

type indexIDKey []byte

func (k defaultKeyer) GenerateIndexIDKey(key, device, folder []byte) (indexIDKey, error) {
	deviceID, err := k.deviceIdx.ID(device)
	if err != nil {
		return nil, err
	}
	folderID, err := k.folderIdx.ID(folder)
	if err != nil {
		return nil, err
	}
	key = resize(key, keyPrefixLen+keyDeviceLen+keyFolderLen)
	key[0] = KeyTypeIndexID
	binary.BigEndian.PutUint32(key[keyPrefixLen:], deviceID)
	binary.BigEndian.PutUint32(key[keyPrefixLen+keyDeviceLen:], folderID)
	return key, nil
}

func (k defaultKeyer) FolderFromIndexIDKey(key []byte) ([]byte, bool) {
	return k.folderIdx.Val(binary.BigEndian.Uint32(key[keyPrefixLen+keyDeviceLen:]))
}

type mtimesKey []byte

func (k defaultKeyer) GenerateMtimesKey(key, folder []byte) (mtimesKey, error) {
	folderID, err := k.folderIdx.ID(folder)
	if err != nil {
		return nil, err
	}
	key = resize(key, keyPrefixLen+keyFolderLen)
	key[0] = KeyTypeVirtualMtime
	binary.BigEndian.PutUint32(key[keyPrefixLen:], folderID)
	return key, nil
}

type folderMetaKey []byte

func (k defaultKeyer) GenerateFolderMetaKey(key, folder []byte) (folderMetaKey, error) {
	folderID, err := k.folderIdx.ID(folder)
	if err != nil {
		return nil, err
	}
	key = resize(key, keyPrefixLen+keyFolderLen)
	key[0] = KeyTypeFolderMeta
	binary.BigEndian.PutUint32(key[keyPrefixLen:], folderID)
	return key, nil
}

type blockListKey []byte

func (defaultKeyer) GenerateBlockListKey(key []byte, hash []byte) blockListKey {
	key = resize(key, keyPrefixLen+len(hash))
	key[0] = KeyTypeBlockList
	copy(key[keyPrefixLen:], hash)
	return key
}

func (k blockListKey) Hash() []byte {
	return k[keyPrefixLen:]
}

type versionKey []byte

func (defaultKeyer) GenerateVersionKey(key []byte, hash []byte) versionKey {
	key = resize(key, keyPrefixLen+len(hash))
	key[0] = KeyTypeVersion
	copy(key[keyPrefixLen:], hash)
	return key
}

func (k versionKey) Hash() []byte {
	return k[keyPrefixLen:]
}

type pendingFolderKey []byte

func (k defaultKeyer) GeneratePendingFolderKey(key, device, folder []byte) (pendingFolderKey, error) {
	deviceID, err := k.deviceIdx.ID(device)
	if err != nil {
		return nil, err
	}
	key = resize(key, keyPrefixLen+keyDeviceLen+len(folder))
	key[0] = KeyTypePendingFolder
	binary.BigEndian.PutUint32(key[keyPrefixLen:], deviceID)
	copy(key[keyPrefixLen+keyDeviceLen:], folder)
	return key, nil
}

func (defaultKeyer) FolderFromPendingFolderKey(key []byte) []byte {
	return key[keyPrefixLen+keyDeviceLen:]
}

func (k defaultKeyer) DeviceFromPendingFolderKey(key []byte) ([]byte, bool) {
	return k.deviceIdx.Val(binary.BigEndian.Uint32(key[keyPrefixLen:]))
}

type pendingDeviceKey []byte

func (defaultKeyer) GeneratePendingDeviceKey(key, device []byte) pendingDeviceKey {
	key = resize(key, keyPrefixLen+len(device))
	key[0] = KeyTypePendingDevice
	copy(key[keyPrefixLen:], device)
	return key
}

func (defaultKeyer) DeviceFromPendingDeviceKey(key []byte) []byte {
	return key[keyPrefixLen:]
}

// resize returns a byte slice of the specified size, reusing bs if possible
func resize(bs []byte, size int) []byte {
	if cap(bs) < size {
		return make([]byte, size)
	}
	return bs[:size]
}
