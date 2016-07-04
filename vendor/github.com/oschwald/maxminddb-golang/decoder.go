package maxminddb

import (
	"encoding/binary"
	"math"
	"math/big"
	"reflect"
	"sync"
)

type decoder struct {
	buffer []byte
}

type dataType int

const (
	_Extended dataType = iota
	_Pointer
	_String
	_Float64
	_Bytes
	_Uint16
	_Uint32
	_Map
	_Int32
	_Uint64
	_Uint128
	_Slice
	_Container
	_Marker
	_Bool
	_Float32
)

func (d *decoder) decode(offset uint, result reflect.Value) (uint, error) {
	typeNum, size, newOffset := d.decodeCtrlData(offset)

	if typeNum != _Pointer && result.Kind() == reflect.Uintptr {
		result.Set(reflect.ValueOf(uintptr(offset)))
		return d.nextValueOffset(offset, 1), nil
	}
	return d.decodeFromType(typeNum, size, newOffset, result)
}

func (d *decoder) decodeCtrlData(offset uint) (dataType, uint, uint) {
	newOffset := offset + 1
	ctrlByte := d.buffer[offset]

	typeNum := dataType(ctrlByte >> 5)
	if typeNum == _Extended {
		typeNum = dataType(d.buffer[newOffset] + 7)
		newOffset++
	}

	var size uint
	size, newOffset = d.sizeFromCtrlByte(ctrlByte, newOffset, typeNum)
	return typeNum, size, newOffset
}

func (d *decoder) sizeFromCtrlByte(ctrlByte byte, offset uint, typeNum dataType) (uint, uint) {
	size := uint(ctrlByte & 0x1f)
	if typeNum == _Extended {
		return size, offset
	}

	var bytesToRead uint
	if size > 28 {
		bytesToRead = size - 28
	}

	newOffset := offset + bytesToRead
	sizeBytes := d.buffer[offset:newOffset]

	switch {
	case size == 29:
		size = 29 + uint(sizeBytes[0])
	case size == 30:
		size = 285 + uint(uintFromBytes(0, sizeBytes))
	case size > 30:
		size = uint(uintFromBytes(0, sizeBytes)) + 65821
	}
	return size, newOffset
}

func (d *decoder) decodeFromType(dtype dataType, size uint, offset uint, result reflect.Value) (uint, error) {
	for result.Kind() == reflect.Ptr {
		if result.IsNil() {
			result.Set(reflect.New(result.Type().Elem()))
		}
		result = result.Elem()
	}

	switch dtype {
	case _Bool:
		return d.unmarshalBool(size, offset, result)
	case _Bytes:
		return d.unmarshalBytes(size, offset, result)
	case _Float32:
		return d.unmarshalFloat32(size, offset, result)
	case _Float64:
		return d.unmarshalFloat64(size, offset, result)
	case _Int32:
		return d.unmarshalInt32(size, offset, result)
	case _Map:
		return d.unmarshalMap(size, offset, result)
	case _Pointer:
		return d.unmarshalPointer(size, offset, result)
	case _Slice:
		return d.unmarshalSlice(size, offset, result)
	case _String:
		return d.unmarshalString(size, offset, result)
	case _Uint16:
		return d.unmarshalUint(size, offset, result, 16)
	case _Uint32:
		return d.unmarshalUint(size, offset, result, 32)
	case _Uint64:
		return d.unmarshalUint(size, offset, result, 64)
	case _Uint128:
		return d.unmarshalUint128(size, offset, result)
	default:
		return 0, newInvalidDatabaseError("unknown type: %d", dtype)
	}
}

func (d *decoder) unmarshalBool(size uint, offset uint, result reflect.Value) (uint, error) {
	if size > 1 {
		return 0, newInvalidDatabaseError("the MaxMind DB file's data section contains bad data (bool size of %v)", size)
	}
	value, newOffset, err := d.decodeBool(size, offset)
	if err != nil {
		return 0, err
	}
	switch result.Kind() {
	default:
		return newOffset, newUnmarshalTypeError(value, result.Type())
	case reflect.Bool:
		result.SetBool(value)
		return newOffset, nil
	case reflect.Interface:
		result.Set(reflect.ValueOf(value))
		return newOffset, nil
	}
}

