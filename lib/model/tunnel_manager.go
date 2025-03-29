// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/hitoshi44/go-uid64"
	"github.com/syncthing/syncthing/internal/gen/bep"
	"github.com/syncthing/syncthing/lib/protocol"
)

type TunnelManager struct {
	sync.Mutex
	config               *bep.TunnelConfig
	idGenerator          *uid64.Generator
	localTunnelEndpoints map[protocol.DeviceID]map[uint64]io.ReadWriteCloser
	deviceConnections    map[protocol.DeviceID]chan<- *protocol.TunnelData
}

func NewTunnelManager(configFile string) *TunnelManager {
	// Replace the filename with "tunnels.json"
	configFile = fmt.Sprintf("%s/tunnels.json", filepath.Dir(configFile))
	l.Debugln("TunnelManager created with config file:", configFile)
	config, err := loadTunnelConfig(configFile)
	if err != nil {
		l.Infoln("failed to load tunnel config:", err)
		config = nil
	}
	return NewTunnelManagerFromConfig(config)
}

func NewTunnelManagerFromConfig(config *bep.TunnelConfig) *TunnelManager {
	gen, err := uid64.NewGenerator(0)
	if err != nil {
		panic(err)
	}
	return &TunnelManager{
		config:               config,
		idGenerator:          gen,
		localTunnelEndpoints: make(map[protocol.DeviceID]map[uint64]io.ReadWriteCloser),
		deviceConnections:    make(map[protocol.DeviceID]chan<- *protocol.TunnelData),
	}
}

func (tm *TunnelManager) getInboundService(name string) *bep.TunnelInbound {
	if tm.config == nil {
		return nil
	}
	for _, service := range tm.config.TunnelsIn {
		if service.LocalServiceName == name {
			return service
		}
	}
	return nil
}

func (tm *TunnelManager) getInboundServiceDeviceIdChecked(name string, byDeviceID protocol.DeviceID) *bep.TunnelInbound {
	service := tm.getInboundService(name)
	if service == nil {
		return nil
	}
	for _, device := range service.AllowedRemoteDeviceIds {
		deviceID, err := protocol.DeviceIDFromString(device)
		if err != nil {
			l.Warnln("failed to parse device ID:", err)
			continue
		}
		if byDeviceID == deviceID {
			return service
		}
	}
	return nil
}

func (tm *TunnelManager) Serve(ctx context.Context) error {
	l.Debugln("TunnelManager Serve started")

	if tm.config != nil {
		for _, tunnel := range tm.config.TunnelsOut {
			l.Debugln("Starting listener for tunnel, device:", tunnel)
			device, err := protocol.DeviceIDFromString(tunnel.RemoteDeviceId)
			if err != nil {
				return fmt.Errorf("failed to parse device ID: %w", err)
			}
			go tm.ServeListener(ctx, tunnel.LocalListenAddress, device, tunnel.RemoteServiceName, tunnel.RemoteAddress)
		}
	}

	<-ctx.Done()
	l.Debugln("TunnelManager Serve stopped")
	return nil
}

func (tm *TunnelManager) ServeListener(ctx context.Context, listenAddress string, destinationDevice protocol.DeviceID, destinationServiceName string, destinationAddress *string) error {
	l.Infoln("ServeListener started for address:", listenAddress, "destination device:", destinationDevice, "destination address:", destinationAddress)
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
		l.Debugln("Accepted connection, tunnel ID:", tunnelID)
		tm.registerLocalTunnelEndpoint(destinationDevice, tunnelID, conn)

		// send open command to the destination device
		tm.deviceConnections[destinationDevice] <- &protocol.TunnelData{
			D: &bep.TunnelData{
				TunnelId:                 tunnelID,
				Command:                  bep.TunnelCommand_TUNNEL_COMMAND_OPEN,
				RemoteServiceName:        &destinationServiceName,
				TunnelDestinationAddress: destinationAddress,
			},
		}

		var optionalDestinationAddress string
		if destinationAddress == nil {
			optionalDestinationAddress = "by-remote"
		} else {
			optionalDestinationAddress = *destinationAddress
		}
		go tm.handleLocalTunnelEndpoint(ctx, tunnelID, conn, destinationDevice, destinationServiceName, optionalDestinationAddress)
	}

	return nil
}

