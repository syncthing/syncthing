// Copyright (C) 2014 The Protocol Authors.

package protocol

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/sha256"
)

const (
	SyntheticDirectorySize        = 128
	HelloMessageMagic      uint32 = 0x2EA7D90B
	Version13HelloMagic    uint32 = 0x9F79BC40 // old
)

var (
	operatingSystem OS
)

func init() {
	switch runtime.GOOS {
	case "linux":
		operatingSystem = OsLinux
	case "windows":
		operatingSystem = OsWindows
	case "darwin":
		operatingSystem = OsMacos
	case "freebsd":
		operatingSystem = OsFreebsd
	case "netbsd":
		operatingSystem = OsNetbsd
	case "openbsd":
		operatingSystem = OsOpenbsd
	case "solaris", "illumos":
		operatingSystem = OsSolaris
	}
}

// FileIntf is the set of methods implemented by both FileInfo and
// db.FileInfoTruncated.
type FileIntf interface {
	FileSize() int64
	FileName() string
	FileLocalFlags() uint32
	IsDeleted() bool
	IsInvalid() bool
	IsIgnored() bool
	IsUnsupported() bool
	MustRescan() bool
	IsReceiveOnlyChanged() bool
	IsDirectory() bool
	IsSymlink() bool
	ShouldConflict() bool
	HasPermissionBits() bool
	SequenceNo() int64
	BlockSize() int
	FileVersion() Vector
	FileType() FileInfoType
	FilePermissions() uint32
	FileModifiedBy() ShortID
	ModTime() time.Time
	LoadOSData(os OS, dst interface{ Unmarshal([]byte) error }) bool
}

func (m Hello) Magic() uint32 {
	return HelloMessageMagic
}

func (f FileInfo) String() string {
	switch f.Type {
	case FileInfoTypeDirectory:
		return fmt.Sprintf("Directory{Name:%q, Sequence:%d, Permissions:0%o, ModTime:%v, Version:%v, VersionHash:%x, Deleted:%v, Invalid:%v, LocalFlags:0x%x, NoPermissions:%v, OSData:%s}",
			f.Name, f.Sequence, f.Permissions, f.ModTime(), f.Version, f.VersionHash, f.Deleted, f.RawInvalid, f.LocalFlags, f.NoPermissions, f.osDataString())
	case FileInfoTypeFile:
		return fmt.Sprintf("File{Name:%q, Sequence:%d, Permissions:0%o, ModTime:%v, Version:%v, VersionHash:%x, Length:%d, Deleted:%v, Invalid:%v, LocalFlags:0x%x, NoPermissions:%v, BlockSize:%d, Blocks:%v, BlocksHash:%x, OSData:%s}",
			f.Name, f.Sequence, f.Permissions, f.ModTime(), f.Version, f.VersionHash, f.Size, f.Deleted, f.RawInvalid, f.LocalFlags, f.NoPermissions, f.RawBlockSize, f.Blocks, f.BlocksHash, f.osDataString())
	case FileInfoTypeSymlink, FileInfoTypeSymlinkDirectory, FileInfoTypeSymlinkFile:
		return fmt.Sprintf("Symlink{Name:%q, Type:%v, Sequence:%d, Version:%v, VersionHash:%x, Deleted:%v, Invalid:%v, LocalFlags:0x%x, NoPermissions:%v, SymlinkTarget:%q, OSData:%s}",
			f.Name, f.Type, f.Sequence, f.Version, f.VersionHash, f.Deleted, f.RawInvalid, f.LocalFlags, f.NoPermissions, f.SymlinkTarget, f.osDataString())
	default:
		panic("mystery file type detected")
	}
}

func (f FileInfo) osDataString() string {
	var parts []string
	if bs, ok := f.OSData[OsPosix]; ok {
		var pd POSIXOSData
		if err := pd.Unmarshal(bs); err == nil {
			parts = append(parts, fmt.Sprintf("Posix{%v}", &pd))
		}
	}
	if bs, ok := f.OSData[OsWindows]; ok {
		var pd WindowsOSData
		if err := pd.Unmarshal(bs); err == nil {
			parts = append(parts, fmt.Sprintf("Windows{%v}", &pd))
		}
	}
	return strings.Join(parts, ",")
}

func (f FileInfo) IsDeleted() bool {
	return f.Deleted
}

func (f FileInfo) IsInvalid() bool {
	return f.RawInvalid || f.LocalFlags&LocalInvalidFlags != 0
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
	return f.RawBlockSize
}

func (f FileInfo) FileName() string {
	return f.Name
}

