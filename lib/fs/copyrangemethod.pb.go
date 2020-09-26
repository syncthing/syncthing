// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: lib/fs/copyrangemethod.proto

package fs

import (
	fmt "fmt"
	_ "github.com/gogo/protobuf/gogoproto"
	proto "github.com/gogo/protobuf/proto"
	math "math"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.GoGoProtoPackageIsVersion3 // please upgrade the proto package

type CopyRangeMethod int32

const (
	CopyRangeMethodStandard         CopyRangeMethod = 0
	CopyRangeMethodIoctl            CopyRangeMethod = 1
	CopyRangeMethodCopyFileRange    CopyRangeMethod = 2
	CopyRangeMethodSendFile         CopyRangeMethod = 3
	CopyRangeMethodDuplicateExtents CopyRangeMethod = 4
	CopyRangeMethodAllWithFallback  CopyRangeMethod = 5
)

var CopyRangeMethod_name = map[int32]string{
	0: "COPY_RANGE_METHOD_STANDARD",
	1: "COPY_RANGE_METHOD_IOCTL",
	2: "COPY_RANGE_METHOD_COPY_FILE_RANGE",
	3: "COPY_RANGE_METHOD_SEND_FILE",
	4: "COPY_RANGE_METHOD_DUPLICATE_EXTENTS",
	5: "COPY_RANGE_METHOD_ALL_WITH_FALLBACK",
}

var CopyRangeMethod_value = map[string]int32{
	"COPY_RANGE_METHOD_STANDARD":          0,
	"COPY_RANGE_METHOD_IOCTL":             1,
	"COPY_RANGE_METHOD_COPY_FILE_RANGE":   2,
	"COPY_RANGE_METHOD_SEND_FILE":         3,
	"COPY_RANGE_METHOD_DUPLICATE_EXTENTS": 4,
	"COPY_RANGE_METHOD_ALL_WITH_FALLBACK": 5,
}

func (CopyRangeMethod) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_78e1061c3022e87e, []int{0}
}

func init() {
	proto.RegisterEnum("fs.CopyRangeMethod", CopyRangeMethod_name, CopyRangeMethod_value)
}

func init() { proto.RegisterFile("lib/fs/copyrangemethod.proto", fileDescriptor_78e1061c3022e87e) }