func (d *decoder) unmarshalBytes(size uint, offset uint, result reflect.Value) (uint, error) {
	value, newOffset, err := d.decodeBytes(size, offset)
	if err != nil {
		return 0, err
	}
	switch result.Kind() {
	default:
		return newOffset, newUnmarshalTypeError(value, result.Type())
	case reflect.Slice:
		result.SetBytes(value)
		return newOffset, nil
	case reflect.Interface:
		result.Set(reflect.ValueOf(value))
		return newOffset, nil
	}
}

func (d *decoder) unmarshalFloat32(size uint, offset uint, result reflect.Value) (uint, error) {
	if size != 4 {
		return 0, newInvalidDatabaseError("the MaxMind DB file's data section contains bad data (float32 size of %v)", size)
	}
	value, newOffset, err := d.decodeFloat32(size, offset)
	if err != nil {
		return 0, err
	}

	switch result.Kind() {
	default:
		return newOffset, newUnmarshalTypeError(value, result.Type())
	case reflect.Float32, reflect.Float64:
		result.SetFloat(float64(value))
		return newOffset, nil
	case reflect.Interface:
		result.Set(reflect.ValueOf(value))
		return newOffset, nil
	}
}

func (d *decoder) unmarshalFloat64(size uint, offset uint, result reflect.Value) (uint, error) {

	if size != 8 {
		return 0, newInvalidDatabaseError("the MaxMind DB file's data section contains bad data (float 64 size of %v)", size)
	}
	value, newOffset, err := d.decodeFloat64(size, offset)
	if err != nil {
		return 0, err
	}
	switch result.Kind() {
	default:
		return newOffset, newUnmarshalTypeError(value, result.Type())
	case reflect.Float32, reflect.Float64:
		if result.OverflowFloat(value) {
			return 0, newUnmarshalTypeError(value, result.Type())
		}
		result.SetFloat(value)
		return newOffset, nil
	case reflect.Interface:
		result.Set(reflect.ValueOf(value))
		return newOffset, nil
	}
}

func (d *decoder) unmarshalInt32(size uint, offset uint, result reflect.Value) (uint, error) {
	if size > 4 {
		return 0, newInvalidDatabaseError("the MaxMind DB file's data section contains bad data (int32 size of %v)", size)
	}
	value, newOffset, err := d.decodeInt(size, offset)
	if err != nil {
		return 0, err
	}

	switch result.Kind() {
	default:
		return newOffset, newUnmarshalTypeError(value, result.Type())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n := int64(value)
		if result.OverflowInt(n) {
			return 0, newUnmarshalTypeError(value, result.Type())
		}
		result.SetInt(n)
		return newOffset, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		n := uint64(value)
		if result.OverflowUint(n) {
			return 0, newUnmarshalTypeError(value, result.Type())
		}
		result.SetUint(n)
		return newOffset, nil
	case reflect.Interface:
		result.Set(reflect.ValueOf(value))
		return newOffset, nil
	}
}

func (d *decoder) unmarshalMap(size uint, offset uint, result reflect.Value) (uint, error) {
	switch result.Kind() {
	default:
		return 0, newUnmarshalTypeError("map", result.Type())
	case reflect.Struct:
		return d.decodeStruct(size, offset, result)
	case reflect.Map:
		return d.decodeMap(size, offset, result)
	case reflect.Interface:
		rv := reflect.ValueOf(make(map[string]interface{}, size))
		newOffset, err := d.decodeMap(size, offset, rv)
		result.Set(rv)
		return newOffset, err
	}
}

func (d *decoder) unmarshalPointer(size uint, offset uint, result reflect.Value) (uint, error) {
	pointer, newOffset := d.decodePointer(size, offset)
	_, err := d.decode(pointer, result)
	return newOffset, err
}

func (d *decoder) unmarshalSlice(size uint, offset uint, result reflect.Value) (uint, error) {

	switch result.Kind() {
	default:
		return 0, newUnmarshalTypeError("array", result.Type())
	case reflect.Slice:
		return d.decodeSlice(size, offset, result)
	case reflect.Interface:
		a := []interface{}{}
		rv := reflect.ValueOf(&a).Elem()
		newOffset, err := d.decodeSlice(size, offset, rv)
		result.Set(rv)
		return newOffset, err
	}
}

