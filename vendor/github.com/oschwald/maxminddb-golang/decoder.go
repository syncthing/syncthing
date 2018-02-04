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

const (
	// This is the value used in libmaxminddb
	maximumDataStructureDepth = 512
)

func (d *decoder) decode(offset uint, result reflect.Value, depth int) (uint, error) {
	if depth > maximumDataStructureDepth {
		return 0, newInvalidDatabaseError("exceeded maximum data structure depth; database is likely corrupt")
	}
	typeNum, size, newOffset, err := d.decodeCtrlData(offset)
	if err != nil {
		return 0, err
	}

	if typeNum != _Pointer && result.Kind() == reflect.Uintptr {
		result.Set(reflect.ValueOf(uintptr(offset)))
		return d.nextValueOffset(offset, 1)
	}
	return d.decodeFromType(typeNum, size, newOffset, result, depth+1)
}

func (d *decoder) decodeCtrlData(offset uint) (dataType, uint, uint, error) {
	newOffset := offset + 1
	if offset >= uint(len(d.buffer)) {
		return 0, 0, 0, newOffsetError()
	}
	ctrlByte := d.buffer[offset]

	typeNum := dataType(ctrlByte >> 5)
	if typeNum == _Extended {
		if newOffset >= uint(len(d.buffer)) {
			return 0, 0, 0, newOffsetError()
		}
		typeNum = dataType(d.buffer[newOffset] + 7)
		newOffset++
	}

	var size uint
	size, newOffset, err := d.sizeFromCtrlByte(ctrlByte, newOffset, typeNum)
	return typeNum, size, newOffset, err
}

func (d *decoder) sizeFromCtrlByte(ctrlByte byte, offset uint, typeNum dataType) (uint, uint, error) {
	size := uint(ctrlByte & 0x1f)
	if typeNum == _Extended {
		return size, offset, nil
	}

	var bytesToRead uint
	if size < 29 {
		return size, offset, nil
	}

	bytesToRead = size - 28
	newOffset := offset + bytesToRead
	if newOffset > uint(len(d.buffer)) {
		return 0, 0, newOffsetError()
	}
	if size == 29 {
		return 29 + uint(d.buffer[offset]), offset + 1, nil
	}

	sizeBytes := d.buffer[offset:newOffset]

	switch {
	case size == 30:
		size = 285 + uintFromBytes(0, sizeBytes)
	case size > 30:
		size = uintFromBytes(0, sizeBytes) + 65821
	}
	return size, newOffset, nil
}

func (d *decoder) decodeFromType(
	dtype dataType,
	size uint,
	offset uint,
	result reflect.Value,
	depth int,
) (uint, error) {
	result = d.indirect(result)

	// For these types, size has a special meaning
	switch dtype {
	case _Bool:
		return d.unmarshalBool(size, offset, result)
	case _Map:
		return d.unmarshalMap(size, offset, result, depth)
	case _Pointer:
		return d.unmarshalPointer(size, offset, result, depth)
	case _Slice:
		return d.unmarshalSlice(size, offset, result, depth)
	}

	// For the remaining types, size is the byte size
	if offset+size > uint(len(d.buffer)) {
		return 0, newOffsetError()
	}
	switch dtype {
	case _Bytes:
		return d.unmarshalBytes(size, offset, result)
	case _Float32:
		return d.unmarshalFloat32(size, offset, result)
	case _Float64:
		return d.unmarshalFloat64(size, offset, result)
	case _Int32:
		return d.unmarshalInt32(size, offset, result)
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
	case reflect.Bool:
		result.SetBool(value)
		return newOffset, nil
	case reflect.Interface:
		if result.NumMethod() == 0 {
			result.Set(reflect.ValueOf(value))
			return newOffset, nil
		}
	}
	return newOffset, newUnmarshalTypeError(value, result.Type())
}

