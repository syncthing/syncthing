// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import "encoding/xml"

type VersioningConfiguration struct {
	Type   string            `xml:"type,attr" json:"type"`
	Params map[string]string `json:"params"`
}

type InternalVersioningConfiguration struct {
	Type   string          `xml:"type,attr,omitempty"`
	Params []InternalParam `xml:"param"`
}

type InternalParam struct {
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

func (c *VersioningConfiguration) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	var tmp InternalVersioningConfiguration
	tmp.Type = c.Type
	for k, v := range c.Params {
		tmp.Params = append(tmp.Params, InternalParam{k, v})
	}

	return e.EncodeElement(tmp, start)

}

func (c *VersioningConfiguration) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var tmp InternalVersioningConfiguration
	err := d.DecodeElement(&tmp, &start)
	if err != nil {
		return err
	}

	c.Type = tmp.Type
	c.Params = make(map[string]string, len(tmp.Params))
	for _, p := range tmp.Params {
		c.Params[p.Key] = p.Val
	}
	return nil
}
