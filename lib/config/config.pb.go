// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: lib/config/config.proto

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

type Configuration struct {
	Version        int                   `protobuf:"varint,1,opt,name=version,proto3,casttype=int" json:"version" xml:"version,attr"`
	Folders        []FolderConfiguration `protobuf:"bytes,2,rep,name=folders,proto3" json:"folders" xml:"folder"`
	Devices        []DeviceConfiguration `protobuf:"bytes,3,rep,name=devices,proto3" json:"devices" xml:"device"`
	GUI            GUIConfiguration      `protobuf:"bytes,4,opt,name=gui,proto3" json:"gui" xml:"gui"`
	LDAP           LDAPConfiguration     `protobuf:"bytes,5,opt,name=ldap,proto3" json:"ldap" xml:"ldap"`
	Options        OptionsConfiguration  `protobuf:"bytes,6,opt,name=options,proto3" json:"options" xml:"options"`
	IgnoredDevices []ObservedDevice      `protobuf:"bytes,7,rep,name=ignored_devices,json=ignoredDevices,proto3" json:"remoteIgnoredDevices" xml:"remoteIgnoredDevice"`
	PendingDevices []ObservedDevice      `protobuf:"bytes,8,rep,name=pending_devices,json=pendingDevices,proto3" json:"pendingDevices" xml:"pendingDevice"`
	Defaults       Defaults              `protobuf:"bytes,9,opt,name=defaults,proto3" json:"defaults" xml:"defaults"`
}

func (m *Configuration) Reset()         { *m = Configuration{} }
func (m *Configuration) String() string { return proto.CompactTextString(m) }
func (*Configuration) ProtoMessage()    {}
func (*Configuration) Descriptor() ([]byte, []int) {
	return fileDescriptor_baadf209193dc627, []int{0}
}
func (m *Configuration) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *Configuration) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_Configuration.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *Configuration) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Configuration.Merge(m, src)
}
func (m *Configuration) XXX_Size() int {
	return m.ProtoSize()
}
func (m *Configuration) XXX_DiscardUnknown() {
	xxx_messageInfo_Configuration.DiscardUnknown(m)
}

var xxx_messageInfo_Configuration proto.InternalMessageInfo

type Defaults struct {
	Folder FolderConfiguration `protobuf:"bytes,1,opt,name=folder,proto3" json:"folder" xml:"folder"`
	Device DeviceConfiguration `protobuf:"bytes,2,opt,name=device,proto3" json:"device" xml:"device"`
}

func (m *Defaults) Reset()         { *m = Defaults{} }
func (m *Defaults) String() string { return proto.CompactTextString(m) }
func (*Defaults) ProtoMessage()    {}
func (*Defaults) Descriptor() ([]byte, []int) {
	return fileDescriptor_baadf209193dc627, []int{1}
}
func (m *Defaults) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *Defaults) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_Defaults.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *Defaults) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Defaults.Merge(m, src)
}
func (m *Defaults) XXX_Size() int {
	return m.ProtoSize()
}
func (m *Defaults) XXX_DiscardUnknown() {
	xxx_messageInfo_Defaults.DiscardUnknown(m)
}

var xxx_messageInfo_Defaults proto.InternalMessageInfo

func init() {
	proto.RegisterType((*Configuration)(nil), "config.Configuration")
	proto.RegisterType((*Defaults)(nil), "config.Defaults")
}

func init() { proto.RegisterFile("lib/config/config.proto", fileDescriptor_baadf209193dc627) }

