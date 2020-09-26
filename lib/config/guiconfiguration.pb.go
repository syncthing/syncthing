// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: lib/config/guiconfiguration.proto

package config

import (
	fmt "fmt"
	proto "github.com/gogo/protobuf/proto"
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

type GUIConfiguration struct {
	Enabled                   bool     `protobuf:"varint,1,opt,name=enabled,proto3" json:"enabled" xml:"enabled,attr" default:"true"`
	RawAddress                string   `protobuf:"bytes,2,opt,name=address,proto3" json:"address" xml:"address" default:"127.0.0.1:8384"`
	RawUnixSocketPermissions  string   `protobuf:"bytes,3,opt,name=unix_socket_permissions,json=unixSocketPermissions,proto3" json:"unixSocketPermissions" xml:"unixSocketPermissions,omitempty"`
	User                      string   `protobuf:"bytes,4,opt,name=user,proto3" json:"user" xml:"user,omitempty"`
	Password                  string   `protobuf:"bytes,5,opt,name=password,proto3" json:"password" xml:"password,omitempty"`
	AuthMode                  AuthMode `protobuf:"varint,6,opt,name=auth_mode,json=authMode,proto3,enum=config.AuthMode" json:"authMode" xml:"authMode,omitempty"`
	RawUseTLS                 bool     `protobuf:"varint,7,opt,name=use_tls,json=useTls,proto3" json:"useTLS" xml:"tls,attr"`
	APIKey                    string   `protobuf:"bytes,8,opt,name=api_key,json=apiKey,proto3" json:"apiKey" xml:"apikey,omitempty"`
	InsecureAdminAccess       bool     `protobuf:"varint,9,opt,name=insecure_admin_access,json=insecureAdminAccess,proto3" json:"insecureAdminAccess" xml:"insecureAdminAccess,omitempty"`
	Theme                     string   `protobuf:"bytes,10,opt,name=theme,proto3" json:"theme" xml:"theme" default:"default"`
	Debugging                 bool     `protobuf:"varint,11,opt,name=debugging,proto3" json:"debugging" xml:"debugging,attr"`
	InsecureSkipHostCheck     bool     `protobuf:"varint,12,opt,name=insecure_skip_host_check,json=insecureSkipHostCheck,proto3" json:"insecureSkipHostcheck" xml:"insecureSkipHostcheck,omitempty"`
	InsecureAllowFrameLoading bool     `protobuf:"varint,13,opt,name=insecure_allow_frame_loading,json=insecureAllowFrameLoading,proto3" json:"insecureAllowFrameLoading" xml:"insecureAllowFrameLoading,omitempty"`
}

func (m *GUIConfiguration) Reset()         { *m = GUIConfiguration{} }
func (m *GUIConfiguration) String() string { return proto.CompactTextString(m) }
func (*GUIConfiguration) ProtoMessage()    {}
func (*GUIConfiguration) Descriptor() ([]byte, []int) {
	return fileDescriptor_2a9586d611855d64, []int{0}
}
func (m *GUIConfiguration) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *GUIConfiguration) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	b = b[:cap(b)]
	n, err := m.MarshalToSizedBuffer(b)
	if err != nil {
		return nil, err
	}
	return b[:n], nil
}
func (m *GUIConfiguration) XXX_Merge(src proto.Message) {
	xxx_messageInfo_GUIConfiguration.Merge(m, src)
}
func (m *GUIConfiguration) XXX_Size() int {
	return m.ProtoSize()
}
func (m *GUIConfiguration) XXX_DiscardUnknown() {
	xxx_messageInfo_GUIConfiguration.DiscardUnknown(m)
}

var xxx_messageInfo_GUIConfiguration proto.InternalMessageInfo

func init() {
	proto.RegisterType((*GUIConfiguration)(nil), "config.GUIConfiguration")
}

func init() { proto.RegisterFile("lib/config/guiconfiguration.proto", fileDescriptor_2a9586d611855d64) }

