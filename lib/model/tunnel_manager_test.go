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

func TestTunnelManager_ServeListener(t *testing.T) {
	// Activate debug logging
	l.SetDebug("module", true)

	// Create a new TunnelManager
	tm := NewTunnelManagerFromConfig(nil)

	// Mock device ID and addresses
	deviceID := protocol.DeviceID{}
	listenAddress := "127.0.0.1:64777"
	destinationAddress := "127.0.0.1:8080"

	// Create a channel to capture the TunnelData sent to the device
	tunnelDataChan := make(chan *protocol.TunnelData, 1)
	tm.RegisterDeviceConnection(deviceID, nil, tunnelDataChan)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the listener
	go func() {
		err := tm.ServeListener(ctx, listenAddress, deviceID, destinationAddress)
		assert.NoError(t, err)
	}()

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
	select {
	case data := <-tunnelDataChan:
		assert.Equal(t, bep.TunnelCommand_TUNNEL_COMMAND_OPEN, data.D.Command)
		assert.Equal(t, destinationAddress, *data.D.TunnelDestination)
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
	case <-time.After(1 * time.Second):
		t.Fatal("Timed out waiting for TunnelData")
	}

	conn.Close()

	// Wait for the TunnelData to be sent
	select {
	case data := <-tunnelDataChan:
		assert.Equal(t, bep.TunnelCommand_TUNNEL_COMMAND_CLOSE, data.D.Command)
	case <-time.After(1 * time.Second):
		t.Fatal("Timed out waiting for TunnelData")
	}
}
