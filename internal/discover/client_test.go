// Copyright (C) 2014 The Syncthing Authors.
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

package discover

import (
	"fmt"
	"net"
	"sync"
	"time"

	"testing"

	"github.com/syncthing/protocol"
)

var device protocol.DeviceID

func init() {
	device, _ = protocol.DeviceIDFromString("P56IOI7-MZJNU2Y-IQGDREY-DM2MGTI-MGL3BXN-PQ6W5BM-TBBZ4TJ-XZWICQ2")
}

func TestUDP4Success(t *testing.T) {
	conn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		t.Fatal(err)
	}

	port := conn.LocalAddr().(*net.UDPAddr).Port

	address := fmt.Sprintf("udp4://127.0.0.1:%d", port)
	pkt := &Announce{
		Magic: AnnouncementMagic,
		This: Device{
			device[:],
			[]Address{{
				IP:   net.IPv4(123, 123, 123, 123),
				Port: 1234,
			}},
		},
	}

	client, err := New(address, pkt)
	if err != nil {
		t.Fatal(err)
	}

	udpclient := client.(*UDPClient)
	if udpclient.errorRetryInterval != DefaultErrorRetryInternval {
		t.Fatal("Incorrect retry interval")
	}

	if udpclient.listenAddress.IP != nil || udpclient.listenAddress.Port != 0 {
		t.Fatal("Wrong listen IP or port", udpclient.listenAddress)
	}

	if client.Address() != address {
		t.Fatal("Incorrect address")
	}

	buf := make([]byte, 2048)

	// First announcement
	conn.SetDeadline(time.Now().Add(time.Millisecond * 100))
	_, err = conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}

	// Announcement verification
	conn.SetDeadline(time.Now().Add(time.Millisecond * 1100))
	_, addr, err := conn.ReadFromUDP(buf)
	if err != nil {
		t.Fatal(err)
	}

	// Reply to it.
	_, err = conn.WriteToUDP(pkt.MustMarshalXDR(), addr)
	if err != nil {
		t.Fatal(err)
	}

	// We should get nothing else
	conn.SetDeadline(time.Now().Add(time.Millisecond * 100))
	_, err = conn.Read(buf)
	if err == nil {
		t.Fatal("Expected error")
	}

	// Status should be ok
	if !client.StatusOK() {
		t.Fatal("Wrong status")
	}

	// Do a lookup in a separate routine
	addrs := []string{}
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		addrs = client.Lookup(device)
		wg.Done()
	}()

	// Receive the lookup and reply
	conn.SetDeadline(time.Now().Add(time.Millisecond * 100))
	_, addr, err = conn.ReadFromUDP(buf)
	if err != nil {
		t.Fatal(err)
	}

	conn.WriteToUDP(pkt.MustMarshalXDR(), addr)

	// Wait for the lookup to arrive, verify that the number of answers is correct
	wg.Wait()

	if len(addrs) != 1 || addrs[0] != "123.123.123.123:1234" {
		t.Fatal("Wrong number of answers")
	}

	client.Stop()
}

func TestUDP4Failure(t *testing.T) {
	conn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		t.Fatal(err)
	}

	port := conn.LocalAddr().(*net.UDPAddr).Port

	address := fmt.Sprintf("udp4://127.0.0.1:%d/?listenaddress=127.0.0.1&retry=5", port)

	pkt := &Announce{
		Magic: AnnouncementMagic,
		This: Device{
			device[:],
			[]Address{{
				IP:   net.IPv4(123, 123, 123, 123),
				Port: 1234,
			}},
		},
	}

	client, err := New(address, pkt)
	if err != nil {
		t.Fatal(err)
	}

	udpclient := client.(*UDPClient)
	if udpclient.errorRetryInterval != time.Second*5 {
		t.Fatal("Incorrect retry interval")
	}

	if !udpclient.listenAddress.IP.Equal(net.IPv4(127, 0, 0, 1)) || udpclient.listenAddress.Port != 0 {
		t.Fatal("Wrong listen IP or port", udpclient.listenAddress)
	}

	if client.Address() != address {
		t.Fatal("Incorrect address")
	}

	buf := make([]byte, 2048)

	// First announcement
	conn.SetDeadline(time.Now().Add(time.Millisecond * 100))
	_, err = conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}

	// Announcement verification
	conn.SetDeadline(time.Now().Add(time.Millisecond * 1100))
	_, _, err = conn.ReadFromUDP(buf)
	if err != nil {
		t.Fatal(err)
	}

	// Don't reply
	// We should get nothing else
	conn.SetDeadline(time.Now().Add(time.Millisecond * 100))
	_, err = conn.Read(buf)
	if err == nil {
		t.Fatal("Expected error")
	}

	// Status should be failure
	if client.StatusOK() {
		t.Fatal("Wrong status")
	}

	// Do a lookup in a separate routine
	addrs := []string{}
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		addrs = client.Lookup(device)
		wg.Done()
	}()

	// Receive the lookup and don't reply
	conn.SetDeadline(time.Now().Add(time.Millisecond * 100))
	_, _, err = conn.ReadFromUDP(buf)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for the lookup to timeout, verify that the number of answers is none
	wg.Wait()

	if len(addrs) != 0 {
		t.Fatal("Wrong number of answers")
	}

	client.Stop()
}