var fileDescriptor_2a9586d611855d64 = []byte{
	// 840 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x8c, 0x55, 0xcd, 0x6e, 0xdb, 0x46,
	0x10, 0x16, 0x5b, 0x47, 0xb2, 0xb6, 0xae, 0x60, 0xb0, 0x4d, 0xcb, 0x04, 0x0d, 0xd7, 0x51, 0xd8,
	0xc2, 0x01, 0x02, 0x39, 0x71, 0x5a, 0x24, 0xf0, 0xa1, 0x80, 0x1c, 0x20, 0x4d, 0x60, 0x17, 0x0d,
	0xe8, 0xfa, 0x92, 0x0b, 0xb1, 0x22, 0xd7, 0xd2, 0x42, 0xfc, 0x2b, 0x77, 0x09, 0x5b, 0x87, 0xf6,
	0x19, 0x0a, 0xf5, 0x5c, 0xa0, 0xcf, 0xd0, 0x4b, 0x5f, 0x21, 0x37, 0xe9, 0x54, 0xf8, 0xb4, 0x40,
	0xa4, 0x1b, 0x8f, 0x3c, 0xe6, 0x54, 0xec, 0xf2, 0x47, 0xa2, 0xac, 0xd4, 0xb9, 0xed, 0x7c, 0xf3,
	0xcd, 0x7c, 0x33, 0xc3, 0x19, 0x10, 0xdc, 0x75, 0x49, 0x6f, 0xcf, 0x0e, 0xfc, 0x33, 0xd2, 0xdf,
	0xeb, 0xc7, 0x24, 0x7b, 0xc5, 0x11, 0x62, 0x24, 0xf0, 0x3b, 0x61, 0x14, 0xb0, 0x40, 0xad, 0x67,
	0xe0, 0xed, 0x5b, 0x4b, 0x54, 0x14, 0xb3, 0x81, 0x17, 0x38, 0x38, 0xa3, 0xdc, 0x6e, 0xe2, 0x0b,
	0x96, 0x3d, 0xdb, 0x97, 0x5b, 0x60, 0xfb, 0x87, 0xd3, 0x97, 0xcf, 0x96, 0x13, 0xa9, 0x3d, 0xd0,
	0xc0, 0x3e, 0xea, 0xb9, 0xd8, 0xd1, 0x94, 0x1d, 0x65, 0x77, 0xf3, 0xf0, 0x45, 0xc2, 0x61, 0x01,
	0xa5, 0x1c, 0xde, 0xbd, 0xf0, 0xdc, 0x83, 0x76, 0x6e, 0x3f, 0x40, 0x8c, 0x45, 0xed, 0x1d, 0x07,
	0x9f, 0xa1, 0xd8, 0x65, 0x07, 0x6d, 0x16, 0xc5, 0xb8, 0x9d, 0x4c, 0x8c, 0xad, 0x65, 0xff, 0xbb,
	0x89, 0xb1, 0x21, 0x1c, 0x66, 0x91, 0x45, 0xfd, 0x15, 0x34, 0x90, 0xe3, 0x44, 0x98, 0x52, 0xed,
	0xa3, 0x1d, 0x65, 0xb7, 0x79, 0x68, 0xcf, 0x38, 0x04, 0x26, 0x3a, 0xef, 0x66, 0xa8, 0x50, 0xcc,
	0x09, 0x29, 0x87, 0xdf, 0x48, 0xc5, 0xdc, 0x5e, 0x12, 0x7b, 0xb4, 0xff, 0xa4, 0xf3, 0xb0, 0xf3,
	0xb0, 0xf3, 0xe8, 0xe0, 0xe9, 0xe3, 0xa7, 0xdf, 0xb6, 0xdf, 0x4d, 0x8c, 0x56, 0x15, 0x1a, 0x4f,
	0x8d, 0xa5, 0xa4, 0x66, 0x91, 0x52, 0xfd, 0x57, 0x01, 0x5f, 0xc6, 0x3e, 0xb9, 0xb0, 0x68, 0x60,
	0x0f, 0x31, 0xb3, 0x42, 0x1c, 0x79, 0x84, 0x52, 0x12, 0xf8, 0x54, 0xfb, 0x58, 0xd6, 0xf3, 0xa7,
	0x32, 0xe3, 0x50, 0x33, 0xd1, 0xf9, 0xa9, 0x4f, 0x2e, 0x4e, 0x24, 0xeb, 0xd5, 0x82, 0x94, 0x70,
	0x78, 0x33, 0x5e, 0xe7, 0x48, 0x39, 0xfc, 0x5a, 0x16, 0xbb, 0xd6, 0xfb, 0x20, 0xf0, 0x08, 0xc3,
	0x5e, 0xc8, 0x46, 0x62, 0x44, 0xf0, 0x1a, 0xce, 0x78, 0x6a, 0xbc, 0xb7, 0x00, 0x73, 0xbd, 0xbc,
	0xfa, 0x1c, 0x6c, 0xc4, 0x14, 0x47, 0xda, 0x86, 0x6c, 0x62, 0x3f, 0xe1, 0x50, 0xda, 0x29, 0x87,
	0x9f, 0x67, 0x65, 0x51, 0x1c, 0x55, 0xab, 0x68, 0x55, 0x21, 0x53, 0xf2, 0xd5, 0xd7, 0x60, 0x33,
	0x44, 0x94, 0x9e, 0x07, 0x91, 0xa3, 0xdd, 0x90, 0xb9, 0xbe, 0x4f, 0x38, 0x2c, 0xb1, 0x94, 0x43,
	0x4d, 0xe6, 0x2b, 0x80, 0x6a, 0x4e, 0xf5, 0x2a, 0x6c, 0x96, 0xb1, 0xaa, 0x07, 0x9a, 0x62, 0x23,
	0x2d, 0xb1, 0x92, 0x5a, 0x7d, 0x47, 0xd9, 0x6d, 0xed, 0x6f, 0x77, 0xb2, 0x55, 0xed, 0x74, 0x63,
	0x36, 0xf8, 0x31, 0x70, 0x70, 0x26, 0x87, 0x72, 0xab, 0x94, 0x2b, 0x80, 0x15, 0xb9, 0xab, 0xb0,
	0x59, 0xc6, 0xaa, 0x18, 0x34, 0x62, 0x8a, 0x2d, 0xe6, 0x52, 0xad, 0x21, 0xd7, 0xf9, 0x78, 0xc6,
	0x61, 0x53, 0x0c, 0x96, 0xe2, 0x9f, 0x8f, 0x4f, 0x12, 0x0e, 0xeb, 0xb1, 0x7c, 0xa5, 0x1c, 0xb6,
	0xa4, 0x0a, 0x73, 0x69, 0xb6, 0xd6, 0xc9, 0xc4, 0xd8, 0x2c, 0x8c, 0x74, 0x62, 0xe4, 0xbc, 0xf1,
	0xd4, 0x58, 0x84, 0x9b, 0x12, 0x74, 0xa9, 0x90, 0x41, 0x21, 0xb1, 0x86, 0x78, 0xa4, 0x6d, 0xca,
	0x81, 0x09, 0x99, 0x7a, 0xf7, 0xd5, 0xcb, 0x23, 0x3c, 0x12, 0x1a, 0x28, 0x24, 0x47, 0x78, 0x94,
	0x72, 0xf8, 0x45, 0xd6, 0x49, 0x48, 0x86, 0x78, 0x54, 0xed, 0x63, 0x7b, 0x15, 0x1c, 0x4f, 0x8d,
	0x3c, 0x83, 0x99, 0xc7, 0xab, 0x7f, 0x28, 0xe0, 0x26, 0xf1, 0x29, 0xb6, 0xe3, 0x08, 0x5b, 0xc8,
	0xf1, 0x88, 0x6f, 0x21, 0xdb, 0x16, 0x77, 0xd4, 0x94, 0xcd, 0x59, 0x09, 0x87, 0x9f, 0x15, 0x84,
	0xae, 0xf0, 0x77, 0xa5, 0x3b, 0xe5, 0xf0, 0x9e, 0x14, 0x5e, 0xe3, 0xab, 0x56, 0x71, 0xe7, 0x7f,
	0x19, 0xe6, 0xba, 0xe4, 0xea, 0x11, 0xb8, 0xc1, 0x06, 0xd8, 0xc3, 0x1a, 0x90, 0xad, 0x7f, 0x97,
	0x70, 0x98, 0x01, 0x29, 0x87, 0x77, 0xb2, 0x99, 0x0a, 0x6b, 0xe9, 0x74, 0xf3, 0x87, 0xb8, 0xd9,
	0x46, 0xfe, 0x36, 0xb3, 0x10, 0xf5, 0x14, 0x34, 0x1d, 0xdc, 0x8b, 0xfb, 0x7d, 0xe2, 0xf7, 0xb5,
	0x4f, 0x64, 0x57, 0x4f, 0x12, 0x0e, 0x17, 0x60, 0xb9, 0xcd, 0x25, 0x52, 0x7e, 0xae, 0x56, 0x15,
	0x32, 0x17, 0x41, 0xea, 0x3f, 0x0a, 0xd0, 0xca, 0xc9, 0xd1, 0x21, 0x09, 0xad, 0x41, 0x40, 0x99,
	0x65, 0x0f, 0xb0, 0x3d, 0xd4, 0xb6, 0xa4, 0xcc, 0x6f, 0xe2, 0xae, 0x0b, 0xce, 0xc9, 0x90, 0x84,
	0x2f, 0x02, 0xca, 0x24, 0xa1, 0xbc, 0xeb, 0xb5, 0xde, 0x95, 0xbb, 0xbe, 0x86, 0x93, 0x4e, 0x8c,
	0xf5, 0x22, 0xe6, 0x15, 0xf8, 0x99, 0x80, 0xd5, 0xbf, 0x15, 0xf0, 0xd5, 0xe2, 0x9b, 0xbb, 0x6e,
	0x70, 0x6e, 0x9d, 0x45, 0xc8, 0xc3, 0x96, 0x1b, 0x20, 0x47, 0x0c, 0xe9, 0x53, 0x59, 0xfd, 0x2f,
	0x09, 0x87, 0xb7, 0xca, 0xaf, 0x23, 0x68, 0xcf, 0x05, 0xeb, 0x38, 0x23, 0xa5, 0x1c, 0xde, 0xaf,
	0x2e, 0xc0, 0x2a, 0xa3, 0xda, 0xc5, 0xbd, 0x0f, 0xe0, 0x99, 0xef, 0x97, 0x3b, 0xfc, 0xe9, 0xcd,
	0x5b, 0xbd, 0x36, 0x7d, 0xab, 0xd7, 0xde, 0xcc, 0x74, 0x65, 0x3a, 0xd3, 0x95, 0xcb, 0x99, 0xae,
	0xfc, 0x3e, 0xd7, 0x6b, 0x7f, 0xcd, 0x75, 0x65, 0x3a, 0xd7, 0x6b, 0x97, 0x73, 0xbd, 0xf6, 0xfa,
	0x7e, 0x9f, 0xb0, 0x41, 0xdc, 0xeb, 0xd8, 0x81, 0xb7, 0x47, 0x47, 0xbe, 0xcd, 0x06, 0xc4, 0xef,
	0x2f, 0xbd, 0x16, 0x7f, 0xb1, 0x5e, 0x5d, 0xfe, 0xb2, 0x1e, 0xff, 0x17, 0x00, 0x00, 0xff, 0xff,
	0x3c, 0xd5, 0xce, 0x57, 0x05, 0x07, 0x00, 0x00,
}

