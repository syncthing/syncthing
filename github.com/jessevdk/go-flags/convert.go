// Copyright 2012 Jesse van den Kieboom. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package flags

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Marshaler is the interface implemented by types that can marshal themselves
// to a string representation of the flag.
type Marshaler interface {
	// MarshalFlag marshals a flag value to its string representation.
	MarshalFlag() (string, error)
}

// Unmarshaler is the interface implemented by types that can unmarshal a flag
// argument to themselves. The provided value is directly passed from the
// command line.
type Unmarshaler interface {
	// UnmarshalFlag unmarshals a string value representation to the flag
	// value (which therefore needs to be a pointer receiver).
	UnmarshalFlag(value string) error
}

func getBase(options multiTag, base int) (int, error) {
	sbase := options.Get("base")

	var err error
	var ivbase int64

	if sbase != "" {
		ivbase, err = strconv.ParseInt(sbase, 10, 32)
		base = int(ivbase)
	}

	return base, err
}

func convertMarshal(val reflect.Value) (bool, string, error) {
	// Check first for the Marshaler interface
	if val.Type().NumMethod() > 0 && val.CanInterface() {
		if marshaler, ok := val.Interface().(Marshaler); ok {
			ret, err := marshaler.MarshalFlag()
			return true, ret, err
		}
	}

	return false, "", nil
}

func convertToString(val reflect.Value, options multiTag) (string, error) {
	if ok, ret, err := convertMarshal(val); ok {
		return ret, err
	}

	tp := val.Type()

	// Support for time.Duration
	if tp == reflect.TypeOf((*time.Duration)(nil)).Elem() {
		stringer := val.Interface().(fmt.Stringer)
		return stringer.String(), nil
	}

	switch tp.Kind() {
	case reflect.String:
		return val.String(), nil
	case reflect.Bool:
		if val.Bool() {
			return "true", nil
		}

		return "false", nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		base, _ := getBase(options, 10)
		return strconv.FormatInt(val.Int(), base), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		base, _ := getBase(options, 10)
		return strconv.FormatUint(val.Uint(), base), nil
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(val.Float(), 'g', -1, tp.Bits()), nil
	case reflect.Slice:
		if val.Len() == 0 {
			return "", nil
		}

		ret := "["

		for i := 0; i < val.Len(); i++ {
			if i != 0 {
				ret += ", "
			}

			item, err := convertToString(val.Index(i), options)

			if err != nil {
				return "", err
			}

			ret += item
		}

		return ret + "]", nil
	case reflect.Map:
		ret := "{"

		for i, key := range val.MapKeys() {
			if i != 0 {
				ret += ", "
			}

			item, err := convertToString(val.MapIndex(key), options)

			if err != nil {
				return "", err
			}

			ret += item
		}

		return ret + "}", nil
	case reflect.Ptr:
		return convertToString(reflect.Indirect(val), options)
	case reflect.Interface:
		if !val.IsNil() {
			return convertToString(val.Elem(), options)
		}
	}

	return "", nil
}

func convertUnmarshal(val string, retval reflect.Value) (bool, error) {
	if retval.Type().NumMethod() > 0 && retval.CanInterface() {
		if unmarshaler, ok := retval.Interface().(Unmarshaler); ok {
			return true, unmarshaler.UnmarshalFlag(val)
		}
	}

	if retval.Type().Kind() != reflect.Ptr && retval.CanAddr() {
		return convertUnmarshal(val, retval.Addr())
	}

	if retval.Type().Kind() == reflect.Interface && !retval.IsNil() {
		return convertUnmarshal(val, retval.Elem())
	}

	return false, nil
}

func convert(val string, retval reflect.Value, options multiTag) error {
	if ok, err := convertUnmarshal(val, retval); ok {
		return err
	}

	tp := retval.Type()

	// Support for time.Duration
	if tp == reflect.TypeOf((*time.Duration)(nil)).Elem() {
		parsed, err := time.ParseDuration(val)

		if err != nil {
			return err
		}

		retval.SetInt(int64(parsed))
		return nil
	}

	switch tp.Kind() {
	case reflect.String:
		retval.SetString(val)
	case reflect.Bool:
		if val == "" {
			retval.SetBool(true)
		} else {
			b, err := strconv.ParseBool(val)

			if err != nil {
				return err
			}

			retval.SetBool(b)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		base, err := getBase(options, 10)

		if err != nil {
			return err
		}

		parsed, err := strconv.ParseInt(val, base, tp.Bits())

		if err != nil {
			return err
		}

		retval.SetInt(parsed)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		base, err := getBase(options, 10)

		if err != nil {
			return err
		}

		parsed, err := strconv.ParseUint(val, base, tp.Bits())

		if err != nil {
			return err
		}

		retval.SetUint(parsed)
	case reflect.Float32, reflect.Float64:
		parsed, err := strconv.ParseFloat(val, tp.Bits())

		if err != nil {
			return err
		}

		retval.SetFloat(parsed)
	case reflect.Slice:
		elemtp := tp.Elem()

		elemvalptr := reflect.New(elemtp)
		elemval := reflect.Indirect(elemvalptr)

		if err := convert(val, elemval, options); err != nil {
			return err
		}

		retval.Set(reflect.Append(retval, elemval))
	case reflect.Map:
		parts := strings.SplitN(val, ":", 2)

		key := parts[0]
		var value string

		if len(parts) == 2 {
			value = parts[1]
		}

		keytp := tp.Key()
		keyval := reflect.New(keytp)

		if err := convert(key, keyval, options); err != nil {
			return err
		}

		valuetp := tp.Elem()
		valueval := reflect.New(valuetp)

		if err := convert(value, valueval, options); err != nil {
			return err
		}

		if retval.IsNil() {
			retval.Set(reflect.MakeMap(tp))
		}

		retval.SetMapIndex(reflect.Indirect(keyval), reflect.Indirect(valueval))
	case reflect.Ptr:
		if retval.IsNil() {
			retval.Set(reflect.New(retval.Type().Elem()))
		}

		return convert(val, reflect.Indirect(retval), options)
	case reflect.Interface:
		if !retval.IsNil() {
			return convert(val, retval.Elem(), options)
		}
	}

	return nil
}

func wrapText(s string, l int, prefix string) string {
	// Basic text wrapping of s at spaces to fit in l
	var ret string

	s = strings.TrimSpace(s)

	for len(s) > l {
		// Try to split on space
		suffix := ""

		pos := strings.LastIndex(s[:l], " ")

		if pos < 0 {
			pos = l - 1
			suffix = "-\n"
		}

		if len(ret) != 0 {
			ret += "\n" + prefix
		}

		ret += strings.TrimSpace(s[:pos]) + suffix
		s = strings.TrimSpace(s[pos:])
	}

	if len(s) > 0 {
		if len(ret) != 0 {
			ret += "\n" + prefix
		}

		return ret + s
	}

	return ret
}
