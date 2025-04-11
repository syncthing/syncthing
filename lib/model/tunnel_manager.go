// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/ek220/guf"
	"github.com/hitoshi44/go-uid64"
	"github.com/syncthing/syncthing/internal/gen/bep"
	"github.com/syncthing/syncthing/lib/protocol"
)

func hashDescriptor(descriptor string) string {
	hash := sha256.Sum256([]byte(descriptor))
	return hex.EncodeToString(hash[:])
}

func getConfigDescriptorOutbound(cfg *bep.TunnelOutbound) string {
	plainDescriptor := fmt.Sprintf("out-%s>%s:%s:%s-%v",
		cfg.LocalListenAddress,
		cfg.RemoteDeviceId,
		cfg.RemoteServiceName,
		guf.DerefOrDefault(cfg.RemoteAddress),
		guf.DerefOr(cfg.Enabled, true),
	)
	return hashDescriptor(plainDescriptor)
}

func getConfigDescriptorInbound(cfg *bep.TunnelInbound, withAllowedDevices bool) string {
	plainDescriptor := fmt.Sprintf("in-%s:%s-%v",
		cfg.LocalServiceName,
		cfg.LocalDialAddress,
		guf.DerefOr(cfg.Enabled, true),
	)
	if withAllowedDevices {
		plainDescriptor += fmt.Sprintf("<%s", cfg.AllowedRemoteDeviceIds)
	}
	return hashDescriptor(plainDescriptor)
}

func getUiTunnelDescriptorOutbound(cfg *bep.TunnelOutbound) string {
	plainDescriptor := fmt.Sprintf("out-%s>%s:%s:%s",
		cfg.LocalListenAddress,
		cfg.RemoteDeviceId,
		cfg.RemoteServiceName,
		guf.DerefOrDefault(cfg.RemoteAddress),
	)
	return fmt.Sprintf("o-%s-%s", cfg.RemoteServiceName, hashDescriptor(plainDescriptor))
}

func getUiTunnelDescriptorInbound(cfg *bep.TunnelInbound) string {
	plainDescriptor := fmt.Sprintf("in-%s:%s",
		cfg.LocalServiceName,
		cfg.LocalDialAddress,
	)
	return fmt.Sprintf("i-%s-%s", cfg.LocalServiceName, hashDescriptor(plainDescriptor))
}

type tunnelBaseConfig struct {
	descriptor string
	ctx        context.Context
	cancel     context.CancelFunc
}

type tunnelOutConfig struct {
	tunnelBaseConfig
	json *bep.TunnelOutbound
}

type tunnelInConfig struct {
	tunnelBaseConfig
	allowedClients map[string]tunnelBaseConfig
	json           *bep.TunnelInbound
}

type DeviceConnection struct {
	tunnelOut        chan<- *protocol.TunnelData
	serviceOfferings map[string]uint32
}

type TunnelManager struct {
	configMutex          sync.Mutex // Mutex for configIn and configOut
	endpointsMutex       sync.Mutex // Mutex for localTunnelEndpoints and deviceConnections
	configFile           string
	configIn             map[string]*tunnelInConfig
	configOut            map[string]*tunnelOutConfig
	serviceRunning       bool
	idGenerator          *uid64.Generator
	localTunnelEndpoints map[protocol.DeviceID]map[uint64]io.ReadWriteCloser
	deviceConnections    map[protocol.DeviceID]*DeviceConnection
}

func (m *TunnelManager) TunnelStatus() []map[string]interface{} {
	return m.Status()
}

func (m *TunnelManager) AddTunnelOutbound(localListenAddress string, remoteDeviceID protocol.DeviceID, remoteServiceName string) error {
	return m.AddOutboundTunnel(localListenAddress, remoteDeviceID, remoteServiceName)
}

func NewTunnelManager(configFile string) *TunnelManager {
	// Replace the filename with "tunnels.json"
	configFile = fmt.Sprintf("%s/tunnels.json", filepath.Dir(configFile))
	l.Debugln("TunnelManager created with config file:", configFile)
	config, err := loadTunnelConfig(configFile)
	if err != nil {
		l.Infoln("failed to load tunnel config:", err)
		config = &bep.TunnelConfig{}
	}
	return NewTunnelManagerFromConfig(config, configFile)
}

