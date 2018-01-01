package altsrc

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"gopkg.in/urfave/cli.v1"
)

// MapInputSource implements InputSourceContext to return
// data from the map that is loaded.
type MapInputSource struct {
	valueMap map[interface{}]interface{}
}

// nestedVal checks if the name has '.' delimiters.
// If so, it tries to traverse the tree by the '.' delimited sections to find
// a nested value for the key.
func nestedVal(name string, tree map[interface{}]interface{}) (interface{}, bool) {
	if sections := strings.Split(name, "."); len(sections) > 1 {
		node := tree
		for _, section := range sections[:len(sections)-1] {
			if child, ok := node[section]; !ok {
				return nil, false
			} else {
				if ctype, ok := child.(map[interface{}]interface{}); !ok {
					return nil, false
				} else {
					node = ctype
				}
			}
		}
		if val, ok := node[sections[len(sections)-1]]; ok {
			return val, true
		}
	}
	return nil, false
}

// Int returns an int from the map if it exists otherwise returns 0
func (fsm *MapInputSource) Int(name string) (int, error) {
	otherGenericValue, exists := fsm.valueMap[name]
	if exists {
		otherValue, isType := otherGenericValue.(int)
		if !isType {
			return 0, incorrectTypeForFlagError(name, "int", otherGenericValue)
		}
		return otherValue, nil
	}
	nestedGenericValue, exists := nestedVal(name, fsm.valueMap)
	if exists {
		otherValue, isType := nestedGenericValue.(int)
		if !isType {
			return 0, incorrectTypeForFlagError(name, "int", nestedGenericValue)
		}
		return otherValue, nil
	}

	return 0, nil
}

// Duration returns a duration from the map if it exists otherwise returns 0
func (fsm *MapInputSource) Duration(name string) (time.Duration, error) {
	otherGenericValue, exists := fsm.valueMap[name]
	if exists {
		otherValue, isType := otherGenericValue.(time.Duration)
		if !isType {
			return 0, incorrectTypeForFlagError(name, "duration", otherGenericValue)
		}
		return otherValue, nil
	}
	nestedGenericValue, exists := nestedVal(name, fsm.valueMap)
	if exists {
		otherValue, isType := nestedGenericValue.(time.Duration)
		if !isType {
			return 0, incorrectTypeForFlagError(name, "duration", nestedGenericValue)
		}
		return otherValue, nil
	}

	return 0, nil
}

// Float64 returns an float64 from the map if it exists otherwise returns 0
func (fsm *MapInputSource) Float64(name string) (float64, error) {
	otherGenericValue, exists := fsm.valueMap[name]
	if exists {
		otherValue, isType := otherGenericValue.(float64)
		if !isType {
			return 0, incorrectTypeForFlagError(name, "float64", otherGenericValue)
		}
		return otherValue, nil
	}
	nestedGenericValue, exists := nestedVal(name, fsm.valueMap)
	if exists {
		otherValue, isType := nestedGenericValue.(float64)
		if !isType {
			return 0, incorrectTypeForFlagError(name, "float64", nestedGenericValue)
		}
		return otherValue, nil
	}

	return 0, nil
}

// String returns a string from the map if it exists otherwise returns an empty string
func (fsm *MapInputSource) String(name string) (string, error) {
	otherGenericValue, exists := fsm.valueMap[name]
	if exists {
		otherValue, isType := otherGenericValue.(string)
		if !isType {
			return "", incorrectTypeForFlagError(name, "string", otherGenericValue)
		}
		return otherValue, nil
	}
	nestedGenericValue, exists := nestedVal(name, fsm.valueMap)
	if exists {
		otherValue, isType := nestedGenericValue.(string)
		if !isType {
			return "", incorrectTypeForFlagError(name, "string", nestedGenericValue)
		}
		return otherValue, nil
	}

	return "", nil
}

