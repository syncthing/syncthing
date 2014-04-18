// Package upnp implements UPnP Internet Gateway upnpDevice port mappings
package upnp

// Adapted from https://github.com/jackpal/Taipei-Torrent/blob/dd88a8bfac6431c01d959ce3c745e74b8a911793/IGD.go
// Copyright (c) 2010 Jack Palevich (https://github.com/jackpal/Taipei-Torrent/blob/dd88a8bfac6431c01d959ce3c745e74b8a911793/LICENSE)
// Copyright (c) 2014 Jakob Borg

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type IGD struct {
	serviceURL string
	ourIP      string
}

type Protocol string

const (
	TCP Protocol = "TCP"
	UDP          = "UDP"
)

type upnpService struct {
	ServiceType string `xml:"serviceType"`
	ControlURL  string `xml:"controlURL"`
}

type upnpDevice struct {
	DeviceType string        `xml:"deviceType"`
	Devices    []upnpDevice  `xml:"deviceList>device"`
	Services   []upnpService `xml:"serviceList>service"`
}

type upnpRoot struct {
	Device upnpDevice `xml:"device"`
}

func Discover() (*IGD, error) {
	ssdp := &net.UDPAddr{IP: []byte{239, 255, 255, 250}, Port: 1900}

	socket, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return nil, err
	}
	defer socket.Close()

	err = socket.SetDeadline(time.Now().Add(3 * time.Second))
	if err != nil {
		return nil, err
	}

	search := []byte(`
M-SEARCH * HTTP/1.1
Host: 239.255.255.250:1900
St: urn:schemas-upnp-org:device:InternetGatewayDevice:1
Man: "ssdp:discover"
Mx: 3

`)

	_, err = socket.WriteTo(search, ssdp)
	if err != nil {
		return nil, err
	}

	resp := make([]byte, 1500)
	n, _, err := socket.ReadFrom(resp)
	if err != nil {
		return nil, err
	}

	if debug {
		dlog.Println(string(resp[:n]))
	}

	reader := bufio.NewReader(bytes.NewBuffer(resp[:n]))
	request := &http.Request{}
	response, err := http.ReadResponse(reader, request)

	if response.Header.Get("St") != "urn:schemas-upnp-org:device:InternetGatewayDevice:1" {
		return nil, errors.New("no igd")
	}

	locURL := response.Header.Get("Location")
	if locURL == "" {
		return nil, errors.New("no location")
	}

	serviceURL, err := getServiceURL(locURL)
	if err != nil {
		return nil, err
	}

	// Figure out our IP number, on the network used to reach the IGD. We
	// do this in a fairly roundabout way by connecting to the IGD and
	// checking the address of the local end of the socket. I'm open to
	// suggestions on a better way to do this...
	ourIP, err := localIP(locURL)
	if err != nil {
		return nil, err
	}

	igd := &IGD{
		serviceURL: serviceURL,
		ourIP:      ourIP,
	}
	return igd, nil
}

func localIP(tgt string) (string, error) {
	url, err := url.Parse(tgt)
	if err != nil {
		return "", err
	}

	conn, err := net.Dial("tcp", url.Host)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	ourIP, _, err := net.SplitHostPort(conn.LocalAddr().String())
	if err != nil {
		return "", err
	}

	return ourIP, nil
}

func getChildDevice(d upnpDevice, deviceType string) (upnpDevice, bool) {
	for _, dev := range d.Devices {
		if dev.DeviceType == deviceType {
			return dev, true
		}
	}
	return upnpDevice{}, false
}

func getChildService(d upnpDevice, serviceType string) (upnpService, bool) {
	for _, svc := range d.Services {
		if svc.ServiceType == serviceType {
			return svc, true
		}
	}
	return upnpService{}, false
}

func getServiceURL(rootURL string) (string, error) {
	r, err := http.Get(rootURL)
	if err != nil {
		return "", err
	}
	defer r.Body.Close()
	if r.StatusCode >= 400 {
		return "", errors.New(r.Status)
	}

	var upnpRoot upnpRoot
	err = xml.NewDecoder(r.Body).Decode(&upnpRoot)
	if err != nil {
		return "", err
	}

	dev := upnpRoot.Device
	if dev.DeviceType != "urn:schemas-upnp-org:device:InternetGatewayDevice:1" {
		return "", errors.New("No InternetGatewayDevice")
	}

	dev, ok := getChildDevice(dev, "urn:schemas-upnp-org:device:WANDevice:1")
	if !ok {
		return "", errors.New("No WANDevice")
	}

	dev, ok = getChildDevice(dev, "urn:schemas-upnp-org:device:WANConnectionDevice:1")
	if !ok {
		return "", errors.New("No WANConnectionDevice")
	}

	svc, ok := getChildService(dev, "urn:schemas-upnp-org:service:WANIPConnection:1")
	if !ok {
		return "", errors.New("No WANIPConnection")
	}

	if len(svc.ControlURL) == 0 {
		return "", errors.New("no controlURL")
	}

	u, _ := url.Parse(rootURL)
	if svc.ControlURL[0] == '/' {
		u.Path = svc.ControlURL
	} else {
		u.Path += svc.ControlURL
	}
	return u.String(), nil
}

func soapRequest(url, function, message string) error {
	tpl := `<?xml version="1.0" ?>
	<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
	<s:Body>%s</s:Body>
	</s:Envelope>
`
	body := fmt.Sprintf(tpl, message)

	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("User-Agent", "syncthing/1.0")
	req.Header.Set("SOAPAction", `"urn:schemas-upnp-org:service:WANIPConnection:1#`+function+`"`)
	req.Header.Set("Connection", "Close")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")

	if debug {
		dlog.Println(req.Header.Get("SOAPAction"))
		dlog.Println(body)
	}

	r, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	if debug {
		resp, _ := ioutil.ReadAll(r.Body)
		dlog.Println(string(resp))
	}

	r.Body.Close()

	if r.StatusCode >= 400 {
		return errors.New(function + ": " + r.Status)
	}

	return nil
}

func (n *IGD) AddPortMapping(protocol Protocol, externalPort, internalPort int, description string, timeout int) error {
	tpl := `<u:AddPortMapping xmlns:u="urn:schemas-upnp-org:service:WANIPConnection:1">
	<NewRemoteHost></NewRemoteHost>
	<NewExternalPort>%d</NewExternalPort>
	<NewProtocol>%s</NewProtocol>
	<NewInternalPort>%d</NewInternalPort>
	<NewInternalClient>%s</NewInternalClient>
	<NewEnabled>1</NewEnabled>
	<NewPortMappingDescription>%s</NewPortMappingDescription>
	<NewLeaseDuration>%d</NewLeaseDuration>
	</u:AddPortMapping>
	`

	body := fmt.Sprintf(tpl, externalPort, protocol, internalPort, n.ourIP, description, timeout)
	return soapRequest(n.serviceURL, "AddPortMapping", body)
}

func (n *IGD) DeletePortMapping(protocol Protocol, externalPort int) (err error) {
	tpl := `<u:DeletePortMapping xmlns:u="urn:schemas-upnp-org:service:WANIPConnection:1">
	<NewRemoteHost></NewRemoteHost>
	<NewExternalPort>%d</NewExternalPort>
	<NewProtocol>%s</NewProtocol>
	</u:DeletePortMapping>
	`

	body := fmt.Sprintf(tpl, externalPort, protocol)
	return soapRequest(n.serviceURL, "DeletePortMapping", body)
}
