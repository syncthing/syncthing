package orderedmap

import "github.com/anacrolix/missinggo/itertools"

func New(lesser func(l, r interface{}) bool) OrderedMap {
	return NewGoogleBTree(lesser)
}

type OrderedMap interface {
	Get(key interface{}) interface{}
	GetOk(key interface{}) (interface{}, bool)
	itertools.Iterable
	Set(key, value interface{})
	Unset(key interface{})
	Len() int
}
