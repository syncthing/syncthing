// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package protocol

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/syncthing/syncthing/internal/gen/bep"
	"github.com/syncthing/syncthing/lib/build"
)

type FlagLocal uint32

// FileInfo.LocalFlags flags
const (
	FlagLocalUnsupported   FlagLocal = 1 << 0 // 1: The kind is unsupported, e.g. symlinks on Windows
	FlagLocalIgnored       FlagLocal = 1 << 1 // 2: Matches local ignore patterns
	FlagLocalMustRescan    FlagLocal = 1 << 2 // 4: Doesn't match content on disk, must be rechecked fully
	FlagLocalReceiveOnly   FlagLocal = 1 << 3 // 8: Change detected on receive only folder
	FlagLocalGlobal        FlagLocal = 1 << 4 // 16: This is the global file version
	FlagLocalNeeded        FlagLocal = 1 << 5 // 32: We need this file
	FlagLocalRemoteInvalid FlagLocal = 1 << 6 // 64: The remote marked this as invalid

	// Flags that should result in the Invalid bit on outgoing updates (or had it on ingoing ones)
	LocalInvalidFlags = FlagLocalUnsupported | FlagLocalIgnored | FlagLocalMustRescan | FlagLocalReceiveOnly | FlagLocalRemoteInvalid

	// Flags that should result in a file being in conflict with its
	// successor, due to us not having an up to date picture of its state on
	// disk.
	LocalConflictFlags = FlagLocalUnsupported | FlagLocalIgnored | FlagLocalReceiveOnly

	LocalAllFlags = FlagLocalUnsupported | FlagLocalIgnored | FlagLocalMustRescan | FlagLocalReceiveOnly | FlagLocalGlobal | FlagLocalNeeded | FlagLocalRemoteInvalid
)

// localFlagBitNames maps flag values to characters which can be used to
// build a permission-like bit string for easier reading.
var localFlagBitNames = map[FlagLocal]string{
	FlagLocalUnsupported:   "u",
	FlagLocalIgnored:       "i",
	FlagLocalMustRescan:    "r",
	FlagLocalReceiveOnly:   "e",
	FlagLocalGlobal:        "G",
	FlagLocalNeeded:        "n",
	FlagLocalRemoteInvalid: "v",
}

func (f FlagLocal) IsInvalid() bool {
	return f&LocalInvalidFlags != 0
}

// HumanString returns a permission-like string representation of the flag bits
func (f FlagLocal) HumanString() string {
	if f == 0 {
		return strings.Repeat("-", len(localFlagBitNames))
	}

	bit := FlagLocal(1)
	var res bytes.Buffer
	var extra strings.Builder
	for f != 0 {
		if f&bit != 0 {
			if name, ok := localFlagBitNames[bit]; ok {
				res.WriteString(name)
			} else {
				fmt.Fprintf(&extra, "+0x%x", bit)
			}
		} else {
			res.WriteString("-")
		}
		f &^= bit
		bit <<= 1
	}
	if res.Len() < len(localFlagBitNames) {
		res.WriteString(strings.Repeat("-", len(localFlagBitNames)-res.Len()))
	}
	base := res.Bytes()
	slices.Reverse(base)
	return string(base) + extra.String()
}

// BlockSizes is the list of valid block sizes, from min to max
var BlockSizes []int

func init() {
	for blockSize := MinBlockSize; blockSize <= MaxBlockSize; blockSize *= 2 {
		BlockSizes = append(BlockSizes, blockSize)
		if _, ok := sha256OfEmptyBlock[blockSize]; !ok {
			panic("missing hard coded value for sha256 of empty block")
		}
	}
	BufferPool = newBufferPool() // must happen after BlockSizes is initialized
}

type FileInfoType = bep.FileInfoType

const (
	FileInfoTypeFile             = bep.FileInfoType_FILE_INFO_TYPE_FILE
	FileInfoTypeDirectory        = bep.FileInfoType_FILE_INFO_TYPE_DIRECTORY
	FileInfoTypeSymlinkFile      = bep.FileInfoType_FILE_INFO_TYPE_SYMLINK_FILE
	FileInfoTypeSymlinkDirectory = bep.FileInfoType_FILE_INFO_TYPE_SYMLINK_DIRECTORY
	FileInfoTypeSymlink          = bep.FileInfoType_FILE_INFO_TYPE_SYMLINK
)