func NewTunnelManagerFromConfig(config *bep.TunnelConfig, configFile string) *TunnelManager {
	gen, err := uid64.NewGenerator(0)
	if err != nil {
		panic(err)
	}

	if config == nil {
		panic("TunnelManager config is nil")
	}

	tm := &TunnelManager{
		configFile:           configFile,
		configIn:             make(map[string]*tunnelInConfig),
		configOut:            make(map[string]*tunnelOutConfig),
		serviceRunning:       false,
		idGenerator:          gen,
		localTunnelEndpoints: make(map[protocol.DeviceID]map[uint64]io.ReadWriteCloser),
		deviceConnections:    make(map[protocol.DeviceID]*DeviceConnection),
	}
	// use update logic to set the initial config as well.
	// this avoids code duplication
	tm.updateOutConfig(config.TunnelsOut)
	tm.updateInConfig(config.TunnelsIn)
	return tm
}

func (tm *TunnelManager) updateInConfig(newInTunnels []*bep.TunnelInbound) {
	tm.configMutex.Lock()
	defer tm.configMutex.Unlock()

	// Generate a new map of inbound tunnels
	newConfigIn := make(map[string]*tunnelInConfig)
	for _, newTun := range newInTunnels {
		descriptor := getConfigDescriptorInbound(newTun, false)
		if existingTun, exists := tm.configIn[descriptor]; exists {
			// Reuse existing context and cancel function
			existingTun.json = newTun // update e.g. suggested port
			// Update allowed devices
			allowedClients := make(map[string]tunnelBaseConfig)
			for _, deviceID := range newTun.AllowedRemoteDeviceIds {
				if _, exists := existingTun.allowedClients[deviceID]; !exists {
					ctx, cancel := context.WithCancel(existingTun.ctx)
					allowedClients[deviceID] = tunnelBaseConfig{
						descriptor: descriptor,
						ctx:        ctx,
						cancel:     cancel,
					}
				} else {
					allowedClients[deviceID] = existingTun.allowedClients[deviceID]
				}
			}
			// Cancel and remove devices no longer allowed
			for deviceID, existingClient := range existingTun.allowedClients {
				if _, exists := allowedClients[deviceID]; !exists {
					existingClient.cancel()
				}
			}
			existingTun.allowedClients = allowedClients

			newConfigIn[descriptor] = existingTun

		} else {
			// Create new context and cancel function
			ctx, cancel := context.WithCancel(context.Background())
			newConfigIn[descriptor] = &tunnelInConfig{
				tunnelBaseConfig: tunnelBaseConfig{
					descriptor: descriptor,
					ctx:        ctx,
					cancel:     cancel,
				},
				json:           newTun,
				allowedClients: make(map[string]tunnelBaseConfig),
			}
		}
	}

	// Cancel and remove tunnels that are no longer in the new configuration
	for descriptor, existing := range tm.configIn {
		if _, exists := newConfigIn[descriptor]; !exists {
			existing.cancel()
		}
	}

	// Replace the old configuration with the new one
	tm.configIn = newConfigIn
}

func (tm *TunnelManager) updateOutConfig(newOutTunnels []*bep.TunnelOutbound) {
	tm.configMutex.Lock()
	defer tm.configMutex.Unlock()

	// Generate a new map of outbound tunnels
	newConfigOut := make(map[string]*tunnelOutConfig)
	for _, tunnel := range newOutTunnels {
		descriptor := getConfigDescriptorOutbound(tunnel)
		if existing, exists := tm.configOut[descriptor]; exists {
			// Reuse existing context and cancel function
			newConfigOut[descriptor] = existing
		} else {
			// Create new context and cancel function
			ctx, cancel := context.WithCancel(context.Background())
			newConfigOut[descriptor] = &tunnelOutConfig{
				tunnelBaseConfig: tunnelBaseConfig{
					descriptor: descriptor,
					ctx:        ctx,
					cancel:     cancel,
				},
				json: tunnel,
			}
			if tm.serviceRunning {
				tm.serveOutboundTunnel(newConfigOut[descriptor])
			}
		}
	}

	// Cancel and remove tunnels that are no longer in the new configuration
	for descriptor, existing := range tm.configOut {
		if _, exists := newConfigOut[descriptor]; !exists {
			existing.cancel()
		}
	}

	// Replace the old configuration with the new one
	tm.configOut = newConfigOut
}

