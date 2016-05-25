package missinggo

import "reflect"

// A sort-of implementation of single dispatch polymorphic inheritance.
type Inheriter interface {
	// Return the base-class object value.
	Base() interface{}
	// Return a pointer to an interface containing the methods retrievable
	// from the base.
	Visible() interface{}
}

// Obtains the desired value pointed to by dest, from source. If it's not
// found in source, it will walk source's inheritance values until either
// blocked by visibility, an implementation is found, or there are no further
// bases. TODO: If an implementation is obtained from a base class that
// contains some methods present on a derived class, this might break
// encapsulation. Maybe there should be a check to ensure this isn't possible.
func Dispatch(dest, source interface{}) bool {
	destValue := reflect.ValueOf(dest).Elem()
	destType := destValue.Type()
	sourceValue := reflect.ValueOf(source)
	sourceType := sourceValue.Type()
	// Try the source first.
	if implements(destType, sourceType) {
		destValue.Set(sourceValue)
		return true
	}
	class, ok := source.(Inheriter)
	if !ok {
		// No bases, give up.
		return false
	}
	if !visible(destType, reflect.TypeOf(class.Visible()).Elem()) {
		// The desired type is masked by the visibility.
		return false
	}
	// Try again from the base class.
	return Dispatch(dest, class.Base())
}

func visible(wanted reflect.Type, mask reflect.Type) bool {
	return mask.ConvertibleTo(wanted)
}

func implements(wanted, source reflect.Type) bool {
	return source.ConvertibleTo(wanted)
}
