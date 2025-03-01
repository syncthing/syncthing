package model

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"sync"

	"github.com/hitoshi44/go-uid64"
	"github.com/syncthing/syncthing/internal/gen/bep"
	"github.com/syncthing/syncthing/lib/protocol"
)

type TunnelManager struct {
	sync.Mutex
	configFile           string
	idGenerator          *uid64.Generator
	localTunnelEndpoints map[uint64]io.ReadWriteCloser
	deviceConnections    map[protocol.DeviceID]chan<- *protocol.TunnelData
}

func NewTunnelManager(configFile string) *TunnelManager {
	gen, err := uid64.NewGenerator(0)
	if err != nil {
		panic(err)
	}
	return &TunnelManager{
		idGenerator:          gen,
		localTunnelEndpoints: make(map[uint64]io.ReadWriteCloser),
		deviceConnections:    make(map[protocol.DeviceID]chan<- *protocol.TunnelData),
	}
}

func (tm *TunnelManager) Serve(ctx context.Context) error {
	// Load listener address and destination device from JSON config file
	config, err := loadTunnelConfig(tm.configFile)
	if err != nil {
		return fmt.Errorf("failed to load tunnel config: %w", err)
	}

	for _, tunnel := range config.Tunnels {
		go tm.ServeListener(ctx, tunnel.LocalListenAddress, protocol.DeviceID(tunnel.DeviceID), tunnel.RemoteAddress)
	}

	<-ctx.Done()
	return nil
}

func (tm *TunnelManager) ServeListener(ctx context.Context, listenAddress string, destinationDevice protocol.DeviceID, destinationAddress string) error {
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

		tunnelID := tm.generateTunnelID()
		tm.registerLocalTunnelEndpoint(tunnelID, conn)

		// send open command to the destination device
		tm.deviceConnections[destinationDevice] <- &protocol.TunnelData{
			D: &bep.TunnelData{
				TunnelId:          tunnelID,
				Command:           bep.TunnelCommand_TUNNEL_COMMAND_OPEN,
				TunnelDestination: &destinationAddress,
			},
		}

		go tm.handleLocalTunnelEndpoint(ctx, tunnelID, conn, destinationDevice, destinationAddress)
	}

	return nil
}

func (tm *TunnelManager) handleLocalTunnelEndpoint(ctx context.Context, tunnelID uint64, conn io.ReadWriter, destinationDevice protocol.DeviceID, destinationAddress string) {
	defer tm.deregisterLocalTunnelEndpoint(tunnelID)
	defer func() {
		// send close command to the destination device
		tm.deviceConnections[destinationDevice] <- &protocol.TunnelData{
			D: &bep.TunnelData{
				TunnelId: tunnelID,
				Command:  bep.TunnelCommand_TUNNEL_COMMAND_CLOSE,
			},
		}
	}()

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
			fmt.Printf("Forwarding data to device %v, %s (%d tunnel id): len: %s\n", destinationDevice, destinationAddress, tunnelID, n)

			// Send the data to the destination device
			tm.deviceConnections[destinationDevice] <- &protocol.TunnelData{
				D: &bep.TunnelData{
					TunnelId: tunnelID,
					Command:  bep.TunnelCommand_TUNNEL_COMMAND_DATA,
					Data:     buffer[:n],
				},
			}
		}
	}
}

func (tm *TunnelManager) registerLocalTunnelEndpoint(tunnelID uint64, conn io.ReadWriteCloser) {
	tm.Lock()
	defer tm.Unlock()
	tm.localTunnelEndpoints[tunnelID] = conn
}

func (tm *TunnelManager) deregisterLocalTunnelEndpoint(tunnelID uint64) {
	tm.Lock()
	defer tm.Unlock()
	delete(tm.localTunnelEndpoints, tunnelID)
}

func (tm *TunnelManager) RegisterDeviceConnection(device protocol.DeviceID, tunnelIn <-chan *protocol.TunnelData, tunnelOut chan<- *protocol.TunnelData) {
	tm.Lock()
	defer tm.Unlock()
	tm.deviceConnections[device] = tunnelOut

	// handle all incoming tunnel data for this device
	go func() {
		for data := range tunnelIn {
			tm.forwardRemoteTunnelData(device, data)
		}
	}()
}

func (tm *TunnelManager) DeregisterDeviceConnection(device protocol.DeviceID) {
	tm.Lock()
	defer tm.Unlock()
	delete(tm.deviceConnections, device)
}

func (tm *TunnelManager) forwardRemoteTunnelData(fromDevice protocol.DeviceID, data *protocol.TunnelData) {

	switch data.D.Command {
	case bep.TunnelCommand_TUNNEL_COMMAND_OPEN:
		if data.D.TunnelDestination == nil {
			l.Warnf("No tunnel destination specified")
			return
		}
		addr, err := net.ResolveTCPAddr("tcp", *data.D.TunnelDestination)
		if err != nil {
			l.Warnf("Failed to resolve tunnel destination: %v", err)
			return
		}
		conn, err := net.DialTCP("tcp", nil, addr)
		if err != nil {
			l.Warnf("Failed to dial tunnel destination: %v", err)
			return
		}
		tm.registerLocalTunnelEndpoint(data.D.TunnelId, conn)
		go tm.handleLocalTunnelEndpoint(context.Background(), data.D.TunnelId, conn, fromDevice, *data.D.TunnelDestination)

	case bep.TunnelCommand_TUNNEL_COMMAND_DATA:
		tm.Lock()
		tcpConn, ok := tm.localTunnelEndpoints[data.D.TunnelId]
		tm.Unlock()
		if ok {
			_, err := tcpConn.Write(data.D.Data)
			if err != nil {
				l.Warnf("Failed to forward tunnel data: %v", err)
			}
		} else {
			l.Warnf("No TCP connection found for TunnelID: %s", data.D.TunnelId)
		}
	case bep.TunnelCommand_TUNNEL_COMMAND_CLOSE:
		tm.Lock()
		tcpConn, ok := tm.localTunnelEndpoints[data.D.TunnelId]
		tm.Unlock()
		if ok {
			tcpConn.Close()
		} else {
			l.Warnf("No TCP connection found for TunnelID: %s", data.D.TunnelId)
		}
	default: // unknown command
		l.Warnf("Unknown tunnel command: %v", data.D.Command)
	}
}

func (tm *TunnelManager) generateTunnelID() uint64 {
	id, err := tm.idGenerator.Gen()
	if err != nil {
		panic(err)
	}
	return uint64(id)
}

func loadTunnelConfig(path string) (*bep.TunnelConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var config bep.TunnelConfig
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return nil, err
	}

	return &config, nil
}