var fileDescriptor_baadf209193dc627 = []byte{
	// 625 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x84, 0x54, 0x4d, 0x8f, 0xd2, 0x4e,
	0x18, 0x6f, 0x97, 0x5d, 0x58, 0x66, 0x77, 0xd9, 0x7f, 0xba, 0xff, 0x68, 0x51, 0xd3, 0xc1, 0x09,
	0x1a, 0x34, 0xca, 0x26, 0xeb, 0xc5, 0x78, 0x13, 0x89, 0x1b, 0xa2, 0x89, 0x9b, 0x9a, 0x35, 0xea,
	0xc5, 0x00, 0x1d, 0xca, 0x24, 0xd0, 0x92, 0xbe, 0x90, 0xf5, 0x2b, 0x78, 0x32, 0x7e, 0x02, 0xaf,
	0xde, 0xfd, 0x10, 0xdc, 0xe0, 0xe8, 0x69, 0x92, 0x85, 0x1b, 0xc7, 0x1e, 0x3d, 0x99, 0x79, 0x69,
	0x69, 0xdd, 0xaa, 0x27, 0xfa, 0xfc, 0xde, 0x9e, 0x27, 0x0f, 0x33, 0x03, 0xae, 0x8f, 0x48, 0xef,
	0xb8, 0xef, 0x3a, 0x03, 0x62, 0xcb, 0x9f, 0xe6, 0xc4, 0x73, 0x03, 0x57, 0x2b, 0x8a, 0xea, 0x46,
	0x3d, 0x25, 0x18, 0xb8, 0x23, 0x0b, 0x7b, 0xa2, 0x08, 0xbd, 0x6e, 0x40, 0x5c, 0x47, 0xa8, 0x33,
	0x2a, 0x0b, 0x4f, 0x49, 0x1f, 0xe7, 0xa9, 0x6e, 0xa7, 0x54, 0x76, 0x48, 0xf2, 0x24, 0x28, 0x25,
	0x19, 0x59, 0xdd, 0x49, 0x9e, 0xe6, 0x4e, 0x4a, 0xe3, 0x4e, 0x18, 0xe1, 0xe7, 0xc9, 0xaa, 0x69,
	0x59, 0xcf, 0xc7, 0xde, 0x14, 0x5b, 0x92, 0x2a, 0xe3, 0x8b, 0x40, 0x7c, 0xa2, 0x4f, 0x25, 0x70,
	0xf0, 0x2c, 0xed, 0xd6, 0x4c, 0x50, 0x9a, 0x62, 0xcf, 0x27, 0xae, 0xa3, 0xab, 0x35, 0xb5, 0xb1,
	0xd3, 0x7a, 0xbc, 0xa6, 0x30, 0x86, 0x22, 0x0a, 0xb5, 0x8b, 0xf1, 0xe8, 0x09, 0x92, 0xf5, 0x83,
	0x6e, 0x10, 0x78, 0xe8, 0x27, 0x85, 0x05, 0xe2, 0x04, 0xeb, 0x79, 0x7d, 0x3f, 0x8d, 0x9b, 0xb1,
	0x4b, 0x7b, 0x03, 0x4a, 0x62, 0x79, 0xbe, 0xbe, 0x55, 0x2b, 0x34, 0xf6, 0x4e, 0x6e, 0x36, 0xe5,
	0xb6, 0x9f, 0x73, 0x38, 0x33, 0x41, 0x0b, 0xce, 0x28, 0x54, 0x58, 0x53, 0xe9, 0x89, 0x28, 0xdc,
	0xe7, 0x4d, 0x45, 0x8d, 0xcc, 0x98, 0x60, 0xb9, 0x62, 0xdd, 0xbe, 0x5e, 0xc8, 0xe6, 0xb6, 0x39,
	0xfc, 0x87, 0x5c, 0xe9, 0x49, 0x72, 0x45, 0x8d, 0xcc, 0x98, 0xd0, 0x4c, 0x50, 0xb0, 0x43, 0xa2,
	0x6f, 0xd7, 0xd4, 0xc6, 0xde, 0x89, 0x1e, 0x67, 0x9e, 0x9e, 0x77, 0xb2, 0x81, 0x77, 0x59, 0xe0,
	0x92, 0xc2, 0xc2, 0xe9, 0x79, 0x67, 0x4d, 0x21, 0xf3, 0x44, 0x14, 0x96, 0x79, 0xa6, 0x1d, 0x12,
	0xf4, 0x65, 0x51, 0x67, 0x94, 0xc9, 0x08, 0xed, 0x1d, 0xd8, 0x66, 0xff, 0xa8, 0xbe, 0xc3, 0x43,
	0xab, 0x71, 0xe8, 0xcb, 0xf6, 0xd3, 0xb3, 0x6c, 0xea, 0x7d, 0x99, 0xba, 0xcd, 0xa8, 0x35, 0x85,
	0xdc, 0x16, 0x51, 0x08, 0x78, 0x2e, 0x2b, 0x58, 0x30, 0x67, 0x4d, 0xce, 0x69, 0x6f, 0x41, 0x49,
	0x1e, 0x04, 0xbd, 0xc8, 0xd3, 0x6f, 0xc5, 0xe9, 0xaf, 0x04, 0x9c, 0x6d, 0x50, 0x8b, 0xf7, 0x20,
	0x4d, 0x11, 0x85, 0x07, 0x3c, 0x5b, 0xd6, 0xc8, 0x8c, 0x19, 0xed, 0x9b, 0x0a, 0x0e, 0x89, 0xed,
	0xb8, 0x1e, 0xb6, 0x3e, 0xc4, 0x9b, 0x2e, 0xf1, 0x4d, 0x5f, 0x4b, 0x5a, 0xc8, 0xb3, 0x25, 0x36,
	0xde, 0x1a, 0xca, 0xf0, 0xff, 0x3d, 0x3c, 0x76, 0x03, 0xdc, 0x11, 0xe6, 0x76, 0xb2, 0xf1, 0x2a,
	0xef, 0x94, 0x43, 0xa2, 0xf5, 0xbc, 0x7e, 0x94, 0x83, 0x47, 0xf3, 0x7a, 0x6e, 0x96, 0x59, 0x21,
	0x99, 0x5a, 0x73, 0xc0, 0xe1, 0x04, 0x3b, 0x16, 0x71, 0xec, 0x64, 0xd4, 0xdd, 0xbf, 0x8e, 0xfa,
	0x50, 0x8e, 0x5a, 0x91, 0xb6, 0xcd, 0x90, 0x47, 0x7c, 0xc8, 0x0c, 0x8c, 0xcc, 0xdf, 0x64, 0xda,
	0x19, 0xd8, 0xb5, 0xf0, 0xa0, 0x1b, 0x8e, 0x02, 0x5f, 0x2f, 0xf3, 0xb5, 0xff, 0xb7, 0x39, 0x7d,
	0x02, 0x6f, 0x21, 0xd9, 0x22, 0x51, 0x46, 0x14, 0x56, 0xe4, 0x99, 0x13, 0x00, 0x32, 0x13, 0x0e,
	0x7d, 0x57, 0xc1, 0x6e, 0x6c, 0xd5, 0x5e, 0x83, 0xa2, 0x38, 0xe6, 0xfc, 0x1a, 0xfe, 0xe3, 0xca,
	0x18, 0xb2, 0x8f, 0xb4, 0x5c, 0xb9, 0x31, 0x12, 0x67, 0xa1, 0x62, 0x37, 0xfa, 0x56, 0x36, 0x34,
	0xef, 0xbe, 0x24, 0xa1, 0xc2, 0x72, 0xe5, 0xba, 0x48, 0xbc, 0xf5, 0x62, 0x76, 0x69, 0x28, 0x8b,
	0x4b, 0x43, 0x99, 0x2d, 0x0d, 0x75, 0xb1, 0x34, 0xd4, 0xcf, 0x2b, 0x43, 0xf9, 0xba, 0x32, 0xd4,
	0xc5, 0xca, 0x50, 0x7e, 0xac, 0x0c, 0xe5, 0xfd, 0x3d, 0x9b, 0x04, 0xc3, 0xb0, 0xd7, 0xec, 0xbb,
	0xe3, 0x63, 0xff, 0xa3, 0xd3, 0x0f, 0x86, 0xc4, 0xb1, 0x53, 0x5f, 0x9b, 0xa7, 0xaa, 0x57, 0xe4,
	0xef, 0xd2, 0xa3, 0x5f, 0x01, 0x00, 0x00, 0xff, 0xff, 0xfa, 0xd8, 0x34, 0x31, 0x9a, 0x05, 0x00,
	0x00,
}

