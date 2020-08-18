// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: lib/config/deviceconfiguration.proto

package config

import (
	fmt "fmt"
	proto "github.com/gogo/protobuf/proto"
	github_com_syncthing_syncthing_lib_protocol "github.com/syncthing/syncthing/lib/protocol"
	protocol "github.com/syncthing/syncthing/lib/protocol"
	_ "github.com/syncthing/syncthing/proto/ext"
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

type DeviceConfiguration struct {
	DeviceID                 github_com_syncthing_syncthing_lib_protocol.DeviceID `protobuf:"bytes,1,opt,name=device_id,json=deviceId,proto3,customtype=github.com/syncthing/syncthing/lib/protocol.DeviceID" json:"deviceID" xml:"id,attr"`
	Name                     string                                               `protobuf:"bytes,2,opt,name=name,proto3" json:"name" xml:"name,attr,omitempty"`
	Addresses                []string                                             `protobuf:"bytes,3,rep,name=addresses,proto3" json:"addresses" xml:"address,omitempty" default:"dynamic"`
	Compression              protocol.Compression                                 `protobuf:"varint,4,opt,name=compression,proto3,enum=protocol.Compression" json:"compression" xml:"compression,attr"`
	CertName                 string                                               `protobuf:"bytes,5,opt,name=cert_name,json=certName,proto3" json:"certName" xml:"certName,attr,omitempty"`
	Introducer               bool                                                 `protobuf:"varint,6,opt,name=introducer,proto3" json:"introducer" xml:"introducer,attr"`
	SkipIntroductionRemovals bool                                                 `protobuf:"varint,7,opt,name=skip_introduction_removals,json=skipIntroductionRemovals,proto3" json:"skipIntroductionRemovals" xml:"skipIntroductionRemovals,attr"`
	IntroducedBy             github_com_syncthing_syncthing_lib_protocol.DeviceID `protobuf:"bytes,8,opt,name=introduced_by,json=introducedBy,proto3,customtype=github.com/syncthing/syncthing/lib/protocol.DeviceID" json:"introducedBy" xml:"introducedBy,attr"`
	Paused                   bool                                                 `protobuf:"varint,9,opt,name=paused,proto3" json:"paused" xml:"paused"`
	AllowedNetworks          []string                                             `protobuf:"bytes,10,rep,name=allowed_networks,json=allowedNetworks,proto3" json:"allowedNetworks" xml:"allowedNetwork,omitempty"`
	AutoAcceptFolders        bool                                                 `protobuf:"varint,11,opt,name=auto_accept_folders,json=autoAcceptFolders,proto3" json:"autoAcceptFolders" xml:"autoAcceptFolders"`
	MaxSendKbps              int32                                                `protobuf:"varint,12,opt,name=max_send_kbps,json=maxSendKbps,proto3" json:"maxSendKbps" xml:"maxSendKbps"`
	MaxRecvKbps              int32                                                `protobuf:"varint,13,opt,name=max_recv_kbps,json=maxRecvKbps,proto3" json:"maxRecvKbps" xml:"maxRecvKbps"`
	IgnoredFolders           []ObservedFolder                                     `protobuf:"bytes,14,rep,name=ignored_folders,json=ignoredFolders,proto3" json:"ignoredFolders" xml:"ignoredFolder"`
	PendingFolders           []ObservedFolder                                     `protobuf:"bytes,15,rep,name=pending_folders,json=pendingFolders,proto3" json:"pendingFolders" xml:"pendingFolder"`
	MaxRequestKiB            int32                                                `protobuf:"varint,16,opt,name=max_request_kib,json=maxRequestKib,proto3" json:"maxRequestKiB" xml:"maxRequestKiB"`
}

func (m *DeviceConfiguration) Reset()         { *m = DeviceConfiguration{} }
func (m *DeviceConfiguration) String() string { return proto.CompactTextString(m) }
func (*DeviceConfiguration) ProtoMessage()    {}
func (*DeviceConfiguration) Descriptor() ([]byte, []int) {
	return fileDescriptor_744b782bd13071dd, []int{0}
}
func (m *DeviceConfiguration) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_DeviceConfiguration.Unmarshal(m, b)
}
func (m *DeviceConfiguration) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_DeviceConfiguration.Marshal(b, m, deterministic)
}
func (m *DeviceConfiguration) XXX_Merge(src proto.Message) {
	xxx_messageInfo_DeviceConfiguration.Merge(m, src)
}
func (m *DeviceConfiguration) XXX_Size() int {
	return xxx_messageInfo_DeviceConfiguration.Size(m)
}
func (m *DeviceConfiguration) XXX_DiscardUnknown() {
	xxx_messageInfo_DeviceConfiguration.DiscardUnknown(m)
}