func (tm *TunnelManager) getInboundService(name string) *tunnelInConfig {
	tm.configMutex.Lock()
	defer tm.configMutex.Unlock()
	for _, service := range tm.configIn {
		if service.json.LocalServiceName == name {
			return service
		}
	}
	return nil
}

func (tm *TunnelManager) getEnabledInboundServiceDeviceIdChecked(name string, byDeviceID protocol.DeviceID) *tunnelInConfig {
	service := tm.getInboundService(name)
	if service == nil {
		return nil
	}
	if !guf.DerefOr(service.json.Enabled, true) {
		l.Warnf("Device %v tries to access disabled service %s", byDeviceID, name)
		return nil
	}
	for _, device := range service.json.AllowedRemoteDeviceIds {
		deviceID, err := protocol.DeviceIDFromString(device)
		if err != nil {
			l.Warnln("failed to parse device ID:", err)
			continue
		}
		if byDeviceID == deviceID {
			return service
		}
	}

	l.Warnf("Device %v is not allowed to access service %s", byDeviceID, name)
	return nil
}

func (tm *TunnelManager) serveOutboundTunnel(tunnel *tunnelOutConfig) {
	if !guf.DerefOr(tunnel.json.Enabled, true) {
		l.Debugln("Tunnel is disabled, skipping:", tunnel)
		return
	}

	l.Debugln("Starting listener for tunnel, device:", tunnel)
	device, err := protocol.DeviceIDFromString(tunnel.json.RemoteDeviceId)
	if err != nil {
		l.Warnln("failed to parse device ID:", err)
	}
	go tm.ServeListener(tunnel.ctx, tunnel.json.LocalListenAddress,
		device, tunnel.json.RemoteServiceName, tunnel.json.RemoteAddress)
}