func (f FileInfo) FileLocalFlags() uint32 {
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

func (f FileInfo) LoadOSData(os OS, dst interface{ Unmarshal([]byte) error }) bool {
	// TODO(jb): This is a candidate for generics when we adopt Go 1.18;
	// LoadOSData[T Unmarshaller](os OS, fi FileInfo) (T, bool) so the
	// caller doesn't need to pre-allocate the struct but will have to
	// specify the type instead.
	bs, ok := f.OSData[os]
	if !ok {
		return false
	}
	return dst.Unmarshal(bs) == nil
}

// WinsConflict returns true if "f" is the one to choose when it is in
// conflict with "other".
func WinsConflict(f, other FileIntf) bool {
	// If only one of the files is invalid, that one loses.
	if f.IsInvalid() != other.IsInvalid() {
		return !f.IsInvalid()
	}

	// If a modification is in conflict with a delete, we pick the
	// modification.
	if !f.IsDeleted() && other.IsDeleted() {
		return true
	}
	if f.IsDeleted() && !other.IsDeleted() {
		return false
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

type FileInfoComparison struct {
	ModTimeWindow   time.Duration
	IgnorePerms     bool
	IgnoreBlocks    bool
	IgnoreFlags     uint32
	IgnoreOwnership bool
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
//  - type
//  - deleted flag
//  - invalid flag
//  - permissions, unless they are ignored
// A file is not "equivalent", if it has different
//  - modification time (difference bigger than modTimeWindow)
//  - size
//  - blocks, unless there are no blocks to compare (scanning)
//  - os data
// A symlink is not "equivalent", if it has different
//  - target
// A directory does not have anything specific to check.
func (f FileInfo) isEquivalent(other FileInfo, comp FileInfoComparison) bool {
	if f.MustRescan() || other.MustRescan() {
		// These are per definition not equivalent because they don't
		// represent a valid state, even if both happen to have the
		// MustRescan bit set.
		return false
	}

	// Mask out the ignored local flags before checking IsInvalid() below
	f.LocalFlags &^= comp.IgnoreFlags
	other.LocalFlags &^= comp.IgnoreFlags

	if f.Name != other.Name || f.Type != other.Type || f.Deleted != other.Deleted || f.IsInvalid() != other.IsInvalid() {
		return false
	}

	// OS data comparison is special: we consider a difference only if an
	// entry for the same OS exists on both sides and they are different.
	// Otherwise a file would become different as soon as it's synced from
	// Windows to Linux, as Linux would add a new POSIX entry for the file.
	//
	// XXX: Technically, the serialized form of protobuf messages isn't
	// guaranteed to be stable. In practice it is, and this is a much easier
	// comparison than deserializing each thing.
	if !comp.IgnoreOwnership {
		for os, bs := range f.OSData {
			if otherBs, ok := other.OSData[os]; ok && !bytes.Equal(bs, otherBs) {
				return false
			}
		}
	}

	if !comp.IgnorePerms && !f.NoPermissions && !other.NoPermissions && !PermsEqual(f.Permissions, other.Permissions) {
		return false
	}

	switch f.Type {
	case FileInfoTypeFile:
		return f.Size == other.Size && ModTimeEqual(f.ModTime(), other.ModTime(), comp.ModTimeWindow) && (comp.IgnoreBlocks || f.BlocksEqual(other))
	case FileInfoTypeSymlink:
		return f.SymlinkTarget == other.SymlinkTarget
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
	switch runtime.GOOS {
	case "windows":
		// There is only writeable and read only, represented for user, group
		// and other equally. We only compare against user.
		return a&0600 == b&0600
	default:
		// All bits count
		return a&0777 == b&0777
	}
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

func (f *FileInfo) setLocalFlags(flags uint32) {
	f.RawInvalid = false
	f.LocalFlags = flags
	f.setNoContent()
}

func (f *FileInfo) setNoContent() {
	f.Blocks = nil
	f.BlocksHash = nil
	f.Size = 0
}

func (b BlockInfo) String() string {
	return fmt.Sprintf("Block{%d/%d/%d/%x}", b.Offset, b.Size, b.WeakHash, b.Hash)
}

// IsEmpty returns true if the block is a full block of zeroes.
func (b BlockInfo) IsEmpty() bool {
	if v, ok := sha256OfEmptyBlock[int(b.Size)]; ok {
		return bytes.Equal(b.Hash, v[:])
	}
	return false
}

type IndexID uint64

func (i IndexID) String() string {
	return fmt.Sprintf("0x%016X", uint64(i))
}

func (i IndexID) Marshal() ([]byte, error) {
	bs := make([]byte, 8)
	binary.BigEndian.PutUint64(bs, uint64(i))
	return bs, nil
}

func (i *IndexID) Unmarshal(bs []byte) error {
	if len(bs) != 8 {
		return errors.New("incorrect IndexID length")
	}
	*i = IndexID(binary.BigEndian.Uint64(bs))
	return nil
}

func NewIndexID() IndexID {
	return IndexID(rand.Uint64())
}

func (f Folder) Description() string {
	// used by logging stuff
	if f.Label == "" {
		return f.ID
	}
	return fmt.Sprintf("%q (%s)", f.Label, f.ID)
}

func BlocksHash(bs []BlockInfo) []byte {
	h := sha256.New()
	for _, b := range bs {
		_, _ = h.Write(b.Hash)
		_ = binary.Write(h, binary.BigEndian, b.WeakHash)
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

func (x *FileInfoType) MarshalJSON() ([]byte, error) {
	return json.Marshal(x.String())
}

func (x *FileInfoType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	n, ok := FileInfoType_value[s]
	if !ok {
		return errors.New("invalid value: " + s)
	}
	*x = FileInfoType(n)
	return nil
}
