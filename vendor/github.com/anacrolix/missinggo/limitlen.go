package missinggo

import (
	"reflect"
)

// Sets an upper bound on the len of b. max can be any type that will cast to
// int64.
func LimitLen(b *[]byte, max interface{}) {
	_max := reflect.ValueOf(max).Int()
	if int64(len(*b)) > _max {
		*b = (*b)[:_max]
	}
}
