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
	"time"

	"github.com/ek220/guf"
	"github.com/hitoshi44/go-uid64"
	"github.com/syncthing/syncthing/internal/gen/bep"
	"github.com/syncthing/syncthing/lib/logger"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/utils"
)

var tl = logger.DefaultLogger.NewFacility("tunnels", "the tunnel manager stuff")

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

type tm_config struct {
	configIn       map[string]*tunnelInConfig
	configOut      map[string]*tunnelOutConfig
	serviceRunning bool
}

type TunnelManager struct {
	config                *utils.Protected[*tm_config] // TunnelManager config
	endpointsMutex        sync.Mutex                   // Mutex for localTunnelEndpoints
	deviceConnectionMutex sync.Mutex                   // Mutex for deviceConnections
	configFile            string
	idGenerator           *uid64.Generator
	localTunnelEndpoints  map[protocol.DeviceID]map[uint64]io.ReadWriteCloser
	deviceConnections     map[protocol.DeviceID]*DeviceConnection
}

func (m *TunnelManager) TunnelStatus() []map[string]interface{} {
	return m.Status()
}

func (m *TunnelManager) AddTunnelOutbound(localListenAddress string, remoteDeviceID protocol.DeviceID, remoteServiceName string) error {
	return m.AddOutboundTunnel(localListenAddress, remoteDeviceID, remoteServiceName)
}

func (m *TunnelManager) TrySendTunnelData(deviceID protocol.DeviceID, data *protocol.TunnelData) error {
	m.deviceConnectionMutex.Lock()
	defer m.deviceConnectionMutex.Unlock()
	if conn, ok := m.deviceConnections[deviceID]; ok {
		select {
		case conn.tunnelOut <- data:
			return nil
		default:
			return fmt.Errorf("failed to send tunnel data to device %v", deviceID)
		}
	} else {
		return fmt.Errorf("device %v not found in TunnelManager", deviceID)
	}
}

