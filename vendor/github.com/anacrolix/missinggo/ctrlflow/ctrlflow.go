package ctrlflow

import (
	"fmt"
)

type valueWrapper struct {
	value interface{}
}

func (me valueWrapper) String() string {
	return fmt.Sprint(me.value)
}

func Panic(val interface{}) {
	panic(valueWrapper{val})
}

func Recover(handler func(interface{}) bool) {
	r := recover()
	if r == nil {
		return
	}
	if vw, ok := r.(valueWrapper); ok {
		if handler(vw.value) {
			return
		}
	}
	panic(r)
}