// indirect follows pointers and create values as necessary. This is
// heavily based on encoding/json as my original version had a subtle
// bug. This method should be considered to be licensed under
// https://golang.org/LICENSE
func (d *decoder) indirect(result reflect.Value) reflect.Value {
	for {
		// Load value from interface, but only if the result will be
		// usefully addressable.
		if result.Kind() == reflect.Interface && !result.IsNil() {
			e := result.Elem()
			if e.Kind() == reflect.Ptr && !e.IsNil() {
				result = e
				continue
			}
		}

		if result.Kind() != reflect.Ptr {
			break
		}

		if result.IsNil() {
			result.Set(reflect.New(result.Type().Elem()))
		}
		result = result.Elem()
	}
	return result
}

var sliceType = reflect.TypeOf([]byte{})

func (d *decoder) unmarshalBytes(size uint, offset uint, result reflect.Value) (uint, error) {
	value, newOffset, err := d.decodeBytes(size, offset)
	if err != nil {
		return 0, err
	}
	switch result.Kind() {
	case reflect.Slice:
		if result.Type() == sliceType {
			result.SetBytes(value)
			return newOffset, nil
		}
	case reflect.Interface:
		if result.NumMethod() == 0 {
			result.Set(reflect.ValueOf(value))
			return newOffset, nil
		}
	}
	return newOffset, newUnmarshalTypeError(value, result.Type())
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
	case reflect.Float32, reflect.Float64:
		result.SetFloat(float64(value))
		return newOffset, nil
	case reflect.Interface:
		if result.NumMethod() == 0 {
			result.Set(reflect.ValueOf(value))
			return newOffset, nil
		}
	}
	return newOffset, newUnmarshalTypeError(value, result.Type())
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
	case reflect.Float32, reflect.Float64:
		if result.OverflowFloat(value) {
			return 0, newUnmarshalTypeError(value, result.Type())
		}
		result.SetFloat(value)
		return newOffset, nil
	case reflect.Interface:
		if result.NumMethod() == 0 {
			result.Set(reflect.ValueOf(value))
			return newOffset, nil
		}
	}
	return newOffset, newUnmarshalTypeError(value, result.Type())
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
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n := int64(value)
		if !result.OverflowInt(n) {
			result.SetInt(n)
			return newOffset, nil
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		n := uint64(value)
		if !result.OverflowUint(n) {
			result.SetUint(n)
			return newOffset, nil
		}
	case reflect.Interface:
		if result.NumMethod() == 0 {
			result.Set(reflect.ValueOf(value))
			return newOffset, nil
		}
	}
	return newOffset, newUnmarshalTypeError(value, result.Type())
}

func (d *decoder) unmarshalMap(
	size uint,
	offset uint,
	result reflect.Value,
	depth int,
) (uint, error) {
	result = d.indirect(result)
	switch result.Kind() {
	default:
		return 0, newUnmarshalTypeError("map", result.Type())
	case reflect.Struct:
		return d.decodeStruct(size, offset, result, depth)
	case reflect.Map:
		return d.decodeMap(size, offset, result, depth)
	case reflect.Interface:
		if result.NumMethod() == 0 {
			rv := reflect.ValueOf(make(map[string]interface{}, size))
			newOffset, err := d.decodeMap(size, offset, rv, depth)
			result.Set(rv)
			return newOffset, err
		}
		return 0, newUnmarshalTypeError("map", result.Type())
	}
}

func (d *decoder) unmarshalPointer(size uint, offset uint, result reflect.Value, depth int) (uint, error) {
	pointer, newOffset, err := d.decodePointer(size, offset)
	if err != nil {
		return 0, err
	}
	_, err = d.decode(pointer, result, depth)
	return newOffset, err
}

func (d *decoder) unmarshalSlice(
	size uint,
	offset uint,
	result reflect.Value,
	depth int,
) (uint, error) {
	switch result.Kind() {
	case reflect.Slice:
		return d.decodeSlice(size, offset, result, depth)
	case reflect.Interface:
		if result.NumMethod() == 0 {
			a := []interface{}{}
			rv := reflect.ValueOf(&a).Elem()
			newOffset, err := d.decodeSlice(size, offset, rv, depth)
			result.Set(rv)
			return newOffset, err
		}
	}
	return 0, newUnmarshalTypeError("array", result.Type())
}