type FileInfo struct {
	Name               string
	Size               int64
	ModifiedS          int64
	ModifiedBy         ShortID
	Version            Vector
	Sequence           int64
	Blocks             []BlockInfo
	SymlinkTarget      []byte
	BlocksHash         []byte
	PreviousBlocksHash []byte
	Encrypted          []byte
	Platform           PlatformData

	Type         FileInfoType
	Permissions  uint32
	ModifiedNs   int32
	RawBlockSize int32

	// The local_flags fields stores flags that are relevant to the local
	// host only. It is not part of the protocol, doesn't get sent or
	// received (we make sure to zero it), nonetheless we need it on our
	// struct and to be able to serialize it to/from the database.
	// It does carry the info to decide if the file is invalid, which is part of
	// the protocol.
	LocalFlags FlagLocal

	// The time when the inode was last changed (i.e., permissions, xattrs
	// etc changed). This is host-local, not sent over the wire.
	InodeChangeNs int64

	// The size of the data appended to the encrypted file on disk. This is
	// host-local, not sent over the wire.
	EncryptionTrailerSize int

	Deleted       bool
	NoPermissions bool

	truncated bool // was created from a truncated file info without blocks
}

func (f *FileInfo) ToWire(withInternalFields bool) *bep.FileInfo {
	if f.truncated && !(f.IsDeleted() || f.IsInvalid() || f.IsIgnored()) {
		panic("bug: must not serialize truncated file info")
	}
	blocks := make([]*bep.BlockInfo, len(f.Blocks))
	for j, b := range f.Blocks {
		blocks[j] = b.ToWire()
	}
	w := &bep.FileInfo{
		Name:               f.Name,
		Size:               f.Size,
		ModifiedS:          f.ModifiedS,
		ModifiedBy:         uint64(f.ModifiedBy),
		Version:            f.Version.ToWire(),
		Sequence:           f.Sequence,
		Blocks:             blocks,
		SymlinkTarget:      f.SymlinkTarget,
		BlocksHash:         f.BlocksHash,
		PreviousBlocksHash: f.PreviousBlocksHash,
		Encrypted:          f.Encrypted,
		Type:               f.Type,
		Permissions:        f.Permissions,
		ModifiedNs:         f.ModifiedNs,
		BlockSize:          f.RawBlockSize,
		Platform:           f.Platform.toWire(),
		Deleted:            f.Deleted,
		Invalid:            f.IsInvalid(),
		NoPermissions:      f.NoPermissions,
	}
	if withInternalFields {
		w.LocalFlags = uint32(f.LocalFlags)
		w.InodeChangeNs = f.InodeChangeNs
		w.EncryptionTrailerSize = int32(f.EncryptionTrailerSize)
	}
	return w
}

func (f *FileInfo) InConflictWith(previous FileInfo) bool {
	if f.Version.GreaterEqual(previous.Version) {
		// If the new file is strictly greater in the ordering than the
		// existing file, it is not a conflict. If any counter has moved
		// backwards, or different counters have increased independently,
		// then the file is not greater but concurrent and we don't take
		// this branch.
		return false
	}

	if len(f.PreviousBlocksHash) == 0 || len(f.BlocksHash) == 0 {
		// Don't have data to make a content determination, or the type has
		// changed (file to directory, etc). Consider it a conflict.
		return true
	}
	// If the new file is based on the old contents we have, it's not really
	// a conflict.
	return !bytes.Equal(f.PreviousBlocksHash, previous.BlocksHash)
}

// WinsConflict returns true if "f" is the one to choose when it is in
// conflict with "other".
func (f *FileInfo) WinsConflict(other FileInfo) bool {
	// If only one of the files is invalid, that one loses.
	if f.IsInvalid() != other.IsInvalid() {
		return !f.IsInvalid()
	}

	// The one with the newer modification time wins.
	if f.ModTime().After(other.ModTime()) {
		return true
	}
	if f.ModTime().Before(other.ModTime()) {
		return false
	}

	// The modification times were equal. Use the device ID in the version
	// vector as tie breaker.
	return f.FileVersion().Compare(other.FileVersion()) == ConcurrentGreater
}

