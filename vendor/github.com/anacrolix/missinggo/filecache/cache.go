package filecache

import (
	"errors"
	"log"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/anacrolix/missinggo"
	"github.com/anacrolix/missinggo/pproffd"
	"github.com/anacrolix/missinggo/resource"
)

const (
	dirPerm  = 0755
	filePerm = 0644
)

type Cache struct {
	mu       sync.Mutex
	capacity int64
	filled   int64
	items    *lruItems
	paths    map[string]ItemInfo
	root     string
}

type CacheInfo struct {
	Capacity int64
	Filled   int64
	NumItems int
}

type ItemInfo struct {
	Accessed time.Time
	Size     int64
	Path     string
}

// Calls the function for every item known to the cache. The ItemInfo should
// not be modified.
func (me *Cache) WalkItems(cb func(ItemInfo)) {
	me.mu.Lock()
	defer me.mu.Unlock()
	for e := me.items.Front(); e != nil; e = e.Next() {
		cb(e.Value().(ItemInfo))
	}
}

func (me *Cache) Info() (ret CacheInfo) {
	me.mu.Lock()
	defer me.mu.Unlock()
	ret.Capacity = me.capacity
	ret.Filled = me.filled
	ret.NumItems = len(me.paths)
	return
}

// Setting a negative capacity means unlimited.
func (me *Cache) SetCapacity(capacity int64) {
	me.mu.Lock()
	defer me.mu.Unlock()
	me.capacity = capacity
}

func NewCache(root string) (ret *Cache, err error) {
	root, err = filepath.Abs(root)
	ret = &Cache{
		root:     root,
		capacity: -1, // unlimited
	}
	ret.mu.Lock()
	go func() {
		defer ret.mu.Unlock()
		ret.rescan()
	}()
	return
}

// An empty return path is an error.
func sanitizePath(p string) (ret string) {
	if p == "" {
		return
	}
	ret = path.Clean("/" + p)
	if ret[0] == '/' {
		ret = ret[1:]
	}
	return
}

// Leaf is a descendent of root.
func pruneEmptyDirs(root string, leaf string) (err error) {
	rootInfo, err := os.Stat(root)
	if err != nil {
		return
	}
	for {
		var leafInfo os.FileInfo
		leafInfo, err = os.Stat(leaf)
		if os.IsNotExist(err) {
			goto parent
		}
		if err != nil {
			return
		}
		if !leafInfo.IsDir() {
			return
		}
		if os.SameFile(rootInfo, leafInfo) {
			return
		}
		if os.Remove(leaf) != nil {
			return
		}
	parent:
		leaf = filepath.Dir(leaf)
	}
}

func (me *Cache) Remove(path string) (err error) {
	path = sanitizePath(path)
	me.mu.Lock()
	defer me.mu.Unlock()
	err = me.remove(path)
	return
}

var (
	ErrBadPath = errors.New("bad path")
	ErrIsDir   = errors.New("is directory")
)

func (me *Cache) StatFile(path string) (os.FileInfo, error) {
	return os.Stat(me.realpath(sanitizePath(path)))
}

func (me *Cache) OpenFile(path string, flag int) (ret *File, err error) {
	path = sanitizePath(path)
	if path == "" {
		err = ErrIsDir
		return
	}
	f, err := os.OpenFile(me.realpath(path), flag, filePerm)
	if flag&os.O_CREATE != 0 && os.IsNotExist(err) {
		os.MkdirAll(me.root, dirPerm)
		os.MkdirAll(filepath.Dir(me.realpath(path)), dirPerm)
		f, err = os.OpenFile(me.realpath(path), flag, filePerm)
		if err != nil {
			me.pruneEmptyDirs(path)
		}
	}
	if err != nil {
		return
	}
	ret = &File{
		c:    me,
		path: path,
		f:    pproffd.WrapOSFile(f),
	}
	me.mu.Lock()
	go func() {
		defer me.mu.Unlock()
		me.statItem(path, time.Now())
	}()
	return
}

func (me *Cache) rescan() {
	me.filled = 0
	me.items = newLRUItems()
	me.paths = make(map[string]ItemInfo)
	err := filepath.Walk(me.root, func(path string, info os.FileInfo, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		path, err = filepath.Rel(me.root, path)
		if err != nil {
			log.Print(err)
			return nil
		}
		me.statItem(path, time.Time{})
		return nil
	})
	if err != nil {
		panic(err)
	}
}

func (me *Cache) insertItem(i ItemInfo) {
	me.items.Insert(i)
}

func (me *Cache) removeInfo(path string) (ret ItemInfo, ok bool) {
	ret, ok = me.paths[path]
	if !ok {
		return
	}
	if !me.items.Remove(ret) {
		panic(ret)
	}
	me.filled -= ret.Size
	delete(me.paths, path)
	return
}

// Triggers the item for path to be updated. If access is non-zero, set the
// item's access time to that value, otherwise deduce it appropriately.
func (me *Cache) statItem(path string, access time.Time) {
	info, ok := me.removeInfo(path)
	fi, err := os.Stat(me.realpath(path))
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		panic(err)
	}
	if !ok {
		info.Path = path
	}
	if !access.IsZero() {
		info.Accessed = access
	}
	if info.Accessed.IsZero() {
		info.Accessed = missinggo.FileInfoAccessTime(fi)
	}
	info.Size = fi.Size()
	me.filled += info.Size
	me.insertItem(info)
	me.paths[path] = info
}

func (me *Cache) realpath(path string) string {
	return filepath.Join(me.root, filepath.FromSlash(sanitizePath(path)))
}

func (me *Cache) TrimToCapacity() {
	me.mu.Lock()
	defer me.mu.Unlock()
	me.trimToCapacity()
}

func (me *Cache) pruneEmptyDirs(path string) {
	pruneEmptyDirs(me.root, me.realpath(path))
}

func (me *Cache) remove(path string) (err error) {
	err = os.Remove(me.realpath(path))
	if os.IsNotExist(err) {
		err = nil
	}
	me.pruneEmptyDirs(path)
	me.removeInfo(path)
	return
}

func (me *Cache) trimToCapacity() {
	if me.capacity < 0 {
		return
	}
	for me.filled > me.capacity {
		item := me.items.LRU()
		me.remove(item.Path)
	}
}

func (me *Cache) pathInfo(p string) ItemInfo {
	return me.paths[p]
}

func (me *Cache) Rename(from, to string) (err error) {
	_from := sanitizePath(from)
	_to := sanitizePath(to)
	me.mu.Lock()
	defer me.mu.Unlock()
	err = os.MkdirAll(filepath.Dir(me.realpath(_to)), dirPerm)
	if err != nil {
		return
	}
	err = os.Rename(me.realpath(_from), me.realpath(_to))
	if err != nil {
		return
	}
	me.removeInfo(_from)
	me.pruneEmptyDirs(_from)
	me.statItem(_to, time.Now())
	return
}

func (me *Cache) Stat(path string) (os.FileInfo, error) {
	return os.Stat(me.realpath(path))
}

func (me *Cache) AsFileStore() missinggo.FileStore {
	return fileStore{me}
}

func (me *Cache) AsResourceProvider() resource.Provider {
	return &uniformResourceProvider{me}
}
