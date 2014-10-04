// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

package protocol

import "fmt"

type IndexMessage struct {
	Folder string // max:64
	Files  []FileInfo
}

type FileInfo struct {
	Name         string // max:8192
	Flags        uint32
	Modified     int64
	Version      uint64
	LocalVersion uint64
	Blocks       []BlockInfo
}

func (f FileInfo) String() string {
	return fmt.Sprintf("File{Name:%q, Flags:0%o, Modified:%d, Version:%d, Size:%d, Blocks:%v}",
		f.Name, f.Flags, f.Modified, f.Version, f.Size(), f.Blocks)
}

func (f FileInfo) Size() (bytes int64) {
	if IsDeleted(f.Flags) || IsDirectory(f.Flags) {
		return 128
	}
	for _, b := range f.Blocks {
		bytes += int64(b.Size)
	}
	return
}

func (f FileInfo) IsDeleted() bool {
	return IsDeleted(f.Flags)
}

func (f FileInfo) IsInvalid() bool {
	return IsInvalid(f.Flags)
}

// Used for unmarshalling a FileInfo structure but skipping the actual block list
type FileInfoTruncated struct {
	Name         string // max:8192
	Flags        uint32
	Modified     int64
	Version      uint64
	LocalVersion uint64
	NumBlocks    uint32
}

// Returns a statistical guess on the size, not the exact figure
func (f FileInfoTruncated) Size() int64 {
	if IsDeleted(f.Flags) || IsDirectory(f.Flags) {
		return 128
	}
	if f.NumBlocks < 2 {
		return BlockSize / 2
	} else {
		return int64(f.NumBlocks-1)*BlockSize + BlockSize/2
	}
}

func (f FileInfoTruncated) IsDeleted() bool {
	return IsDeleted(f.Flags)
}

func (f FileInfoTruncated) IsInvalid() bool {
	return IsInvalid(f.Flags)
}

type FileIntf interface {
	Size() int64
	IsDeleted() bool
	IsInvalid() bool
}

type BlockInfo struct {
	Offset int64 // noencode (cache only)
	Size   uint32
	Hash   []byte // max:64
}

func (b BlockInfo) String() string {
	return fmt.Sprintf("Block{%d/%d/%x}", b.Offset, b.Size, b.Hash)
}

type RequestMessage struct {
	Folder string // max:64
	Name   string // max:8192
	Offset uint64
	Size   uint32
}

type ResponseMessage struct {
	Data []byte
}

type ClusterConfigMessage struct {
	ClientName    string   // max:64
	ClientVersion string   // max:64
	Folders       []Folder // max:64
	Options       []Option // max:64
}

func (o *ClusterConfigMessage) GetOption(key string) string {
	for _, option := range o.Options {
		if option.Key == key {
			return option.Value
		}
	}
	return ""
}

type Folder struct {
	ID      string   // max:64
	Devices []Device // max:64
}

type Device struct {
	ID              []byte // max:32
	Flags           uint32
	MaxLocalVersion uint64
}

type Option struct {
	Key   string // max:64
	Value string // max:1024
}

type CloseMessage struct {
	Reason string // max:1024
}

type EmptyMessage struct{}