func (f *FileInfo) LogAttr() slog.Attr {
	attrs := []any{slog.String("name", f.Name)}
	var kind string
	switch f.Type {
	case FileInfoTypeFile:
		kind = "file"
		if !f.Deleted {
			attrs = append(attrs,
				slog.Any("modified", f.ModTime()),
				slog.String("permissions", fmt.Sprintf("0%03o", f.Permissions)),
				slog.Int64("size", f.Size),
				slog.Int("blocksize", f.BlockSize()),
			)
		}
	case FileInfoTypeDirectory:
		kind = "dir"
		attrs = append(attrs, slog.String("permissions", fmt.Sprintf("0%03o", f.Permissions)))
	case FileInfoTypeSymlink:
		kind = "symlink"
		attrs = append(attrs, slog.String("target", string(f.SymlinkTarget)))
	}
	return slog.Group(kind, attrs...)
}

func FileInfoFromWire(w *bep.FileInfo) FileInfo {
	var blocks []BlockInfo
	if len(w.Blocks) > 0 {
		blocks = make([]BlockInfo, len(w.Blocks))
		for j, b := range w.Blocks {
			blocks[j] = BlockInfoFromWire(b)
		}
	}
	return fileInfoFromWireWithBlocks(w, blocks)
}

type FileInfoWithoutBlocks interface {
	GetName() string
	GetSize() int64
	GetModifiedS() int64
	GetModifiedBy() uint64
	GetVersion() *bep.Vector
	GetSequence() int64
	// GetBlocks() []*bep.BlockInfo // not included
	GetSymlinkTarget() []byte
	GetBlocksHash() []byte
	GetPreviousBlocksHash() []byte
	GetEncrypted() []byte
	GetType() FileInfoType
	GetPermissions() uint32
	GetModifiedNs() int32
	GetBlockSize() int32
	GetPlatform() *bep.PlatformData
	GetLocalFlags() uint32
	GetInodeChangeNs() int64
	GetEncryptionTrailerSize() int32
	GetDeleted() bool
	GetInvalid() bool
	GetNoPermissions() bool
}

func fileInfoFromWireWithBlocks(w FileInfoWithoutBlocks, blocks []BlockInfo) FileInfo {
	var localFlags FlagLocal
	if w.GetInvalid() {
		localFlags = FlagLocalRemoteInvalid
	}
	return FileInfo{
		Name:               w.GetName(),
		Size:               w.GetSize(),
		ModifiedS:          w.GetModifiedS(),
		ModifiedBy:         ShortID(w.GetModifiedBy()),
		Version:            VectorFromWire(w.GetVersion()),
		Sequence:           w.GetSequence(),
		Blocks:             blocks,
		SymlinkTarget:      w.GetSymlinkTarget(),
		BlocksHash:         w.GetBlocksHash(),
		PreviousBlocksHash: w.GetPreviousBlocksHash(),
		Encrypted:          w.GetEncrypted(),
		Type:               w.GetType(),
		Permissions:        w.GetPermissions(),
		ModifiedNs:         w.GetModifiedNs(),
		RawBlockSize:       w.GetBlockSize(),
		Platform:           platformDataFromWire(w.GetPlatform()),
		Deleted:            w.GetDeleted(),
		LocalFlags:         localFlags,
		NoPermissions:      w.GetNoPermissions(),
	}
}

func FileInfoFromDB(w *bep.FileInfo) FileInfo {
	f := FileInfoFromWire(w)
	f.LocalFlags = FlagLocal(w.LocalFlags)
	f.InodeChangeNs = w.InodeChangeNs
	f.EncryptionTrailerSize = int(w.EncryptionTrailerSize)
	return f
}

func FileInfoFromDBTruncated(w FileInfoWithoutBlocks) FileInfo {
	f := fileInfoFromWireWithBlocks(w, nil)
	f.LocalFlags = FlagLocal(w.GetLocalFlags())
	f.InodeChangeNs = w.GetInodeChangeNs()
	f.EncryptionTrailerSize = int(w.GetEncryptionTrailerSize())
	f.truncated = true
	return f
}

