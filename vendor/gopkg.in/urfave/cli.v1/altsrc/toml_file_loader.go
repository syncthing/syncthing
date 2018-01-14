// Disabling building of toml support in cases where golang is 1.0 or 1.1
// as the encoding library is not implemented or supported.

// +build go1.2

package altsrc

import (
	"fmt"
	"reflect"

	"github.com/BurntSushi/toml"
	"gopkg.in/urfave/cli.v1"
)

type tomlMap struct {
	Map map[interface{}]interface{}
}

func unmarshalMap(i interface{}) (ret map[interface{}]interface{}, err error) {
	ret = make(map[interface{}]interface{})
	m := i.(map[string]interface{})
	for key, val := range m {
		v := reflect.ValueOf(val)
		switch v.Kind() {
		case reflect.Bool:
			ret[key] = val.(bool)
		case reflect.String:
			ret[key] = val.(string)
		case reflect.Int:
			ret[key] = int(val.(int))
		case reflect.Int8:
			ret[key] = int(val.(int8))
		case reflect.Int16:
			ret[key] = int(val.(int16))
		case reflect.Int32:
			ret[key] = int(val.(int32))
		case reflect.Int64:
			ret[key] = int(val.(int64))
		case reflect.Uint:
			ret[key] = int(val.(uint))
		case reflect.Uint8:
			ret[key] = int(val.(uint8))
		case reflect.Uint16:
			ret[key] = int(val.(uint16))
		case reflect.Uint32:
			ret[key] = int(val.(uint32))
		case reflect.Uint64:
			ret[key] = int(val.(uint64))
		case reflect.Float32:
			ret[key] = float64(val.(float32))
		case reflect.Float64:
			ret[key] = float64(val.(float64))
		case reflect.Map:
			if tmp, err := unmarshalMap(val); err == nil {
				ret[key] = tmp
			} else {
				return nil, err
			}
		case reflect.Array, reflect.Slice:
			ret[key] = val.([]interface{})
		default:
			return nil, fmt.Errorf("Unsupported: type = %#v", v.Kind())
		}
	}
	return ret, nil
}

func (self *tomlMap) UnmarshalTOML(i interface{}) error {
	if tmp, err := unmarshalMap(i); err == nil {
		self.Map = tmp
	} else {
		return err
	}
	return nil
}

type tomlSourceContext struct {
	FilePath string
}

// NewTomlSourceFromFile creates a new TOML InputSourceContext from a filepath.
func NewTomlSourceFromFile(file string) (InputSourceContext, error) {
	tsc := &tomlSourceContext{FilePath: file}
	var results tomlMap = tomlMap{}
	if err := readCommandToml(tsc.FilePath, &results); err != nil {
		return nil, fmt.Errorf("Unable to load TOML file '%s': inner error: \n'%v'", tsc.FilePath, err.Error())
	}
	return &MapInputSource{valueMap: results.Map}, nil
}

// NewTomlSourceFromFlagFunc creates a new TOML InputSourceContext from a provided flag name and source context.
func NewTomlSourceFromFlagFunc(flagFileName string) func(context *cli.Context) (InputSourceContext, error) {
	return func(context *cli.Context) (InputSourceContext, error) {
		filePath := context.String(flagFileName)
		return NewTomlSourceFromFile(filePath)
	}
}

func readCommandToml(filePath string, container interface{}) (err error) {
	b, err := loadDataFrom(filePath)
	if err != nil {
		return err
	}

	err = toml.Unmarshal(b, container)
	if err != nil {
		return err
	}

	err = nil
	return
}
