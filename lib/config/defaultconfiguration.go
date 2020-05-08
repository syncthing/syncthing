package config

import "github.com/syncthing/syncthing/lib/protocol"

type DefaultConfiguration struct {
	Folder FolderDefaultConfig
	Device DeviceDefaultConfig
}

type FolderDefaultConfig struct {
	RescanIntervalS int                     `xml:"rescanIntervalS,attr" json:"rescanIntervalS" default:"3600"`
	Type            FolderType              `xml:"type,attr" json:"type"`
	Versioning      VersioningConfiguration `xml:"versioning" json:"versioning"`
	Order           PullOrder               `xml:"order" json:"order"`
	MinDiskFree     Size                    `xml:"minDiskFree" json:"minDiskFree" default:"1%"`
}

type DeviceDefaultConfig struct {
	Compression       protocol.Compression `xml:"compression,attr" json:"compression"`
	Introducer        bool                 `xml:"introducer,attr" json:"introducer"`
	AutoAcceptFolders bool                 `xml:"autoAcceptFolders" json:"autoAcceptFolders"`
	MaxSendKbps       int                  `xml:"maxSendKbps" json:"maxSendKbps"`
	MaxRecvKbps       int                  `xml:"maxRecvKbps" json:"maxRecvKbps"`
}