func (m *GUIConfiguration) Marshal() (dAtA []byte, err error) {
	size := m.ProtoSize()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *GUIConfiguration) MarshalTo(dAtA []byte) (int, error) {
	size := m.ProtoSize()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *GUIConfiguration) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if m.InsecureAllowFrameLoading {
		i--
		if m.InsecureAllowFrameLoading {
			dAtA[i] = 1
		} else {
			dAtA[i] = 0
		}
		i--
		dAtA[i] = 0x68
	}
	if m.InsecureSkipHostCheck {
		i--
		if m.InsecureSkipHostCheck {
			dAtA[i] = 1
		} else {
			dAtA[i] = 0
		}
		i--
		dAtA[i] = 0x60
	}
	if m.Debugging {
		i--
		if m.Debugging {
			dAtA[i] = 1
		} else {
			dAtA[i] = 0
		}
		i--
		dAtA[i] = 0x58
	}
	if len(m.Theme) > 0 {
		i -= len(m.Theme)
		copy(dAtA[i:], m.Theme)
		i = encodeVarintGuiconfiguration(dAtA, i, uint64(len(m.Theme)))
		i--
		dAtA[i] = 0x52
	}
	if m.InsecureAdminAccess {
		i--
		if m.InsecureAdminAccess {
			dAtA[i] = 1
		} else {
			dAtA[i] = 0
		}
		i--
		dAtA[i] = 0x48
	}
	if len(m.APIKey) > 0 {
		i -= len(m.APIKey)
		copy(dAtA[i:], m.APIKey)
		i = encodeVarintGuiconfiguration(dAtA, i, uint64(len(m.APIKey)))
		i--
		dAtA[i] = 0x42
	}
	if m.RawUseTLS {
		i--
		if m.RawUseTLS {
			dAtA[i] = 1
		} else {
			dAtA[i] = 0
		}
		i--
		dAtA[i] = 0x38
	}
	if m.AuthMode != 0 {
		i = encodeVarintGuiconfiguration(dAtA, i, uint64(m.AuthMode))
		i--
		dAtA[i] = 0x30
	}
	if len(m.Password) > 0 {
		i -= len(m.Password)
		copy(dAtA[i:], m.Password)
		i = encodeVarintGuiconfiguration(dAtA, i, uint64(len(m.Password)))
		i--
		dAtA[i] = 0x2a
	}
	if len(m.User) > 0 {
		i -= len(m.User)
		copy(dAtA[i:], m.User)
		i = encodeVarintGuiconfiguration(dAtA, i, uint64(len(m.User)))
		i--
		dAtA[i] = 0x22
	}
	if len(m.RawUnixSocketPermissions) > 0 {
		i -= len(m.RawUnixSocketPermissions)
		copy(dAtA[i:], m.RawUnixSocketPermissions)
		i = encodeVarintGuiconfiguration(dAtA, i, uint64(len(m.RawUnixSocketPermissions)))
		i--
		dAtA[i] = 0x1a
	}
	if len(m.RawAddress) > 0 {
		i -= len(m.RawAddress)
		copy(dAtA[i:], m.RawAddress)
		i = encodeVarintGuiconfiguration(dAtA, i, uint64(len(m.RawAddress)))
		i--
		dAtA[i] = 0x12
	}
	if m.Enabled {
		i--
		if m.Enabled {
			dAtA[i] = 1
		} else {
			dAtA[i] = 0
		}
		i--
		dAtA[i] = 0x8
	}
	return len(dAtA) - i, nil
}

