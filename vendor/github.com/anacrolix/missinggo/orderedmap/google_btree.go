package orderedmap

import "github.com/google/btree"

type GoogleBTree struct {
	bt     *btree.BTree
	lesser func(l, r interface{}) bool
}

type googleBTreeItem struct {
	less  func(l, r interface{}) bool
	key   interface{}
	value interface{}
}

func (me googleBTreeItem) Less(right btree.Item) bool {
	return me.less(me.key, right.(*googleBTreeItem).key)
}

func NewGoogleBTree(lesser func(l, r interface{}) bool) *GoogleBTree {
	return &GoogleBTree{
		bt:     btree.New(32),
		lesser: lesser,
	}
}

func (me *GoogleBTree) Set(key interface{}, value interface{}) {
	me.bt.ReplaceOrInsert(&googleBTreeItem{me.lesser, key, value})
}

func (me *GoogleBTree) Get(key interface{}) interface{} {
	ret, _ := me.GetOk(key)
	return ret
}

func (me *GoogleBTree) GetOk(key interface{}) (interface{}, bool) {
	item := me.bt.Get(&googleBTreeItem{me.lesser, key, nil})
	if item == nil {
		return nil, false
	}
	return item.(*googleBTreeItem).value, true
}

type googleBTreeIter struct {
	i  btree.Item
	bt *btree.BTree
}

func (me *googleBTreeIter) Next() bool {
	if me.bt == nil {
		return false
	}
	if me.i == nil {
		me.bt.Ascend(func(i btree.Item) bool {
			me.i = i
			return false
		})
	} else {
		var n int
		me.bt.AscendGreaterOrEqual(me.i, func(i btree.Item) bool {
			n++
			if n == 1 {
				return true
			}
			me.i = i
			return false
		})
		if n != 2 {
			me.i = nil
		}
	}
	return me.i != nil
}

func (me *googleBTreeIter) Value() interface{} {
	return me.i.(*googleBTreeItem).value
}

func (me *googleBTreeIter) Stop() {
	me.bt = nil
	me.i = nil
}

func (me *GoogleBTree) Iter(f func(value interface{}) bool) {
	me.bt.Ascend(func(i btree.Item) bool {
		return f(i.(*googleBTreeItem).value)
	})
}

func (me *GoogleBTree) Unset(key interface{}) {
	me.bt.Delete(&googleBTreeItem{me.lesser, key, nil})
}

func (me *GoogleBTree) Len() int {
	return me.bt.Len()
}