func (f FileInfo) String() string {
	switch f.Type {
	case FileInfoTypeDirectory:
		return fmt.Sprintf("Directory{Name:%q, Sequence:%d, Permissions:0%o, ModTime:%v, Version:%v, Deleted:%v, Invalid:%v, LocalFlags:0x%x, NoPermissions:%v, Platform:%v, InodeChangeTime:%v}",
			f.Name, f.Sequence, f.Permissions, f.ModTime(), f.Version, f.Deleted, f.IsInvalid(), f.LocalFlags, f.NoPermissions, f.Platform, f.InodeChangeTime())
	case FileInfoTypeFile:
		return fmt.Sprintf("File{Name:%q, Sequence:%d, Permissions:0%o, ModTime:%v, Version:%v, Length:%d, Deleted:%v, Invalid:%v, LocalFlags:0x%x, NoPermissions:%v, BlockSize:%d, NumBlocks:%d, BlocksHash:%x, Platform:%v, InodeChangeTime:%v}",
			f.Name, f.Sequence, f.Permissions, f.ModTime(), f.Version, f.Size, f.Deleted, f.IsInvalid(), f.LocalFlags, f.NoPermissions, f.RawBlockSize, len(f.Blocks), f.BlocksHash, f.Platform, f.InodeChangeTime())
	case FileInfoTypeSymlink, FileInfoTypeSymlinkDirectory, FileInfoTypeSymlinkFile:
		return fmt.Sprintf("Symlink{Name:%q, Type:%v, Sequence:%d, Version:%v, Deleted:%v, Invalid:%v, LocalFlags:0x%x, NoPermissions:%v, SymlinkTarget:%q, Platform:%v, InodeChangeTime:%v}",
			f.Name, f.Type, f.Sequence, f.Version, f.Deleted, f.IsInvalid(), f.LocalFlags, f.NoPermissions, f.SymlinkTarget, f.Platform, f.InodeChangeTime())
	default:
		panic("mystery file type detected")
	}
}

func (f FileInfo) IsDeleted() bool {
	return f.Deleted
}

func (f FileInfo) IsInvalid() bool {
	return f.LocalFlags.IsInvalid()
}

func (f FileInfo) IsUnsupported() bool {
	return f.LocalFlags&FlagLocalUnsupported != 0
}

func (f FileInfo) IsIgnored() bool {
	return f.LocalFlags&FlagLocalIgnored != 0
}

func (f FileInfo) MustRescan() bool {
	return f.LocalFlags&FlagLocalMustRescan != 0
}

func (f FileInfo) IsReceiveOnlyChanged() bool {
	return f.LocalFlags&FlagLocalReceiveOnly != 0
}

func (f FileInfo) IsDirectory() bool {
	return f.Type == FileInfoTypeDirectory
}

func (f FileInfo) ShouldConflict() bool {
	return f.LocalFlags&LocalConflictFlags != 0
}

func (f FileInfo) IsSymlink() bool {
	switch f.Type {
	case FileInfoTypeSymlink, FileInfoTypeSymlinkDirectory, FileInfoTypeSymlinkFile:
		return true
	default:
		return false
	}
}

func (f FileInfo) HasPermissionBits() bool {
	return !f.NoPermissions
}

func (f FileInfo) FileSize() int64 {
	if f.Deleted {
		return 0
	}
	if f.IsDirectory() || f.IsSymlink() {
		return SyntheticDirectorySize
	}
	return f.Size
}

func (f FileInfo) BlockSize() int {
	if f.RawBlockSize < MinBlockSize {
		return MinBlockSize
	}
	return int(f.RawBlockSize)
}

// BlockSize returns the block size to use for the given file size
func BlockSize(fileSize int64) int {
	var blockSize int
	for _, blockSize = range BlockSizes {
		if fileSize < DesiredPerFileBlocks*int64(blockSize) {
			break
		}
	}

	return blockSize
}

func (f FileInfo) FileName() string {
	return f.Name
}

func (f FileInfo) FileLocalFlags() FlagLocal {
	return f.LocalFlags
}

func (f FileInfo) ModTime() time.Time {
	return time.Unix(f.ModifiedS, int64(f.ModifiedNs))
}

func (f FileInfo) SequenceNo() int64 {
	return f.Sequence
}

func (f FileInfo) FileVersion() Vector {
	return f.Version
}

func (f FileInfo) FileType() FileInfoType {
	return f.Type
}

func (f FileInfo) FilePermissions() uint32 {
	return f.Permissions
}

func (f FileInfo) FileModifiedBy() ShortID {
	return f.ModifiedBy
}

func (f FileInfo) PlatformData() PlatformData {
	return f.Platform
}

func (f FileInfo) InodeChangeTime() time.Time {
	return time.Unix(0, f.InodeChangeNs)
}

func (f FileInfo) FileBlocksHash() []byte {
	return f.BlocksHash
}

