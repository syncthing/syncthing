package missinggo

import (
	"reflect"
)

type MapKeyValue struct {
	Key, Value interface{}
}

// Creates a []struct{Key K; Value V} for map[K]V.
func MapAsSlice(m interface{}) (slice []MapKeyValue) {
	mapValue := reflect.ValueOf(m)
	for _, key := range mapValue.MapKeys() {
		slice = append(slice, MapKeyValue{key.Interface(), mapValue.MapIndex(key).Interface()})
	}
	return
}
