package missinggo

import (
	"reflect"

	"github.com/bradfitz/iter"
)

func ConvertToSliceOfEmptyInterface(slice interface{}) (ret []interface{}) {
	v := reflect.ValueOf(slice)
	l := v.Len()
	ret = make([]interface{}, 0, l)
	for i := range iter.N(v.Len()) {
		ret = append(ret, v.Index(i).Interface())
	}
	return
}

func CastSlice(slicePtr interface{}, fromSlice interface{}) {
	fromSliceValue := reflect.ValueOf(fromSlice)
	fromLen := fromSliceValue.Len()
	if fromLen == 0 {
		return
	}
	// Deref the pointer to slice.
	slicePtrValue := reflect.ValueOf(slicePtr)
	if slicePtrValue.Kind() != reflect.Ptr {
		panic("destination is not a pointer")
	}
	destSliceValue := slicePtrValue.Elem()
	// The type of the elements of the destination slice.
	destSliceElemType := destSliceValue.Type().Elem()
	destSliceValue.Set(reflect.MakeSlice(destSliceValue.Type(), fromLen, fromLen))
	for i := range iter.N(fromSliceValue.Len()) {
		// The value inside the interface in the slice element.
		itemValue := fromSliceValue.Index(i)
		if itemValue.Kind() == reflect.Interface {
			itemValue = itemValue.Elem()
		}
		convertedItem := itemValue.Convert(destSliceElemType)
		destSliceValue.Index(i).Set(convertedItem)
	}
}