func (tm *TunnelManager) Serve(ctx context.Context) error {
	l.Debugln("TunnelManager Serve started")

	func() {
		tm.configMutex.Lock()
		defer tm.configMutex.Unlock()
		tm.serviceRunning = true
		for _, tunnel := range tm.configOut {
			tm.serveOutboundTunnel(tunnel)
		}
	}()

	<-ctx.Done()
	l.Debugln("TunnelManager Serve stopping")

	// Cancel all active tunnels
	func() {
		tm.configMutex.Lock()
		defer tm.configMutex.Unlock()
		tm.serviceRunning = false
		for _, tunnel := range tm.configIn {
			tunnel.cancel()
		}
		for _, tunnel := range tm.configOut {
			tunnel.cancel()
		}
	}()

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
		tm.deviceConnections[destinationDevice].tunnelOut <- &protocol.TunnelData{
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
}

func (tm *TunnelManager) handleLocalTunnelEndpoint(ctx context.Context, tunnelID uint64, conn io.ReadWriteCloser, destinationDevice protocol.DeviceID, destinationServiceName string, destinationAddress string) {
	l.Debugln("Handling local tunnel endpoint, tunnel ID:", tunnelID, "destination device:", destinationDevice, "destination service name:", destinationServiceName, "destination address:", destinationAddress)
	defer tm.deregisterLocalTunnelEndpoint(destinationDevice, tunnelID)
	defer func() {
		// send close command to the destination device
		tm.deviceConnections[destinationDevice].tunnelOut <- &protocol.TunnelData{
			D: &bep.TunnelData{
				TunnelId: tunnelID,
				Command:  bep.TunnelCommand_TUNNEL_COMMAND_CLOSE,
			},
		}
		l.Debugln("Closed local tunnel endpoint, tunnel ID:", tunnelID)
	}()

	stop := context.AfterFunc(ctx, func() {
		l.Debugln("Stopping local tunnel endpoint, tunnel ID:", tunnelID)
		conn.Close()
	})

	defer func() {
		stop()
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
			tm.deviceConnections[destinationDevice].tunnelOut <- &protocol.TunnelData{
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
	tm.endpointsMutex.Lock()
	defer tm.endpointsMutex.Unlock()
	if tm.localTunnelEndpoints[deviceID] == nil {
		tm.localTunnelEndpoints[deviceID] = make(map[uint64]io.ReadWriteCloser)
	}
	tm.localTunnelEndpoints[deviceID][tunnelID] = conn
}

func (tm *TunnelManager) deregisterLocalTunnelEndpoint(deviceID protocol.DeviceID, tunnelID uint64) {
	l.Debugln("Deregistering local tunnel endpoint, device ID:", deviceID, "tunnel ID:", tunnelID)
	tm.endpointsMutex.Lock()
	defer tm.endpointsMutex.Unlock()
	delete(tm.localTunnelEndpoints[deviceID], tunnelID)
	if len(tm.localTunnelEndpoints[deviceID]) == 0 {
		delete(tm.localTunnelEndpoints, deviceID) // Ensure map cleanup
	}
}

func (tm *TunnelManager) RegisterDeviceConnection(device protocol.DeviceID, tunnelIn <-chan *protocol.TunnelData, tunnelOut chan<- *protocol.TunnelData) {
	func() {
		l.Debugln("Registering device connection, device ID:", device)
		tm.endpointsMutex.Lock()
		defer tm.endpointsMutex.Unlock()
		tm.deviceConnections[device] = &DeviceConnection{
			tunnelOut:        tunnelOut,
			serviceOfferings: make(map[string]uint32),
		}
	}()

	// handle all incoming tunnel data for this device
	go func() {
		for data := range tunnelIn {
			tm.forwardRemoteTunnelData(device, data)
		}
	}()

	go func() {
		// send tunnel service offerings
		tm.configMutex.Lock()
		defer tm.configMutex.Unlock()
		for _, inboundSerice := range tm.configIn {
			if slices.Contains(inboundSerice.json.AllowedRemoteDeviceIds, device.String()) {
				suggestedPort := strconv.FormatUint(uint64(guf.DerefOr(inboundSerice.json.SuggestedPort, 0)), 10)
				tunnelOut <- &protocol.TunnelData{
					D: &bep.TunnelData{
						Command:                  bep.TunnelCommand_TUNNEL_COMMAND_OFFER,
						RemoteServiceName:        &inboundSerice.json.LocalServiceName,
						TunnelDestinationAddress: &suggestedPort,
					},
				}
			}
		}
	}()
}

func (tm *TunnelManager) DeregisterDeviceConnection(device protocol.DeviceID) {
	l.Debugln("Deregistering device connection, device ID:", device)
	tm.endpointsMutex.Lock()
	defer tm.endpointsMutex.Unlock()
	delete(tm.deviceConnections, device)
}

func parseUint32Or(input string, defaultValue uint32) uint32 {
	// Parse the input string as a uint32
	value, err := strconv.ParseUint(input, 10, 32)
	if err != nil {
		l.Warnf("Failed to parse %s as uint32: %v", input, err)
		return defaultValue
	}
	return uint32(value)
}

func (tm *TunnelManager) forwardRemoteTunnelData(fromDevice protocol.DeviceID, data *protocol.TunnelData) {
	l.Debugln("Forwarding remote tunnel data, from device ID:", fromDevice, "command:", data.D.Command)
	switch data.D.Command {
	case bep.TunnelCommand_TUNNEL_COMMAND_OPEN:
		if data.D.RemoteServiceName == nil {
			l.Warnf("No remote service name specified")
			return
		}
		service := tm.getEnabledInboundServiceDeviceIdChecked(*data.D.RemoteServiceName, fromDevice)
		if service == nil {
			return
		}
		var TunnelDestinationAddress string
		if service.json.LocalDialAddress == "any" {
			if data.D.TunnelDestinationAddress == nil {
				l.Warnf("No tunnel destination specified")
				return
			}
			TunnelDestinationAddress = *data.D.TunnelDestinationAddress
		} else {
			TunnelDestinationAddress = service.json.LocalDialAddress
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
		go tm.handleLocalTunnelEndpoint(service.ctx, data.D.TunnelId, conn, fromDevice, *data.D.RemoteServiceName, TunnelDestinationAddress)

	case bep.TunnelCommand_TUNNEL_COMMAND_DATA:
		tm.endpointsMutex.Lock()
		tcpConn, ok := tm.localTunnelEndpoints[fromDevice][data.D.TunnelId]
		tm.endpointsMutex.Unlock()
		if ok {
			_, err := tcpConn.Write(data.D.Data)
			if err != nil {
				l.Warnf("Failed to forward tunnel data: %v", err)
			}
		} else {
			l.Infof("Data: No TCP connection found for device %v, TunnelID: %s", fromDevice, data.D.TunnelId)
		}
	case bep.TunnelCommand_TUNNEL_COMMAND_CLOSE:
		tm.endpointsMutex.Lock()
		tcpConn, ok := tm.localTunnelEndpoints[fromDevice][data.D.TunnelId]
		tm.endpointsMutex.Unlock()
		if ok {
			tcpConn.Close()
		} else {
			l.Infof("Close: No TCP connection found for device %v, TunnelID: %s", fromDevice, data.D.TunnelId)
		}
	case bep.TunnelCommand_TUNNEL_COMMAND_OFFER:
		func() {
			tm.endpointsMutex.Lock()
			defer tm.endpointsMutex.Unlock()
			tm.deviceConnections[fromDevice].serviceOfferings[*data.D.RemoteServiceName] = parseUint32Or(guf.DerefOrDefault(data.D.TunnelDestinationAddress), 0)
		}()
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

func getRandomFreePort() int {
	if a, err := net.ResolveTCPAddr("tcp", "localhost:0"); err == nil {
		var l *net.TCPListener
		if l, err = net.ListenTCP("tcp", a); err == nil {
			defer l.Close()
			return l.Addr().(*net.TCPAddr).Port
		}
	}
	panic("no free ports")
}

func (m *TunnelManager) AddOutboundTunnel(localListenAddress string, remoteDeviceID protocol.DeviceID, remoteServiceName string) error {

	err := func() error {
		m.configMutex.Lock()
		defer m.configMutex.Unlock()

		if localListenAddress == "127.0.0.1:0" {
			suggestedPort := getRandomFreePort()
			localListenAddress = fmt.Sprintf("127.0.0.1:%d", suggestedPort)
		}

		newConfig := &bep.TunnelOutbound{
			LocalListenAddress: localListenAddress,
			RemoteDeviceId:     remoteDeviceID.String(),
			RemoteServiceName:  remoteServiceName,
		}
		descriptor := getConfigDescriptorOutbound(newConfig)

		// Check if the tunnel already exists
		if _, exists := m.configOut[descriptor]; exists {
			return fmt.Errorf("tunnel with descriptor %s already exists", descriptor)
		}

		config := &bep.TunnelConfig{
			TunnelsIn:  m.getInboundTunnelsConfig(),
			TunnelsOut: m.getOutboundTunnelsConfig(),
		}

		config.TunnelsOut = append(config.TunnelsOut, newConfig)

		if err := saveTunnelConfig(m.configFile, config); err != nil {
			return fmt.Errorf("failed to save tunnel config: %w", err)
		}

		return nil
	}()

	if err != nil {
		return err
	}

	m.reloadConfig()
	return nil
}

// Status returns information about active tunnels
func (m *TunnelManager) Status() []map[string]interface{} {

	status := make([]map[string]interface{}, 0, len(m.configIn)+len(m.configOut))
	offerings := make(map[string]map[string]map[string]interface{})

	func() {
		m.endpointsMutex.Lock()
		defer m.endpointsMutex.Unlock()

		for deviceID, connection := range m.deviceConnections {
			if offerings[deviceID.String()] == nil {
				offerings[deviceID.String()] = make(map[string]map[string]interface{})
			}
			for serviceName, suggestedPort := range connection.serviceOfferings {
				info := map[string]interface{}{
					"localListenAddress": "127.0.0.1:" + strconv.Itoa(int(suggestedPort)),
					"remoteDeviceID":     deviceID.String(),
					"serviceID":          serviceName,
					"offered":            true,
					"type":               "outbound",
					"uiID": getUiTunnelDescriptorOutbound(&bep.TunnelOutbound{
						LocalListenAddress: "127.0.0.1:" + strconv.Itoa(int(suggestedPort)),
						RemoteDeviceId:     deviceID.String(),
						RemoteServiceName:  serviceName,
						RemoteAddress:      nil,
					}),
				}
				offerings[deviceID.String()][serviceName] = info
			}
		}
	}()

	func() {
		m.configMutex.Lock()
		defer m.configMutex.Unlock()

		for descriptor, tunnel := range m.configIn {
			info := map[string]interface{}{
				"id":                     descriptor,
				"serviceID":              tunnel.json.LocalServiceName,
				"allowedRemoteDeviceIDs": tunnel.json.AllowedRemoteDeviceIds,
				"localDialAddress":       tunnel.json.LocalDialAddress,
				"active":                 guf.DerefOr(tunnel.json.Enabled, true),
				"type":                   "inbound",
				"uiID":                   getUiTunnelDescriptorInbound(tunnel.json),
			}
			status = append(status, info)
		}
		for descriptor, tunnel := range m.configOut {

			// remove offering when already used
			delete(offerings[tunnel.json.RemoteDeviceId], tunnel.json.RemoteServiceName)

			info := map[string]interface{}{
				"id":                 descriptor,
				"localListenAddress": tunnel.json.LocalListenAddress,
				"remoteDeviceID":     tunnel.json.RemoteDeviceId,
				"serviceID":          tunnel.json.RemoteServiceName,
				"remoteAddress":      tunnel.json.RemoteAddress,
				"active":             guf.DerefOr(tunnel.json.Enabled, true),
				"type":               "outbound",
				"uiID":               getUiTunnelDescriptorOutbound(tunnel.json),
			}
			status = append(status, info)
		}
	}()

	// add remaining offerings:
	for _, services := range offerings {
		for _, info := range services {
			status = append(status, info)
		}
	}

	// sort by uiID
	slices.SortFunc(status, func(a map[string]interface{}, b map[string]interface{}) int {
		aID := a["uiID"].(string)
		bID := b["uiID"].(string)
		return strings.Compare(aID, bID)
	})

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

func saveTunnelConfig(path string, config *bep.TunnelConfig) error {
	l.Debugln("Saving tunnel config to file:", path)

	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Create a temporary file in the same directory
	tmpFile, err := os.CreateTemp(dir, "tunnels.*.json.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Clean up in case of failure
	success := false
	defer func() {
		if !success {
			os.Remove(tmpPath)
		}
	}()

	// Write to the temporary file
	encoder := json.NewEncoder(tmpFile)
	encoder.SetIndent("", "  ") // Pretty print with 2-space indentation
	if err := encoder.Encode(config); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to encode config: %w", err)
	}

	// Close the file before renaming
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temporary file: %w", err)
	}

	// Atomic rename to ensure the config file is not corrupted if the process is interrupted
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to replace config file: %w", err)
	}

	success = true
	l.Debugln("Saved tunnel config:", config)
	return nil
}

// setter for TunnelConfig.TunnelsIn/Out.Enabled (by id "inbound/outbound-idx") which also saves the config to file
func (tm *TunnelManager) ModifyTunnel(id string, action string, params map[string]string) error {
	if err := tm.modifyAndSaveConfig(id, action, params); err != nil {
		return err
	}

	return tm.reloadConfig()
}

func (tm *TunnelManager) modifyAndSaveConfig(id string, action string, params map[string]string) error {
	tm.configMutex.Lock()
	defer tm.configMutex.Unlock()

	if action == "enable" || action == "disable" {
		enabled := action == "enable"
		// Check if the ID corresponds to an inbound tunnel
		if tunnel, exists := tm.configIn[id]; exists {
			tunnel.json.Enabled = &enabled
			return tm.saveFullConfig_no_lock()
		}

		// Check if the ID corresponds to an outbound tunnel
		if tunnel, exists := tm.configOut[id]; exists {
			tunnel.json.Enabled = &enabled
			return tm.saveFullConfig_no_lock()
		}
	} else if action == "delete" {
		// Check if the ID corresponds to an inbound tunnel
		if tunnel, exists := tm.configIn[id]; exists {
			tunnel.cancel()
			delete(tm.configIn, id)
			return tm.saveFullConfig_no_lock()
		}
		// Check if the ID corresponds to an outbound tunnel
		if tunnel, exists := tm.configOut[id]; exists {
			tunnel.cancel()
			delete(tm.configOut, id)
			return tm.saveFullConfig_no_lock()
		}
	} else if action == "add-allowed-device" {
		// Check if the ID corresponds to an inbound tunnel
		if tunnel, exists := tm.configIn[id]; exists {
			// Add the allowed device ID to the tunnel
			newAllowedDeviceID := params["deviceID"]
			if index := slices.IndexFunc(tunnel.json.AllowedRemoteDeviceIds,
				func(device string) bool { return device == newAllowedDeviceID }); index < 0 {
				tunnel.json.AllowedRemoteDeviceIds = append(tunnel.json.AllowedRemoteDeviceIds, newAllowedDeviceID)
				return tm.saveFullConfig_no_lock()
			}
			return fmt.Errorf("allowed device ID %s already exists in tunnel %s", newAllowedDeviceID, id)
		}
		return fmt.Errorf("inbound tunnel not found: %s", id)
	} else if action == "remove-allowed-device" {
		// Check if the ID corresponds to an inbound tunnel
		if tunnel, exists := tm.configIn[id]; exists {
			// Remove the allowed device ID from the tunnel
			disallowedDeviceID := params["deviceID"]
			if index := slices.IndexFunc(tunnel.json.AllowedRemoteDeviceIds,
				func(device string) bool { return device == disallowedDeviceID }); index >= 0 {
				tunnel.json.AllowedRemoteDeviceIds = slices.Delete(tunnel.json.AllowedRemoteDeviceIds, index, index+1)
				return tm.saveFullConfig_no_lock()
			}
			return fmt.Errorf("disallowed device ID %s not found in tunnel %s", disallowedDeviceID, id)
		}
		return fmt.Errorf("inbound tunnel not found: %s", id)
	} else {
		return fmt.Errorf("invalid action: %s", action)
	}

	// If the ID is not found, return an error
	return fmt.Errorf("tunnel with ID %s not found", id)
}

func (tm *TunnelManager) reloadConfig() error {
	config, err := loadTunnelConfig(tm.configFile)
	if err != nil {
		return fmt.Errorf("failed to reload tunnel config: %w", err)
	}

	tm.updateInConfig(config.TunnelsIn)
	tm.updateOutConfig(config.TunnelsOut)

	return nil
}

// Helper method to retrieve all inbound tunnels as a slice
func (tm *TunnelManager) getInboundTunnelsConfig() []*bep.TunnelInbound {
	tunnels := make([]*bep.TunnelInbound, 0, len(tm.configIn))
	for _, tunnel := range tm.configIn {
		tunnels = append(tunnels, tunnel.json)
	}
	return tunnels
}

// Helper method to retrieve all outbound tunnels as a slice
func (tm *TunnelManager) getOutboundTunnelsConfig() []*bep.TunnelOutbound {
	tunnels := make([]*bep.TunnelOutbound, 0, len(tm.configOut))
	for _, tunnel := range tm.configOut {
		tunnels = append(tunnels, tunnel.json)
	}
	return tunnels
}

// SaveFullConfig saves the current configuration (both inbound and outbound tunnels) to the config file.
func (tm *TunnelManager) saveFullConfig_no_lock() error {

	config := &bep.TunnelConfig{
		TunnelsIn:  tm.getInboundTunnelsConfig(),
		TunnelsOut: tm.getOutboundTunnelsConfig(),
	}

	return saveTunnelConfig(tm.configFile, config)
}

func (tm *TunnelManager) ReloadConfig() error {
	config, err := loadTunnelConfig(tm.configFile)
	if err != nil {
		return fmt.Errorf("failed to reload tunnel config: %w", err)
	}

	tm.updateInConfig(config.TunnelsIn)
	tm.updateOutConfig(config.TunnelsOut)

	return nil
}
