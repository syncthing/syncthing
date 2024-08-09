package own_fuse

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/syncthing/syncthing/lib/protocol"
)

type OwnFuseFilesystem struct {
}

func New() OwnFuseFilesystem {

	server, err := fs.Mount(mnt, root, nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("zip file mounted")
	fmt.Printf("to unmount: fusermount -u %s\n", mnt)
	server.Wait()

	return OwnFuseFilesystem{}
}

func (*OwnFuseFilesystem) Chmod(name string, mode FileMode) error {

}
func (*OwnFuseFilesystem) Lchown(name string, uid, gid string) error // uid/gid as strings; numeric on POSIX, SID on Windows, like in os/user package
func (*OwnFuseFilesystem) Chtimes(name string, atime time.Time, mtime time.Time) error
func (*OwnFuseFilesystem) Create(name string) (File, error)
func (*OwnFuseFilesystem) CreateSymlink(target, name string) error
func (*OwnFuseFilesystem) DirNames(name string) ([]string, error)
func (*OwnFuseFilesystem) Lstat(name string) (FileInfo, error)
func (*OwnFuseFilesystem) Mkdir(name string, perm FileMode) error
func (*OwnFuseFilesystem) MkdirAll(name string, perm FileMode) error
func (*OwnFuseFilesystem) Open(name string) (File, error)
func (*OwnFuseFilesystem) OpenFile(name string, flags int, mode FileMode) (File, error)
func (*OwnFuseFilesystem) ReadSymlink(name string) (string, error)
func (*OwnFuseFilesystem) Remove(name string) error
func (*OwnFuseFilesystem) RemoveAll(name string) error
func (*OwnFuseFilesystem) Rename(oldname, newname string) error
func (*OwnFuseFilesystem) Stat(name string) (FileInfo, error)
func (*OwnFuseFilesystem) SymlinksSupported() bool
func (*OwnFuseFilesystem) Walk(name string, walkFn WalkFunc) error

// If setup fails, returns non-nil error, and if afterwards a fatal (!)
// error occurs, sends that error on the channel. Afterwards this watch
// can be considered stopped.
func (*OwnFuseFilesystem) Watch(path string, ignore Matcher, ctx context.Context, ignorePerms bool) (<-chan Event, <-chan error, error)
func (*OwnFuseFilesystem) Hide(name string) error
func (*OwnFuseFilesystem) Unhide(name string) error
func (*OwnFuseFilesystem) Glob(pattern string) ([]string, error)
func (*OwnFuseFilesystem) Roots() ([]string, error)
func (*OwnFuseFilesystem) Usage(name string) (Usage, error)
func (*OwnFuseFilesystem) Type() FilesystemType
func (*OwnFuseFilesystem) URI() string
func (*OwnFuseFilesystem) Options() []Option
func (*OwnFuseFilesystem) SameFile(fi1, fi2 FileInfo) bool
func (*OwnFuseFilesystem) PlatformData(name string, withOwnership, withXattrs bool, xattrFilter XattrFilter) (protocol.PlatformData, error)
func (*OwnFuseFilesystem) GetXattr(name string, xattrFilter XattrFilter) ([]protocol.Xattr, error)
func (*OwnFuseFilesystem) SetXattr(path string, xattrs []protocol.Xattr, xattrFilter XattrFilter) error

// Used for unwrapping things
func (*OwnFuseFilesystem) underlying() (Filesystem, bool)
func (*OwnFuseFilesystem) wrapperType() filesystemWrapperType