func NewTunnelManager(configFile string) *TunnelManager {
	// Replace the filename with "tunnels.json"
	configFile = fmt.Sprintf("%s/tunnels.json", filepath.Dir(configFile))
	tl.Debugln("TunnelManager created with config file:", configFile)
	config, err := loadTunnelConfig(configFile)
	if err != nil {
		tl.Infoln("failed to load tunnel config:", err)
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
		configFile: configFile,
		config: utils.NewProtected(&tm_config{
			configIn:       make(map[string]*tunnelInConfig),
			configOut:      make(map[string]*tunnelOutConfig),
			serviceRunning: false,
		}),
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
	tm.config.DoProtected(func(config *tm_config) {
		// Generate a new map of inbound tunnels
		newConfigIn := make(map[string]*tunnelInConfig)
		for _, newTun := range newInTunnels {
			descriptor := getConfigDescriptorInbound(newTun, false)
			if existingTun, exists := config.configIn[descriptor]; exists {
				// Reuse existing context and cancel function
				existingTun.json = newTun // update e.g. suggested port
				// Update allowed devices
				allowedClients := make(map[string]tunnelBaseConfig)
				for _, deviceIDStr := range newTun.AllowedRemoteDeviceIds {
					deviceID, err := protocol.DeviceIDFromString(deviceIDStr)
					if err != nil {
						tl.Warnf("failed to parse device ID: %v", err)
						continue
					}
					if _, exists := existingTun.allowedClients[deviceIDStr]; !exists {
						ctx, cancel := context.WithCancel(existingTun.ctx)
						allowedClients[deviceIDStr] = tunnelBaseConfig{
							descriptor: descriptor,
							ctx:        ctx,
							cancel:     cancel,
						}
						_ = tm.TrySendTunnelData(deviceID, tm.generateOfferCommand(newTun))
					} else {
						allowedClients[deviceIDStr] = existingTun.allowedClients[deviceIDStr]
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
		for descriptor, existing := range config.configIn {
			if _, exists := newConfigIn[descriptor]; !exists {
				existing.cancel()
			}
		}

		// Replace the old configuration with the new one
		config.configIn = newConfigIn
	})
}

func (tm *TunnelManager) updateOutConfig(newOutTunnels []*bep.TunnelOutbound) {
	tm.config.DoProtected(func(config *tm_config) {

		// Generate a new map of outbound tunnels
		newConfigOut := make(map[string]*tunnelOutConfig)
		for _, tunnel := range newOutTunnels {
			descriptor := getConfigDescriptorOutbound(tunnel)
			if existing, exists := config.configOut[descriptor]; exists {
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
				if config.serviceRunning {
					tm.serveOutboundTunnel(newConfigOut[descriptor])
				}
			}
		}

		// Cancel and remove tunnels that are no longer in the new configuration
		for descriptor, existing := range config.configOut {
			if _, exists := newConfigOut[descriptor]; !exists {
				existing.cancel()
			}
		}

		// Replace the old configuration with the new one
		config.configOut = newConfigOut
	})
}

func (tm *TunnelManager) getInboundService(name string) *tunnelInConfig {
	return utils.DoProtected(tm.config, func(config *tm_config) *tunnelInConfig {
		for _, service := range config.configIn {
			if service.json.LocalServiceName == name {
				// Explicitly copy the struct (as we leave the mutex protected scope)
				copied := *service
				return &copied
			}
		}
		return nil
	})
}

func (tm *TunnelManager) getEnabledInboundServiceDeviceIdChecked(name string, byDeviceID protocol.DeviceID) *tunnelInConfig {
	service := tm.getInboundService(name)
	if service == nil {
		return nil
	}
	if !guf.DerefOr(service.json.Enabled, true) {
		tl.Warnf("Device %v tries to access disabled service %s", byDeviceID, name)
		return nil
	}
	for _, device := range service.json.AllowedRemoteDeviceIds {
		deviceID, err := protocol.DeviceIDFromString(device)
		if err != nil {
			tl.Warnln("failed to parse device ID:", err)
			continue
		}
		if byDeviceID == deviceID {
			return service
		}
	}

	tl.Warnf("Device %v is not allowed to access service %s", byDeviceID, name)
	return nil
}

func (tm *TunnelManager) serveOutboundTunnel(tunnel *tunnelOutConfig) {
	if !guf.DerefOr(tunnel.json.Enabled, true) {
		tl.Debugln("Tunnel is disabled, skipping:", tunnel)
		return
	}

	tl.Debugln("Starting listener for tunnel, device:", tunnel)
	device, err := protocol.DeviceIDFromString(tunnel.json.RemoteDeviceId)
	if err != nil {
		tl.Warnln("failed to parse device ID:", err)
	}
	go tm.ServeListener(tunnel.ctx, tunnel.json.LocalListenAddress,
		device, tunnel.json.RemoteServiceName, tunnel.json.RemoteAddress)
}

func (tm *TunnelManager) Serve(ctx context.Context) error {
	tl.Debugln("TunnelManager Serve started")

	tm.config.DoProtected(func(config *tm_config) {
		config.serviceRunning = true
		for _, tunnel := range config.configOut {
			tm.serveOutboundTunnel(tunnel)
		}
	})

	<-ctx.Done()
	tl.Debugln("TunnelManager Serve stopping")

	// Cancel all active tunnels
	tm.config.DoProtected(func(config *tm_config) {
		config.serviceRunning = false
		for _, tunnel := range config.configIn {
			tunnel.cancel()
		}
		for _, tunnel := range config.configOut {
			tunnel.cancel()
		}
	})

	tl.Debugln("TunnelManager Serve stopped")
	return nil
}

func (tm *TunnelManager) ServeListener(
	ctx context.Context,
	listenAddress string,
	destinationDevice protocol.DeviceID,
	destinationServiceName string,
	destinationAddress *string,
) error {
	tl.Infoln("ServeListener started for address:", listenAddress, "destination device:", destinationDevice, "destination address:", destinationAddress)
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
		tl.Debugln("TunnelManager: Accepted connection from:", conn.RemoteAddr())

		go tm.handleAcceptedListenConnection(ctx, conn, destinationDevice, destinationServiceName, destinationAddress)
	}
}

func (tm *TunnelManager) handleAcceptedListenConnection(
	ctx context.Context,
	conn net.Conn,
	destinationDevice protocol.DeviceID,
	destinationServiceName string,
	destinationAddress *string,
) {
	defer conn.Close()

	tunnelID := tm.generateTunnelID()
	tl.Debugln("Accepted connection, tunnel ID:", tunnelID)
	tm.registerLocalTunnelEndpoint(destinationDevice, tunnelID, conn)

	// send open command to the destination device
	maxRetries := 5
	for try := 0; (try < maxRetries) && ctx.Err() == nil; try++ {
		err := tm.TrySendTunnelData(destinationDevice, &protocol.TunnelData{
			D: &bep.TunnelData{
				TunnelId:                 tunnelID,
				Command:                  bep.TunnelCommand_TUNNEL_COMMAND_OPEN,
				RemoteServiceName:        &destinationServiceName,
				TunnelDestinationAddress: destinationAddress,
			},
		})

		if err == nil {
			break
		} else {
			// sleep and retry - device might not yet be connected
			retry := func() bool {
				timer := time.NewTimer(time.Second)
				defer timer.Stop()

				tl.Warnf("Failed to send tunnel data to device %v: %v", destinationDevice, err)
				select {
				case <-ctx.Done():
					tl.Debugln("Context done, stopping listener")
					return false
				case <-timer.C:
					tl.Debugln("Retrying to send tunnel data to device", destinationDevice)
					return true
				}
			}()

			if !retry {
				tl.Debugln("Stopping listener due to context done")
				conn.Close()
				return
			}
		}
	}

	var optionalDestinationAddress string
	if destinationAddress == nil {
		optionalDestinationAddress = "by-remote"
	} else {
		optionalDestinationAddress = *destinationAddress
	}
	tm.handleLocalTunnelEndpoint(ctx, tunnelID, conn, destinationDevice, destinationServiceName, optionalDestinationAddress)
}

func (tm *TunnelManager) handleLocalTunnelEndpoint(ctx context.Context, tunnelID uint64, conn io.ReadWriteCloser, destinationDevice protocol.DeviceID, destinationServiceName string, destinationAddress string) {
	tl.Infoln("TunnelManager: Handling local tunnel endpoint, tunnel ID:", tunnelID,
		"destination device:", destinationDevice,
		"destination service name:", destinationServiceName,
		"destination address:", destinationAddress)

	defer tm.deregisterLocalTunnelEndpoint(destinationDevice, tunnelID)
	defer func() {
		// send close command to the destination device
		_ = tm.TrySendTunnelData(destinationDevice, &protocol.TunnelData{
			D: &bep.TunnelData{
				TunnelId: tunnelID,
				Command:  bep.TunnelCommand_TUNNEL_COMMAND_CLOSE,
			},
		})
		tl.Infoln("Closed local tunnel endpoint, tunnel ID:", tunnelID)
	}()

	stop := context.AfterFunc(ctx, func() {
		tl.Debugln("Stopping local tunnel endpoint, tunnel ID:", tunnelID)
		conn.Close()
	})

	defer func() {
		stop()
	}()

	destinationDeviceTunnel := func() chan<- *protocol.TunnelData {
		tm.deviceConnectionMutex.Lock()
		defer tm.deviceConnectionMutex.Unlock()
		if conn, ok := tm.deviceConnections[destinationDevice]; ok {
			return conn.tunnelOut
		}
		return nil
	}()

	// Example: Forward data to the destination address
	// This is a placeholder implementation
	for {
		select {
		case <-ctx.Done():
			tl.Debugln("Context done for tunnel ID:", tunnelID)
			return
		default:
			// Read data from the connection and forward it
			buffer := make([]byte, 1024*4)
			n, err := conn.Read(buffer)
			if err != nil {
				tl.Debugf("Error reading from connection: %v", err)
				return
			}
			// Forward data to the destination
			// This is a placeholder implementation
			tl.Debugf("Forwarding data to device %v, %s (%d tunnel id): len: %d\n", destinationDevice, destinationAddress, tunnelID, n)

			// Send the data to the destination device
			destinationDeviceTunnel <- &protocol.TunnelData{
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
	tl.Debugln("Registering local tunnel endpoint, device ID:", deviceID, "tunnel ID:", tunnelID)
	tm.endpointsMutex.Lock()
	defer tm.endpointsMutex.Unlock()
	if tm.localTunnelEndpoints[deviceID] == nil {
		tm.localTunnelEndpoints[deviceID] = make(map[uint64]io.ReadWriteCloser)
	}
	tm.localTunnelEndpoints[deviceID][tunnelID] = conn
}

func (tm *TunnelManager) deregisterLocalTunnelEndpoint(deviceID protocol.DeviceID, tunnelID uint64) {
	tl.Debugln("Deregistering local tunnel endpoint, device ID:", deviceID, "tunnel ID:", tunnelID)
	tm.endpointsMutex.Lock()
	defer tm.endpointsMutex.Unlock()
	delete(tm.localTunnelEndpoints[deviceID], tunnelID)
	if len(tm.localTunnelEndpoints[deviceID]) == 0 {
		delete(tm.localTunnelEndpoints, deviceID) // Ensure map cleanup
	}
}

func (tm *TunnelManager) RegisterDeviceConnection(device protocol.DeviceID, tunnelIn <-chan *protocol.TunnelData, tunnelOut chan<- *protocol.TunnelData) {
	func() {
		tl.Debugln("Registering device connection, device ID:", device)
		tm.deviceConnectionMutex.Lock()
		defer tm.deviceConnectionMutex.Unlock()

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

	go tm.config.DoProtected(func(config *tm_config) {
		// send tunnel service offerings
		for _, inboundSerice := range config.configIn {
			if slices.Contains(inboundSerice.json.AllowedRemoteDeviceIds, device.String()) {
				tunnelOut <- tm.generateOfferCommand(inboundSerice.json)
			}
		}
	})
}

func (tm *TunnelManager) generateOfferCommand(json *bep.TunnelInbound) *protocol.TunnelData {
	suggestedPort := strconv.FormatUint(uint64(guf.DerefOr(json.SuggestedPort, 0)), 10)
	return &protocol.TunnelData{
		D: &bep.TunnelData{
			Command:                  bep.TunnelCommand_TUNNEL_COMMAND_OFFER,
			RemoteServiceName:        &json.LocalServiceName,
			TunnelDestinationAddress: &suggestedPort,
		},
	}
}

func (tm *TunnelManager) DeregisterDeviceConnection(device protocol.DeviceID) {
	tl.Debugln("Deregistering device connection, device ID:", device)
	tm.deviceConnectionMutex.Lock()
	defer tm.deviceConnectionMutex.Unlock()
	delete(tm.deviceConnections, device)
}

func parseUint32Or(input string, defaultValue uint32) uint32 {
	// Parse the input string as a uint32
	value, err := strconv.ParseUint(input, 10, 32)
	if err != nil {
		tl.Warnf("Failed to parse %s as uint32: %v", input, err)
		return defaultValue
	}
	return uint32(value)
}

func (tm *TunnelManager) forwardRemoteTunnelData(fromDevice protocol.DeviceID, data *protocol.TunnelData) {
	tl.Debugln("Forwarding remote tunnel data, from device ID:", fromDevice, "command:", data.D.Command)
	switch data.D.Command {
	case bep.TunnelCommand_TUNNEL_COMMAND_OPEN:
		if data.D.RemoteServiceName == nil {
			tl.Warnf("No remote service name specified")
			return
		}
		service := tm.getEnabledInboundServiceDeviceIdChecked(*data.D.RemoteServiceName, fromDevice)
		if service == nil {
			return
		}
		var TunnelDestinationAddress string
		if service.json.LocalDialAddress == "any" {
			if data.D.TunnelDestinationAddress == nil {
				tl.Warnf("No tunnel destination specified")
				return
			}
			TunnelDestinationAddress = *data.D.TunnelDestinationAddress
		} else {
			TunnelDestinationAddress = service.json.LocalDialAddress
		}

		addr, err := net.ResolveTCPAddr("tcp", TunnelDestinationAddress)
		if err != nil {
			tl.Warnf("Failed to resolve tunnel destination: %v", err)
			return
		}
		conn, err := net.DialTCP("tcp", nil, addr)
		if err != nil {
			tl.Warnf("Failed to dial tunnel destination: %v", err)
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
				tl.Warnf("Failed to forward tunnel data: %v", err)
			}
		} else {
			tl.Infof("Data: No TCP connection found for device %v, TunnelID: %d", fromDevice, data.D.TunnelId)
		}
	case bep.TunnelCommand_TUNNEL_COMMAND_CLOSE:
		tm.endpointsMutex.Lock()
		tcpConn, ok := tm.localTunnelEndpoints[fromDevice][data.D.TunnelId]
		tm.endpointsMutex.Unlock()
		if ok {
			tcpConn.Close()
		} else {
			tl.Infof("Close: No TCP connection found for device %v, TunnelID: %d", fromDevice, data.D.TunnelId)
		}
	case bep.TunnelCommand_TUNNEL_COMMAND_OFFER:
		func() {
			tm.deviceConnectionMutex.Lock()
			defer tm.deviceConnectionMutex.Unlock()
			if conn, ok := tm.deviceConnections[fromDevice]; ok {
				conn.serviceOfferings[*data.D.RemoteServiceName] = parseUint32Or(guf.DerefOrDefault(data.D.TunnelDestinationAddress), 0)
			}
		}()
	default: // unknown command
		tl.Warnf("Unknown tunnel command: %v", data.D.Command)
	}
}

func (tm *TunnelManager) generateTunnelID() uint64 {
	id, err := tm.idGenerator.Gen()
	if err != nil {
		panic(err)
	}
	tl.Debugln("Generated tunnel ID:", id)
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

	err := utils.DoProtected(m.config, func(config *tm_config) error {
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
		if _, exists := config.configOut[descriptor]; exists {
			return fmt.Errorf("tunnel with descriptor %s already exists", descriptor)
		}

		bepConfig := &bep.TunnelConfig{
			TunnelsIn:  config.getInboundTunnelsConfig(),
			TunnelsOut: config.getOutboundTunnelsConfig(),
		}

		bepConfig.TunnelsOut = append(bepConfig.TunnelsOut, newConfig)

		if err := saveTunnelConfig(m.configFile, bepConfig); err != nil {
			return fmt.Errorf("failed to save tunnel config: %w", err)
		}

		return nil
	})

	if err != nil {
		return err
	}

	m.reloadConfig()
	return nil
}

// Status returns information about active tunnels
func (m *TunnelManager) Status() []map[string]interface{} {

	offerings := make(map[string]map[string]map[string]interface{})

	func() {
		m.deviceConnectionMutex.Lock()
		defer m.deviceConnectionMutex.Unlock()

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

	status := utils.DoProtected(m.config, func(config *tm_config) []map[string]interface{} {
		status := make([]map[string]interface{}, 0, len(config.configIn)+len(config.configOut))

		for descriptor, tunnel := range config.configIn {
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
		for descriptor, tunnel := range config.configOut {

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

		return status
	})

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
	tl.Debugln("Loading tunnel config from file:", path)
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

	tl.Debugln("Loaded tunnel config:", &config)
	return &config, nil
}

func saveTunnelConfig(path string, config *bep.TunnelConfig) error {
	tl.Debugln("Saving tunnel config to file:", path)

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
	tl.Debugln("Saved tunnel config:", config)
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
	return utils.DoProtected(tm.config, func(config *tm_config) error {
		if action == "enable" || action == "disable" {
			enabled := action == "enable"
			// Check if the ID corresponds to an inbound tunnel
			if tunnel, exists := config.configIn[id]; exists {
				tunnel.json.Enabled = &enabled
				return config.saveFullConfig_no_lock(tm.configFile)
			}

			// Check if the ID corresponds to an outbound tunnel
			if tunnel, exists := config.configOut[id]; exists {
				tunnel.json.Enabled = &enabled
				return config.saveFullConfig_no_lock(tm.configFile)
			}
		} else if action == "delete" {
			// Check if the ID corresponds to an inbound tunnel
			if tunnel, exists := config.configIn[id]; exists {
				tunnel.cancel()
				delete(config.configIn, id)
				return config.saveFullConfig_no_lock(tm.configFile)
			}
			// Check if the ID corresponds to an outbound tunnel
			if tunnel, exists := config.configOut[id]; exists {
				tunnel.cancel()
				delete(config.configOut, id)
				return config.saveFullConfig_no_lock(tm.configFile)
			}
		} else if action == "add-allowed-device" {
			// Check if the ID corresponds to an inbound tunnel
			if tunnel, exists := config.configIn[id]; exists {
				// Add the allowed device ID to the tunnel
				newAllowedDeviceID := params["deviceID"]
				if index := slices.IndexFunc(tunnel.json.AllowedRemoteDeviceIds,
					func(device string) bool { return device == newAllowedDeviceID }); index < 0 {
					tunnel.json.AllowedRemoteDeviceIds = append(tunnel.json.AllowedRemoteDeviceIds, newAllowedDeviceID)
					return config.saveFullConfig_no_lock(tm.configFile)
				}
				return fmt.Errorf("allowed device ID %s already exists in tunnel %s", newAllowedDeviceID, id)
			}
			return fmt.Errorf("inbound tunnel not found: %s", id)
		} else if action == "remove-allowed-device" {
			// Check if the ID corresponds to an inbound tunnel
			if tunnel, exists := config.configIn[id]; exists {
				// Remove the allowed device ID from the tunnel
				disallowedDeviceID := params["deviceID"]
				if index := slices.IndexFunc(tunnel.json.AllowedRemoteDeviceIds,
					func(device string) bool { return device == disallowedDeviceID }); index >= 0 {
					tunnel.json.AllowedRemoteDeviceIds = slices.Delete(tunnel.json.AllowedRemoteDeviceIds, index, index+1)
					return config.saveFullConfig_no_lock(tm.configFile)
				}
				return fmt.Errorf("disallowed device ID %s not found in tunnel %s", disallowedDeviceID, id)
			}
			return fmt.Errorf("inbound tunnel not found: %s", id)
		} else {
			return fmt.Errorf("invalid action: %s", action)
		}

		// If the ID is not found, return an error
		return fmt.Errorf("tunnel with ID %s not found", id)
	})
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
func (tm *tm_config) getInboundTunnelsConfig() []*bep.TunnelInbound {
	tunnels := make([]*bep.TunnelInbound, 0, len(tm.configIn))
	for _, tunnel := range tm.configIn {
		tunnels = append(tunnels, tunnel.json)
	}
	return tunnels
}

// Helper method to retrieve all outbound tunnels as a slice
func (tm *tm_config) getOutboundTunnelsConfig() []*bep.TunnelOutbound {
	tunnels := make([]*bep.TunnelOutbound, 0, len(tm.configOut))
	for _, tunnel := range tm.configOut {
		tunnels = append(tunnels, tunnel.json)
	}
	return tunnels
}

// SaveFullConfig saves the current configuration (both inbound and outbound tunnels) to the config file.
func (tm *tm_config) saveFullConfig_no_lock(filename string) error {

	config := &bep.TunnelConfig{
		TunnelsIn:  tm.getInboundTunnelsConfig(),
		TunnelsOut: tm.getOutboundTunnelsConfig(),
	}

	return saveTunnelConfig(filename, config)
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
