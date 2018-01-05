package binutil

import (
	"errors"
	"fmt"
	"path"
	"reflect"
)

var (
	WALK_SKIP = errors.New("")
)

type Walker func(v reflect.Value, path string) error

func Walk(value interface{}, walker Walker) error {
	err := walk(reflect.ValueOf(value), "/", walker)
	if err == WALK_SKIP {
		err = nil
	}
	return err
}

func stopping(err error) bool {
	return err != nil && err != WALK_SKIP
}

func walk(v reflect.Value, spath string, walker Walker) error {
	err := walker(v, spath)
	if err != nil {
		return err
	}
	v = reflect.Indirect(v)
	switch v.Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			err = walk(v.Index(i), spath+fmt.Sprintf("[%d]", i), walker)
			if stopping(err) {
				return err
			}
		}
	case reflect.Interface:
		err = walk(v.Elem(), spath, walker)
		if stopping(err) {
			return err
		}
	case reflect.Struct:
		//t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			//f := t.Field(i) //TODO: handle unexported fields
			vv := v.Field(i)
			err = walk(vv, path.Join(spath, v.Type().Field(i).Name), walker)
			if stopping(err) {
				return err
			}
		}
	default:
		// FIXME: handle other special cases too
		// String
		return nil
	}
	return nil
}
