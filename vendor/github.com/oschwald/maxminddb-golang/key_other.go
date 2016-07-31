// +build !appengine

package maxminddb

import (
	"reflect"
	"unsafe"
)

// decodeStructKey returns a string which points into the database. Don't keep
// it around.
func (d *decoder) decodeStructKey(offset uint) (string, uint, error) {
	typeNum, size, newOffset := d.decodeCtrlData(offset)
	switch typeNum {
	case _Pointer:
		pointer, ptrOffset := d.decodePointer(size, newOffset)
		s, _, err := d.decodeStructKey(pointer)
		return s, ptrOffset, err
	case _String:
		var s string
		val := (*reflect.StringHeader)(unsafe.Pointer(&s))
		val.Data = uintptr(unsafe.Pointer(&d.buffer[newOffset]))
		val.Len = int(size)
		return s, newOffset + size, nil
	default:
		return "", 0, newInvalidDatabaseError("unexpected type when decoding struct key: %v", typeNum)
	}
}
