package model

import (
	"context"
	"syscall"

	ffs "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/syncthing/syncthing/lib/db"
)

type SyncthingVirtualFolderAccessI interface {
	getInoOf(path string) uint64
	lookupFile(path string) (info *db.FileInfoTruncated, eno syscall.Errno)
	readDir(path string) (stream ffs.DirStream, eno syscall.Errno)
	readFile(path string, buf []byte, off int64) (res fuse.ReadResult, errno syscall.Errno)
	createFile(Permissions *uint32, path string) (info *db.FileInfoTruncated, eno syscall.Errno)
	writeFile(ctx context.Context, path string, offset uint64, inputData []byte) syscall.Errno
	deleteFile(ctx context.Context, path string) syscall.Errno
	createDir(ctx context.Context, path string) syscall.Errno
	deleteDir(ctx context.Context, path string) syscall.Errno
	renameFileOrDir(ctx context.Context, existingPath string, newPath string) syscall.Errno
	renameExchangeFileOrDir(ctx context.Context, path1 string, path2 string) syscall.Errno
	createSymlink(ctx context.Context, path, target string) syscall.Errno
}