func (m *Configuration) Marshal() (dAtA []byte, err error) {
	size := m.ProtoSize()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *Configuration) MarshalTo(dAtA []byte) (int, error) {
	size := m.ProtoSize()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *Configuration) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	{
		size, err := m.Defaults.MarshalToSizedBuffer(dAtA[:i])
		if err != nil {
			return 0, err
		}
		i -= size
		i = encodeVarintConfig(dAtA, i, uint64(size))
	}
	i--
	dAtA[i] = 0x4a
	if len(m.PendingDevices) > 0 {
		for iNdEx := len(m.PendingDevices) - 1; iNdEx >= 0; iNdEx-- {
			{
				size, err := m.PendingDevices[iNdEx].MarshalToSizedBuffer(dAtA[:i])
				if err != nil {
					return 0, err
				}
				i -= size
				i = encodeVarintConfig(dAtA, i, uint64(size))
			}
			i--
			dAtA[i] = 0x42
		}
	}
	if len(m.IgnoredDevices) > 0 {
		for iNdEx := len(m.IgnoredDevices) - 1; iNdEx >= 0; iNdEx-- {
			{
				size, err := m.IgnoredDevices[iNdEx].MarshalToSizedBuffer(dAtA[:i])
				if err != nil {
					return 0, err
				}
				i -= size
				i = encodeVarintConfig(dAtA, i, uint64(size))
			}
			i--
			dAtA[i] = 0x3a
		}
	}
	{
		size, err := m.Options.MarshalToSizedBuffer(dAtA[:i])
		if err != nil {
			return 0, err
		}
		i -= size
		i = encodeVarintConfig(dAtA, i, uint64(size))
	}
	i--
	dAtA[i] = 0x32
	{
		size, err := m.LDAP.MarshalToSizedBuffer(dAtA[:i])
		if err != nil {
			return 0, err
		}
		i -= size
		i = encodeVarintConfig(dAtA, i, uint64(size))
	}
	i--
	dAtA[i] = 0x2a
	{
		size, err := m.GUI.MarshalToSizedBuffer(dAtA[:i])
		if err != nil {
			return 0, err
		}
		i -= size
		i = encodeVarintConfig(dAtA, i, uint64(size))
	}
	i--
	dAtA[i] = 0x22
	if len(m.Devices) > 0 {
		for iNdEx := len(m.Devices) - 1; iNdEx >= 0; iNdEx-- {
			{
				size, err := m.Devices[iNdEx].MarshalToSizedBuffer(dAtA[:i])
				if err != nil {
					return 0, err
				}
				i -= size
				i = encodeVarintConfig(dAtA, i, uint64(size))
			}
			i--
			dAtA[i] = 0x1a
		}
	}
	if len(m.Folders) > 0 {
		for iNdEx := len(m.Folders) - 1; iNdEx >= 0; iNdEx-- {
			{
				size, err := m.Folders[iNdEx].MarshalToSizedBuffer(dAtA[:i])
				if err != nil {
					return 0, err
				}
				i -= size
				i = encodeVarintConfig(dAtA, i, uint64(size))
			}
			i--
			dAtA[i] = 0x12
		}
	}
	if m.Version != 0 {
		i = encodeVarintConfig(dAtA, i, uint64(m.Version))
		i--
		dAtA[i] = 0x8
	}
	return len(dAtA) - i, nil
}

