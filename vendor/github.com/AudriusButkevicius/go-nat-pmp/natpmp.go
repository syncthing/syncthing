package natpmp

import (
	"fmt"
	"net"
	"time"
)

// Implement the NAT-PMP protocol, typically supported by Apple routers and open source
// routers such as DD-WRT and Tomato.
//
// See http://tools.ietf.org/html/draft-cheshire-nat-pmp-03
//
// Usage:
//
//    client := natpmp.NewClient(gatewayIP)
//    response, err := client.GetExternalAddress()

const nAT_PMP_PORT = 5351

// The recommended mapping lifetime for AddPortMapping
const RECOMMENDED_MAPPING_LIFETIME_SECONDS = 3600

// Client is a NAT-PMP protocol client.
type Client struct {
	gateway net.IP
	timeout time.Duration
}

// Create a NAT-PMP client for the NAT-PMP server at the gateway.
func NewClient(gateway net.IP, timeout time.Duration) (nat *Client) {
	return &Client{gateway, timeout}
}

// Results of the NAT-PMP GetExternalAddress operation
type GetExternalAddressResult struct {
	SecondsSinceStartOfEpoc uint32
	ExternalIPAddress       [4]byte
}

// Get the external address of the router.
func (n *Client) GetExternalAddress() (result *GetExternalAddressResult, err error) {
	msg := make([]byte, 2)
	msg[0] = 0 // Version 0
	msg[1] = 0 // OP Code 0
	response, err := n.rpc(msg, 12)
	if err != nil {
		return
	}
	result = &GetExternalAddressResult{}
	result.SecondsSinceStartOfEpoc = readNetworkOrderUint32(response[4:8])
	copy(result.ExternalIPAddress[:], response[8:12])
	return
}

// Results of the NAT-PMP AddPortMapping operation
type AddPortMappingResult struct {
	SecondsSinceStartOfEpoc      uint32
	InternalPort                 uint16
	MappedExternalPort           uint16
	PortMappingLifetimeInSeconds uint32
}

// Add (or delete) a port mapping. To delete a mapping, set the requestedExternalPort and lifetime to 0
func (n *Client) AddPortMapping(protocol string, internalPort, requestedExternalPort int, lifetime int) (result *AddPortMappingResult, err error) {
	var opcode byte
	if protocol == "udp" {
		opcode = 1
	} else if protocol == "tcp" {
		opcode = 2
	} else {
		err = fmt.Errorf("unknown protocol %v", protocol)
		return
	}
	msg := make([]byte, 12)
	msg[0] = 0 // Version 0
	msg[1] = opcode
	writeNetworkOrderUint16(msg[4:6], uint16(internalPort))
	writeNetworkOrderUint16(msg[6:8], uint16(requestedExternalPort))
	writeNetworkOrderUint32(msg[8:12], uint32(lifetime))
	response, err := n.rpc(msg, 16)
	if err != nil {
		return
	}
	result = &AddPortMappingResult{}
	result.SecondsSinceStartOfEpoc = readNetworkOrderUint32(response[4:8])
	result.InternalPort = readNetworkOrderUint16(response[8:10])
	result.MappedExternalPort = readNetworkOrderUint16(response[10:12])
	result.PortMappingLifetimeInSeconds = readNetworkOrderUint32(response[12:16])
	return
}

func (n *Client) rpc(msg []byte, resultSize int) (result []byte, err error) {
	var server net.UDPAddr
	server.IP = n.gateway
	server.Port = nAT_PMP_PORT
	conn, err := net.DialUDP("udp", nil, &server)
	if err != nil {
		return
	}
	defer conn.Close()

	result = make([]byte, resultSize)

	timeout := time.Now().Add(n.timeout)

	err = conn.SetDeadline(timeout)
	if err != nil {
		return
	}

	var bytesRead int
	var remoteAddr *net.UDPAddr
	for time.Now().Before(timeout) {
		_, err = conn.Write(msg)
		if err != nil {
			return
		}

		bytesRead, remoteAddr, err = conn.ReadFromUDP(result)
		if err != nil {
			if err.(net.Error).Timeout() {
				continue
			}
			return
		}

		if !remoteAddr.IP.Equal(n.gateway) {
			// Ignore this packet.
			continue
		}
		if bytesRead != resultSize {
			err = fmt.Errorf("unexpected result size %d, expected %d", bytesRead, resultSize)
			return
		}
		if result[0] != 0 {
			err = fmt.Errorf("unknown protocol version %d", result[0])
			return
		}
		expectedOp := msg[1] | 0x80
		if result[1] != expectedOp {
			err = fmt.Errorf("Unexpected opcode %d. Expected %d", result[1], expectedOp)
			return
		}
		resultCode := readNetworkOrderUint16(result[2:4])
		if resultCode != 0 {
			err = fmt.Errorf("Non-zero result code %d", resultCode)
			return
		}
		// If we got here the RPC is good.
		return
	}

	err = fmt.Errorf("Timed out trying to contact gateway")
	return
}

func writeNetworkOrderUint16(buf []byte, d uint16) {
	buf[0] = byte(d >> 8)
	buf[1] = byte(d)
}

func writeNetworkOrderUint32(buf []byte, d uint32) {
	buf[0] = byte(d >> 24)
	buf[1] = byte(d >> 16)
	buf[2] = byte(d >> 8)
	buf[3] = byte(d)
}

func readNetworkOrderUint16(buf []byte) uint16 {
	return (uint16(buf[0]) << 8) | uint16(buf[1])
}

func readNetworkOrderUint32(buf []byte) uint32 {
	return (uint32(buf[0]) << 24) | (uint32(buf[1]) << 16) | (uint32(buf[2]) << 8) | uint32(buf[3])
}
