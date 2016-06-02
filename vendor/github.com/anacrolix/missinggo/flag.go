package missinggo

import "sync"

// Flag represents a boolean value, that signals sync.Cond's when it changes.
// It's not concurrent safe by intention.
type Flag struct {
	Conds map[*sync.Cond]struct{}
	value bool
}

func (me *Flag) Set(value bool) {
	if value != me.value {
		me.broadcastChange()
	}
	me.value = value
}

func (me *Flag) Get() bool {
	return me.value
}

func (me *Flag) broadcastChange() {
	for cond := range me.Conds {
		cond.Broadcast()
	}
}

func (me *Flag) addCond(c *sync.Cond) {
	if me.Conds == nil {
		me.Conds = make(map[*sync.Cond]struct{})
	}
	me.Conds[c] = struct{}{}
}

// Adds the sync.Cond to all the given Flag's.
func AddCondToFlags(cond *sync.Cond, flags ...*Flag) {
	for _, f := range flags {
		f.addCond(cond)
	}
}
