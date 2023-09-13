// Copyright (C) 2014 The Syncthing Authors.
//
// Adapted from https://github.com/jackpal/Taipei-Torrent/blob/dd88a8bfac6431c01d959ce3c745e74b8a911793/IGD.go
// Copyright (c) 2010 Jack Palevich (https://github.com/jackpal/Taipei-Torrent/blob/dd88a8bfac6431c01d959ce3c745e74b8a911793/LICENSE)
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are
// met:
//
//    * Redistributions of source code must retain the above copyright
// notice, this list of conditions and the following disclaimer.
//    * Redistributions in binary form must reproduce the above
// copyright notice, this list of conditions and the following disclaimer
// in the documentation and/or other materials provided with the
// distribution.
//    * Neither the name of Google Inc. nor the names of its
// contributors may be used to endorse or promote products derived from
// this software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
// "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
// LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
// A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
// OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
// SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
// LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
// DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
// THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

// Package upnp implements UPnP InternetGatewayDevice discovery, querying, and port mapping.
package upnp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/dialer"
	"github.com/syncthing/syncthing/lib/nat"
	"github.com/syncthing/syncthing/lib/osutil"
)

func init() {
	nat.Register(Discover)
}

type upnpService struct {
	ID         string `xml:"serviceId"`
	Type       string `xml:"serviceType"`
	ControlURL string `xml:"controlURL"`
}

type upnpDevice struct {
	IsIPv6       bool
	DeviceType   string        `xml:"deviceType"`
	FriendlyName string        `xml:"friendlyName"`
	Devices      []upnpDevice  `xml:"deviceList>device"`
	Services     []upnpService `xml:"serviceList>service"`
}

type upnpRoot struct {
	Device upnpDevice `xml:"device"`
}

// UnsupportedDeviceTypeError for unsupported UPnP device types (i.e upnp:rootdevice)
type UnsupportedDeviceTypeError struct {
	deviceType string
}

func (e *UnsupportedDeviceTypeError) Error() string {
	return fmt.Sprintf("Unsupported UPnP device of type %s", e.deviceType)
}

const (
	urnIgdV1                    = "urn:schemas-upnp-org:device:InternetGatewayDevice:1"
	urnIgdV2                    = "urn:schemas-upnp-org:device:InternetGatewayDevice:2"
	urnWANDeviceV1              = "urn:schemas-upnp-org:device:WANDevice:1"
	urnWANDeviceV2              = "urn:schemas-upnp-org:device:WANDevice:2"
	urnWANConnectionDeviceV1    = "urn:schemas-upnp-org:device:WANConnectionDevice:1"
	urnWANConnectionDeviceV2    = "urn:schemas-upnp-org:device:WANConnectionDevice:2"
	urnWANIPConnectionV1        = "urn:schemas-upnp-org:service:WANIPConnection:1"
	urnWANIPConnectionV2        = "urn:schemas-upnp-org:service:WANIPConnection:2"
	urnWANIPv6FirewallControlV1 = "urn:schemas-upnp-org:service:WANIPv6FirewallControl:1"
	urnWANPPPConnectionV1       = "urn:schemas-upnp-org:service:WANPPPConnection:1"
	urnWANPPPConnectionV2       = "urn:schemas-upnp-org:service:WANPPPConnection:2"
)

