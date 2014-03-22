// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

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
		l += syscall.CmsgSpace(syscall.SizeofInet6Pktinfo)
	}
	if l > 0 {
		oob = make([]byte, l)
		if opt.isset(FlagHopLimit) {
			m := (*syscall.Cmsghdr)(unsafe.Pointer(&oob[off]))
			m.Level = ianaProtocolIPv6
			m.Type = syscall.IPV6_2292HOPLIMIT
			m.SetLen(syscall.CmsgLen(4))
			off += syscall.CmsgSpace(4)
		}
		if opt.isset(pktinfo) {
			m := (*syscall.Cmsghdr)(unsafe.Pointer(&oob[off]))
			m.Level = ianaProtocolIPv6
			m.Type = syscall.IPV6_2292PKTINFO
			m.SetLen(syscall.CmsgLen(syscall.SizeofInet6Pktinfo))
			off += syscall.CmsgSpace(syscall.SizeofInet6Pktinfo)
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
		case syscall.IPV6_2292HOPLIMIT:
			cm.HopLimit = int(*(*byte)(unsafe.Pointer(&m.Data[:1][0])))
		case syscall.IPV6_2292PKTINFO:
			pi := (*syscall.Inet6Pktinfo)(unsafe.Pointer(&m.Data[0]))
			cm.IfIndex = int(pi.Ifindex)
			cm.Dst = pi.Addr[:]
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
		l += syscall.CmsgSpace(syscall.SizeofInet6Pktinfo)
	}
	if len(cm.NextHop) == net.IPv6len {
		l += syscall.CmsgSpace(syscall.SizeofSockaddrInet6)
	}
	if l > 0 {
		oob = make([]byte, l)
		if cm.HopLimit > 0 {
			m := (*syscall.Cmsghdr)(unsafe.Pointer(&oob[off]))
			m.Level = ianaProtocolIPv6
			m.Type = syscall.IPV6_2292HOPLIMIT
			m.SetLen(syscall.CmsgLen(4))
			data := oob[off+syscall.CmsgLen(0):]
			*(*byte)(unsafe.Pointer(&data[:1][0])) = byte(cm.HopLimit)
			off += syscall.CmsgSpace(4)
		}
		if pion {
			m := (*syscall.Cmsghdr)(unsafe.Pointer(&oob[off]))
			m.Level = ianaProtocolIPv6
			m.Type = syscall.IPV6_2292PKTINFO
			m.SetLen(syscall.CmsgLen(syscall.SizeofInet6Pktinfo))
			pi := (*syscall.Inet6Pktinfo)(unsafe.Pointer(&oob[off+syscall.CmsgLen(0)]))
			if ip := cm.Src.To16(); ip != nil && ip.To4() == nil {
				copy(pi.Addr[:], ip)
			}
			if cm.IfIndex != 0 {
				pi.Ifindex = uint32(cm.IfIndex)
			}
			off += syscall.CmsgSpace(syscall.SizeofInet6Pktinfo)
		}
		if len(cm.NextHop) == net.IPv6len {
			m := (*syscall.Cmsghdr)(unsafe.Pointer(&oob[off]))
			m.Level = ianaProtocolIPv6
			m.Type = syscall.IPV6_2292NEXTHOP
			m.SetLen(syscall.CmsgLen(syscall.SizeofSockaddrInet6))
			sa := (*syscall.RawSockaddrInet6)(unsafe.Pointer(&oob[off+syscall.CmsgLen(0)]))
			sa.Len = syscall.SizeofSockaddrInet6
			sa.Family = syscall.AF_INET6
			copy(sa.Addr[:], cm.NextHop)
			off += syscall.CmsgSpace(syscall.SizeofSockaddrInet6)
		}
	}
	return
}