func (d *decoder) unmarshalString(size uint, offset uint, result reflect.Value) (uint, error) {
	value, newOffset, err := d.decodeString(size, offset)

	if err != nil {
		return 0, err
	}
	switch result.Kind() {
	case reflect.String:
		result.SetString(value)
		return newOffset, nil
	case reflect.Interface:
		if result.NumMethod() == 0 {
			result.Set(reflect.ValueOf(value))
			return newOffset, nil
		}
	}
	return newOffset, newUnmarshalTypeError(value, result.Type())

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
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n := int64(value)
		if !result.OverflowInt(n) {
			result.SetInt(n)
			return newOffset, nil
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if !result.OverflowUint(value) {
			result.SetUint(value)
			return newOffset, nil
		}
	case reflect.Interface:
		if result.NumMethod() == 0 {
			result.Set(reflect.ValueOf(value))
			return newOffset, nil
		}
	}
	return newOffset, newUnmarshalTypeError(value, result.Type())
}

var bigIntType = reflect.TypeOf(big.Int{})

func (d *decoder) unmarshalUint128(size uint, offset uint, result reflect.Value) (uint, error) {
	if size > 16 {
		return 0, newInvalidDatabaseError("the MaxMind DB file's data section contains bad data (uint128 size of %v)", size)
	}
	value, newOffset, err := d.decodeUint128(size, offset)
	if err != nil {
		return 0, err
	}

	switch result.Kind() {
	case reflect.Struct:
		if result.Type() == bigIntType {
			result.Set(reflect.ValueOf(*value))
			return newOffset, nil
		}
	case reflect.Interface:
		if result.NumMethod() == 0 {
			result.Set(reflect.ValueOf(value))
			return newOffset, nil
		}
	}
	return newOffset, newUnmarshalTypeError(value, result.Type())
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

func (d *decoder) decodeMap(
	size uint,
	offset uint,
	result reflect.Value,
	depth int,
) (uint, error) {
	if result.IsNil() {
		result.Set(reflect.MakeMap(result.Type()))
	}

	for i := uint(0); i < size; i++ {
		var key []byte
		var err error
		key, offset, err = d.decodeKey(offset)

		if err != nil {
			return 0, err
		}

		value := reflect.New(result.Type().Elem())
		offset, err = d.decode(offset, value, depth)
		if err != nil {
			return 0, err
		}
		result.SetMapIndex(reflect.ValueOf(string(key)), value.Elem())
	}
	return offset, nil
}

func (d *decoder) decodePointer(
	size uint,
	offset uint,
) (uint, uint, error) {
	pointerSize := ((size >> 3) & 0x3) + 1
	newOffset := offset + pointerSize
	if newOffset > uint(len(d.buffer)) {
		return 0, 0, newOffsetError()
	}
	pointerBytes := d.buffer[offset:newOffset]
	var prefix uint
	if pointerSize == 4 {
		prefix = 0
	} else {
		prefix = uint(size & 0x7)
	}
	unpacked := uintFromBytes(prefix, pointerBytes)

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

	return pointer, newOffset, nil
}

func (d *decoder) decodeSlice(
	size uint,
	offset uint,
	result reflect.Value,
	depth int,
) (uint, error) {
	result.Set(reflect.MakeSlice(result.Type(), int(size), int(size)))
	for i := 0; i < int(size); i++ {
		var err error
		offset, err = d.decode(offset, result.Index(i), depth)
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

type fieldsType struct {
	namedFields     map[string]int
	anonymousFields []int
}

var (
	fieldMap   = map[reflect.Type]*fieldsType{}
	fieldMapMu sync.RWMutex
)

func (d *decoder) decodeStruct(
	size uint,
	offset uint,
	result reflect.Value,
	depth int,
) (uint, error) {
	resultType := result.Type()

	fieldMapMu.RLock()
	fields, ok := fieldMap[resultType]
	fieldMapMu.RUnlock()
	if !ok {
		numFields := resultType.NumField()
		namedFields := make(map[string]int, numFields)
		var anonymous []int
		for i := 0; i < numFields; i++ {
			field := resultType.Field(i)

			fieldName := field.Name
			if tag := field.Tag.Get("maxminddb"); tag != "" {
				if tag == "-" {
					continue
				}
				fieldName = tag
			}
			if field.Anonymous {
				anonymous = append(anonymous, i)
				continue
			}
			namedFields[fieldName] = i
		}
		fieldMapMu.Lock()
		fields = &fieldsType{namedFields, anonymous}
		fieldMap[resultType] = fields
		fieldMapMu.Unlock()
	}

	// This fills in embedded structs
	for _, i := range fields.anonymousFields {
		_, err := d.unmarshalMap(size, offset, result.Field(i), depth)
		if err != nil {
			return 0, err
		}
	}

	// This handles named fields
	for i := uint(0); i < size; i++ {
		var (
			err error
			key []byte
		)
		key, offset, err = d.decodeKey(offset)
		if err != nil {
			return 0, err
		}
		// The string() does not create a copy due to this compiler
		// optimization: https://github.com/golang/go/issues/3512
		j, ok := fields.namedFields[string(key)]
		if !ok {
			offset, err = d.nextValueOffset(offset, 1)
			if err != nil {
				return 0, err
			}
			continue
		}

		offset, err = d.decode(offset, result.Field(j), depth)
		if err != nil {
			return 0, err
		}
	}
	return offset, nil
}

func (d *decoder) decodeUint(size uint, offset uint) (uint64, uint, error) {
	newOffset := offset + size
	bytes := d.buffer[offset:newOffset]

	var val uint64
	for _, b := range bytes {
		val = (val << 8) | uint64(b)
	}
	return val, newOffset, nil
}

func (d *decoder) decodeUint128(size uint, offset uint) (*big.Int, uint, error) {
	newOffset := offset + size
	val := new(big.Int)
	val.SetBytes(d.buffer[offset:newOffset])

	return val, newOffset, nil
}

func uintFromBytes(prefix uint, uintBytes []byte) uint {
	val := prefix
	for _, b := range uintBytes {
		val = (val << 8) | uint(b)
	}
	return val
}

// decodeKey decodes a map key into []byte slice. We use a []byte so that we
// can take advantage of https://github.com/golang/go/issues/3512 to avoid
// copying the bytes when decoding a struct. Previously, we achieved this by
// using unsafe.
func (d *decoder) decodeKey(offset uint) ([]byte, uint, error) {
	typeNum, size, dataOffset, err := d.decodeCtrlData(offset)
	if err != nil {
		return nil, 0, err
	}
	if typeNum == _Pointer {
		pointer, ptrOffset, err := d.decodePointer(size, dataOffset)
		if err != nil {
			return nil, 0, err
		}
		key, _, err := d.decodeKey(pointer)
		return key, ptrOffset, err
	}
	if typeNum != _String {
		return nil, 0, newInvalidDatabaseError("unexpected type when decoding string: %v", typeNum)
	}
	newOffset := dataOffset + size
	if newOffset > uint(len(d.buffer)) {
		return nil, 0, newOffsetError()
	}
	return d.buffer[dataOffset:newOffset], newOffset, nil
}

// This function is used to skip ahead to the next value without decoding
// the one at the offset passed in. The size bits have different meanings for
// different data types
func (d *decoder) nextValueOffset(offset uint, numberToSkip uint) (uint, error) {
	if numberToSkip == 0 {
		return offset, nil
	}
	typeNum, size, offset, err := d.decodeCtrlData(offset)
	if err != nil {
		return 0, err
	}
	switch typeNum {
	case _Pointer:
		_, offset, err = d.decodePointer(size, offset)
		if err != nil {
			return 0, err
		}
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