// Discover discovers UPnP InternetGatewayDevices.
// The order in which the devices appear in the results list is not deterministic.
func Discover(ctx context.Context, _, timeout time.Duration) []nat.Device {
	var results []nat.Device

	interfaces, err := net.Interfaces()
	if err != nil {
		l.Infoln("Listing network interfaces:", err)
		return results
	}

	resultChan := make(chan nat.Device)

	wg := &sync.WaitGroup{}

	for _, intf := range interfaces {
		if intf.Flags&net.FlagRunning == 0 || intf.Flags&net.FlagMulticast == 0 {
			continue
		}

		wg.Add(1)
		// Discovery is done sequentially per interface because we discovered that
		// FritzBox routers return a broken result sometimes if the IPv4 and IPv6
		// request arrive at the same time.
		go func(iface net.Interface) {
			nonLLIPv6Found := false
			addrs, err := iface.Addrs()

			if err == nil {
				for _, addr := range addrs {
					ip, _, err := net.ParseCIDR(addr.String())
					// Use the same condition that igd_service.go uses so we only discover
					// IPv6 gateways if we have a "useful" IPv6 address.
					if err == nil && ip.IsGlobalUnicast() && ip.To4() == nil {
						nonLLIPv6Found = true
						break
					}
				}
			}

			// Discover IPv6 gateways on interface. Only discover IGDv2, since IGDv1
			// + IPv6 is not standardized and will lead to duplicates on routers.
			// Only do this when a non-link-local IPv6 is available. if we can't
			// enumerate the interface, the IPv6 code will not work anyway
			if nonLLIPv6Found {
				discover(ctx, &iface, urnIgdV2, timeout, resultChan, true)
			}

			// Discover IPv4 gateways on interface.
			for _, deviceType := range []string{urnIgdV2, urnIgdV1} {
				discover(ctx, &iface, deviceType, timeout, resultChan, false)
			}
			wg.Done()
		}(intf)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	seenResults := make(map[string]bool)
	for {
		select {
		case result, ok := <-resultChan:
			if !ok {
				return results
			}
			if seenResults[result.ID()] {
				l.Debugf("Skipping duplicate result %s", result.ID())
				continue
			}

			results = append(results, result)
			seenResults[result.ID()] = true

			l.Debugf("UPnP discovery result %s", result.ID())
		case <-ctx.Done():
			return nil
		}
	}
}

// Search for UPnP InternetGatewayDevices for <timeout> seconds.
// The order in which the devices appear in the result list is not deterministic
func discover(ctx context.Context, intf *net.Interface, deviceType string, timeout time.Duration, results chan<- nat.Device, ip6 bool) {
	var ssdp net.UDPAddr
	var template string
	if ip6 {
		ssdp = net.UDPAddr{IP: []byte{0xFF, 0x05, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0C}, Port: 1900}

		template = `M-SEARCH * HTTP/1.1
HOST: [FF05::C]:1900
ST: %s
MAN: "ssdp:discover"
MX: %d
USER-AGENT: syncthing/%s

`
	} else {
		ssdp = net.UDPAddr{IP: []byte{239, 255, 255, 250}, Port: 1900}

		template = `M-SEARCH * HTTP/1.1
HOST: 239.255.255.250:1900
ST: %s
MAN: "ssdp:discover"
MX: %d
USER-AGENT: syncthing/%s

`
	}

	searchStr := fmt.Sprintf(template, deviceType, timeout/time.Second, build.Version)

	search := []byte(strings.ReplaceAll(searchStr, "\n", "\r\n") + "\r\n")

	l.Debugln("Starting discovery of device type", deviceType, "on", intf.Name)

	proto := "udp4"
	if ip6 {
		proto = "udp6"
	}
	socket, err := net.ListenMulticastUDP(proto, intf, &net.UDPAddr{IP: ssdp.IP})
	if err != nil {
		l.Debugln("UPnP discovery: listening to udp multicast:", err)
		return
	}
	defer socket.Close() // Make sure our socket gets closed

	l.Debugln("Sending search request for device type", deviceType, "on", intf.Name)

	_, err = socket.WriteTo(search, &ssdp)
	if err != nil {
		if e, ok := err.(net.Error); !ok || !e.Timeout() {
			l.Debugln("UPnP discovery: sending search request:", err)
		}
		return
	}

	l.Debugln("Listening for UPnP response for device type", deviceType, "on", intf.Name)

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Listen for responses until a timeout is reached or the context is
	// cancelled
	resp := make([]byte, 65536)
loop:
	for {
		if err := socket.SetDeadline(time.Now().Add(250 * time.Millisecond)); err != nil {
			l.Infoln("UPnP socket:", err)
			break
		}

		n, udpAddr, err := socket.ReadFromUDP(resp)
		if err != nil {
			select {
			case <-ctx.Done():
				break loop
			default:
			}
			if e, ok := err.(net.Error); ok && e.Timeout() {
				continue // continue reading
			}
			l.Infoln("UPnP read:", err) //legitimate error, not a timeout.
			break
		}

		igds, err := parseResponse(ctx, deviceType, udpAddr, resp[:n], intf)
		if err != nil {
			switch err.(type) {
			case *UnsupportedDeviceTypeError:
				l.Debugln(err.Error())
			default:
				if !errors.Is(err, context.Canceled) {
					l.Infoln("UPnP parse:", err)
				}
			}
			continue
		}
		for _, igd := range igds {
			igd := igd // Copy before sending pointer to the channel.
			select {
			case results <- &igd:
			case <-ctx.Done():
				return
			}
		}
	}
	l.Debugln("Discovery for device type", deviceType, "on", intf.Name, "finished.")
}

func parseResponse(ctx context.Context, deviceType string, addr *net.UDPAddr, resp []byte, netInterface *net.Interface) ([]IGDService, error) {
	l.Debugln("Handling UPnP response:\n\n" + string(resp))

	reader := bufio.NewReader(bytes.NewBuffer(resp))
	request := &http.Request{}
	response, err := http.ReadResponse(reader, request)
	if err != nil {
		return nil, err
	}

	respondingDeviceType := response.Header.Get("St")
	if respondingDeviceType != deviceType {
		return nil, &UnsupportedDeviceTypeError{deviceType: respondingDeviceType}
	}

	deviceDescriptionLocation := response.Header.Get("Location")
	if deviceDescriptionLocation == "" {
		return nil, errors.New("invalid IGD response: no location specified")
	}

	deviceDescriptionURL, err := url.Parse(deviceDescriptionLocation)
	if err != nil {
		l.Infoln("Invalid IGD location: " + err.Error())
		return nil, err
	}

	if err != nil {
		l.Infoln("Invalid source IP for IGD: " + err.Error())
		return nil, err
	}

	deviceUSN := response.Header.Get("USN")
	if deviceUSN == "" {
		return nil, errors.New("invalid IGD response: USN not specified")
	}

	deviceIP := net.ParseIP(deviceDescriptionURL.Hostname())
	// If the hostname of the device parses as an IPv6 link-local address, we need to use the source IP address
	// of the response as the hostname instead of the one given, since only the former contains the zone index, while the URL returned from the gateway
	// cannot contain the zone index. (It can't know how interfaces are named/numbered on our machine)
	if deviceIP != nil && deviceIP.To4() == nil && deviceIP.IsLinkLocalUnicast() {
		ipAddr := net.IPAddr{
			IP:   addr.IP,
			Zone: addr.Zone,
		}

		deviceDescriptionPort := deviceDescriptionURL.Port()
		deviceDescriptionURL.Host = "[" + ipAddr.String() + "]"
		if deviceDescriptionPort != "" {
			deviceDescriptionURL.Host += ":" + deviceDescriptionPort
		}
		deviceDescriptionLocation = deviceDescriptionURL.String()
	}

	deviceUUID := strings.TrimPrefix(strings.Split(deviceUSN, "::")[0], "uuid:")
	response, err = http.Get(deviceDescriptionLocation)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode >= 400 {
		return nil, errors.New("bad status code:" + response.Status)
	}

	var upnpRoot upnpRoot
	err = xml.NewDecoder(response.Body).Decode(&upnpRoot)
	if err != nil {
		return nil, err
	}

	// Figure out our IPv4 address on the interface used to reach the IGD.
	localIPv4Address, err := localIPv4(netInterface)
	if err != nil {
		// On Android, we cannot enumerate IP addresses on interfaces directly. Therefore, we just try to connect to the IGD
		// and look at which source IP address was used. This is not ideal, but it's the best we can do.
		// Maybe we are on an IPv6-only network though, so don't error out in case pinholing is available.
		localIPv4Address, err = localIPv4Fallback(ctx, deviceDescriptionURL)
		if err != nil {
			l.Infoln("Unable to determine local IPv4 address for IGD: " + err.Error())
		}
	}

	// This differs from IGDService.IsIPv6GatewayDevice(). While that method determines whether an already
	// completely discovered device uses the IPv6 firewall protocol, this just checks if the gateway's is IPv6.
	// Currently we only want to discover IPv6 UPnP endpoints on IPv6 gateways and vice versa, which is why this needs to be stored
	// but technically we could forgo this check and try WANIPv6FirewallControl via IPv4. This leads to errors though so we don't do it.
	upnpRoot.Device.IsIPv6 = addr.IP.To4() == nil
	services, err := getServiceDescriptions(deviceUUID, localIPv4Address, deviceDescriptionLocation, upnpRoot.Device, netInterface)
	if err != nil {
		return nil, err
	}

	return services, nil
}

func localIPv4(netInterface *net.Interface) (net.IP, error) {
	addrs, err := netInterface.Addrs()
	if err != nil {
		return nil, err
	}

	for _, addr := range addrs {
		ip, _, err := net.ParseCIDR(addr.String())
		if err != nil {
			continue
		}

		if ip.To4() != nil {
			return ip, nil
		}
	}

	return nil, errors.New("no IPv4 address found for interface " + netInterface.Name)
}

func localIPv4Fallback(ctx context.Context, url *url.URL) (net.IP, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	conn, err := dialer.DialContext(timeoutCtx, "udp4", url.Host)

	if err != nil {
		return nil, err
	}

	defer conn.Close()

	ip, err := osutil.IPFromAddr(conn.LocalAddr())
	if err != nil {
		return nil, err
	}
	if ip.To4() == nil {
		return nil, errors.New("tried to obtain IPv4 through fallback but got IPv6 address")
	}
	return ip, nil
}

func getChildDevices(d upnpDevice, deviceType string) []upnpDevice {
	var result []upnpDevice
	for _, dev := range d.Devices {
		if dev.DeviceType == deviceType {
			result = append(result, dev)
		}
	}
	return result
}

func getChildServices(d upnpDevice, serviceType string) []upnpService {
	var result []upnpService
	for _, service := range d.Services {
		if service.Type == serviceType {
			result = append(result, service)
		}
	}
	return result
}

func getServiceDescriptions(deviceUUID string, localIPAddress net.IP, rootURL string, device upnpDevice, netInterface *net.Interface) ([]IGDService, error) {
	var result []IGDService

	if device.IsIPv6 && device.DeviceType == urnIgdV1 {
		// IPv6 UPnP is only standardized for IGDv2. Furthermore, any WANIPConn services for IPv4 that
		// we may discover here are likely to be broken because many routers make the choice to not allow
		// port mappings for IPs differing from the source IP of the device making the request (which would be v6 here)
		return nil, nil
	} else if device.IsIPv6 && device.DeviceType == urnIgdV2 {
		descriptions := getIGDServices(deviceUUID, localIPAddress, rootURL, device,
			urnWANDeviceV2,
			urnWANConnectionDeviceV2,
			[]string{urnWANIPv6FirewallControlV1},
			netInterface)

		result = append(result, descriptions...)
	} else if device.DeviceType == urnIgdV1 {
		descriptions := getIGDServices(deviceUUID, localIPAddress, rootURL, device,
			urnWANDeviceV1,
			urnWANConnectionDeviceV1,
			[]string{urnWANIPConnectionV1, urnWANPPPConnectionV1},
			netInterface)

		result = append(result, descriptions...)
	} else if device.DeviceType == urnIgdV2 {
		descriptions := getIGDServices(deviceUUID, localIPAddress, rootURL, device,
			urnWANDeviceV2,
			urnWANConnectionDeviceV2,
			[]string{urnWANIPConnectionV2, urnWANPPPConnectionV2},
			netInterface)

		result = append(result, descriptions...)
	} else {
		return result, errors.New("[" + rootURL + "] Malformed root device description: not an InternetGatewayDevice.")
	}

	if len(result) < 1 {
		return result, errors.New("[" + rootURL + "] Malformed device description: no compatible service descriptions found.")
	}
	return result, nil
}

func getIGDServices(deviceUUID string, localIPAddress net.IP, rootURL string, device upnpDevice, wanDeviceURN string, wanConnectionURN string, URNs []string, netInterface *net.Interface) []IGDService {
	var result []IGDService

	devices := getChildDevices(device, wanDeviceURN)

	if len(devices) < 1 {
		l.Infoln(rootURL, "- malformed InternetGatewayDevice description: no WANDevices specified.")
		return result
	}

	for _, device := range devices {
		connections := getChildDevices(device, wanConnectionURN)

		if len(connections) < 1 {
			l.Infoln(rootURL, "- malformed ", wanDeviceURN, "description: no WANConnectionDevices specified.")
		}

		for _, connection := range connections {
			for _, URN := range URNs {
				services := getChildServices(connection, URN)

				if len(services) == 0 {
					l.Debugln(rootURL, "- no services of type", URN, " found on connection.")
				}

				for _, service := range services {
					if service.ControlURL == "" {
						l.Infoln(rootURL+"- malformed", service.Type, "description: no control URL.")
					} else {
						u, _ := url.Parse(rootURL)
						replaceRawPath(u, service.ControlURL)

						l.Debugln(rootURL, "- found", service.Type, "with URL", u)

						service := IGDService{
							UUID:      deviceUUID,
							Device:    device,
							ServiceID: service.ID,
							URL:       u.String(),
							URN:       service.Type,
							Interface: netInterface,
							LocalIPv4: localIPAddress,
						}

						result = append(result, service)
					}
				}
			}
		}
	}

	return result
}

func replaceRawPath(u *url.URL, rp string) {
	asURL, err := url.Parse(rp)
	if err != nil {
		return
	} else if asURL.IsAbs() {
		u.Path = asURL.Path
		u.RawQuery = asURL.RawQuery
	} else {
		var p, q string
		fs := strings.Split(rp, "?")
		p = fs[0]
		if len(fs) > 1 {
			q = fs[1]
		}

		if p[0] == '/' {
			u.Path = p
		} else {
			u.Path += p
		}
		u.RawQuery = q
	}
}

func soapRequest(ctx context.Context, url, service, function, message string) ([]byte, error) {
	return soapRequestWithIP(ctx, url, service, function, message, nil)
}

func soapRequestWithIP(ctx context.Context, url, service, function, message string, localIP *net.TCPAddr) ([]byte, error) {
	const template = `<?xml version="1.0" ?>
	<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
	<s:Body>%s</s:Body>
	</s:Envelope>
`
	var resp []byte

	body := fmt.Sprintf(template, message)

	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(body))
	if err != nil {
		return resp, err
	}
	req.Close = true
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("User-Agent", "syncthing/1.0")
	req.Header["SOAPAction"] = []string{fmt.Sprintf(`"%s#%s"`, service, function)} // Enforce capitalization in header-entry for sensitive routers. See issue #1696
	req.Header.Set("Connection", "Close")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")

	l.Debugln("SOAP Request URL: " + url)
	l.Debugln("SOAP Action: " + req.Header.Get("SOAPAction"))
	l.Debugln("SOAP Request:\n\n" + body)

	dialer := net.Dialer{
		LocalAddr: localIP,
	}
	transport := &http.Transport{
		DialContext: dialer.DialContext,
	}
	httpClient := &http.Client{
		Transport: transport,
	}
	r, err := httpClient.Do(req)
	if err != nil {
		l.Debugln("SOAP do:", err)
		return resp, err
	}

	resp, err = io.ReadAll(r.Body)
	if err != nil {
		l.Debugf("Error reading SOAP response: %s, partial response (if present):\n\n%s", resp)
		return resp, err
	}

	l.Debugf("SOAP Response: %s\n\n%s\n\n", r.Status, resp)

	r.Body.Close()

	if r.StatusCode >= 400 {
		return resp, errors.New(function + ": " + r.Status)
	}

	return resp, nil
}

type soapGetExternalIPAddressResponseEnvelope struct {
	XMLName xml.Name
	Body    soapGetExternalIPAddressResponseBody `xml:"Body"`
}

type soapGetExternalIPAddressResponseBody struct {
	XMLName                      xml.Name
	GetExternalIPAddressResponse getExternalIPAddressResponse `xml:"GetExternalIPAddressResponse"`
}

type getExternalIPAddressResponse struct {
	NewExternalIPAddress string `xml:"NewExternalIPAddress"`
}

type soapErrorResponse struct {
	ErrorCode        int    `xml:"Body>Fault>detail>UPnPError>errorCode"`
	ErrorDescription string `xml:"Body>Fault>detail>UPnPError>errorDescription"`
}

type soapAddPinholeResponse struct {
	UniqueID int `xml:"Body>AddPinholeResponse>UniqueID"`
}