var fileDescriptor_78e1061c3022e87e = []byte{
	// 395 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x6c, 0x92, 0x3f, 0x6f, 0xd3, 0x40,
	0x18, 0xc6, 0xed, 0xb6, 0x30, 0x78, 0xc1, 0xb2, 0x90, 0x8a, 0xae, 0xd5, 0x61, 0x88, 0x58, 0x18,
	0xea, 0x01, 0x31, 0xc1, 0x72, 0xb5, 0xcf, 0xad, 0x95, 0xab, 0x53, 0x25, 0x46, 0x01, 0x16, 0xcb,
	0xff, 0x62, 0x5b, 0x5c, 0x7c, 0x96, 0x7d, 0x91, 0xc8, 0x57, 0xf0, 0xc4, 0x17, 0xb0, 0xc4, 0xc0,
	0xc0, 0xc2, 0xf7, 0xc8, 0x98, 0x31, 0x6b, 0xe2, 0x2f, 0x82, 0x72, 0x59, 0x2a, 0x27, 0xdb, 0xfb,
	0x3e, 0x7a, 0x7f, 0x3f, 0xbd, 0xc3, 0xa3, 0x5c, 0xd3, 0x3c, 0x34, 0x66, 0xb5, 0x11, 0xb1, 0x72,
	0x59, 0x05, 0x45, 0x9a, 0xcc, 0x13, 0x9e, 0xb1, 0xf8, 0xa6, 0xac, 0x18, 0x67, 0xda, 0xd9, 0xac,
	0x06, 0x83, 0x2a, 0x29, 0x59, 0x6d, 0x88, 0x20, 0x5c, 0xcc, 0x8c, 0x94, 0xa5, 0x4c, 0x2c, 0x62,
	0x3a, 0x1c, 0xbe, 0xff, 0x77, 0xae, 0xbc, 0x30, 0x59, 0xb9, 0x1c, 0xef, 0x15, 0x0f, 0x42, 0xa1,
	0x7d, 0x52, 0x80, 0x39, 0x7a, 0xfc, 0xe6, 0x8f, 0x91, 0x7b, 0x87, 0xfd, 0x07, 0xec, 0xdd, 0x8f,
	0x2c, 0x7f, 0xe2, 0x21, 0xd7, 0x42, 0x63, 0x4b, 0x95, 0xc0, 0x55, 0xd3, 0xea, 0x97, 0x3d, 0x68,
	0xc2, 0x83, 0x22, 0x0e, 0xaa, 0x58, 0xfb, 0xa8, 0x5c, 0x1e, 0xc3, 0xce, 0xc8, 0xf4, 0x88, 0x2a,
	0x83, 0x57, 0x4d, 0xab, 0xbf, 0xec, 0x91, 0x0e, 0x8b, 0x38, 0xd5, 0xee, 0x94, 0x37, 0xc7, 0x98,
	0x48, 0x6c, 0x87, 0xe0, 0x43, 0xac, 0x9e, 0x01, 0xbd, 0x69, 0xf5, 0xeb, 0x9e, 0x60, 0xbf, 0xda,
	0x39, 0x4d, 0x44, 0xa4, 0x7d, 0x56, 0xae, 0x4e, 0x3c, 0x8f, 0x5d, 0x4b, 0x88, 0xd4, 0xf3, 0xd3,
	0xdf, 0x27, 0x45, 0xbc, 0x57, 0x68, 0x44, 0x19, 0x1c, 0xd3, 0xd6, 0x97, 0x47, 0xe2, 0x98, 0xc8,
	0xc3, 0x3e, 0xfe, 0xea, 0x61, 0xd7, 0x9b, 0xa8, 0x17, 0x60, 0xd0, 0xb4, 0xfa, 0xeb, 0x9e, 0xc5,
	0x5a, 0x94, 0x34, 0x8f, 0x02, 0x9e, 0xe0, 0x9f, 0x3c, 0x29, 0x78, 0xad, 0x0d, 0x4f, 0xd9, 0x10,
	0x21, 0xfe, 0xd4, 0xf1, 0xee, 0x7d, 0x1b, 0x11, 0x72, 0x8b, 0xcc, 0xa1, 0xfa, 0x0c, 0xbc, 0x6d,
	0x5a, 0x1d, 0xf6, 0x6c, 0x88, 0xd2, 0x69, 0xce, 0x33, 0x3b, 0xa0, 0x34, 0x0c, 0xa2, 0x1f, 0xe0,
	0xe2, 0xef, 0x1f, 0x28, 0xdd, 0x0e, 0x57, 0x5b, 0x28, 0xad, 0xb7, 0x50, 0x5a, 0xed, 0xa0, 0xbc,
	0xde, 0x41, 0x79, 0xb3, 0x83, 0xf2, 0xaf, 0x0e, 0x4a, 0xbf, 0x3b, 0x28, 0xaf, 0x3b, 0x28, 0x6d,
	0x3a, 0x28, 0x7d, 0x7f, 0x97, 0xe6, 0x3c, 0x5b, 0x84, 0x37, 0x11, 0x9b, 0x1b, 0xf5, 0xb2, 0x88,
	0x78, 0x96, 0x17, 0xe9, 0x93, 0xe9, 0xd0, 0x9d, 0xf0, 0xb9, 0xe8, 0xc0, 0x87, 0xff, 0x01, 0x00,
	0x00, 0xff, 0xff, 0x72, 0xfc, 0x31, 0xcb, 0x4c, 0x02, 0x00, 0x00,
}
