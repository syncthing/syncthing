// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
	"encoding/json"
	"encoding/xml"
	"slices"
	"strings"

	"github.com/syncthing/syncthing/lib/structutil"
)

// VersioningConfiguration is used in the code and for JSON serialization
type VersioningConfiguration struct {
	Type             string            `json:"type" xml:"type,attr"`
	Params           map[string]string `json:"params" xml:"parameter" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
	CleanupIntervalS int               `json:"cleanupIntervalS" xml:"cleanupIntervalS" default:"3600"`
	FSPath           string            `json:"fsPath" xml:"fsPath"`
	FSType           FilesystemType    `json:"fsType" xml:"fsType" default:"basic"`
}

func (c *VersioningConfiguration) Reset() {
	*c = VersioningConfiguration{}
}

// internalVersioningConfiguration is used in XML serialization
type internalVersioningConfiguration struct {
	Type             string          `xml:"type,attr,omitempty"`
	Params           []internalParam `xml:"param"`
	CleanupIntervalS int             `xml:"cleanupIntervalS" default:"3600"`
	FSPath           string          `xml:"fsPath"`
	FSType           FilesystemType  `xml:"fsType" default:"basic"`
}

type internalParam struct {
	Key string `xml:"key,attr"`
	Val string `xml:"val,attr"`
}

func (c VersioningConfiguration) Copy() VersioningConfiguration {
	cp := c
	cp.Params = make(map[string]string, len(c.Params))
	for k, v := range c.Params {
		cp.Params[k] = v
	}
	return cp
}

func (c *VersioningConfiguration) UnmarshalJSON(data []byte) error {
	structutil.SetDefaults(c)
	type noCustomUnmarshal VersioningConfiguration
	ptr := (*noCustomUnmarshal)(c)
	return json.Unmarshal(data, ptr)
}

func (c *VersioningConfiguration) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var intCfg internalVersioningConfiguration
	structutil.SetDefaults(&intCfg)
	if err := d.DecodeElement(&intCfg, &start); err != nil {
		return err
	}
	c.fromInternal(intCfg)
	return nil
}

func (c VersioningConfiguration) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	// Using EncodeElement instead of plain Encode ensures that we use the
	// outer tag name from the VersioningConfiguration (i.e.,
	// `<versioning>`) rather than whatever the internal representation
	// would otherwise be.
	return e.EncodeElement(c.toInternal(), start)
}

func (c *VersioningConfiguration) toInternal() internalVersioningConfiguration {
	var tmp internalVersioningConfiguration
	tmp.Type = c.Type
	tmp.CleanupIntervalS = c.CleanupIntervalS
	tmp.FSPath = c.FSPath
	tmp.FSType = c.FSType
	for k, v := range c.Params {
		tmp.Params = append(tmp.Params, internalParam{k, v})
	}
	slices.SortFunc(tmp.Params, func(a, b internalParam) int {
		return strings.Compare(a.Key, b.Key)
	})
	return tmp
}

func (c *VersioningConfiguration) fromInternal(intCfg internalVersioningConfiguration) {
	c.Type = intCfg.Type
	c.CleanupIntervalS = intCfg.CleanupIntervalS
	c.FSPath = intCfg.FSPath
	c.FSType = intCfg.FSType
	c.Params = make(map[string]string, len(intCfg.Params))
	for _, p := range intCfg.Params {
		c.Params[p.Key] = p.Val
	}
}
