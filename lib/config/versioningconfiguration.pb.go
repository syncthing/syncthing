// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: lib/config/versioningconfiguration.proto

package config

import (
	fmt "fmt"
	proto "github.com/gogo/protobuf/proto"
	github_com_gogo_protobuf_sortkeys "github.com/gogo/protobuf/sortkeys"
	_ "github.com/syncthing/syncthing/proto/ext"
	io "io"
	math "math"
	math_bits "math/bits"
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

// VersioningConfiguration is used in the code and for JSON serialization
type VersioningConfiguration struct {
	Type             string            `protobuf:"bytes,1,opt,name=type,proto3" json:"type" `
	Params           map[string]string `protobuf:"bytes,2,rep,name=parameters,proto3" json:"params"  protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
	CleanupIntervalS int               `protobuf:"varint,3,opt,name=cleanup_interval_s,json=cleanupIntervalS,proto3,casttype=int" json:"cleanupIntervalS" default:"3600"`
}

func (m *VersioningConfiguration) Reset()         { *m = VersioningConfiguration{} }
func (m *VersioningConfiguration) String() string { return proto.CompactTextString(m) }
func (*VersioningConfiguration) ProtoMessage()    {}
func (*VersioningConfiguration) Descriptor() ([]byte, []int) {
	return fileDescriptor_95ba6bdb22ffea81, []int{0}
}
func (m *VersioningConfiguration) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *VersioningConfiguration) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	b = b[:cap(b)]
	n, err := m.MarshalToSizedBuffer(b)
	if err != nil {
		return nil, err
	}
	return b[:n], nil
}
func (m *VersioningConfiguration) XXX_Merge(src proto.Message) {
	xxx_messageInfo_VersioningConfiguration.Merge(m, src)
}
func (m *VersioningConfiguration) XXX_Size() int {
	return m.ProtoSize()
}
func (m *VersioningConfiguration) XXX_DiscardUnknown() {
	xxx_messageInfo_VersioningConfiguration.DiscardUnknown(m)
}

var xxx_messageInfo_VersioningConfiguration proto.InternalMessageInfo

func init() {
	proto.RegisterType((*VersioningConfiguration)(nil), "config.VersioningConfiguration")
	proto.RegisterMapType((map[string]string)(nil), "config.VersioningConfiguration.ParametersEntry")
}

func init() {
	proto.RegisterFile("lib/config/versioningconfiguration.proto", fileDescriptor_95ba6bdb22ffea81)
}

var fileDescriptor_95ba6bdb22ffea81 = []byte{
	// 389 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x74, 0x52, 0xbf, 0x8b, 0xd4, 0x40,
	0x18, 0x9d, 0xd9, 0xec, 0x05, 0x6e, 0xce, 0x5f, 0xa4, 0x31, 0x1c, 0x38, 0x13, 0xd6, 0x14, 0xb1,
	0x49, 0x16, 0x0f, 0x45, 0xb6, 0x8c, 0x58, 0x58, 0x29, 0x2b, 0x08, 0xda, 0x1c, 0xd9, 0x38, 0x97,
	0x1b, 0xcc, 0x4d, 0x42, 0x32, 0x59, 0x4c, 0x69, 0x21, 0x58, 0xaa, 0x7f, 0x81, 0x7f, 0xce, 0x76,
	0x9b, 0x72, 0xab, 0x81, 0x4d, 0xba, 0x94, 0x5b, 0x6e, 0x25, 0x99, 0x84, 0x75, 0x51, 0xae, 0x7b,
	0xef, 0x7d, 0xef, 0x7d, 0x2f, 0x7c, 0x19, 0xe4, 0xc4, 0x6c, 0xe1, 0x85, 0x09, 0xbf, 0x62, 0x91,
	0xb7, 0xa4, 0x59, 0xce, 0x12, 0xce, 0x78, 0xd4, 0x0b, 0x45, 0x16, 0x08, 0x96, 0x70, 0x37, 0xcd,
	0x12, 0x91, 0x18, 0x7a, 0x2f, 0x9e, 0x9f, 0xd2, 0x2f, 0xa2, 0x97, 0x26, 0xdf, 0x34, 0xf4, 0xf0,
	0xfd, 0x21, 0xf4, 0xf2, 0x38, 0x64, 0x58, 0x68, 0x2c, 0xca, 0x94, 0x9a, 0xd0, 0x82, 0xce, 0xa9,
	0x7f, 0xa7, 0x95, 0x44, 0xf1, 0x9d, 0x24, 0x60, 0xae, 0x90, 0xf1, 0x15, 0x22, 0x94, 0x06, 0x59,
	0x70, 0x43, 0x05, 0xcd, 0x72, 0x73, 0x64, 0x69, 0xce, 0xd9, 0x53, 0xcf, 0xed, 0x6b, 0xdc, 0x5b,
	0xf6, 0xba, 0x6f, 0x0f, 0x89, 0x57, 0x5c, 0x64, 0xa5, 0x3f, 0x5d, 0x49, 0x02, 0x6a, 0x49, 0x74,
	0x35, 0xc8, 0x5b, 0x49, 0x74, 0xb5, 0x34, 0xef, 0x9a, 0x76, 0x6b, 0x7b, 0x60, 0xbf, 0x2a, 0x7b,
	0x70, 0xcc, 0x8f, 0x4a, 0x8d, 0x10, 0x19, 0x61, 0x4c, 0x03, 0x5e, 0xa4, 0x97, 0x8c, 0x0b, 0x9a,
	0x2d, 0x83, 0xf8, 0x32, 0x37, 0x35, 0x0b, 0x3a, 0x27, 0xfe, 0xb3, 0x56, 0x92, 0x07, 0xc3, 0xf4,
	0xf5, 0x30, 0x7c, 0xb7, 0x93, 0xe4, 0xde, 0x27, 0x7a, 0x15, 0x14, 0xb1, 0x98, 0x4d, 0x2e, 0x9e,
	0x4f, 0xa7, 0x93, 0xbd, 0x24, 0x1a, 0xe3, 0x62, 0xbf, 0xb6, 0xc7, 0x1d, 0x9f, 0xff, 0x17, 0x39,
	0xff, 0x80, 0xee, 0xff, 0xf3, 0xd5, 0xc6, 0x23, 0xa4, 0x7d, 0xa6, 0xe5, 0x70, 0x9c, 0xb3, 0x56,
	0x92, 0x8e, 0xaa, 0xdb, 0x74, 0xc0, 0x78, 0x8c, 0x4e, 0x96, 0x41, 0x5c, 0x50, 0x73, 0xa4, 0x0c,
	0x77, 0x5b, 0x49, 0x7a, 0x41, 0x59, 0x7a, 0x38, 0x1b, 0xbd, 0x80, 0xb3, 0xf1, 0xf7, 0x9f, 0x36,
	0xf0, 0xdf, 0xac, 0xb6, 0x18, 0x54, 0x5b, 0x0c, 0x56, 0x35, 0x86, 0x55, 0x8d, 0xe1, 0xa6, 0xc6,
	0xf0, 0x47, 0x83, 0xc1, 0xef, 0x06, 0xc3, 0xaa, 0xc1, 0x60, 0xd3, 0x60, 0xf0, 0xf1, 0x49, 0xc4,
	0xc4, 0x75, 0xb1, 0x70, 0xc3, 0xe4, 0xc6, 0xcb, 0x4b, 0x1e, 0x8a, 0x6b, 0xc6, 0xa3, 0x23, 0xf4,
	0xf7, 0x25, 0x2c, 0x74, 0xf5, 0x7f, 0x2f, 0xfe, 0x04, 0x00, 0x00, 0xff, 0xff, 0x3a, 0xf7, 0x8a,
	0xe0, 0x1e, 0x02, 0x00, 0x00,
}

func (m *VersioningConfiguration) Marshal() (dAtA []byte, err error) {
	size := m.ProtoSize()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *VersioningConfiguration) MarshalTo(dAtA []byte) (int, error) {
	size := m.ProtoSize()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *VersioningConfiguration) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if m.CleanupIntervalS != 0 {
		i = encodeVarintVersioningconfiguration(dAtA, i, uint64(m.CleanupIntervalS))
		i--
		dAtA[i] = 0x18
	}
	if len(m.Params) > 0 {
		keysForParams := make([]string, 0, len(m.Params))
		for k := range m.Params {
			keysForParams = append(keysForParams, string(k))
		}
		github_com_gogo_protobuf_sortkeys.Strings(keysForParams)
		for iNdEx := len(keysForParams) - 1; iNdEx >= 0; iNdEx-- {
			v := m.Params[string(keysForParams[iNdEx])]
			baseI := i
			i -= len(v)
			copy(dAtA[i:], v)
			i = encodeVarintVersioningconfiguration(dAtA, i, uint64(len(v)))
			i--
			dAtA[i] = 0x12
			i -= len(keysForParams[iNdEx])
			copy(dAtA[i:], keysForParams[iNdEx])
			i = encodeVarintVersioningconfiguration(dAtA, i, uint64(len(keysForParams[iNdEx])))
			i--
			dAtA[i] = 0xa
			i = encodeVarintVersioningconfiguration(dAtA, i, uint64(baseI-i))
			i--
			dAtA[i] = 0x12
		}
	}
	if len(m.Type) > 0 {
		i -= len(m.Type)
		copy(dAtA[i:], m.Type)
		i = encodeVarintVersioningconfiguration(dAtA, i, uint64(len(m.Type)))
		i--
		dAtA[i] = 0xa
	}
	return len(dAtA) - i, nil
}

func encodeVarintVersioningconfiguration(dAtA []byte, offset int, v uint64) int {
	offset -= sovVersioningconfiguration(v)
	base := offset
	for v >= 1<<7 {
		dAtA[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	dAtA[offset] = uint8(v)
	return base
}
func (m *VersioningConfiguration) ProtoSize() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	l = len(m.Type)
	if l > 0 {
		n += 1 + l + sovVersioningconfiguration(uint64(l))
	}
	if len(m.Params) > 0 {
		for k, v := range m.Params {
			_ = k
			_ = v
			mapEntrySize := 1 + len(k) + sovVersioningconfiguration(uint64(len(k))) + 1 + len(v) + sovVersioningconfiguration(uint64(len(v)))
			n += mapEntrySize + 1 + sovVersioningconfiguration(uint64(mapEntrySize))
		}
	}
	if m.CleanupIntervalS != 0 {
		n += 1 + sovVersioningconfiguration(uint64(m.CleanupIntervalS))
	}
	return n
}

func sovVersioningconfiguration(x uint64) (n int) {
	return (math_bits.Len64(x|1) + 6) / 7
}
func sozVersioningconfiguration(x uint64) (n int) {
	return sovVersioningconfiguration(uint64((x << 1) ^ uint64((int64(x) >> 63))))
}
func (m *VersioningConfiguration) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowVersioningconfiguration
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: VersioningConfiguration: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: VersioningConfiguration: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Type", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowVersioningconfiguration
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthVersioningconfiguration
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLengthVersioningconfiguration
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Type = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Params", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowVersioningconfiguration
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthVersioningconfiguration
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthVersioningconfiguration
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if m.Params == nil {
				m.Params = make(map[string]string)
			}
			var mapkey string
			var mapvalue string
			for iNdEx < postIndex {
				entryPreIndex := iNdEx
				var wire uint64
				for shift := uint(0); ; shift += 7 {
					if shift >= 64 {
						return ErrIntOverflowVersioningconfiguration
					}
					if iNdEx >= l {
						return io.ErrUnexpectedEOF
					}
					b := dAtA[iNdEx]
					iNdEx++
					wire |= uint64(b&0x7F) << shift
					if b < 0x80 {
						break
					}
				}
				fieldNum := int32(wire >> 3)
				if fieldNum == 1 {
					var stringLenmapkey uint64
					for shift := uint(0); ; shift += 7 {
						if shift >= 64 {
							return ErrIntOverflowVersioningconfiguration
						}
						if iNdEx >= l {
							return io.ErrUnexpectedEOF
						}
						b := dAtA[iNdEx]
						iNdEx++
						stringLenmapkey |= uint64(b&0x7F) << shift
						if b < 0x80 {
							break
						}
					}
					intStringLenmapkey := int(stringLenmapkey)
					if intStringLenmapkey < 0 {
						return ErrInvalidLengthVersioningconfiguration
					}
					postStringIndexmapkey := iNdEx + intStringLenmapkey
					if postStringIndexmapkey < 0 {
						return ErrInvalidLengthVersioningconfiguration
					}
					if postStringIndexmapkey > l {
						return io.ErrUnexpectedEOF
					}
					mapkey = string(dAtA[iNdEx:postStringIndexmapkey])
					iNdEx = postStringIndexmapkey
				} else if fieldNum == 2 {
					var stringLenmapvalue uint64
					for shift := uint(0); ; shift += 7 {
						if shift >= 64 {
							return ErrIntOverflowVersioningconfiguration
						}
						if iNdEx >= l {
							return io.ErrUnexpectedEOF
						}
						b := dAtA[iNdEx]
						iNdEx++
						stringLenmapvalue |= uint64(b&0x7F) << shift
						if b < 0x80 {
							break
						}
					}
					intStringLenmapvalue := int(stringLenmapvalue)
					if intStringLenmapvalue < 0 {
						return ErrInvalidLengthVersioningconfiguration
					}
					postStringIndexmapvalue := iNdEx + intStringLenmapvalue
					if postStringIndexmapvalue < 0 {
						return ErrInvalidLengthVersioningconfiguration
					}
					if postStringIndexmapvalue > l {
						return io.ErrUnexpectedEOF
					}
					mapvalue = string(dAtA[iNdEx:postStringIndexmapvalue])
					iNdEx = postStringIndexmapvalue
				} else {
					iNdEx = entryPreIndex
					skippy, err := skipVersioningconfiguration(dAtA[iNdEx:])
					if err != nil {
						return err
					}
					if skippy < 0 {
						return ErrInvalidLengthVersioningconfiguration
					}
					if (iNdEx + skippy) > postIndex {
						return io.ErrUnexpectedEOF
					}
					iNdEx += skippy
				}
			}
			m.Params[mapkey] = mapvalue
			iNdEx = postIndex
		case 3:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field CleanupIntervalS", wireType)
			}
			m.CleanupIntervalS = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowVersioningconfiguration
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.CleanupIntervalS |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		default:
			iNdEx = preIndex
			skippy, err := skipVersioningconfiguration(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthVersioningconfiguration
			}
			if (iNdEx + skippy) < 0 {
				return ErrInvalidLengthVersioningconfiguration
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func skipVersioningconfiguration(dAtA []byte) (n int, err error) {
	l := len(dAtA)
	iNdEx := 0
	depth := 0
	for iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, ErrIntOverflowVersioningconfiguration
			}
			if iNdEx >= l {
				return 0, io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		wireType := int(wire & 0x7)
		switch wireType {
		case 0:
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowVersioningconfiguration
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				iNdEx++
				if dAtA[iNdEx-1] < 0x80 {
					break
				}
			}
		case 1:
			iNdEx += 8
		case 2:
			var length int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowVersioningconfiguration
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				length |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if length < 0 {
				return 0, ErrInvalidLengthVersioningconfiguration
			}
			iNdEx += length
		case 3:
			depth++
		case 4:
			if depth == 0 {
				return 0, ErrUnexpectedEndOfGroupVersioningconfiguration
			}
			depth--
		case 5:
			iNdEx += 4
		default:
			return 0, fmt.Errorf("proto: illegal wireType %d", wireType)
		}
		if iNdEx < 0 {
			return 0, ErrInvalidLengthVersioningconfiguration
		}
		if depth == 0 {
			return iNdEx, nil
		}
	}
	return 0, io.ErrUnexpectedEOF
}

var (
	ErrInvalidLengthVersioningconfiguration        = fmt.Errorf("proto: negative length found during unmarshaling")
	ErrIntOverflowVersioningconfiguration          = fmt.Errorf("proto: integer overflow")
	ErrUnexpectedEndOfGroupVersioningconfiguration = fmt.Errorf("proto: unexpected end of group")
)