// StringSlice returns an []string from the map if it exists otherwise returns nil
func (fsm *MapInputSource) StringSlice(name string) ([]string, error) {
	otherGenericValue, exists := fsm.valueMap[name]
	if !exists {
		otherGenericValue, exists = nestedVal(name, fsm.valueMap)
		if !exists {
			return nil, nil
		}
	}

	otherValue, isType := otherGenericValue.([]interface{})
	if !isType {
		return nil, incorrectTypeForFlagError(name, "[]interface{}", otherGenericValue)
	}

	var stringSlice = make([]string, 0, len(otherValue))
	for i, v := range otherValue {
		stringValue, isType := v.(string)

		if !isType {
			return nil, incorrectTypeForFlagError(fmt.Sprintf("%s[%d]", name, i), "string", v)
		}

		stringSlice = append(stringSlice, stringValue)
	}

	return stringSlice, nil
}

// IntSlice returns an []int from the map if it exists otherwise returns nil
func (fsm *MapInputSource) IntSlice(name string) ([]int, error) {
	otherGenericValue, exists := fsm.valueMap[name]
	if !exists {
		otherGenericValue, exists = nestedVal(name, fsm.valueMap)
		if !exists {
			return nil, nil
		}
	}

	otherValue, isType := otherGenericValue.([]interface{})
	if !isType {
		return nil, incorrectTypeForFlagError(name, "[]interface{}", otherGenericValue)
	}

	var intSlice = make([]int, 0, len(otherValue))
	for i, v := range otherValue {
		intValue, isType := v.(int)

		if !isType {
			return nil, incorrectTypeForFlagError(fmt.Sprintf("%s[%d]", name, i), "int", v)
		}

		intSlice = append(intSlice, intValue)
	}

	return intSlice, nil
}

// Generic returns an cli.Generic from the map if it exists otherwise returns nil
func (fsm *MapInputSource) Generic(name string) (cli.Generic, error) {
	otherGenericValue, exists := fsm.valueMap[name]
	if exists {
		otherValue, isType := otherGenericValue.(cli.Generic)
		if !isType {
			return nil, incorrectTypeForFlagError(name, "cli.Generic", otherGenericValue)
		}
		return otherValue, nil
	}
	nestedGenericValue, exists := nestedVal(name, fsm.valueMap)
	if exists {
		otherValue, isType := nestedGenericValue.(cli.Generic)
		if !isType {
			return nil, incorrectTypeForFlagError(name, "cli.Generic", nestedGenericValue)
		}
		return otherValue, nil
	}

	return nil, nil
}

// Bool returns an bool from the map otherwise returns false
func (fsm *MapInputSource) Bool(name string) (bool, error) {
	otherGenericValue, exists := fsm.valueMap[name]
	if exists {
		otherValue, isType := otherGenericValue.(bool)
		if !isType {
			return false, incorrectTypeForFlagError(name, "bool", otherGenericValue)
		}
		return otherValue, nil
	}
	nestedGenericValue, exists := nestedVal(name, fsm.valueMap)
	if exists {
		otherValue, isType := nestedGenericValue.(bool)
		if !isType {
			return false, incorrectTypeForFlagError(name, "bool", nestedGenericValue)
		}
		return otherValue, nil
	}

	return false, nil
}

// BoolT returns an bool from the map otherwise returns true
func (fsm *MapInputSource) BoolT(name string) (bool, error) {
	otherGenericValue, exists := fsm.valueMap[name]
	if exists {
		otherValue, isType := otherGenericValue.(bool)
		if !isType {
			return true, incorrectTypeForFlagError(name, "bool", otherGenericValue)
		}
		return otherValue, nil
	}
	nestedGenericValue, exists := nestedVal(name, fsm.valueMap)
	if exists {
		otherValue, isType := nestedGenericValue.(bool)
		if !isType {
			return true, incorrectTypeForFlagError(name, "bool", nestedGenericValue)
		}
		return otherValue, nil
	}

	return true, nil
}

func incorrectTypeForFlagError(name, expectedTypeName string, value interface{}) error {
	valueType := reflect.TypeOf(value)
	valueTypeName := ""
	if valueType != nil {
		valueTypeName = valueType.Name()
	}

	return fmt.Errorf("Mismatched type for flag '%s'. Expected '%s' but actual is '%s'", name, expectedTypeName, valueTypeName)
}
