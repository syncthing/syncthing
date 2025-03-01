package model

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

type TunnelManager struct {
	sync.Mutex
	tunnels map[uint64]io.ReadWriter
}

func NewTunnelManager() *TunnelManager {
	return &TunnelManager{
		tunnels: make(map[uint64]io.ReadWriter),
	}
}

func (tm *TunnelManager) Serve(ctx context.Context, listenAddress string, destinationDevice string, destinationAddress string) error {
	listener, err := net.Listen("tcp", listenAddress)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", listenAddress, err)
	}
	defer listener.Close()

	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("failed to accept connection: %w", err)
		}

		tunnelID := generateTunnelID()
		tm.RegisterTunnel(tunnelID, conn)

		go tm.handleConnection(ctx, tunnelID, destinationDevice, destinationAddress)
	}

	return nil
}

func (tm *TunnelManager) handleConnection(ctx context.Context, tunnelID uint64, destinationDevice string, destinationAddress string) {
	// Implement the logic to handle the connection
	// This is a placeholder implementation
	conn, ok := tm.tunnels[tunnelID]
	if !ok {
		return
	}
	defer tm.DeregisterTunnel(tunnelID)

	// Example: Forward data to the destination address
	// This is a placeholder implementation
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Read data from the connection and forward it
			buffer := make([]byte, 1024)
			n, err := conn.Read(buffer)
			if err != nil {
				return
			}
			// Forward data to the destination
			// This is a placeholder implementation
			fmt.Printf("Forwarding data to %s: %s\n", destinationAddress, string(buffer[:n]))
		}
	}
}

func (tm *TunnelManager) RegisterTunnel(tunnelID uint64, conn io.ReadWriter) {
	tm.Lock()
	defer tm.Unlock()
	tm.tunnels[tunnelID] = conn
}

func (tm *TunnelManager) DeregisterTunnel(tunnelID uint64) {
	tm.Lock()
	defer tm.Unlock()
	delete(tm.tunnels, tunnelID)
}

func (tm *TunnelManager) ForwardTunnelData(tunnelID uint64, data []byte) error {
	tm.Lock()
	conn, ok := tm.tunnels[tunnelID]
	tm.Unlock()
	if !ok {
		return fmt.Errorf("no tunnel found for ID %d", tunnelID)
	}
	_, err := conn.Write(data)
	return err
}

func generateTunnelID() uint64 {
	// Implement a function to generate a unique tunnel ID
	// This is a placeholder implementation
	return uint64(time.Now().UnixNano())
}