var xxx_messageInfo_DeviceConfiguration proto.InternalMessageInfo

func init() {
	proto.RegisterType((*DeviceConfiguration)(nil), "config.DeviceConfiguration")
}

func init() {
	proto.RegisterFile("lib/config/deviceconfiguration.proto", fileDescriptor_744b782bd13071dd)
}

var fileDescriptor_744b782bd13071dd = []byte{
	// 892 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xa4, 0x54, 0x31, 0x6f, 0xdb, 0x46,
	0x18, 0x15, 0x9b, 0xc4, 0xb6, 0xce, 0x96, 0x65, 0xd3, 0x88, 0xc3, 0x18, 0x88, 0x4e, 0x60, 0x35,
	0x28, 0x40, 0x2a, 0x17, 0x6e, 0x27, 0xa3, 0x1d, 0xca, 0x04, 0x6d, 0x82, 0xa0, 0x69, 0x7b, 0xdd,
	0xbc, 0xb0, 0x24, 0xef, 0xac, 0x1c, 0x4c, 0xf2, 0x58, 0x92, 0x52, 0x24, 0xa0, 0x43, 0xc7, 0x0e,
	0x1d, 0x8a, 0xa0, 0x3f, 0xa0, 0xe8, 0xd0, 0xa1, 0x3f, 0xa0, 0xbf, 0x21, 0x9b, 0x35, 0x16, 0x1d,
	0x0e, 0x88, 0xbd, 0x14, 0x1c, 0x39, 0x76, 0x2a, 0xee, 0x8e, 0xa2, 0x48, 0x3a, 0x2e, 0x02, 0x74,
	0xbb, 0x7b, 0xef, 0xdd, 0x7b, 0xc7, 0xa7, 0xfb, 0x04, 0x06, 0x3e, 0x75, 0x0f, 0x3d, 0x16, 0x9e,
	0xd2, 0xf1, 0x21, 0x26, 0x53, 0xea, 0x11, 0xb5, 0x99, 0xc4, 0x4e, 0x4a, 0x59, 0x38, 0x8a, 0x62,
	0x96, 0x32, 0x7d, 0x4d, 0x81, 0x07, 0xfb, 0x42, 0x2d, 0x21, 0x8f, 0xf9, 0x87, 0x2e, 0x89, 0x14,
	0x7f, 0x70, 0xb7, 0xe2, 0xc2, 0xdc, 0x84, 0xc4, 0x53, 0x82, 0x0b, 0xaa, 0x4d, 0x66, 0xa9, 0x5a,
	0x9a, 0x7f, 0x6c, 0x83, 0xbd, 0x47, 0x32, 0xe3, 0x61, 0x35, 0x43, 0xff, 0x5d, 0x03, 0x6d, 0x95,
	0x6d, 0x53, 0x6c, 0x68, 0x7d, 0x6d, 0xb8, 0x65, 0xfd, 0xa8, 0xbd, 0xe2, 0xb0, 0xf5, 0x17, 0x87,
	0x1f, 0x8e, 0x69, 0xfa, 0x7c, 0xe2, 0x8e, 0x3c, 0x16, 0x1c, 0x26, 0xf3, 0xd0, 0x4b, 0x9f, 0xd3,
	0x70, 0x5c, 0x59, 0x55, 0x6f, 0x34, 0x52, 0xee, 0x4f, 0x1e, 0x5d, 0x70, 0xb8, 0xb1, 0x5c, 0x67,
	0x1c, 0x6e, 0xe0, 0x62, 0x9d, 0x73, 0xd8, 0x99, 0x05, 0xfe, 0xb1, 0x49, 0xf1, 0x03, 0x27, 0x4d,
	0x63, 0x33, 0x3b, 0x1f, 0xac, 0x17, 0xeb, 0xfc, 0x7c, 0x50, 0xea, 0x7e, 0x58, 0x0c, 0xb4, 0x97,
	0x8b, 0x41, 0xe9, 0x81, 0x96, 0x0c, 0xd6, 0xbf, 0x04, 0x37, 0x43, 0x27, 0x20, 0xc6, 0x3b, 0x7d,
	0x6d, 0xd8, 0xb6, 0x3e, 0xca, 0x38, 0x94, 0xfb, 0x9c, 0xc3, 0xbb, 0xd2, 0x59, 0x6c, 0xa4, 0xdf,
	0x03, 0x16, 0xd0, 0x94, 0x04, 0x51, 0x3a, 0x17, 0x29, 0x7b, 0x6f, 0xc0, 0x91, 0x3c, 0xa9, 0xcf,
	0x40, 0xdb, 0xc1, 0x38, 0x26, 0x49, 0x42, 0x12, 0xe3, 0x46, 0xff, 0xc6, 0xb0, 0x6d, 0x9d, 0x64,
	0x1c, 0xae, 0xc0, 0x9c, 0xc3, 0xfb, 0xd2, 0xbb, 0x40, 0x2a, 0xce, 0x7d, 0x4c, 0x4e, 0x9d, 0x89,
	0x9f, 0x1e, 0x9b, 0x78, 0x1e, 0x3a, 0x01, 0xf5, 0x44, 0xd6, 0xee, 0x15, 0xdd, 0x3f, 0xe7, 0x83,
	0xf5, 0x42, 0x80, 0x56, 0xbe, 0xfa, 0x14, 0x6c, 0x7a, 0x2c, 0x88, 0xc4, 0x8e, 0xb2, 0xd0, 0xb8,
	0xd9, 0xd7, 0x86, 0xdb, 0x47, 0xb7, 0x47, 0x65, 0x9d, 0x0f, 0x57, 0xa4, 0xf5, 0x71, 0xc6, 0x61,
	0x55, 0x9d, 0x73, 0xb8, 0x2f, 0x2f, 0x55, 0xc1, 0xca, 0x4e, 0x77, 0x9a, 0x20, 0xaa, 0x1e, 0xd5,
	0x09, 0x68, 0x7b, 0x24, 0x4e, 0x6d, 0x59, 0xe4, 0x2d, 0x59, 0xe4, 0x63, 0xf1, 0x33, 0x09, 0xf0,
	0x99, 0x2a, 0xf3, 0x9e, 0xf2, 0x2e, 0x80, 0x37, 0x14, 0x7a, 0xe7, 0x1a, 0x0e, 0x95, 0x2e, 0xfa,
	0x09, 0x00, 0x34, 0x4c, 0x63, 0x86, 0x27, 0x1e, 0x89, 0x8d, 0xb5, 0xbe, 0x36, 0xdc, 0xb0, 0x8e,
	0x33, 0x0e, 0x2b, 0x68, 0xce, 0xe1, 0x6d, 0xf5, 0x20, 0x4a, 0xa8, 0xfc, 0x88, 0x6e, 0x03, 0x43,
	0x95, 0x73, 0xfa, 0xaf, 0x1a, 0x38, 0x48, 0xce, 0x68, 0x64, 0x2f, 0x31, 0xf1, 0x92, 0xed, 0x98,
	0x04, 0x6c, 0xea, 0xf8, 0x89, 0xb1, 0x2e, 0xc3, 0x70, 0xc6, 0xa1, 0x21, 0x54, 0x4f, 0x2a, 0x22,
	0x54, 0x68, 0x72, 0x0e, 0xdf, 0x95, 0xd1, 0xd7, 0x09, 0xca, 0x8b, 0xdc, 0xfb, 0x4f, 0x05, 0xba,
	0x36, 0x41, 0xff, 0x4d, 0x03, 0x9d, 0xf2, 0xce, 0xd8, 0x76, 0xe7, 0xc6, 0x86, 0x1c, 0xae, 0xef,
	0xff, 0xd7, 0x70, 0x65, 0x1c, 0x6e, 0xad, 0x5c, 0xad, 0x79, 0xce, 0xe1, 0x9d, 0x7a, 0x87, 0xd8,
	0x9a, 0x97, 0x97, 0xdf, 0xbd, 0x82, 0x8a, 0xe1, 0x42, 0x35, 0x07, 0xfd, 0x08, 0xac, 0x45, 0xce,
	0x24, 0x21, 0xd8, 0x68, 0xcb, 0xe2, 0x0e, 0x32, 0x0e, 0x0b, 0x24, 0xe7, 0x70, 0x4b, 0xba, 0xab,
	0xad, 0x89, 0x0a, 0x5c, 0xff, 0x0e, 0xec, 0x38, 0xbe, 0xcf, 0x5e, 0x10, 0x6c, 0x87, 0x24, 0x7d,
	0xc1, 0xe2, 0xb3, 0xc4, 0x00, 0x72, 0x7a, 0xbe, 0xca, 0x38, 0xec, 0x16, 0xdc, 0xb3, 0x82, 0xca,
	0x39, 0xec, 0xa9, 0x19, 0xaa, 0xe1, 0xf5, 0x37, 0x65, 0x5c, 0x47, 0xa2, 0xa6, 0x9d, 0xfe, 0x0d,
	0xd8, 0x73, 0x26, 0x29, 0xb3, 0x1d, 0xcf, 0x23, 0x51, 0x6a, 0x9f, 0x32, 0x1f, 0x93, 0x38, 0x31,
	0x36, 0xe5, 0xf5, 0xdf, 0xcf, 0x38, 0xdc, 0x15, 0xf4, 0x27, 0x92, 0xfd, 0x54, 0x91, 0x65, 0x4f,
	0x57, 0x18, 0x13, 0x5d, 0x55, 0xeb, 0x8f, 0x41, 0x27, 0x70, 0x66, 0x76, 0x42, 0x42, 0x6c, 0x9f,
	0xb9, 0x51, 0x62, 0x6c, 0xf5, 0xb5, 0xe1, 0x2d, 0x6b, 0x20, 0xe6, 0x30, 0x70, 0x66, 0x5f, 0x93,
	0x10, 0x3f, 0x75, 0x23, 0xe1, 0xba, 0x2b, 0x5d, 0x2b, 0x98, 0x89, 0xaa, 0x8a, 0xa5, 0x53, 0x4c,
	0xbc, 0xa9, 0x72, 0xea, 0xd4, 0x9c, 0x10, 0xf1, 0xa6, 0x4d, 0xa7, 0x25, 0xa6, 0x9c, 0x96, 0x3b,
	0x3d, 0x04, 0x5d, 0x3a, 0x0e, 0x59, 0x4c, 0x70, 0xf9, 0xc5, 0xdb, 0xfd, 0x1b, 0xc3, 0xcd, 0xa3,
	0xfd, 0x91, 0xfa, 0xf7, 0x1f, 0x7d, 0x51, 0xfc, 0xfb, 0xab, 0xaf, 0xb0, 0xde, 0x13, 0x0f, 0x2d,
	0xe3, 0x70, 0xbb, 0x38, 0xb6, 0xaa, 0x62, 0x4f, 0x3d, 0x99, 0x2a, 0x6c, 0xa2, 0x86, 0x4c, 0xe4,
	0x45, 0x24, 0xc4, 0x34, 0x1c, 0x97, 0x79, 0xdd, 0xb7, 0xcb, 0x2b, 0x8e, 0x35, 0xf3, 0x6a, 0xb0,
	0x89, 0x1a, 0x32, 0xfd, 0x67, 0x0d, 0x74, 0x55, 0x55, 0xdf, 0x4e, 0x48, 0x92, 0xda, 0x67, 0xd4,
	0x35, 0x76, 0x64, 0x59, 0xfe, 0x05, 0x87, 0x9d, 0xcf, 0x45, 0x15, 0x92, 0x79, 0x4a, 0xad, 0x8c,
	0xc3, 0x4e, 0x50, 0x05, 0xca, 0x90, 0x1a, 0x2a, 0xde, 0x55, 0x43, 0xd7, 0x04, 0x5e, 0x2e, 0x06,
	0x75, 0x6b, 0x54, 0xe3, 0x5d, 0xeb, 0xb3, 0x57, 0xaf, 0x7b, 0xad, 0xc5, 0xeb, 0x5e, 0xeb, 0xef,
	0x8b, 0x5e, 0xeb, 0xa7, 0xcb, 0x5e, 0xeb, 0x97, 0xcb, 0x9e, 0xb6, 0xb8, 0xec, 0xb5, 0xfe, 0xbc,
	0xec, 0xb5, 0x4e, 0xee, 0xbf, 0xc5, 0x10, 0xab, 0xb6, 0xdc, 0x35, 0x39, 0xcc, 0x1f, 0xfc, 0x1b,
	0x00, 0x00, 0xff, 0xff, 0x54, 0xce, 0xa8, 0x2e, 0xf6, 0x07, 0x00, 0x00,
}

