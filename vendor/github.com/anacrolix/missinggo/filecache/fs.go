package filecache

import "github.com/anacrolix/missinggo"

type fileStore struct {
	*Cache
}

func (me fileStore) OpenFile(p string, f int) (missinggo.File, error) {
	return me.Cache.OpenFile(p, f)
}