type FileInfoComparison struct {
	ModTimeWindow   time.Duration
	IgnorePerms     bool
	IgnoreBlocks    bool
	IgnoreFlags     FlagLocal
	IgnoreOwnership bool
	IgnoreXattrs    bool
}

func (f FileInfo) IsEquivalent(other FileInfo, modTimeWindow time.Duration) bool {
	return f.isEquivalent(other, FileInfoComparison{ModTimeWindow: modTimeWindow})
}

func (f FileInfo) IsEquivalentOptional(other FileInfo, comp FileInfoComparison) bool {
	return f.isEquivalent(other, comp)
}

// isEquivalent checks that the two file infos represent the same actual file content,
// i.e. it does purposely not check only selected (see below) struct members.
// Permissions (config) and blocks (scanning) can be excluded from the comparison.
// Any file info is not "equivalent", if it has different
//   - type
//   - deleted flag
//   - invalid flag
//   - permissions, unless they are ignored
//
// A file is not "equivalent", if it has different
//   - modification time (difference bigger than modTimeWindow)
//   - size
//   - blocks, unless there are no blocks to compare (scanning)
//   - os data
//
// A symlink is not "equivalent", if it has different
//   - target
//
// A directory does not have anything specific to check.
func (f FileInfo) isEquivalent(other FileInfo, comp FileInfoComparison) bool {
	if f.MustRescan() || other.MustRescan() {
		// These are per definition not equivalent because they don't
		// represent a valid state, even if both happen to have the
		// MustRescan bit set.
		return false
	}

	// If we care about either ownership or xattrs, are recording inode change
	// times and it changed, they are not equal.
	if !(comp.IgnoreOwnership && comp.IgnoreXattrs) && f.InodeChangeNs != 0 && other.InodeChangeNs != 0 && f.InodeChangeNs != other.InodeChangeNs {
		return false
	}

	// Mask out the ignored local flags before checking IsInvalid() below
	f.LocalFlags &^= comp.IgnoreFlags
	other.LocalFlags &^= comp.IgnoreFlags

	if f.Name != other.Name || f.Type != other.Type || f.Deleted != other.Deleted || f.IsInvalid() != other.IsInvalid() {
		return false
	}

	if !comp.IgnoreOwnership && f.Platform != other.Platform {
		if !unixOwnershipEqual(f.Platform.Unix, other.Platform.Unix) {
			return false
		}
		if !windowsOwnershipEqual(f.Platform.Windows, other.Platform.Windows) {
			return false
		}
	}
	if !comp.IgnoreXattrs && f.Platform != other.Platform {
		if !xattrsEqual(f.Platform.Linux, other.Platform.Linux) {
			return false
		}
		if !xattrsEqual(f.Platform.Darwin, other.Platform.Darwin) {
			return false
		}
		if !xattrsEqual(f.Platform.FreeBSD, other.Platform.FreeBSD) {
			return false
		}
		if !xattrsEqual(f.Platform.NetBSD, other.Platform.NetBSD) {
			return false
		}
	}

	if !comp.IgnorePerms && !f.NoPermissions && !other.NoPermissions && !PermsEqual(f.Permissions, other.Permissions) {
		return false
	}

	switch f.Type {
	case FileInfoTypeFile:
		return f.Size == other.Size && ModTimeEqual(f.ModTime(), other.ModTime(), comp.ModTimeWindow) && (comp.IgnoreBlocks || f.BlocksEqual(other))
	case FileInfoTypeSymlink:
		return bytes.Equal(f.SymlinkTarget, other.SymlinkTarget)
	case FileInfoTypeDirectory:
		return true
	}

	return false
}

func ModTimeEqual(a, b time.Time, modTimeWindow time.Duration) bool {
	if a.Equal(b) {
		return true
	}
	diff := a.Sub(b)
	if diff < 0 {
		diff *= -1
	}
	return diff < modTimeWindow
}

func PermsEqual(a, b uint32) bool {
	if build.IsWindows {
		// There is only writeable and read only, represented for user, group
		// and other equally. We only compare against user.
		return a&0o600 == b&0o600
	}
	// All bits count
	return a&0o777 == b&0o777
}