func (d *decoder) unmarshalString(size uint, offset uint, result reflect.Value) (uint, error) {
	value, newOffset, err := d.decodeString(size, offset)

	if err != nil {
		return 0, err
	}
	switch result.Kind() {
	default:
		return newOffset, newUnmarshalTypeError(value, result.Type())
	case reflect.String:
		result.SetString(value)
		return newOffset, nil
	case reflect.Interface:
		result.Set(reflect.ValueOf(value))
		return newOffset, nil
	}
}

func (d *decoder) unmarshalUint(size uint, offset uint, result reflect.Value, uintType uint) (uint, error) {
	if size > uintType/8 {
		return 0, newInvalidDatabaseError("the MaxMind DB file's data section contains bad data (uint%v size of %v)", uintType, size)
	}

	value, newOffset, err := d.decodeUint(size, offset)
	if err != nil {
		return 0, err
	}

	switch result.Kind() {
	default:
		return newOffset, newUnmarshalTypeError(value, result.Type())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n := int64(value)
		if result.OverflowInt(n) {
			return 0, newUnmarshalTypeError(value, result.Type())
		}
		result.SetInt(n)
		return newOffset, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if result.OverflowUint(value) {
			return 0, newUnmarshalTypeError(value, result.Type())
		}
		result.SetUint(value)
		return newOffset, nil
	case reflect.Interface:
		result.Set(reflect.ValueOf(value))
		return newOffset, nil
	}
}

func (d *decoder) unmarshalUint128(size uint, offset uint, result reflect.Value) (uint, error) {
	if size > 16 {
		return 0, newInvalidDatabaseError("the MaxMind DB file's data section contains bad data (uint128 size of %v)", size)
	}
	value, newOffset, err := d.decodeUint128(size, offset)
	if err != nil {
		return 0, err
	}

	// XXX - this should allow *big.Int rather than just bigInt
	// Currently this is reported as invalid
	switch result.Kind() {
	default:
		return newOffset, newUnmarshalTypeError(value, result.Type())
	case reflect.Struct:
		result.Set(reflect.ValueOf(*value))
		return newOffset, nil
	case reflect.Interface, reflect.Ptr:
		result.Set(reflect.ValueOf(value))
		return newOffset, nil
	}
}

func (d *decoder) decodeBool(size uint, offset uint) (bool, uint, error) {
	return size != 0, offset, nil
}

func (d *decoder) decodeBytes(size uint, offset uint) ([]byte, uint, error) {
	newOffset := offset + size
	bytes := make([]byte, size)
	copy(bytes, d.buffer[offset:newOffset])
	return bytes, newOffset, nil
}

func (d *decoder) decodeFloat64(size uint, offset uint) (float64, uint, error) {
	newOffset := offset + size
	bits := binary.BigEndian.Uint64(d.buffer[offset:newOffset])
	return math.Float64frombits(bits), newOffset, nil
}

func (d *decoder) decodeFloat32(size uint, offset uint) (float32, uint, error) {
	newOffset := offset + size
	bits := binary.BigEndian.Uint32(d.buffer[offset:newOffset])
	return math.Float32frombits(bits), newOffset, nil
}

func (d *decoder) decodeInt(size uint, offset uint) (int, uint, error) {
	newOffset := offset + size
	var val int32
	for _, b := range d.buffer[offset:newOffset] {
		val = (val << 8) | int32(b)
	}
	return int(val), newOffset, nil
}

func (d *decoder) decodeMap(size uint, offset uint, result reflect.Value) (uint, error) {
	if result.IsNil() {
		result.Set(reflect.MakeMap(result.Type()))
	}

	for i := uint(0); i < size; i++ {
		var key string
		var err error
		key, offset, err = d.decodeKeyString(offset)

		if err != nil {
			return 0, err
		}

		value := reflect.New(result.Type().Elem())
		offset, err = d.decode(offset, value)
		if err != nil {
			return 0, err
		}
		result.SetMapIndex(reflect.ValueOf(key), value.Elem())
	}
	return offset, nil
}

