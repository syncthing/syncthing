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
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"

	"github.com/syncthing/syncthing/lib/dialer"
	"github.com/syncthing/syncthing/lib/nat"
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
		// Interface flags seem to always be 0 on Windows
		if runtime.GOOS != "windows" && (intf.Flags&net.FlagUp == 0 || intf.Flags&net.FlagMulticast == 0) {
			continue
		}

		for _, deviceType := range []string{"urn:schemas-upnp-org:device:InternetGatewayDevice:1", "urn:schemas-upnp-org:device:InternetGatewayDevice:2"} {
			wg.Add(1)
			go func(intf net.Interface, deviceType string) {
				discover(ctx, &intf, deviceType, timeout, resultChan)
				wg.Done()
			}(intf, deviceType)
		}
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
func discover(ctx context.Context, intf *net.Interface, deviceType string, timeout time.Duration, results chan<- nat.Device) {
	ssdp := &net.UDPAddr{IP: []byte{239, 255, 255, 250}, Port: 1900}

	tpl := `M-SEARCH * HTTP/1.1
HOST: 239.255.255.250:1900
ST: %s
MAN: "ssdp:discover"
MX: %d
USER-AGENT: syncthing/1.0

`
	searchStr := fmt.Sprintf(tpl, deviceType, timeout/time.Second)

	search := []byte(strings.ReplaceAll(searchStr, "\n", "\r\n") + "\r\n")

	l.Debugln("Starting discovery of device type", deviceType, "on", intf.Name)

	socket, err := net.ListenMulticastUDP("udp4", intf, &net.UDPAddr{IP: ssdp.IP})
	if err != nil {
		l.Debugln("UPnP discovery: listening to udp multicast:", err)
		return
	}
	defer socket.Close() // Make sure our socket gets closed

	l.Debugln("Sending search request for device type", deviceType, "on", intf.Name)

	_, err = socket.WriteTo(search, ssdp)
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

		n, _, err := socket.ReadFrom(resp)
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

		igds, err := parseResponse(ctx, deviceType, resp[:n])
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

func parseResponse(ctx context.Context, deviceType string, resp []byte) ([]IGDService, error) {
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
	}

	deviceUSN := response.Header.Get("USN")
	if deviceUSN == "" {
		return nil, errors.New("invalid IGD response: USN not specified")
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

	// Figure out our IP number, on the network used to reach the IGD.
	// We do this in a fairly roundabout way by connecting to the IGD and
	// checking the address of the local end of the socket. I'm open to
	// suggestions on a better way to do this...
	localIPAddress, err := localIP(ctx, deviceDescriptionURL)
	if err != nil {
		return nil, err
	}

	services, err := getServiceDescriptions(deviceUUID, localIPAddress, deviceDescriptionLocation, upnpRoot.Device)
	if err != nil {
		return nil, err
	}

	return services, nil
}

func localIP(ctx context.Context, url *url.URL) (net.IP, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	conn, err := dialer.DialContext(timeoutCtx, "tcp", url.Host)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	localIPAddress, _, err := net.SplitHostPort(conn.LocalAddr().String())
	if err != nil {
		return nil, err
	}

	return net.ParseIP(localIPAddress), nil
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

func getServiceDescriptions(deviceUUID string, localIPAddress net.IP, rootURL string, device upnpDevice) ([]IGDService, error) {
	var result []IGDService

	if device.DeviceType == "urn:schemas-upnp-org:device:InternetGatewayDevice:1" {
		descriptions := getIGDServices(deviceUUID, localIPAddress, rootURL, device,
			"urn:schemas-upnp-org:device:WANDevice:1",
			"urn:schemas-upnp-org:device:WANConnectionDevice:1",
			[]string{"urn:schemas-upnp-org:service:WANIPConnection:1", "urn:schemas-upnp-org:service:WANPPPConnection:1"})

		result = append(result, descriptions...)
	} else if device.DeviceType == "urn:schemas-upnp-org:device:InternetGatewayDevice:2" {
		descriptions := getIGDServices(deviceUUID, localIPAddress, rootURL, device,
			"urn:schemas-upnp-org:device:WANDevice:2",
			"urn:schemas-upnp-org:device:WANConnectionDevice:2",
			[]string{"urn:schemas-upnp-org:service:WANIPConnection:2", "urn:schemas-upnp-org:service:WANPPPConnection:2"})

		result = append(result, descriptions...)
	} else {
		return result, errors.New("[" + rootURL + "] Malformed root device description: not an InternetGatewayDevice.")
	}

	if len(result) < 1 {
		return result, errors.New("[" + rootURL + "] Malformed device description: no compatible service descriptions found.")
	}
	return result, nil
}

func getIGDServices(deviceUUID string, localIPAddress net.IP, rootURL string, device upnpDevice, wanDeviceURN string, wanConnectionURN string, URNs []string) []IGDService {
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

				l.Debugln(rootURL, "- no services of type", URN, " found on connection.")

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
							LocalIP:   localIPAddress,
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
	tpl := `<?xml version="1.0" ?>
	<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
	<s:Body>%s</s:Body>
	</s:Envelope>
`
	var resp []byte

	body := fmt.Sprintf(tpl, message)

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

	r, err := http.DefaultClient.Do(req)
	if err != nil {
		l.Debugln("SOAP do:", err)
		return resp, err
	}

	resp, _ = io.ReadAll(r.Body)
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