// BlocksEqual returns true when the two files have identical block lists.
func (f FileInfo) BlocksEqual(other FileInfo) bool {
	// If both sides have blocks hashes and they match, we are good. If they
	// don't match still check individual block hashes to catch differences
	// in weak hashes only (e.g. after switching weak hash algo).
	if len(f.BlocksHash) > 0 && len(other.BlocksHash) > 0 && bytes.Equal(f.BlocksHash, other.BlocksHash) {
		return true
	}

	// Actually compare the block lists in full.
	return blocksEqual(f.Blocks, other.Blocks)
}

func (f *FileInfo) SetMustRescan() {
	f.setLocalFlags(FlagLocalMustRescan)
}

func (f *FileInfo) SetIgnored() {
	f.setLocalFlags(FlagLocalIgnored)
}

func (f *FileInfo) SetUnsupported() {
	f.setLocalFlags(FlagLocalUnsupported)
}

func (f *FileInfo) SetDeleted(by ShortID) {
	f.ModifiedBy = by
	f.Deleted = true
	f.Version = f.Version.Update(by)
	f.ModifiedS = time.Now().Unix()
	f.setNoContent()
}

func (f *FileInfo) setLocalFlags(flags FlagLocal) {
	f.LocalFlags = flags
	f.setNoContent()
}

func (f *FileInfo) setNoContent() {
	f.Blocks = nil
	f.BlocksHash = nil
	f.Size = 0
}

type BlockInfo struct {
	Hash   []byte
	Offset int64
	Size   int
}

func (b BlockInfo) ToWire() *bep.BlockInfo {
	return &bep.BlockInfo{
		Hash:   b.Hash,
		Offset: b.Offset,
		Size:   int32(b.Size),
	}
}

func BlockInfoFromWire(w *bep.BlockInfo) BlockInfo {
	return BlockInfo{
		Hash:   w.Hash,
		Offset: w.Offset,
		Size:   int(w.Size),
	}
}

func (b BlockInfo) String() string {
	return fmt.Sprintf("Block{%d/%d/%x}", b.Offset, b.Size, b.Hash)
}

// For each block size, the hash of a block of all zeroes
var sha256OfEmptyBlock = map[int][sha256.Size]byte{
	128 << KiB: {0xfa, 0x43, 0x23, 0x9b, 0xce, 0xe7, 0xb9, 0x7c, 0xa6, 0x2f, 0x0, 0x7c, 0xc6, 0x84, 0x87, 0x56, 0xa, 0x39, 0xe1, 0x9f, 0x74, 0xf3, 0xdd, 0xe7, 0x48, 0x6d, 0xb3, 0xf9, 0x8d, 0xf8, 0xe4, 0x71},
	256 << KiB: {0x8a, 0x39, 0xd2, 0xab, 0xd3, 0x99, 0x9a, 0xb7, 0x3c, 0x34, 0xdb, 0x24, 0x76, 0x84, 0x9c, 0xdd, 0xf3, 0x3, 0xce, 0x38, 0x9b, 0x35, 0x82, 0x68, 0x50, 0xf9, 0xa7, 0x0, 0x58, 0x9b, 0x4a, 0x90},
	512 << KiB: {0x7, 0x85, 0x4d, 0x2f, 0xef, 0x29, 0x7a, 0x6, 0xba, 0x81, 0x68, 0x5e, 0x66, 0xc, 0x33, 0x2d, 0xe3, 0x6d, 0x5d, 0x18, 0xd5, 0x46, 0x92, 0x7d, 0x30, 0xda, 0xad, 0x6d, 0x7f, 0xda, 0x15, 0x41},
	1 << MiB:   {0x30, 0xe1, 0x49, 0x55, 0xeb, 0xf1, 0x35, 0x22, 0x66, 0xdc, 0x2f, 0xf8, 0x6, 0x7e, 0x68, 0x10, 0x46, 0x7, 0xe7, 0x50, 0xab, 0xb9, 0xd3, 0xb3, 0x65, 0x82, 0xb8, 0xaf, 0x90, 0x9f, 0xcb, 0x58},
	2 << MiB:   {0x56, 0x47, 0xf0, 0x5e, 0xc1, 0x89, 0x58, 0x94, 0x7d, 0x32, 0x87, 0x4e, 0xeb, 0x78, 0x8f, 0xa3, 0x96, 0xa0, 0x5d, 0xb, 0xab, 0x7c, 0x1b, 0x71, 0xf1, 0x12, 0xce, 0xb7, 0xe9, 0xb3, 0x1e, 0xee},
	4 << MiB:   {0xbb, 0x9f, 0x8d, 0xf6, 0x14, 0x74, 0xd2, 0x5e, 0x71, 0xfa, 0x0, 0x72, 0x23, 0x18, 0xcd, 0x38, 0x73, 0x96, 0xca, 0x17, 0x36, 0x60, 0x5e, 0x12, 0x48, 0x82, 0x1c, 0xc0, 0xde, 0x3d, 0x3a, 0xf8},
	8 << MiB:   {0x2d, 0xae, 0xb1, 0xf3, 0x60, 0x95, 0xb4, 0x4b, 0x31, 0x84, 0x10, 0xb3, 0xf4, 0xe8, 0xb5, 0xd9, 0x89, 0xdc, 0xc7, 0xbb, 0x2, 0x3d, 0x14, 0x26, 0xc4, 0x92, 0xda, 0xb0, 0xa3, 0x5, 0x3e, 0x74},
	16 << MiB:  {0x8, 0xa, 0xcf, 0x35, 0xa5, 0x7, 0xac, 0x98, 0x49, 0xcf, 0xcb, 0xa4, 0x7d, 0xc2, 0xad, 0x83, 0xe0, 0x1b, 0x75, 0x66, 0x3a, 0x51, 0x62, 0x79, 0xc8, 0xb9, 0xd2, 0x43, 0xb7, 0x19, 0x64, 0x3e},
}