func (m *Defaults) Marshal() (dAtA []byte, err error) {
	size := m.ProtoSize()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *Defaults) MarshalTo(dAtA []byte) (int, error) {
	size := m.ProtoSize()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *Defaults) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	{
		size, err := m.Device.MarshalToSizedBuffer(dAtA[:i])
		if err != nil {
			return 0, err
		}
		i -= size
		i = encodeVarintConfig(dAtA, i, uint64(size))
	}
	i--
	dAtA[i] = 0x12
	{
		size, err := m.Folder.MarshalToSizedBuffer(dAtA[:i])
		if err != nil {
			return 0, err
		}
		i -= size
		i = encodeVarintConfig(dAtA, i, uint64(size))
	}
	i--
	dAtA[i] = 0xa
	return len(dAtA) - i, nil
}

func encodeVarintConfig(dAtA []byte, offset int, v uint64) int {
	offset -= sovConfig(v)
	base := offset
	for v >= 1<<7 {
		dAtA[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	dAtA[offset] = uint8(v)
	return base
}
func (m *Configuration) ProtoSize() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	if m.Version != 0 {
		n += 1 + sovConfig(uint64(m.Version))
	}
	if len(m.Folders) > 0 {
		for _, e := range m.Folders {
			l = e.ProtoSize()
			n += 1 + l + sovConfig(uint64(l))
		}
	}
	if len(m.Devices) > 0 {
		for _, e := range m.Devices {
			l = e.ProtoSize()
			n += 1 + l + sovConfig(uint64(l))
		}
	}
	l = m.GUI.ProtoSize()
	n += 1 + l + sovConfig(uint64(l))
	l = m.LDAP.ProtoSize()
	n += 1 + l + sovConfig(uint64(l))
	l = m.Options.ProtoSize()
	n += 1 + l + sovConfig(uint64(l))
	if len(m.IgnoredDevices) > 0 {
		for _, e := range m.IgnoredDevices {
			l = e.ProtoSize()
			n += 1 + l + sovConfig(uint64(l))
		}
	}
	if len(m.PendingDevices) > 0 {
		for _, e := range m.PendingDevices {
			l = e.ProtoSize()
			n += 1 + l + sovConfig(uint64(l))
		}
	}
	l = m.Defaults.ProtoSize()
	n += 1 + l + sovConfig(uint64(l))
	return n
}

func (m *Defaults) ProtoSize() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	l = m.Folder.ProtoSize()
	n += 1 + l + sovConfig(uint64(l))
	l = m.Device.ProtoSize()
	n += 1 + l + sovConfig(uint64(l))
	return n
}

