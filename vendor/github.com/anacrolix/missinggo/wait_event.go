package missinggo

import (
	"reflect"
	"sync"
)

func WaitEvents(l sync.Locker, evs ...*Event) {
	cases := make([]reflect.SelectCase, 0, len(evs))
	for _, ev := range evs {
		cases = append(cases, reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(ev.C()),
		})
	}
	l.Unlock()
	reflect.Select(cases)
	l.Lock()
}