// IsEmpty returns true if the block is a full block of zeroes.
func (b BlockInfo) IsEmpty() bool {
	if v, ok := sha256OfEmptyBlock[b.Size]; ok {
		return bytes.Equal(b.Hash, v[:])
	}
	return false
}

func BlocksHash(bs []BlockInfo) []byte {
	h := sha256.New()
	for _, b := range bs {
		_, _ = h.Write(b.Hash)
	}
	return h.Sum(nil)
}

func VectorHash(v Vector) []byte {
	h := sha256.New()
	for _, c := range v.Counters {
		if err := binary.Write(h, binary.BigEndian, c.ID); err != nil {
			panic("impossible: failed to write c.ID to hash function: " + err.Error())
		}
		if err := binary.Write(h, binary.BigEndian, c.Value); err != nil {
			panic("impossible: failed to write c.Value to hash function: " + err.Error())
		}
	}
	return h.Sum(nil)
}

// Xattrs is a convenience method to return the extended attributes of the
// file for the current platform.
func (p *PlatformData) Xattrs() []Xattr {
	switch {
	case build.IsLinux && p.Linux != nil:
		return p.Linux.Xattrs
	case build.IsDarwin && p.Darwin != nil:
		return p.Darwin.Xattrs
	case build.IsFreeBSD && p.FreeBSD != nil:
		return p.FreeBSD.Xattrs
	case build.IsNetBSD && p.NetBSD != nil:
		return p.NetBSD.Xattrs
	default:
		return nil
	}
}

// SetXattrs is a convenience method to set the extended attributes of the
// file for the current platform.
func (p *PlatformData) SetXattrs(xattrs []Xattr) {
	switch {
	case build.IsLinux:
		if p.Linux == nil {
			p.Linux = &XattrData{}
		}
		p.Linux.Xattrs = xattrs

	case build.IsDarwin:
		if p.Darwin == nil {
			p.Darwin = &XattrData{}
		}
		p.Darwin.Xattrs = xattrs

	case build.IsFreeBSD:
		if p.FreeBSD == nil {
			p.FreeBSD = &XattrData{}
		}
		p.FreeBSD.Xattrs = xattrs

	case build.IsNetBSD:
		if p.NetBSD == nil {
			p.NetBSD = &XattrData{}
		}
		p.NetBSD.Xattrs = xattrs
	}
}

// MergeWith copies platform data from other, for platforms where it's not
// already set on p.
func (p *PlatformData) MergeWith(other *PlatformData) {
	if p.Unix == nil {
		p.Unix = other.Unix
	}
	if p.Windows == nil {
		p.Windows = other.Windows
	}
	if p.Linux == nil {
		p.Linux = other.Linux
	}
	if p.Darwin == nil {
		p.Darwin = other.Darwin
	}
	if p.FreeBSD == nil {
		p.FreeBSD = other.FreeBSD
	}
	if p.NetBSD == nil {
		p.NetBSD = other.NetBSD
	}
}

