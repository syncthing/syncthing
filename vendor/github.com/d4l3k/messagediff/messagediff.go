package messagediff

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// PrettyDiff does a deep comparison and returns the nicely formated results.
func PrettyDiff(a, b interface{}) (string, bool) {
	d, equal := DeepDiff(a, b)
	var dstr []string
	for path, added := range d.Added {
		dstr = append(dstr, fmt.Sprintf("added: %s = %#v\n", path.String(), added))
	}
	for path, removed := range d.Removed {
		dstr = append(dstr, fmt.Sprintf("removed: %s = %#v\n", path.String(), removed))
	}
	for path, modified := range d.Modified {
		dstr = append(dstr, fmt.Sprintf("modified: %s = %#v\n", path.String(), modified))
	}
	sort.Strings(dstr)
	return strings.Join(dstr, ""), equal
}

// DeepDiff does a deep comparison and returns the results.
func DeepDiff(a, b interface{}) (*Diff, bool) {
	d := newdiff()
	return d, diff(a, b, nil, d)
}

func newdiff() *Diff {
	return &Diff{
		Added:    make(map[*Path]interface{}),
		Removed:  make(map[*Path]interface{}),
		Modified: make(map[*Path]interface{}),
	}
}

func diff(a, b interface{}, path Path, d *Diff) bool {
	aVal := reflect.ValueOf(a)
	bVal := reflect.ValueOf(b)
	if !aVal.IsValid() && !bVal.IsValid() {
		// Both are nil.
		return true
	}
	if !aVal.IsValid() || !bVal.IsValid() {
		// One is nil and the other isn't.
		d.Modified[&path] = b
		return false
	}
	if aVal.Type() != bVal.Type() {
		d.Modified[&path] = b
		return false
	}
	kind := aVal.Type().Kind()
	equal := true
	switch kind {
	case reflect.Array, reflect.Slice:
		aLen := aVal.Len()
		bLen := bVal.Len()
		for i := 0; i < min(aLen, bLen); i++ {
			localPath := append(path, SliceIndex(i))
			if eq := diff(aVal.Index(i).Interface(), bVal.Index(i).Interface(), localPath, d); !eq {
				equal = false
			}
		}
		if aLen > bLen {
			for i := bLen; i < aLen; i++ {
				localPath := append(path, SliceIndex(i))
				d.Removed[&localPath] = aVal.Index(i).Interface()
				equal = false
			}
		} else if aLen < bLen {
			for i := aLen; i < bLen; i++ {
				localPath := append(path, SliceIndex(i))
				d.Added[&localPath] = bVal.Index(i).Interface()
				equal = false
			}
		}
	case reflect.Map:
		for _, key := range aVal.MapKeys() {
			aI := aVal.MapIndex(key)
			bI := bVal.MapIndex(key)
			localPath := append(path, MapKey{key.Interface()})
			if !bI.IsValid() {
				d.Removed[&localPath] = aI.Interface()
				equal = false
			} else if eq := diff(aI.Interface(), bI.Interface(), localPath, d); !eq {
				equal = false
			}
		}
		for _, key := range bVal.MapKeys() {
			aI := aVal.MapIndex(key)
			if !aI.IsValid() {
				bI := bVal.MapIndex(key)
				localPath := append(path, MapKey{key.Interface()})
				d.Added[&localPath] = bI.Interface()
				equal = false
			}
		}
	case reflect.Struct:
		typ := aVal.Type()
		for i := 0; i < typ.NumField(); i++ {
			index := []int{i}
			field := typ.FieldByIndex(index)
			localPath := append(path, StructField(field.Name))
			aI := unsafeReflectValue(aVal.FieldByIndex(index)).Interface()
			bI := unsafeReflectValue(bVal.FieldByIndex(index)).Interface()
			if eq := diff(aI, bI, localPath, d); !eq {
				equal = false
			}
		}
	case reflect.Ptr:
		aVal = aVal.Elem()
		bVal = bVal.Elem()
		if !aVal.IsValid() && !bVal.IsValid() {
			// Both are nil.
			equal = true
		} else if !aVal.IsValid() || !bVal.IsValid() {
			// One is nil and the other isn't.
			d.Modified[&path] = b
			equal = false
		} else {
			equal = diff(aVal.Interface(), bVal.Interface(), path, d)
		}
	default:
		if reflect.DeepEqual(a, b) {
			equal = true
		} else {
			d.Modified[&path] = b
			equal = false
		}
	}
	return equal
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Diff represents a change in a struct.
type Diff struct {
	Added, Removed, Modified map[*Path]interface{}
}

// Path represents a path to a changed datum.
type Path []PathNode

func (p Path) String() string {
	var out string
	for _, n := range p {
		out += n.String()
	}
	return out
}

// PathNode represents one step in the path.
type PathNode interface {
	String() string
}

// StructField is a path element representing a field of a struct.
type StructField string

func (n StructField) String() string {
	return fmt.Sprintf(".%s", string(n))
}

// MapKey is a path element representing a key of a map.
type MapKey struct {
	Key interface{}
}

func (n MapKey) String() string {
	return fmt.Sprintf("[%#v]", n.Key)
}

// SliceIndex is a path element representing a index of a slice.
type SliceIndex int

func (n SliceIndex) String() string {
	return fmt.Sprintf("[%d]", n)
}