func (d *decoder) decodePointer(size uint, offset uint) (uint, uint) {
	pointerSize := ((size >> 3) & 0x3) + 1
	newOffset := offset + pointerSize
	pointerBytes := d.buffer[offset:newOffset]
	var prefix uint64
	if pointerSize == 4 {
		prefix = 0
	} else {
		prefix = uint64(size & 0x7)
	}
	unpacked := uint(uintFromBytes(prefix, pointerBytes))

	var pointerValueOffset uint
	switch pointerSize {
	case 1:
		pointerValueOffset = 0
	case 2:
		pointerValueOffset = 2048
	case 3:
		pointerValueOffset = 526336
	case 4:
		pointerValueOffset = 0
	}

	pointer := unpacked + pointerValueOffset

	return pointer, newOffset
}

func (d *decoder) decodeSlice(size uint, offset uint, result reflect.Value) (uint, error) {
	result.Set(reflect.MakeSlice(result.Type(), int(size), int(size)))
	for i := 0; i < int(size); i++ {
		var err error
		offset, err = d.decode(offset, result.Index(i))
		if err != nil {
			return 0, err
		}
	}
	return offset, nil
}

func (d *decoder) decodeString(size uint, offset uint) (string, uint, error) {
	newOffset := offset + size
	return string(d.buffer[offset:newOffset]), newOffset, nil
}

var (
	fieldMap   = map[reflect.Type]map[string]int{}
	fieldMapMu sync.RWMutex
)

func (d *decoder) decodeStruct(size uint, offset uint, result reflect.Value) (uint, error) {
	resultType := result.Type()

	fieldMapMu.RLock()
	fields, ok := fieldMap[resultType]
	fieldMapMu.RUnlock()
	if !ok {
		numFields := resultType.NumField()
		fields = make(map[string]int, numFields)
		for i := 0; i < numFields; i++ {
			fieldType := resultType.Field(i)

			fieldName := fieldType.Name
			if tag := fieldType.Tag.Get("maxminddb"); tag != "" {
				fieldName = tag
			}
			fields[fieldName] = i
		}
		fieldMapMu.Lock()
		fieldMap[resultType] = fields
		fieldMapMu.Unlock()
	}

	for i := uint(0); i < size; i++ {
		var (
			err error
			key string
		)
		key, offset, err = d.decodeStructKey(offset)
		if err != nil {
			return 0, err
		}
		i, ok := fields[key]
		if !ok {
			offset = d.nextValueOffset(offset, 1)
			continue
		}
		offset, err = d.decode(offset, result.Field(i))
		if err != nil {
			return 0, err
		}
	}
	return offset, nil
}

func (d *decoder) decodeUint(size uint, offset uint) (uint64, uint, error) {
	newOffset := offset + size
	val := uintFromBytes(0, d.buffer[offset:newOffset])

	return val, newOffset, nil
}

func (d *decoder) decodeUint128(size uint, offset uint) (*big.Int, uint, error) {
	newOffset := offset + size
	val := new(big.Int)
	val.SetBytes(d.buffer[offset:newOffset])

	return val, newOffset, nil
}

func uintFromBytes(prefix uint64, uintBytes []byte) uint64 {
	val := prefix
	for _, b := range uintBytes {
		val = (val << 8) | uint64(b)
	}
	return val
}

func (d *decoder) decodeKeyString(offset uint) (string, uint, error) {
	typeNum, size, newOffset := d.decodeCtrlData(offset)
	if typeNum == _Pointer {
		pointer, ptrOffset := d.decodePointer(size, newOffset)
		key, _, err := d.decodeKeyString(pointer)
		return key, ptrOffset, err
	}
	if typeNum != _String {
		return "", 0, newInvalidDatabaseError("unexpected type when decoding string: %v", typeNum)
	}
	return d.decodeString(size, newOffset)
}

// This function is used to skip ahead to the next value without decoding
// the one at the offset passed in. The size bits have different meanings for
// different data types
func (d *decoder) nextValueOffset(offset uint, numberToSkip uint) uint {
	if numberToSkip == 0 {
		return offset
	}
	typeNum, size, offset := d.decodeCtrlData(offset)
	switch typeNum {
	case _Pointer:
		_, offset = d.decodePointer(size, offset)
	case _Map:
		numberToSkip += 2 * size
	case _Slice:
		numberToSkip += size
	case _Bool:
	default:
		offset += size
	}
	return d.nextValueOffset(offset, numberToSkip-1)
}