// blocksEqual returns whether two slices of blocks are exactly the same hash
// and index pair wise.
func blocksEqual(a, b []BlockInfo) bool {
	if len(b) != len(a) {
		return false
	}

	for i, sblk := range a {
		if !bytes.Equal(sblk.Hash, b[i].Hash) {
			return false
		}
	}

	return true
}

type PlatformData struct {
	Unix    *UnixData
	Windows *WindowsData
	Linux   *XattrData
	Darwin  *XattrData
	FreeBSD *XattrData
	NetBSD  *XattrData
}

func (p *PlatformData) toWire() *bep.PlatformData {
	return &bep.PlatformData{
		Unix:    p.Unix.toWire(),
		Windows: p.Windows,
		Linux:   p.Linux.toWire(),
		Darwin:  p.Darwin.toWire(),
		Freebsd: p.FreeBSD.toWire(),
		Netbsd:  p.NetBSD.toWire(),
	}
}

func platformDataFromWire(w *bep.PlatformData) PlatformData {
	if w == nil {
		return PlatformData{}
	}
	return PlatformData{
		Unix:    unixDataFromWire(w.Unix),
		Windows: w.Windows,
		Linux:   xattrDataFromWire(w.Linux),
		Darwin:  xattrDataFromWire(w.Darwin),
		FreeBSD: xattrDataFromWire(w.Freebsd),
		NetBSD:  xattrDataFromWire(w.Netbsd),
	}
}

type UnixData struct {
	// The owner name and group name are set when known (i.e., could be
	// resolved on the source device), while the UID and GID are always set
	// as they come directly from the stat() call.
	OwnerName string
	GroupName string
	UID       int
	GID       int
}

func (u *UnixData) toWire() *bep.UnixData {
	if u == nil {
		return nil
	}
	return &bep.UnixData{
		OwnerName: u.OwnerName,
		GroupName: u.GroupName,
		Uid:       int32(u.UID),
		Gid:       int32(u.GID),
	}
}

func unixDataFromWire(w *bep.UnixData) *UnixData {
	if w == nil {
		return nil
	}
	return &UnixData{
		OwnerName: w.OwnerName,
		GroupName: w.GroupName,
		UID:       int(w.Uid),
		GID:       int(w.Gid),
	}
}

type WindowsData = bep.WindowsData

type XattrData struct {
	Xattrs []Xattr
}

func (x *XattrData) toWire() *bep.XattrData {
	if x == nil {
		return nil
	}
	xattrs := make([]*bep.Xattr, len(x.Xattrs))
	for i, a := range x.Xattrs {
		xattrs[i] = a.toWire()
	}
	return &bep.XattrData{
		Xattrs: xattrs,
	}
}

func xattrDataFromWire(w *bep.XattrData) *XattrData {
	if w == nil {
		return nil
	}
	x := &XattrData{}
	x.Xattrs = make([]Xattr, len(w.Xattrs))
	for i, a := range w.Xattrs {
		x.Xattrs[i] = xattrFromWire(a)
	}
	return x
}

type Xattr struct {
	Name  string
	Value []byte
}

func (a Xattr) toWire() *bep.Xattr {
	return &bep.Xattr{
		Name:  a.Name,
		Value: a.Value,
	}
}

func xattrFromWire(w *bep.Xattr) Xattr {
	return Xattr{
		Name:  w.Name,
		Value: w.Value,
	}
}

func xattrsEqual(a, b *XattrData) bool {
	aEmpty := a == nil || len(a.Xattrs) == 0
	bEmpty := b == nil || len(b.Xattrs) == 0
	if aEmpty && bEmpty {
		return true
	}
	if aEmpty || bEmpty {
		// Only one side is empty, so they can't be equal.
		return false
	}
	if len(a.Xattrs) != len(b.Xattrs) {
		return false
	}
	for i := range a.Xattrs {
		if a.Xattrs[i].Name != b.Xattrs[i].Name {
			return false
		}
		if !bytes.Equal(a.Xattrs[i].Value, b.Xattrs[i].Value) {
			return false
		}
	}
	return true
}

func unixOwnershipEqual(a, b *UnixData) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.UID == b.UID && a.GID == b.GID {
		return true
	}
	if a.OwnerName == b.OwnerName && a.GroupName == b.GroupName {
		return true
	}
	return false
}

func windowsOwnershipEqual(a, b *WindowsData) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.OwnerName == b.OwnerName && a.OwnerIsGroup == b.OwnerIsGroup
}