func sovConfig(x uint64) (n int) {
	return (math_bits.Len64(x|1) + 6) / 7
}
func sozConfig(x uint64) (n int) {
	return sovConfig(uint64((x << 1) ^ uint64((int64(x) >> 63))))
}
func (m *Configuration) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowConfig
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
			return fmt.Errorf("proto: Configuration: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Configuration: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Version", wireType)
			}
			m.Version = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowConfig
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Version |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Folders", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowConfig
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
				return ErrInvalidLengthConfig
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthConfig
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Folders = append(m.Folders, FolderConfiguration{})
			if err := m.Folders[len(m.Folders)-1].Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 3:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Devices", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowConfig
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
				return ErrInvalidLengthConfig
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthConfig
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Devices = append(m.Devices, DeviceConfiguration{})
			if err := m.Devices[len(m.Devices)-1].Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 4:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field GUI", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowConfig
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
				return ErrInvalidLengthConfig
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthConfig
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if err := m.GUI.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 5:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field LDAP", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowConfig
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
				return ErrInvalidLengthConfig
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthConfig
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if err := m.LDAP.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 6:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Options", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowConfig
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
				return ErrInvalidLengthConfig
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthConfig
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if err := m.Options.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 7:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field IgnoredDevices", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowConfig
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
				return ErrInvalidLengthConfig
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthConfig
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.IgnoredDevices = append(m.IgnoredDevices, ObservedDevice{})
			if err := m.IgnoredDevices[len(m.IgnoredDevices)-1].Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 8:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field PendingDevices", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowConfig
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
				return ErrInvalidLengthConfig
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthConfig
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.PendingDevices = append(m.PendingDevices, ObservedDevice{})
			if err := m.PendingDevices[len(m.PendingDevices)-1].Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 9:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Defaults", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowConfig
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
				return ErrInvalidLengthConfig
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthConfig
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if err := m.Defaults.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipConfig(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthConfig
			}
			if (iNdEx + skippy) < 0 {
				return ErrInvalidLengthConfig
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
func (m *Defaults) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowConfig
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
			return fmt.Errorf("proto: Defaults: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Defaults: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Folder", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowConfig
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
				return ErrInvalidLengthConfig
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthConfig
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if err := m.Folder.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Device", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowConfig
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
				return ErrInvalidLengthConfig
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthConfig
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if err := m.Device.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipConfig(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthConfig
			}
			if (iNdEx + skippy) < 0 {
				return ErrInvalidLengthConfig
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
func skipConfig(dAtA []byte) (n int, err error) {
	l := len(dAtA)
	iNdEx := 0
	depth := 0
	for iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, ErrIntOverflowConfig
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
					return 0, ErrIntOverflowConfig
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
					return 0, ErrIntOverflowConfig
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
				return 0, ErrInvalidLengthConfig
			}
			iNdEx += length
		case 3:
			depth++
		case 4:
			if depth == 0 {
				return 0, ErrUnexpectedEndOfGroupConfig
			}
			depth--
		case 5:
			iNdEx += 4
		default:
			return 0, fmt.Errorf("proto: illegal wireType %d", wireType)
		}
		if iNdEx < 0 {
			return 0, ErrInvalidLengthConfig
		}
		if depth == 0 {
			return iNdEx, nil
		}
	}
	return 0, io.ErrUnexpectedEOF
}

var (
	ErrInvalidLengthConfig        = fmt.Errorf("proto: negative length found during unmarshaling")
	ErrIntOverflowConfig          = fmt.Errorf("proto: integer overflow")
	ErrUnexpectedEndOfGroupConfig = fmt.Errorf("proto: unexpected end of group")
)