func encodeVarintGuiconfiguration(dAtA []byte, offset int, v uint64) int {
	offset -= sovGuiconfiguration(v)
	base := offset
	for v >= 1<<7 {
		dAtA[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	dAtA[offset] = uint8(v)
	return base
}
func (m *GUIConfiguration) ProtoSize() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	if m.Enabled {
		n += 2
	}
	l = len(m.RawAddress)
	if l > 0 {
		n += 1 + l + sovGuiconfiguration(uint64(l))
	}
	l = len(m.RawUnixSocketPermissions)
	if l > 0 {
		n += 1 + l + sovGuiconfiguration(uint64(l))
	}
	l = len(m.User)
	if l > 0 {
		n += 1 + l + sovGuiconfiguration(uint64(l))
	}
	l = len(m.Password)
	if l > 0 {
		n += 1 + l + sovGuiconfiguration(uint64(l))
	}
	if m.AuthMode != 0 {
		n += 1 + sovGuiconfiguration(uint64(m.AuthMode))
	}
	if m.RawUseTLS {
		n += 2
	}
	l = len(m.APIKey)
	if l > 0 {
		n += 1 + l + sovGuiconfiguration(uint64(l))
	}
	if m.InsecureAdminAccess {
		n += 2
	}
	l = len(m.Theme)
	if l > 0 {
		n += 1 + l + sovGuiconfiguration(uint64(l))
	}
	if m.Debugging {
		n += 2
	}
	if m.InsecureSkipHostCheck {
		n += 2
	}
	if m.InsecureAllowFrameLoading {
		n += 2
	}
	return n
}

func sovGuiconfiguration(x uint64) (n int) {
	return (math_bits.Len64(x|1) + 6) / 7
}
func sozGuiconfiguration(x uint64) (n int) {
	return sovGuiconfiguration(uint64((x << 1) ^ uint64((int64(x) >> 63))))
}
func (m *GUIConfiguration) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowGuiconfiguration
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
			return fmt.Errorf("proto: GUIConfiguration: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: GUIConfiguration: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Enabled", wireType)
			}
			var v int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowGuiconfiguration
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				v |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			m.Enabled = bool(v != 0)
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field RawAddress", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowGuiconfiguration
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
				return ErrInvalidLengthGuiconfiguration
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLengthGuiconfiguration
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.RawAddress = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 3:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field RawUnixSocketPermissions", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowGuiconfiguration
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
				return ErrInvalidLengthGuiconfiguration
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLengthGuiconfiguration
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.RawUnixSocketPermissions = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 4:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field User", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowGuiconfiguration
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
				return ErrInvalidLengthGuiconfiguration
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLengthGuiconfiguration
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.User = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 5:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Password", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowGuiconfiguration
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
				return ErrInvalidLengthGuiconfiguration
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLengthGuiconfiguration
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Password = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 6:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field AuthMode", wireType)
			}
			m.AuthMode = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowGuiconfiguration
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.AuthMode |= AuthMode(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 7:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field RawUseTLS", wireType)
			}
			var v int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowGuiconfiguration
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				v |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			m.RawUseTLS = bool(v != 0)
		case 8:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field APIKey", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowGuiconfiguration
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
				return ErrInvalidLengthGuiconfiguration
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLengthGuiconfiguration
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.APIKey = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 9:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field InsecureAdminAccess", wireType)
			}
			var v int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowGuiconfiguration
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				v |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			m.InsecureAdminAccess = bool(v != 0)
		case 10:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Theme", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowGuiconfiguration
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
				return ErrInvalidLengthGuiconfiguration
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLengthGuiconfiguration
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Theme = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 11:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Debugging", wireType)
			}
			var v int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowGuiconfiguration
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				v |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			m.Debugging = bool(v != 0)
		case 12:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field InsecureSkipHostCheck", wireType)
			}
			var v int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowGuiconfiguration
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				v |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			m.InsecureSkipHostCheck = bool(v != 0)
		case 13:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field InsecureAllowFrameLoading", wireType)
			}
			var v int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowGuiconfiguration
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				v |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			m.InsecureAllowFrameLoading = bool(v != 0)
		default:
			iNdEx = preIndex
			skippy, err := skipGuiconfiguration(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthGuiconfiguration
			}
			if (iNdEx + skippy) < 0 {
				return ErrInvalidLengthGuiconfiguration
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
func skipGuiconfiguration(dAtA []byte) (n int, err error) {
	l := len(dAtA)
	iNdEx := 0
	depth := 0
	for iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, ErrIntOverflowGuiconfiguration
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
					return 0, ErrIntOverflowGuiconfiguration
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
					return 0, ErrIntOverflowGuiconfiguration
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
				return 0, ErrInvalidLengthGuiconfiguration
			}
			iNdEx += length
		case 3:
			depth++
		case 4:
			if depth == 0 {
				return 0, ErrUnexpectedEndOfGroupGuiconfiguration
			}
			depth--
		case 5:
			iNdEx += 4
		default:
			return 0, fmt.Errorf("proto: illegal wireType %d", wireType)
		}
		if iNdEx < 0 {
			return 0, ErrInvalidLengthGuiconfiguration
		}
		if depth == 0 {
			return iNdEx, nil
		}
	}
	return 0, io.ErrUnexpectedEOF
}

var (
	ErrInvalidLengthGuiconfiguration        = fmt.Errorf("proto: negative length found during unmarshaling")
	ErrIntOverflowGuiconfiguration          = fmt.Errorf("proto: integer overflow")
	ErrUnexpectedEndOfGroupGuiconfiguration = fmt.Errorf("proto: unexpected end of group")
)