func (m *DeviceConfiguration) ProtoSize() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	l = m.DeviceID.ProtoSize()
	n += 1 + l + sovDeviceconfiguration(uint64(l))
	l = len(m.Name)
	if l > 0 {
		n += 1 + l + sovDeviceconfiguration(uint64(l))
	}
	if len(m.Addresses) > 0 {
		for _, s := range m.Addresses {
			l = len(s)
			n += 1 + l + sovDeviceconfiguration(uint64(l))
		}
	}
	if m.Compression != 0 {
		n += 1 + sovDeviceconfiguration(uint64(m.Compression))
	}
	l = len(m.CertName)
	if l > 0 {
		n += 1 + l + sovDeviceconfiguration(uint64(l))
	}
	if m.Introducer {
		n += 2
	}
	if m.SkipIntroductionRemovals {
		n += 2
	}
	l = m.IntroducedBy.ProtoSize()
	n += 1 + l + sovDeviceconfiguration(uint64(l))
	if m.Paused {
		n += 2
	}
	if len(m.AllowedNetworks) > 0 {
		for _, s := range m.AllowedNetworks {
			l = len(s)
			n += 1 + l + sovDeviceconfiguration(uint64(l))
		}
	}
	if m.AutoAcceptFolders {
		n += 2
	}
	if m.MaxSendKbps != 0 {
		n += 1 + sovDeviceconfiguration(uint64(m.MaxSendKbps))
	}
	if m.MaxRecvKbps != 0 {
		n += 1 + sovDeviceconfiguration(uint64(m.MaxRecvKbps))
	}
	if len(m.IgnoredFolders) > 0 {
		for _, e := range m.IgnoredFolders {
			l = e.ProtoSize()
			n += 1 + l + sovDeviceconfiguration(uint64(l))
		}
	}
	if len(m.PendingFolders) > 0 {
		for _, e := range m.PendingFolders {
			l = e.ProtoSize()
			n += 1 + l + sovDeviceconfiguration(uint64(l))
		}
	}
	if m.MaxRequestKiB != 0 {
		n += 2 + sovDeviceconfiguration(uint64(m.MaxRequestKiB))
	}
	return n
}

func sovDeviceconfiguration(x uint64) (n int) {
	return (math_bits.Len64(x|1) + 6) / 7
}
func sozDeviceconfiguration(x uint64) (n int) {
	return sovDeviceconfiguration(uint64((x << 1) ^ uint64((int64(x) >> 63))))
}
