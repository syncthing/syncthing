package itertools

type groupBy struct {
	curKey     interface{}
	curKeyOk   bool
	curValue   interface{}
	keyFunc    func(interface{}) interface{}
	input      Iterator
	groupKey   interface{}
	groupKeyOk bool
}

type Group interface {
	Iterator
	Key() interface{}
}

type group struct {
	gb      *groupBy
	key     interface{}
	first   bool
	stopped bool
}

func (me *group) Stop() {
	me.stopped = true
}

func (me *group) Next() (ok bool) {
	if me.stopped {
		return false
	}
	if me.first {
		me.first = false
		return true
	}
	me.gb.advance()
	if !me.gb.curKeyOk || me.gb.curKey != me.key {
		me.Stop()
		return
	}
	ok = true
	return
}

func (me group) Value() (ret interface{}) {
	if me.stopped {
		panic("iterator stopped")
	}
	ret = me.gb.curValue
	return
}

func (me group) Key() interface{} {
	return me.key
}

func (me *groupBy) advance() {
	me.curKeyOk = me.input.Next()
	if me.curKeyOk {
		me.curValue = me.input.Value()
		me.curKey = me.keyFunc(me.curValue)
	}
}

func (me *groupBy) Next() (ok bool) {
	for me.curKey == me.groupKey {
		ok = me.input.Next()
		if !ok {
			return
		}
		me.curValue = me.input.Value()
		me.curKey = me.keyFunc(me.curValue)
		me.curKeyOk = true
	}
	me.groupKey = me.curKey
	me.groupKeyOk = true
	return true
}

func (me *groupBy) Value() (ret interface{}) {
	return &group{me, me.groupKey, true, false}
}

func (me *groupBy) Stop() {
}

// Allows use of nil as a return from the key func.
var uniqueKey = new(int)

// Group by returns an iterator of iterators over the values of the input
// iterator that consecutively return the same value when input to the key
// function. Note that repeated calls to each value of the GroupBy Iterator
// does not return a new iterator over the values for that key.
func GroupBy(input Iterator, keyFunc func(interface{}) interface{}) Iterator {
	if keyFunc == nil {
		keyFunc = func(a interface{}) interface{} { return a }
	}
	return &groupBy{
		input:    input,
		keyFunc:  keyFunc,
		groupKey: uniqueKey,
		curKey:   uniqueKey,
	}
}
