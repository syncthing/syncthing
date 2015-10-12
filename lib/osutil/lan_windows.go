// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build windows

package osutil

import (
	"net"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

// Modified version of:
// http://stackoverflow.com/questions/23529663/how-to-get-all-addresses-and-masks-from-local-interfaces-in-go
// v4 only!

func getAdapterList() (*syscall.IpAdapterInfo, error) {
	b := make([]byte, 10240)
	l := uint32(len(b))
	a := (*syscall.IpAdapterInfo)(unsafe.Pointer(&b[0]))
	// TODO(mikio): GetAdaptersInfo returns IP_ADAPTER_INFO that
	// contains IPv4 address list only. We should use another API
	// for fetching IPv6 stuff from the kernel.
	err := syscall.GetAdaptersInfo(a, &l)
	if err == syscall.ERROR_BUFFER_OVERFLOW {
		b = make([]byte, l)
		a = (*syscall.IpAdapterInfo)(unsafe.Pointer(&b[0]))
		err = syscall.GetAdaptersInfo(a, &l)
	}
	if err != nil {
		return nil, os.NewSyscallError("GetAdaptersInfo", err)
	}
	return a, nil
}

func GetLans() ([]*net.IPNet, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	nets := make([]*net.IPNet, 0, len(ifaces))

	aList, err := getAdapterList()
	if err != nil {
		return nil, err
	}

	for _, ifi := range ifaces {
		for ai := aList; ai != nil; ai = ai.Next {
			index := ai.Index

			if ifi.Index == int(index) {
				ipl := &ai.IpAddressList
				for ; ipl != nil; ipl = ipl.Next {
					ipStr := strings.Trim(string(ipl.IpAddress.String[:]), "\x00")
					maskStr := strings.Trim(string(ipl.IpMask.String[:]), "\x00")
					ip := net.ParseIP(ipStr)
					maskip := net.ParseIP(maskStr)
					if ip.IsUnspecified() || maskip.IsUnspecified() {
						continue
					}
					nets = append(nets, &net.IPNet{
						IP: ip,
						Mask: net.IPv4Mask(
							maskip[net.IPv6len-net.IPv4len],
							maskip[net.IPv6len-net.IPv4len+1],
							maskip[net.IPv6len-net.IPv4len+2],
							maskip[net.IPv6len-net.IPv4len+3],
						),
					})
				}
			}
		}
	}
	return nets, err
}