func (tm *TunnelManager) handleLocalTunnelEndpoint(ctx context.Context, tunnelID uint64, conn io.ReadWriter, destinationDevice protocol.DeviceID, destinationServiceName string, destinationAddress string) {
	l.Debugln("Handling local tunnel endpoint, tunnel ID:", tunnelID, "destination device:", destinationDevice, "destination service name:", destinationServiceName, "destination address:", destinationAddress)
	defer tm.deregisterLocalTunnelEndpoint(destinationDevice, tunnelID)
	defer func() {
		// send close command to the destination device
		tm.deviceConnections[destinationDevice] <- &protocol.TunnelData{
			D: &bep.TunnelData{
				TunnelId: tunnelID,
				Command:  bep.TunnelCommand_TUNNEL_COMMAND_CLOSE,
			},
		}
		l.Debugln("Closed local tunnel endpoint, tunnel ID:", tunnelID)
	}()

	// Example: Forward data to the destination address
	// This is a placeholder implementation
	for {
		select {
		case <-ctx.Done():
			l.Debugln("Context done for tunnel ID:", tunnelID)
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
			l.Debugf("Forwarding data to device %v, %s (%d tunnel id): len: %d\n", destinationDevice, destinationAddress, tunnelID, n)

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

func (tm *TunnelManager) registerLocalTunnelEndpoint(deviceID protocol.DeviceID, tunnelID uint64, conn io.ReadWriteCloser) {
	l.Debugln("Registering local tunnel endpoint, device ID:", deviceID, "tunnel ID:", tunnelID)
	tm.Lock()
	defer tm.Unlock()
	if tm.localTunnelEndpoints[deviceID] == nil {
		tm.localTunnelEndpoints[deviceID] = make(map[uint64]io.ReadWriteCloser)
	}
	tm.localTunnelEndpoints[deviceID][tunnelID] = conn
}

func (tm *TunnelManager) deregisterLocalTunnelEndpoint(deviceID protocol.DeviceID, tunnelID uint64) {
	l.Debugln("Deregistering local tunnel endpoint, device ID:", deviceID, "tunnel ID:", tunnelID)
	tm.Lock()
	defer tm.Unlock()
	delete(tm.localTunnelEndpoints[deviceID], tunnelID)
}

func (tm *TunnelManager) RegisterDeviceConnection(device protocol.DeviceID, tunnelIn <-chan *protocol.TunnelData, tunnelOut chan<- *protocol.TunnelData) {
	l.Debugln("Registering device connection, device ID:", device)
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
	l.Debugln("Deregistering device connection, device ID:", device)
	tm.Lock()
	defer tm.Unlock()
	delete(tm.deviceConnections, device)
}

func (tm *TunnelManager) forwardRemoteTunnelData(fromDevice protocol.DeviceID, data *protocol.TunnelData) {
	l.Debugln("Forwarding remote tunnel data, from device ID:", fromDevice, "command:", data.D.Command)
	switch data.D.Command {
	case bep.TunnelCommand_TUNNEL_COMMAND_OPEN:
		if data.D.RemoteServiceName == nil {
			l.Warnf("No remote service name specified")
			return
		}
		service := tm.getInboundServiceDeviceIdChecked(*data.D.RemoteServiceName, fromDevice)
		if service == nil {
			l.Warnf("Device %v is not allowed to access service %s", fromDevice, *data.D.RemoteServiceName)
			return
		}
		var TunnelDestinationAddress string
		if service.LocalDialAddress == "any" {
			if data.D.TunnelDestinationAddress == nil {
				l.Warnf("No tunnel destination specified")
				return
			}
			TunnelDestinationAddress = *data.D.TunnelDestinationAddress
		} else {
			TunnelDestinationAddress = service.LocalDialAddress
		}

		addr, err := net.ResolveTCPAddr("tcp", TunnelDestinationAddress)
		if err != nil {
			l.Warnf("Failed to resolve tunnel destination: %v", err)
			return
		}
		conn, err := net.DialTCP("tcp", nil, addr)
		if err != nil {
			l.Warnf("Failed to dial tunnel destination: %v", err)
			return
		}
		tm.registerLocalTunnelEndpoint(fromDevice, data.D.TunnelId, conn)
		go tm.handleLocalTunnelEndpoint(context.Background(), data.D.TunnelId, conn, fromDevice, *data.D.RemoteServiceName, TunnelDestinationAddress)

	case bep.TunnelCommand_TUNNEL_COMMAND_DATA:
		tm.Lock()
		tcpConn, ok := tm.localTunnelEndpoints[fromDevice][data.D.TunnelId]
		tm.Unlock()
		if ok {
			_, err := tcpConn.Write(data.D.Data)
			if err != nil {
				l.Warnf("Failed to forward tunnel data: %v", err)
			}
		} else {
			l.Warnf("Data: No TCP connection found for device %v, TunnelID: %s", fromDevice, data.D.TunnelId)
		}
	case bep.TunnelCommand_TUNNEL_COMMAND_CLOSE:
		tm.Lock()
		tcpConn, ok := tm.localTunnelEndpoints[fromDevice][data.D.TunnelId]
		tm.Unlock()
		if ok {
			tcpConn.Close()
		} else {
			l.Infof("Close: No TCP connection found for device %v, TunnelID: %s", fromDevice, data.D.TunnelId)
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
	l.Debugln("Generated tunnel ID:", id)
	return uint64(id)
}

// Status returns information about active tunnels
func (m *TunnelManager) Status() []map[string]interface{} {
	m.Lock()
	defer m.Unlock()

	status := make([]map[string]interface{}, 0, len(m.config.TunnelsIn)+len(m.config.TunnelsOut))

	for _, tunnel := range m.config.TunnelsIn {
		info := map[string]interface{}{
			"serviceID":              tunnel.LocalServiceName,
			"allowedRemoteDeviceIDs": tunnel.AllowedRemoteDeviceIds,
			"localDialAddress":       tunnel.LocalDialAddress,
			"active":                 true,
			"type":                   "inbound",
		}
		status = append(status, info)
	}
	for _, tunnel := range m.config.TunnelsOut {
		info := map[string]interface{}{
			"localListenAddress": tunnel.LocalListenAddress,
			"remoteDeviceID":     tunnel.RemoteDeviceId,
			"serviceID":          tunnel.RemoteServiceName,
			"remoteAddress":      tunnel.RemoteAddress,
			"active":             true,
			"type":               "outbound",
		}
		status = append(status, info)
	}

	return status
}

func loadTunnelConfig(path string) (*bep.TunnelConfig, error) {
	l.Debugln("Loading tunnel config from file:", path)
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

	l.Debugln("Loaded tunnel config:", &config)
	return &config, nil
}
