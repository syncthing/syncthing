package missinggo

import (
	"bytes"
	"expvar"
	"fmt"
)

type IndentMap struct {
	expvar.Map
}

var _ expvar.Var = &IndentMap{}

func NewExpvarIndentMap(name string) *IndentMap {
	v := new(IndentMap)
	v.Init()
	expvar.Publish(name, v)
	return v
}

func (v *IndentMap) String() string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "{")
	first := true
	v.Do(func(kv expvar.KeyValue) {
		if !first {
			fmt.Fprintf(&b, ",")
		}
		fmt.Fprintf(&b, "\n\t%q: %v", kv.Key, kv.Value)
		first = false
	})
	fmt.Fprintf(&b, "}")
	return b.String()
}
