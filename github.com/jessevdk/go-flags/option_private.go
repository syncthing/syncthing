package flags

import (
	"reflect"
)

// Set the value of an option to the specified value. An error will be returned
// if the specified value could not be converted to the corresponding option
// value type.
func (option *Option) set(value *string) error {
	if option.isFunc() {
		return option.call(value)
	} else if value != nil {
		return convert(*value, option.value, option.tag)
	} else {
		return convert("", option.value, option.tag)
	}

	return nil
}

func (option *Option) canCli() bool {
	return option.ShortName != 0 || len(option.LongName) != 0
}

func (option *Option) canArgument() bool {
	if u := option.isUnmarshaler(); u != nil {
		return true
	}

	return !option.isBool()
}

func (option *Option) clear() {
	tp := option.value.Type()

	switch tp.Kind() {
	case reflect.Func:
		// Skip
	case reflect.Map:
		// Empty the map
		option.value.Set(reflect.MakeMap(tp))
	default:
		zeroval := reflect.Zero(tp)
		option.value.Set(zeroval)
	}
}

func (option *Option) isUnmarshaler() Unmarshaler {
	v := option.value

	for {
		if !v.CanInterface() {
			return nil
		}

		i := v.Interface()

		if u, ok := i.(Unmarshaler); ok {
			return u
		}

		if !v.CanAddr() {
			return nil
		}

		v = v.Addr()
	}

	return nil
}

func (option *Option) isBool() bool {
	tp := option.value.Type()

	for {
		switch tp.Kind() {
		case reflect.Bool:
			return true
		case reflect.Slice:
			return (tp.Elem().Kind() == reflect.Bool)
		case reflect.Func:
			return tp.NumIn() == 0
		case reflect.Ptr:
			tp = tp.Elem()
		default:
			return false
		}
	}

	return false
}

func (option *Option) isFunc() bool {
	return option.value.Type().Kind() == reflect.Func
}

func (option *Option) call(value *string) error {
	var retval []reflect.Value

	if value == nil {
		retval = option.value.Call(nil)
	} else {
		tp := option.value.Type().In(0)

		val := reflect.New(tp)
		val = reflect.Indirect(val)

		if err := convert(*value, val, option.tag); err != nil {
			return err
		}

		retval = option.value.Call([]reflect.Value{val})
	}

	if len(retval) == 1 && retval[0].Type() == reflect.TypeOf((*error)(nil)).Elem() {
		if retval[0].Interface() == nil {
			return nil
		}

		return retval[0].Interface().(error)
	}

	return nil
}
