package missinggo

import "reflect"

// Returns whether the value represents the empty value for its type. Used for
// example to determine if complex types satisfy the common "omitempty" tag
// option for marshalling. Taken from
// http://stackoverflow.com/a/23555352/149482.
func IsEmptyValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Func, reflect.Map, reflect.Slice:
		return v.IsNil()
	case reflect.Array:
		z := true
		for i := 0; i < v.Len(); i++ {
			z = z && IsEmptyValue(v.Index(i))
		}
		return z
	case reflect.Struct:
		z := true
		for i := 0; i < v.NumField(); i++ {
			z = z && IsEmptyValue(v.Field(i))
		}
		return z
	}
	// Compare other types directly:
	z := reflect.Zero(v.Type())
	return v.Interface() == z.Interface()
}
