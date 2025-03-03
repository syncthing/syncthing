package model

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/syncthing/syncthing/internal/gen/bep"

	"github.com/syncthing/syncthing/lib/protocol"
)

func repeatedDeviceID(v byte) (d protocol.DeviceID) {
	for i := range d {
		d[i] = v
	}
	return
}

func TestTunnelManager_ServeListener(t *testing.T) {
	// Activate debug logging
	l.SetDebug("module", true)

	// Mock device ID and addresses
	serverDeviceID := repeatedDeviceID(0x11)
	listenAddress := "127.0.0.1:64777"
	destinationAddress := "127.0.0.1:8080"

	// Create a new TunnelManager
	proxyServiceName := "proxy"
	tm := NewTunnelManagerFromConfig(
		&bep.TunnelConfig{
			TunnelsOut: []*bep.TunnelOutbound{
				{
					LocalListenAddress: listenAddress,
					RemoteDeviceId:     serverDeviceID.String(),
					RemoteServiceName:  &proxyServiceName,
					RemoteAddress:      &destinationAddress,
				},
			},
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the listener
	go tm.Serve(ctx)

	// Create a channel to capture the TunnelData sent to the device
	tunnelDataChan := make(chan *protocol.TunnelData, 1)
	tm.RegisterDeviceConnection(serverDeviceID, nil, tunnelDataChan)

	var conn net.Conn
	var err error
	for range 100 {
		// Give the listener some time to start
		time.Sleep(100 * time.Millisecond)

		// Connect to the listener
		conn, err = net.Dial("tcp", listenAddress)
		if err == nil {
			break
		}
	}
	assert.NoError(t, err)

	// Wait for the TunnelData to be sent
	var tunnelID uint64
	select {
	case data := <-tunnelDataChan:
		assert.Equal(t, bep.TunnelCommand_TUNNEL_COMMAND_OPEN, data.D.Command)
		assert.Equal(t, proxyServiceName, *data.D.RemoteServiceName)
		assert.Equal(t, destinationAddress, *data.D.TunnelDestinationAddress)
		tunnelID = data.D.TunnelId
		assert.NotEqual(t, 0, tunnelID)
	case <-time.After(1 * time.Second):
		t.Fatal("Timed out waiting for TunnelData")
	}

	msg := []byte("hello")
	conn.Write(msg)

	// Wait for the TunnelData to be sent
	select {
	case data := <-tunnelDataChan:
		assert.Equal(t, bep.TunnelCommand_TUNNEL_COMMAND_DATA, data.D.Command)
		assert.Equal(t, msg, data.D.Data)
		assert.Equal(t, tunnelID, data.D.TunnelId)
	case <-time.After(1 * time.Second):
		t.Fatal("Timed out waiting for TunnelData")
	}

	conn.Close()

	// Wait for the TunnelData to be sent
	select {
	case data := <-tunnelDataChan:
		assert.Equal(t, bep.TunnelCommand_TUNNEL_COMMAND_CLOSE, data.D.Command)
		assert.Equal(t, tunnelID, data.D.TunnelId)
	case <-time.After(1 * time.Second):
		t.Fatal("Timed out waiting for TunnelData")
	}
}

func TestTunnelManager_HandleOpenCommand(t *testing.T) {
	// Activate debug logging
	l.SetDebug("module", true)

	clientDeviceID := repeatedDeviceID(0x22)

	// Create a new TunnelManager
	tm := NewTunnelManagerFromConfig(
		&bep.TunnelConfig{
			TunnelsIn: []*bep.TunnelInbound{
				{
					LocalServiceName: "proxy",
					LocalDialAddress: "any",
					AllowedRemoteDeviceIds: []string{
						clientDeviceID.String(),
					},
				},
			},
		},
	)

	// Mock device ID and addresses
	destinationAddress := "127.0.0.1:64780"

	// Create a channel to capture the TunnelData sent to the device
	tunnelDataChanIn := make(chan *protocol.TunnelData, 1)
	tunnelDataChanOut := make(chan *protocol.TunnelData, 1)
	tm.RegisterDeviceConnection(clientDeviceID, tunnelDataChanIn, tunnelDataChanOut)

	// Start a listener on the destination address
	listener, err := net.Listen("tcp", destinationAddress)
	assert.NoError(t, err)
	defer listener.Close()

	// Send an open command to the TunnelManager
	tunnelID := tm.generateTunnelID()
	proxyServiceName := "proxy"
	tunnelDataChanIn <- &protocol.TunnelData{
		D: &bep.TunnelData{
			TunnelId:                 tunnelID,
			Command:                  bep.TunnelCommand_TUNNEL_COMMAND_OPEN,
			RemoteServiceName:        &proxyServiceName,
			TunnelDestinationAddress: &destinationAddress,
		},
	}

	// Wait for the TunnelManager to connect to the listener
	conn, err := listener.Accept()
	assert.NoError(t, err)

	// Verify the connection
	msg_from_server := []byte("hello from server")
	_, err = conn.Write(msg_from_server)
	assert.NoError(t, err)

	// Wait for the TunnelData to be sent
	select {
	case data := <-tunnelDataChanOut:
		assert.Equal(t, bep.TunnelCommand_TUNNEL_COMMAND_DATA, data.D.Command)
		assert.Equal(t, msg_from_server, data.D.Data)
		assert.Equal(t, tunnelID, data.D.TunnelId)
	case <-time.After(1 * time.Second):
		t.Fatal("Timed out waiting for TunnelData")
	}

	msg_from_client := []byte("hello from client")
	tunnelDataChanIn <- &protocol.TunnelData{
		D: &bep.TunnelData{
			TunnelId: tunnelID,
			Command:  bep.TunnelCommand_TUNNEL_COMMAND_DATA,
			Data:     msg_from_client,
		},
	}

	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, msg_from_client, buf[:n])

	conn.Close()

	// Wait for the TunnelData to be sent
	select {
	case data := <-tunnelDataChanOut:
		assert.Equal(t, bep.TunnelCommand_TUNNEL_COMMAND_CLOSE, data.D.Command)
		assert.Equal(t, tunnelID, data.D.TunnelId)
	case <-time.After(1 * time.Second):
		t.Fatal("Timed out waiting for TunnelData")
	}
}
