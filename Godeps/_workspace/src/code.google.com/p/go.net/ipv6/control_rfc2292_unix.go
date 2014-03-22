// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build darwin

package ipv6

import (
	"net"
	"os"
	"syscall"
	"unsafe"
)

const pktinfo = FlagDst | FlagInterface

func setControlMessage(fd int, opt *rawOpt, cf ControlFlags, on bool) error {
	opt.Lock()
	defer opt.Unlock()
	if cf&FlagHopLimit != 0 {
		if err := setIPv6ReceiveHopLimit(fd, on); err != nil {
			return err
		}
		if on {
			opt.set(FlagHopLimit)
		} else {
			opt.clear(FlagHopLimit)
		}
	}
	if cf&pktinfo != 0 {
		if err := setIPv6ReceivePacketInfo(fd, on); err != nil {
			return err
		}
		if on {
			opt.set(cf & pktinfo)
		} else {
			opt.clear(cf & pktinfo)
		}
	}
	return nil
}

func newControlMessage(opt *rawOpt) (oob []byte) {
	opt.Lock()
	defer opt.Unlock()
	l, off := 0, 0
	if opt.isset(FlagHopLimit) {
		l += syscall.CmsgSpace(4)
	}
	if opt.isset(pktinfo) {
		l += syscall.CmsgSpace(sysSizeofPacketInfo)
	}
	if l > 0 {
		oob = make([]byte, l)
		if opt.isset(FlagHopLimit) {
			m := (*syscall.Cmsghdr)(unsafe.Pointer(&oob[off]))
			m.Level = ianaProtocolIPv6
			m.Type = sysSockopt2292HopLimit
			m.SetLen(syscall.CmsgLen(4))
			off += syscall.CmsgSpace(4)
		}
		if opt.isset(pktinfo) {
			m := (*syscall.Cmsghdr)(unsafe.Pointer(&oob[off]))
			m.Level = ianaProtocolIPv6
			m.Type = sysSockopt2292PacketInfo
			m.SetLen(syscall.CmsgLen(sysSizeofPacketInfo))
			off += syscall.CmsgSpace(sysSizeofPacketInfo)
		}
	}
	return
}

func parseControlMessage(b []byte) (*ControlMessage, error) {
	if len(b) == 0 {
		return nil, nil
	}
	cmsgs, err := syscall.ParseSocketControlMessage(b)
	if err != nil {
		return nil, os.NewSyscallError("parse socket control message", err)
	}
	cm := &ControlMessage{}
	for _, m := range cmsgs {
		if m.Header.Level != ianaProtocolIPv6 {
			continue
		}
		switch m.Header.Type {
		case sysSockopt2292HopLimit:
			cm.HopLimit = int(*(*byte)(unsafe.Pointer(&m.Data[:1][0])))
		case sysSockopt2292PacketInfo:
			pi := (*sysPacketInfo)(unsafe.Pointer(&m.Data[0]))
			cm.IfIndex = int(pi.IfIndex)
			cm.Dst = pi.IP[:]
		}
	}
	return cm, nil
}

func marshalControlMessage(cm *ControlMessage) (oob []byte) {
	if cm == nil {
		return
	}
	l, off := 0, 0
	if cm.HopLimit > 0 {
		l += syscall.CmsgSpace(4)
	}
	pion := false
	if cm.Src.To4() == nil && cm.Src.To16() != nil || cm.IfIndex != 0 {
		pion = true
		l += syscall.CmsgSpace(sysSizeofPacketInfo)
	}
	if len(cm.NextHop) == net.IPv6len {
		l += syscall.CmsgSpace(syscall.SizeofSockaddrInet6)
	}
	if l > 0 {
		oob = make([]byte, l)
		if cm.HopLimit > 0 {
			m := (*syscall.Cmsghdr)(unsafe.Pointer(&oob[off]))
			m.Level = ianaProtocolIPv6
			m.Type = sysSockopt2292HopLimit
			m.SetLen(syscall.CmsgLen(4))
			data := oob[off+syscall.CmsgLen(0):]
			*(*byte)(unsafe.Pointer(&data[:1][0])) = byte(cm.HopLimit)
			off += syscall.CmsgSpace(4)
		}
		if pion {
			m := (*syscall.Cmsghdr)(unsafe.Pointer(&oob[off]))
			m.Level = ianaProtocolIPv6
			m.Type = sysSockopt2292PacketInfo
			m.SetLen(syscall.CmsgLen(sysSizeofPacketInfo))
			pi := (*sysPacketInfo)(unsafe.Pointer(&oob[off+syscall.CmsgLen(0)]))
			if ip := cm.Src.To16(); ip != nil && ip.To4() == nil {
				copy(pi.IP[:], ip)
			}
			if cm.IfIndex != 0 {
				pi.IfIndex = uint32(cm.IfIndex)
			}
			off += syscall.CmsgSpace(sysSizeofPacketInfo)
		}
		if len(cm.NextHop) == net.IPv6len {
			m := (*syscall.Cmsghdr)(unsafe.Pointer(&oob[off]))
			m.Level = ianaProtocolIPv6
			m.Type = sysSockopt2292NextHop
			m.SetLen(syscall.CmsgLen(syscall.SizeofSockaddrInet6))
			sa := (*syscall.RawSockaddrInet6)(unsafe.Pointer(&oob[off+syscall.CmsgLen(0)]))
			setSockaddr(sa, cm.NextHop, cm.IfIndex)
			off += syscall.CmsgSpace(syscall.SizeofSockaddrInet6)
		}
	}
	return
}
